package tcpserver

import (
	"errors"
	"net"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// ConnContext 为每个 TCP 连接提供读/写循环与回调能力
type ConnContext struct {
	s      *Server
	c      net.Conn
	id     uint64
	writeC chan []byte
	closed int32
	onRead func([]byte)
	doneC  chan struct{}
	proto  atomic.Value // string: 协议标记，如 "ap3000" | "bkv"
}

func newConnContext(s *Server, c net.Conn) *ConnContext {
	cc := &ConnContext{
		s:      s,
		c:      c,
		id:     atomic.AddUint64(&s.nextConnID, 1),
		writeC: make(chan []byte, 128),
		doneC:  make(chan struct{}),
	}
	cc.proto.Store("")
	return cc
}

// ID 返回连接ID（单进程唯一递增）
func (cc *ConnContext) ID() uint64 { return cc.id }

// RemoteAddr 返回远端地址
func (cc *ConnContext) RemoteAddr() net.Addr { return cc.c.RemoteAddr() }

// Server 返回所属的服务器实例
func (cc *ConnContext) Server() *Server { return cc.s }

// SetOnRead 安装读取回调（收到上行原始字节时触发）
func (cc *ConnContext) SetOnRead(h func([]byte)) { cc.onRead = h }

// SetProtocol 设置连接所使用的协议标记（在 Mux 决策后调用）
func (cc *ConnContext) SetProtocol(p string) { cc.proto.Store(p) }

// Protocol 返回连接的协议标记
func (cc *ConnContext) Protocol() string {
	v := cc.proto.Load()
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// Write 异步写入，受写队列与写超时影响
func (cc *ConnContext) Write(b []byte) error {
	if atomic.LoadInt32(&cc.closed) == 1 {
		return errors.New("connection closed")
	}
	// 复制一份，避免调用方复用底层切片
	dup := make([]byte, len(b))
	copy(dup, b)
	to := cc.s.cfg.WriteTimeout
	if to <= 0 {
		to = 5 * time.Second
	}
	select {
	case cc.writeC <- dup:
		return nil
	case <-time.After(to):
		return errors.New("write queue timeout")
	}
}

// Close 关闭连接与写队列
func (cc *ConnContext) Close() error {
	if !atomic.CompareAndSwapInt32(&cc.closed, 0, 1) {
		return nil
	}
	close(cc.writeC)
	return cc.c.Close()
}

// run 启动读/写循环，阻塞直至连接结束
func (cc *ConnContext) run() {
	defer cc.Close()

	// ✅ 优化2: 协议识别阶段使用较短的超时 (5秒)
	// 识别完成后会切换到正常超时 (300秒)
	identificationTimeout := 5 * time.Second
	if cc.s.cfg.ReadTimeout > 0 && cc.s.cfg.ReadTimeout < identificationTimeout {
		identificationTimeout = cc.s.cfg.ReadTimeout
	}

	// 初始超时 (用于协议识别阶段)
	_ = cc.c.SetReadDeadline(time.Now().Add(identificationTimeout))
	_ = cc.c.SetWriteDeadline(time.Now().Add(cc.s.cfg.WriteTimeout))

	// 写循环
	doneW := make(chan struct{})
	go func() {
		defer close(doneW)
		for msg := range cc.writeC {
			if cc.s.cfg.WriteTimeout > 0 {
				_ = cc.c.SetWriteDeadline(time.Now().Add(cc.s.cfg.WriteTimeout))
			}
			_, _ = cc.c.Write(msg)
		}
	}()

	// 读循环
	buf := make([]byte, 4096)
	protocolIdentified := false // 标记协议是否已识别

	for {
		n, err := cc.c.Read(buf)
		if n > 0 {
			// ✅ 首次收到数据,标记协议识别阶段结束
			if !protocolIdentified {
				protocolIdentified = true
			}

			// 记录接收到的数据
			if cc.s.logger != nil {
				cc.s.logger.Debug("TCP data received",
					zap.String("remote_addr", cc.c.RemoteAddr().String()),
					zap.Int("bytes", n),
				)
			}
			if cc.s.onRecvBytes != nil {
				cc.s.onRecvBytes(n)
			}
			if cc.onRead != nil {
				cc.onRead(buf[:n])
			}
		}
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				// ✅ 区分协议识别超时和正常超时
				if !protocolIdentified {
					// 协议识别阶段超时 → 直接关闭连接
					if cc.s.logger != nil {
						cc.s.logger.Warn("Protocol identification timeout, closing connection",
							zap.String("remote_addr", cc.c.RemoteAddr().String()),
							zap.Duration("timeout", identificationTimeout),
						)
					}
					break
				}

				// 已识别协议后的正常超时 → 刷新deadline继续
				if cc.s.cfg.ReadTimeout > 0 {
					_ = cc.c.SetReadDeadline(time.Now().Add(cc.s.cfg.ReadTimeout))
				}
				continue
			}
			// 记录连接错误
			if cc.s.logger != nil {
				cc.s.logger.Info("TCP read error, connection will close",
					zap.String("remote_addr", cc.c.RemoteAddr().String()),
					zap.Error(err),
				)
			}
			break
		}
	}
	// 等待写循环退出
	<-doneW
	// 广播关闭
	select {
	case <-cc.doneC:
	default:
		close(cc.doneC)
	}
}

// Done 返回连接关闭通知通道
func (cc *ConnContext) Done() <-chan struct{} { return cc.doneC }

// RestoreNormalTimeout 恢复正常的读超时 (协议识别完成后调用)
func (cc *ConnContext) RestoreNormalTimeout() {
	if cc.s.cfg.ReadTimeout > 0 {
		_ = cc.c.SetReadDeadline(time.Now().Add(cc.s.cfg.ReadTimeout))
	}
}
