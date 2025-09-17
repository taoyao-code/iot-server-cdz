package bkv

// Adapter BKV 协议最小适配器：基于流式解码与路由表分发
type Adapter struct {
	decoder *StreamDecoder
	table   *Table
}

// NewAdapter 创建最小适配器
func NewAdapter() *Adapter { return &Adapter{decoder: NewStreamDecoder(), table: NewTable()} }

// Register 注册指令处理器
func (a *Adapter) Register(cmd uint16, h Handler) { a.table.Register(cmd, h) }

// ProcessBytes 处理原始字节流：切分帧并路由
func (a *Adapter) ProcessBytes(p []byte) error {
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
