package bkv

import "sync"

type Handler func(*Frame) error

type Table struct{
    mu sync.RWMutex
    m map[byte]Handler
}

func NewTable() *Table { return &Table{m: make(map[byte]Handler)} }

func (t *Table) Register(cmd byte, h Handler) {
    t.mu.Lock(); defer t.mu.Unlock()
    t.m[cmd] = h
}

func (t *Table) Route(f *Frame) error {
    t.mu.RLock(); h := t.m[f.Cmd]; t.mu.RUnlock()
    if h == nil { return nil }
    return h(f)
}


