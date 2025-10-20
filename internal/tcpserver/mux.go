package tcpserver

import (
	padapter "github.com/taoyao-code/iot-server/internal/protocol/adapter"
	ap "github.com/taoyao-code/iot-server/internal/protocol/ap3000"
	bk "github.com/taoyao-code/iot-server/internal/protocol/bkv"
	"go.uber.org/zap"
)

// Mux 多协议复用器：首帧初判 -> 绑定协议 -> 直通处理
type Mux struct {
	adapters []padapter.Adapter
	server   *Server // 添加server引用以使用logger
}

func NewMux(adapters ...padapter.Adapter) *Mux { return &Mux{adapters: adapters} }

// SetServer 设置server引用（用于日志）
func (m *Mux) SetServer(s *Server) { m.server = s }

// BindToConn 为连接安装 onRead，根据首包前缀判断协议后固定处理路径
func (m *Mux) BindToConn(cc *ConnContext) {
	var decided bool
	var handler func([]byte)

	// ✅ 优化2: 记录协议识别开始时间 (用于统计耗时)
	identificationStartTime := cc.Server().GetCurrentTime()

	cc.SetOnRead(func(p []byte) {
		if !decided {
			// 取前缀若干字节用于初判
			pref := p
			if len(pref) > 8 {
				pref = pref[:8]
			}
			for _, a := range m.adapters {
				if a.Sniff(pref) {
					aa := a
					handler = func(b []byte) { _ = aa.ProcessBytes(b) }
					// 标记协议
					var proto string
					switch aa.(type) {
					case *ap.Adapter:
						proto = "ap3000"
						cc.SetProtocol("ap3000")
					case *bk.Adapter:
						proto = "bkv"
						cc.SetProtocol("bkv")
					default:
						proto = "unknown"
						cc.SetProtocol("")
					}
					// ✅ 记录协议识别耗时
					identDuration := cc.Server().GetCurrentTime().Sub(identificationStartTime)
					if m.server != nil && m.server.logger != nil {
						m.server.logger.Info("Protocol identified",
							zap.String("remote_addr", cc.RemoteAddr().String()),
							zap.String("protocol", proto),
							zap.Duration("identification_duration", identDuration),
						)
					}

					// ✅ 识别完成后,恢复正常的读超时 (从5秒→300秒)
					cc.RestoreNormalTimeout()

					decided = true
					break
				}
			}
			if !decided {
				// 未识别协议
				if m.server != nil && m.server.logger != nil {
					m.server.logger.Warn("Unknown protocol, trying all adapters",
						zap.String("remote_addr", cc.RemoteAddr().String()),
						zap.Int("data_len", len(p)),
					)
				}
				// 未识别，尝试全部投递一次（容错），后续仍可被识别
				for _, a := range m.adapters {
					_ = a.ProcessBytes(p)
				}
				return
			}
		}
		if handler != nil {
			handler(p)
		}
	})
}
