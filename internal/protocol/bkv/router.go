package bkv

import "sync"

type Handler func(*Frame) error

type Table struct {
	mu sync.RWMutex
	m  map[uint16]Handler
}

func NewTable() *Table { return &Table{m: make(map[uint16]Handler)} }

func (t *Table) Register(cmd uint16, h Handler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.m[cmd] = h
}

func (t *Table) Route(f *Frame) error {
	t.mu.RLock()
	h := t.m[f.Cmd]
	t.mu.RUnlock()
	if h == nil {
		return nil
	}
	return h(f)
}
