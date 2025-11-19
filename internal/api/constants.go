package api

// 订单状态常量
const (
	OrderStatusPending     = 0  // 待确认
	OrderStatusConfirmed   = 1  // 已确认
	OrderStatusCharging    = 2  // 充电中
	OrderStatusCompleted   = 3  // 已完成
	OrderStatusFailed      = 6  // 失败
	OrderStatusCancelled   = 5  // 已取消
	OrderStatusCancelling  = 8  // 取消中 (P1-5中间态)
	OrderStatusStopping    = 9  // 停止中 (P1-5中间态)
	OrderStatusInterrupted = 10 // 中断 (P0-2断线恢复)
)
