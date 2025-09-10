package tcpserver

import (
	"context"
	"io"
	"net"
	"sync"
	"time"

	cfgpkg "github.com/taoyao-code/iot-server/internal/config"
)

// Server TCP 网关最小实现
type Server struct {
	cfg     cfgpkg.TCPConfig
	ln      net.Listener
	wg      sync.WaitGroup
	stopC   chan struct{}
	handler func([]byte)
	// 可选指标回调
	onAccept    func()
	onRecvBytes func(n int)
}

// New 创建 TCP 网关
func New(cfg cfgpkg.TCPConfig) *Server {
	return &Server{cfg: cfg, stopC: make(chan struct{})}
}

// SetHandler 设置上行报文处理回调（raw bytes）
func (s *Server) SetHandler(h func([]byte)) { s.handler = h }

// SetMetricsCallbacks 设置指标回调
func (s *Server) SetMetricsCallbacks(onAccept func(), onRecvBytes func(int)) {
	s.onAccept, s.onRecvBytes = onAccept, onRecvBytes
}

// Start 监听并接受连接（非阻塞，内部 goroutine）
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return err
	}
	s.ln = ln

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			conn, err := s.ln.Accept()
			if err != nil {
				select {
				case <-s.stopC:
					return
				default:
				}
				// 短暂错误等待后重试
				time.Sleep(50 * time.Millisecond)
				continue
			}
			if s.onAccept != nil {
				s.onAccept()
			}

			s.wg.Add(1)
			go func(c net.Conn) {
				defer s.wg.Done()
				defer c.Close()
				_ = c.SetReadDeadline(time.Now().Add(s.cfg.ReadTimeout))
				_ = c.SetWriteDeadline(time.Now().Add(s.cfg.WriteTimeout))
				buf := make([]byte, 4096)
				for {
					n, err := c.Read(buf)
					if n > 0 {
						if s.onRecvBytes != nil {
							s.onRecvBytes(n)
						}
						if s.handler != nil {
							s.handler(buf[:n])
						}
					}
					if err != nil {
						if ne, ok := err.(net.Error); ok && ne.Timeout() {
							continue
						}
						if err == io.EOF {
							return
						}
						return
					}
				}
			}(conn)
		}
	}()
	return nil
}

// Shutdown 优雅关闭监听并等待连接退出
func (s *Server) Shutdown(ctx context.Context) error {
	close(s.stopC)
	if s.ln != nil {
		_ = s.ln.Close()
	}
	ch := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(ch)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ch:
		return nil
	}
}
