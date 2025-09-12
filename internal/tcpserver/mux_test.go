package tcpserver

import (
	"testing"

	ap "github.com/taoyao-code/iot-server/internal/protocol/ap3000"
	bk "github.com/taoyao-code/iot-server/internal/protocol/bkv"
)

func TestMux_SniffAndDispatch(t *testing.T) {
	mux := NewMux(ap.NewAdapter(), bk.NewAdapter())
	// 构造一个假的连接上下文，仅测试回调链路
	cc := &ConnContext{}
	mux.BindToConn(cc)
	// 手动设置 onRead 后直接触发
	if cc.onRead == nil {
		t.Fatalf("onRead not set")
	}
	// AP3000 前缀 'D''N''Y'
	cc.onRead([]byte{0x44, 0x4E, 0x59, 0x01, 0x02})
	// BKV 前缀 0xFC 0xFE
	cc = &ConnContext{}
	mux.BindToConn(cc)
	if cc.onRead == nil {
		t.Fatalf("onRead not set 2")
	}
	cc.onRead([]byte{0xFC, 0xFE, 0x05, 0x10, 0x00})
}
