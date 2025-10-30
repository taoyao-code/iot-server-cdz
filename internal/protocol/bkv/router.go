package bkv

import (
	"encoding/hex"
	"sync"

	"go.uber.org/zap"
)

type Handler func(*Frame) error

type Table struct {
	mu     sync.RWMutex
	m      map[uint16]Handler
	logger *zap.Logger // 添加logger用于详细记录
}

func NewTable() *Table { return &Table{m: make(map[uint16]Handler)} }

// SetLogger 设置logger
func (t *Table) SetLogger(logger *zap.Logger) {
	t.logger = logger
}

func (t *Table) Register(cmd uint16, h Handler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.m[cmd] = h
}

func (t *Table) Route(f *Frame) error {
	// 记录每一个解码后的帧（详细信息）
	if t.logger != nil {
		direction := "上行"
		if !f.IsUplink() {
			direction = "下行"
		}
		t.logger.Info("📨 BKV帧详情",
			zap.String("方向", direction),
			zap.String("cmd", formatCmd(f.Cmd)),
			zap.String("gateway_id", f.GatewayID),
			zap.Uint32("msg_id", f.MsgID),
			zap.Int("data_len", len(f.Data)),
			zap.String("data_hex", hex.EncodeToString(f.Data)))
	}

	t.mu.RLock()
	h := t.m[f.Cmd]
	t.mu.RUnlock()
	if h == nil {
		if t.logger != nil {
			t.logger.Warn("⚠️  未注册的BKV命令",
				zap.String("cmd", formatCmd(f.Cmd)),
				zap.String("gateway_id", f.GatewayID))
		}
		return nil
	}
	return h(f)
}

// formatCmd 格式化命令码为十六进制字符串
func formatCmd(cmd uint16) string {
	return "0x" + hex.EncodeToString([]byte{byte(cmd >> 8), byte(cmd)})
}
