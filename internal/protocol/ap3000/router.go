package ap3000

// Router AP3000 路由接口（占位）
type Router interface {
	// Route 根据原始数据路由到处理器
	Route(raw []byte) error
}

// SimpleRouter 最简路由实现（仅占位）
type SimpleRouter struct{}

// NewSimpleRouter 创建占位路由
func NewSimpleRouter() *SimpleRouter { return &SimpleRouter{} }

// Route 占位实现：直接返回 nil
func (r *SimpleRouter) Route(_ []byte) error { return nil }
