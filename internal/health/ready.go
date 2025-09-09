package health

import "sync/atomic"

// Readiness 就绪状态聚合（DB、TCP等）
type Readiness struct {
	dbReady  atomic.Bool
	tcpReady atomic.Bool
}

func New() *Readiness { return &Readiness{} }

func (r *Readiness) SetDBReady(v bool)  { r.dbReady.Store(v) }
func (r *Readiness) SetTCPReady(v bool) { r.tcpReady.Store(v) }

// Ready 总体就绪：各子系统均为 true
func (r *Readiness) Ready() bool {
	return r.dbReady.Load() && r.tcpReady.Load()
}
