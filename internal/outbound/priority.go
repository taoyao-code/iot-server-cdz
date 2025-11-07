package outbound

// P1-6修复: 下行指令优先级定义
// 注意: 数值越小=优先级越高（Redis ZPOPMIN取最小score）
const (
	// PriorityEmergency 紧急指令（立即执行）
	// 场景: 停止充电、取消订单、紧急断电
	PriorityEmergency = 1

	// PriorityHigh 高优先级指令
	// 场景: 启动充电、查询端口状态
	PriorityHigh = 2

	// PriorityNormal 普通优先级指令
	// 场景: 参数设置、查询设备信息
	PriorityNormal = 3

	// PriorityLow 低优先级指令
	// 场景: OTA升级、日志上传
	PriorityLow = 4

	// PriorityBackground 后台任务
	// 场景: 定期同步、统计查询
	PriorityBackground = 5
)

// GetCommandPriority 根据BKV命令码返回优先级
func GetCommandPriority(cmd uint16) int {
	switch cmd {
	case 0x1010: // 停止充电
		return PriorityEmergency
	case 0x1013: // 取消订单（如果有此命令）
		return PriorityEmergency

	case 0x1011: // 启动充电
		return PriorityHigh
	case 0x1012: // 查询端口状态
		return PriorityHigh

	case 0x1003: // 参数设置
		return PriorityNormal
	case 0x100F: // 查询设备信息
		return PriorityNormal

	case 0x1014: // OTA升级
		return PriorityLow

	case 0x0000: // 心跳ACK（响应设备心跳）
		return PriorityNormal // 心跳响应不需要最高优先级

	default:
		return PriorityNormal
	}
}
