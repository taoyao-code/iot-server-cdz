package bkv

import (
	"encoding/hex"

	"go.uber.org/zap"
)

// Adapter BKV 协议最小适配器：基于流式解码与路由表分发
type Adapter struct {
	decoder *StreamDecoder
	table   *Table
	logger  *zap.Logger
}

// NewAdapter 创建最小适配器
func NewAdapter() *Adapter { return &Adapter{decoder: NewStreamDecoder(), table: NewTable()} }

// SetLogger 设置logger
func (a *Adapter) SetLogger(logger *zap.Logger) {
	a.logger = logger
	// 同时设置table的logger以便Route时记录详情
	if a.table != nil {
		a.table.SetLogger(logger)
	}
}

// Register 注册指令处理器
func (a *Adapter) Register(cmd uint16, h Handler) { a.table.Register(cmd, h) }

// ProcessBytes 处理原始字节流：切分帧并路由
func (a *Adapter) ProcessBytes(p []byte) error {
	// 记录原始数据包（仅当有完整帧时）
	if len(p) > 0 && a.logger != nil {
		a.logger.Info("📦 BKV原始数据包",
			zap.String("hex", hex.EncodeToString(p)),
			zap.Int("bytes", len(p)))
	}

	frames, err := a.decoder.Feed(p)
	if err != nil {
		return err
	}
	for _, fr := range frames {
		if err := a.table.Route(fr); err != nil {
			return err
		}
	}
	return nil
}

// Sniff 初判是否为 BKV 协议（检查 magic 0xFC,0xFE/0xFF）
func (a *Adapter) Sniff(prefix []byte) bool {
	if len(prefix) < 2 {
		return false
	}
	return (prefix[0] == magicUplink[0] && prefix[1] == magicUplink[1]) ||
		(prefix[0] == magicDownlink[0] && prefix[1] == magicDownlink[1])
}
