package ap3000

// Adapter AP3000 协议适配器：流式解码 + 路由表
type Adapter struct {
	decoder *StreamDecoder
	table   *Table
}

func NewAdapter() *Adapter { return &Adapter{decoder: NewStreamDecoder(1024), table: NewTable()} }

// Register 注册指令处理器
func (a *Adapter) Register(cmd uint8, h Handler) { a.table.Register(cmd, h) }

// ProcessBytes 处理上行字节流
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

// Sniff 粗略判断是否为 AP3000 协议（检查 magic 'D”N”Y'）
func (a *Adapter) Sniff(prefix []byte) bool {
	if len(prefix) < 3 {
		return false
	}
	return prefix[0] == magic[0] && prefix[1] == magic[1] && prefix[2] == magic[2]
}
