package ap3000

import "sync"

// Handler 处理器函数类型（骨架）
type Handler func(f *Frame) error

// Table 路由表（cmd -> handler）
type Table struct {
	mu       sync.RWMutex
	handlers map[uint8]Handler
}

func NewTable() *Table { return &Table{handlers: make(map[uint8]Handler)} }

func (t *Table) Register(cmd uint8, h Handler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.handlers[cmd] = h
}

func (t *Table) Route(f *Frame) error {
	t.mu.RLock()
	h := t.handlers[f.Cmd]
	t.mu.RUnlock()
	if h == nil {
		return nil
	}
	return h(f)
}
