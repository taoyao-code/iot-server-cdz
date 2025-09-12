package tcpserver

import (
	padapter "github.com/taoyao-code/iot-server/internal/protocol/adapter"
	ap "github.com/taoyao-code/iot-server/internal/protocol/ap3000"
	bk "github.com/taoyao-code/iot-server/internal/protocol/bkv"
)

// Mux 多协议复用器：首帧初判 -> 绑定协议 -> 直通处理
type Mux struct {
	adapters []padapter.Adapter
}

func NewMux(adapters ...padapter.Adapter) *Mux { return &Mux{adapters: adapters} }

// BindToConn 为连接安装 onRead，根据首包前缀判断协议后固定处理路径
func (m *Mux) BindToConn(cc *ConnContext) {
	var decided bool
	var handler func([]byte)
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
					switch aa.(type) {
					case *ap.Adapter:
						cc.SetProtocol("ap3000")
					case *bk.Adapter:
						cc.SetProtocol("bkv")
					default:
						cc.SetProtocol("")
					}
					decided = true
					break
				}
			}
			if !decided {
				// 未识别，尝试全部投递一次（容错），后续仍可被识别
				for _, a := range m.adapters {
					_ = a.ProcessBytes(p)
				}
				return
			}
		}
		if handler != nil { handler(p) }
	})
}
