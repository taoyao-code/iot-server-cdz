package tcpserver

import (
	ap "github.com/taoyao-code/iot-server/internal/protocol/ap3000"
	bk "github.com/taoyao-code/iot-server/internal/protocol/bkv"
)

// ProtoAdapter 统一协议适配器接口（最小）
type ProtoAdapter interface {
	ProcessBytes([]byte) error
	Sniff([]byte) bool
}

// Mux 多协议复用器：首帧初判 -> 绑定协议 -> 直通处理
type Mux struct {
	ap3000 *ap.Adapter
	bkv    *bk.Adapter
}

func NewMux(apAdapter *ap.Adapter, bkvAdapter *bk.Adapter) *Mux {
	return &Mux{ap3000: apAdapter, bkv: bkvAdapter}
}

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
			if m.ap3000.Sniff(pref) {
				handler = func(b []byte) { _ = m.ap3000.ProcessBytes(b) }
				decided = true
			} else if m.bkv.Sniff(pref) {
				handler = func(b []byte) { _ = m.bkv.ProcessBytes(b) }
				decided = true
			} else {
				// 未识别，尝试两边都投递（容错），后续仍可被识别
				_ = m.ap3000.ProcessBytes(p)
				_ = m.bkv.ProcessBytes(p)
				return
			}
		}
		if handler != nil {
			handler(p)
		}
	})
}
