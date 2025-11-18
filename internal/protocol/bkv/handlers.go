package bkv

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	"github.com/taoyao-code/iot-server/internal/thirdparty"
)

// repoAPI 抽象（与 ap3000 对齐一部分能力）
// P0修复: 扩展接口支持参数持久化
type repoAPI interface {
	EnsureDevice(ctx context.Context, phyID string) (int64, error)
	TouchDeviceLastSeen(ctx context.Context, phyID string, at time.Time) error
	InsertCmdLog(ctx context.Context, deviceID int64, msgID int, cmd int, direction int16, payload []byte, success bool) error
	UpsertPortState(ctx context.Context, deviceID int64, portNo int, status int, powerW *int) error
	UpsertOrderProgress(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh01 int, status int, powerW01 *int) error
	SettleOrder(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh01 int, reason int) error
	AckOutboundByMsgID(ctx context.Context, deviceID int64, msgID int, ok bool, errCode *int) error

	// P0修复: 参数持久化方法（数据库存储）
	StoreParamWrite(ctx context.Context, deviceID int64, paramID int, value []byte, msgID int) error
	GetParamWritePending(ctx context.Context, deviceID int64, paramID int) ([]byte, int, error) // value, msgID, error
	ConfirmParamWrite(ctx context.Context, deviceID int64, paramID int, msgID int) error
	FailParamWrite(ctx context.Context, deviceID int64, paramID int, msgID int, errMsg string) error

	// Week 6: 组网管理方法
	UpsertGatewaySocket(ctx context.Context, socket *pgstorage.GatewaySocket) error
	DeleteGatewaySocket(ctx context.Context, gatewayID string, socketNo int) error
	GetGatewaySockets(ctx context.Context, gatewayID string) ([]pgstorage.GatewaySocket, error)

	// Week 7: OTA升级方法
	CreateOTATask(ctx context.Context, task *pgstorage.OTATask) (int64, error)
	GetOTATask(ctx context.Context, taskID int64) (*pgstorage.OTATask, error)
	UpdateOTATaskStatus(ctx context.Context, taskID int64, status int, errorMsg *string) error
	UpdateOTATaskProgress(ctx context.Context, taskID int64, progress int, status int) error
	GetDeviceOTATasks(ctx context.Context, deviceID int64, limit int) ([]pgstorage.OTATask, error)

	// P0修复: 订单状态管理方法
	GetPendingOrderByPort(ctx context.Context, deviceID int64, portNo int) (*pgstorage.Order, error)
	UpdateOrderToCharging(ctx context.Context, orderNo string, startTime time.Time) error
	CancelOrderByPort(ctx context.Context, deviceID int64, portNo int) error
	GetChargingOrderByPort(ctx context.Context, deviceID int64, portNo int) (*pgstorage.Order, error)
	CompleteOrderByPort(ctx context.Context, deviceID int64, portNo int, endTime time.Time, reason int) error

	// P0-2修复: interrupted订单恢复方法
	GetInterruptedOrders(ctx context.Context, deviceID int64) ([]pgstorage.Order, error)
	RecoverOrder(ctx context.Context, orderNo string) error
	FailOrder(ctx context.Context, orderNo, reason string) error
}

// CardServiceAPI 刷卡充电服务接口
type CardServiceAPI interface {
	HandleCardSwipe(ctx context.Context, req *CardSwipeRequest) (*ChargeCommand, error)
	HandleOrderConfirmation(ctx context.Context, conf *OrderConfirmation) error
	HandleChargeEnd(ctx context.Context, report *ChargeEndReport) error
	HandleBalanceQuery(ctx context.Context, query *BalanceQuery) (*BalanceResponse, error)
}

// OutboundSender Week5: 下行消息发送接口
type OutboundSender interface {
	// SendDownlink 发送下行消息
	// gatewayID: 网关ID
	// cmd: 命令码
	// msgID: 消息ID
	// data: 数据payload
	SendDownlink(gatewayID string, cmd uint16, msgID uint32, data []byte) error
}

// MetricsAPI 监控指标接口（2025-10-31新增）
type MetricsAPI interface {
	GetChargeReportTotal() *prometheus.CounterVec
	GetChargeReportPowerGauge() *prometheus.GaugeVec
	GetChargeReportCurrentGauge() *prometheus.GaugeVec
	GetChargeReportEnergyTotal() *prometheus.CounterVec
	GetPortStatusQueryResponseTotal() *prometheus.CounterVec // P1-4新增
}

// Handlers BKV 协议处理器集合
type Handlers struct {
	Repo        repoAPI
	Reason      *ReasonMap
	CardService CardServiceAPI         // Week4: 刷卡充电服务
	Outbound    OutboundSender         // Week5: 下行消息发送器
	EventQueue  *thirdparty.EventQueue // v2.1: 事件队列（第三方推送）
	Deduper     *thirdparty.Deduper    // v2.1: 去重器
	Metrics     MetricsAPI             // v2.1: 监控指标（Prometheus）
}

// HandleHeartbeat 处理心跳帧 (cmd=0x0000 或 BKV cmd=0x1017)
func (h *Handlers) HandleHeartbeat(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}

	// 使用网关ID作为设备标识
	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	devID, err := h.Repo.EnsureDevice(ctx, devicePhyID)
	if err != nil {
		return err
	}

	// 刷新数据库中的最近心跳时间（与 Redis 会话一致）
	_ = h.Repo.TouchDeviceLastSeen(ctx, devicePhyID, time.Now())

	// v2.1.3: 新设备注册时推送设备注册事件
	// 注意：这里简化处理，实际应该在首次注册时才推送
	// 可以通过检查设备是否是新创建来判断（比如检查created_at和updated_at是否相同）
	// 这里为了示例，暂时不推送（避免每次心跳都推送注册事件）

	// 记录心跳日志
	success := true
	err = h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), getDirection(f.IsUplink()), f.Data, success)

	// v2.1: 推送设备心跳事件（采样推送，避免过于频繁）
	// 使用msgID进行采样，每10次心跳推送1次
	if h.EventQueue != nil && f.MsgID%10 == 0 {
		// 心跳数据简化处理，实际应从f.Data解析
		h.pushDeviceHeartbeatEvent(
			ctx,
			devicePhyID,
			220.0, // voltage - 默认值，实际应解析
			-50,   // rssi - 默认值，实际应解析
			25.0,  // temp - 默认值，实际应解析
			nil,   // ports - 可选
			nil,   // logger可选
		)
	}

	// 🔥 关键修复：回复心跳ACK，否则设备会在60秒后断开连接
	if h.Outbound != nil {
		ackPayload := encodeHeartbeatAck(devicePhyID)
		// 2-A: 复用上行帧的MsgID，便于设备匹配应答
		_ = h.Outbound.SendDownlink(devicePhyID, 0x0000, f.MsgID, ackPayload)
	}

	// P0-2修复: 检查是否有interrupted订单需要恢复
	if err := h.checkInterruptedOrdersRecovery(ctx, devicePhyID, devID); err != nil {
		// 恢复失败不影响心跳处理,仅记录错误
		_ = h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), 0xFFFF, 0,
			[]byte(fmt.Sprintf("interrupted recovery failed: %v", err)), false)
	}

	return err
}

// encodeHeartbeatAck 构造心跳ACK的payload（当前时间）
// 格式：YYYYMMDDHHmmss (14字节ASCII)
func encodeHeartbeatAck(gatewayID string) []byte {
	now := time.Now()
	timeStr := now.Format("20060102150405") // YYYYMMDDHHmmss
	return []byte(timeStr)
}

// HandleBKVStatus 处理BKV插座状态上报 (cmd=0x1000 with BKV payload)
func (h *Handlers) HandleBKVStatus(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}

	// 获取BKV载荷
	payload, err := f.GetBKVPayload()
	if err != nil {
		return fmt.Errorf("failed to parse BKV payload: %w", err)
	}

	// 使用BKV载荷中的网关ID
	devicePhyID := payload.GatewayID
	if devicePhyID == "" {
		devicePhyID = f.GatewayID
	}
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	devID, err := h.Repo.EnsureDevice(ctx, devicePhyID)
	if err != nil {
		return err
	}

	// 记录命令日志
	if err := h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), getDirection(f.IsUplink()), f.Data, true); err != nil {
		return err
	}

	// 如果是状态上报，尝试解析并更新端口状态
	if payload.IsStatusReport() {
		return h.handleSocketStatusUpdate(ctx, devID, payload)
	}

	// 如果是充电结束上报，处理订单结算
	if payload.IsChargingEnd() {
		return h.handleBKVChargingEnd(ctx, devID, payload)
	}

	// 如果是异常事件上报，处理异常信息
	if payload.IsExceptionReport() {
		return h.handleExceptionEvent(ctx, devID, payload)
	}

	// 如果是参数查询，记录参数信息
	if payload.IsParameterQuery() {
		return h.handleParameterQuery(ctx, devID, payload)
	}

	// 如果是控制命令，转发到控制处理器
	if payload.IsControlCommand() {
		return h.handleBKVControlCommand(ctx, devID, payload)
	}

	return nil
}

// handleSocketStatusUpdate 处理插座状态更新
// P0修复: 增强订单状态同步和事件推送
func (h *Handlers) handleSocketStatusUpdate(ctx context.Context, deviceID int64, payload *BKVPayload) error {
	// 使用GetSocketStatus方法解析完整的插座状态
	socketStatus, err := payload.GetSocketStatus()
	if err != nil {
		// 如果解析失败，回退到简化解析
		return h.handleSocketStatusUpdateSimple(ctx, deviceID, payload)
	}

	devicePhyID := payload.GatewayID

	// 更新端口A状态并检查订单
	if socketStatus.PortA != nil {
		if err := h.updatePortAndOrder(ctx, deviceID, devicePhyID, socketStatus.PortA); err != nil {
			return fmt.Errorf("failed to update port A: %w", err)
		}
	}

	// 更新端口B状态并检查订单
	if socketStatus.PortB != nil {
		if err := h.updatePortAndOrder(ctx, deviceID, devicePhyID, socketStatus.PortB); err != nil {
			return fmt.Errorf("failed to update port B: %w", err)
		}
	}

	return nil
}

// updatePortAndOrder 更新端口状态并同步订单状态
// P0修复: 核心逻辑 - 当端口开始充电时自动更新订单状态
func (h *Handlers) updatePortAndOrder(ctx context.Context, deviceID int64, devicePhyID string, port *PortStatus) error {
	status := int(port.Status)
	var powerW *int
	if port.Power > 0 {
		power := int(port.Power) / 10 // 从0.1W转换为W
		powerW = &power
	}

	// 1. 更新端口状态到数据库
	if err := h.Repo.UpsertPortState(ctx, deviceID, int(port.PortNo), status, powerW); err != nil {
		return fmt.Errorf("upsert port state: %w", err)
	}

	// 2. P0修复: 检查是否需要更新订单状态
	if port.IsCharging() && port.BusinessNo > 0 {
		// 端口正在充电且有业务号，查找对应的pending订单
		order, err := h.Repo.GetPendingOrderByPort(ctx, deviceID, int(port.PortNo))
		if err != nil {
			// 订单不存在或查询失败，只记录警告
			// 不返回错误，因为端口状态已成功更新
			return nil
		}

		// 3. 如果订单存在且是pending状态，更新为charging
		if order != nil && order.Status == 0 {
			startTime := time.Now()
			if err := h.Repo.UpdateOrderToCharging(ctx, order.OrderNo, startTime); err != nil {
				return fmt.Errorf("update order to charging: %w", err)
			}

			// 4. P0修复: 推送charging.started事件
			if h.EventQueue != nil {
				h.pushChargingStartedEventWithPort(
					ctx,
					devicePhyID,
					order.OrderNo,
					port,
					startTime,
				)
			}
		}

		// 5. P0修复: 如果订单已经是charging状态，推送progress事件
		if order != nil && order.Status == 1 {
			if h.EventQueue != nil {
				h.pushChargingProgressEvent(
					ctx,
					devicePhyID,
					order.OrderNo,
					port,
				)
			}
		}
	}

	return nil
}

// handleSocketStatusUpdateSimple 简化的插座状态更新（回退方案）
func (h *Handlers) handleSocketStatusUpdateSimple(ctx context.Context, deviceID int64, payload *BKVPayload) error {
	// 原有的简化解析逻辑作为回退方案
	var portAStatus, portBStatus int = 0, 0
	var portAPower, portBPower *int

	// 简化的字段解析
	for _, field := range payload.Fields {
		switch field.Tag {
		case 0x03:
			// 插座相关字段，暂时使用默认状态
		case 0x00:
			if len(field.Value) >= 3 && field.Value[1] == 0x09 {
				// 插座状态字段
				portAStatus = int(field.Value[2])
			}
		}
	}

	// 更新端口A状态
	if err := h.Repo.UpsertPortState(ctx, deviceID, 0, portAStatus, portAPower); err != nil {
		return fmt.Errorf("failed to update port A state: %w", err)
	}

	// 更新端口B状态
	if err := h.Repo.UpsertPortState(ctx, deviceID, 1, portBStatus, portBPower); err != nil {
		return fmt.Errorf("failed to update port B state: %w", err)
	}

	return nil
}

// handleBKVChargingEnd 处理BKV格式的充电结束上报
func (h *Handlers) handleBKVChargingEnd(ctx context.Context, deviceID int64, payload *BKVPayload) error {
	var portNo int = 0
	var orderID int = 0
	var kwh01 int = 0
	var durationSec int = 0
	var reason int = 0

	// 解析BKV字段
	for _, field := range payload.Fields {
		switch field.Tag {
		case 0x08: // 插孔号
			if len(field.Value) >= 1 {
				portNo = int(field.Value[0])
			}
		case 0x0A: // 订单号
			if len(field.Value) >= 2 {
				orderID = int(field.Value[0])<<8 | int(field.Value[1])
			}
		case 0x0D: // 已用电量
			if len(field.Value) >= 2 {
				kwh01 = int(field.Value[0])<<8 | int(field.Value[1])
			}
		case 0x0E: // 已充电时间（分钟）
			if len(field.Value) >= 2 {
				durationMin := int(field.Value[0])<<8 | int(field.Value[1])
				durationSec = durationMin * 60
			}
		case 0x2F: // 结束原因
			if len(field.Value) >= 1 {
				reason = int(field.Value[0])
			}
		}
	}

	// 如果有结束原因映射，进行转换
	if h.Reason != nil {
		if mappedReason, ok := h.Reason.Translate(reason); ok {
			reason = mappedReason
		}
	}

	// 生成订单号
	orderHex := fmt.Sprintf("%04X", orderID)

	// 结算订单
	if err := h.Repo.SettleOrder(ctx, deviceID, portNo, orderHex, durationSec, kwh01, reason); err != nil {
		return err
	}

	// 更新端口状态为空闲
	idleStatus := 0 // 0=空闲
	return h.Repo.UpsertPortState(ctx, deviceID, portNo, idleStatus, nil)
}

// HandleControl 处理控制指令 (cmd=0x0015)
func (h *Handlers) HandleControl(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	devID, err := h.Repo.EnsureDevice(ctx, devicePhyID)
	if err != nil {
		return err
	}

	success := true

	// 如果是下行控制指令（平台发给设备）
	if !f.IsUplink() {
		// 使用增强的解析器解析控制指令
		cmd, err := ParseBKVControlCommand(f.Data)
		if err != nil {
			success = false
			return h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), getDirection(f.IsUplink()), f.Data, success)
		}

		if cmd.Switch == SwitchOn {
			// 开始充电：创建订单并更新端口状态
			orderHex := fmt.Sprintf("%04X%02X%02X", f.MsgID, cmd.SocketNo, cmd.Port)

			// 根据充电模式确定充电参数
			var durationSec int
			var kwhTarget int

			switch cmd.Mode {
			case ChargingModeByTime:
				durationSec = int(cmd.Duration) * 60 // 分钟转秒
			case ChargingModeByPower:
				kwhTarget = int(cmd.Energy) // Wh转换为0.01kWh需要除以10
			case ChargingModeByLevel:
				// 按功率充电使用总支付金额作为目标
				durationSec = int(cmd.Duration) * 60
			}

			// 创建充电订单（状态1=进行中）
			if err := h.Repo.UpsertOrderProgress(ctx, devID, int(cmd.Port), orderHex, durationSec, kwhTarget, 1, nil); err != nil {
				success = false
			} else {
				// 更新端口状态为充电中
				chargingStatus := 1 // 1=充电中
				if err := h.Repo.UpsertPortState(ctx, devID, int(cmd.Port), chargingStatus, nil); err != nil {
					success = false
				}
			}
		} else {
			// 停止充电：更新端口状态为空闲
			idleStatus := 0 // 0=空闲
			if err := h.Repo.UpsertPortState(ctx, devID, int(cmd.Port), idleStatus, nil); err != nil {
				success = false
			}
		}
	} else {
		// 上行：设备回复
		// 按协议2.2.8示例：内层长度0005，格式为[07][01][插座号][插孔号][业务号2字节]
		if len(f.Data) >= 2 && len(f.Data) < 15 {
			innerLen := (int(f.Data[0]) << 8) | int(f.Data[1])

			// 🔥 关键修复: 协议2.2.8控制回复格式: [07][结果][插座号][插孔号][业务号2字节]
			// 参考协议文档line 273-283示例
			if innerLen == 5 && len(f.Data) >= 7 {
				inner := f.Data[2:7]

				// 🔥 ACK数据字段映射（协议2.2.8标准格式）
				//
				// 【协议格式】设备对接指引-组网设备2024(1).txt 章节2.2.8：
				// ACK应答：[长度2B][0x07][结果1B][插座号1B][插孔号1B][业务号2B]
				//
				// 【字段说明】
				// inner[0] = 0x07          - 命令标识（控制命令）
				// inner[1] = result        - 执行结果（01=成功，00=失败）
				// inner[2] = socketNo      - 插座号（单机版=0，组网版=1-250）
				// inner[3] = portNo        - 插孔号（0=A孔，1=B孔）
				// inner[4] = businessNo    - 业务号低字节（关联订单）
				//
				// 【协议示例】
				// 成功: 0005 07 01 00 00 01
				//            ^^ ^^ ^^ ^^ ^^
				//            |  |  |  |  └─ 业务号=0x01
				//            |  |  |  └──── 插孔0(A孔)
				//            |  |  └─────── 插座0(单机版)
				//            |  └────────── 成功
				//            └───────────── 控制命令
				//
				// 失败: 0005 07 00 02 00 00
				//            ^^ ^^ ^^ ^^ ^^
				//            |  |  |  |  └─ 业务号
				//            |  |  |  └──── 插孔0
				//            |  |  └─────── 插座2(设备不支持)
				//            |  └────────── 失败
				//            └───────────── 控制命令
				//
				// 【历史Bug】2025-10-30之前错误实现为：
				//   inner[2] = portNo   ❌ 顺序错误
				//   inner[3] = socketNo ❌ 顺序错误
				// 导致端口映射混乱，已于2025-10-31修复
				subCmd := inner[0]             // 0x07
				result := inner[1]             // 01=成功, 00=失败
				socketNo := inner[2]           // 插座号
				portNo := inner[3]             // 插孔号 0=A孔,1=B孔
				businessNo := uint16(inner[4]) // 业务号（1字节）

				// 记录ACK详情
				ackLog := fmt.Sprintf("0x0015控制回复: 子命令=0x%02X 插座=%d 插孔=%d 结果=%d(1=成功,0=失败) 业务号=0x%02X",
					subCmd, socketNo, portNo, result, businessNo)
				_ = h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), getDirection(f.IsUplink()), []byte(ackLog), result == 0x01)

				// 直接使用协议端口号（0=A孔, 1=B孔）查询订单
				// 数据库统一使用协议端口号，无需转换
				protocolPortNo := int(portNo)

				// 如果结果=01(成功)，根据当前订单状态判断是启动还是停止
				if result == 0x01 {
					// 先检查是否有charging订单（停止充电）
					chargingOrder, chargingErr := h.Repo.GetChargingOrderByPort(ctx, devID, protocolPortNo)
					if chargingErr != nil && chargingErr.Error() != "no rows in result set" {
						errorLog := fmt.Sprintf("❌查询charging订单失败: port=%d err=%v", protocolPortNo, chargingErr)
						_ = h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), 0x0015, getDirection(f.IsUplink()), []byte(errorLog), false)
					}

					if chargingOrder != nil {
						// 🔥 Bug#4修复: 停止充电ACK - 完成订单
						endTime := time.Now()
						endReason := 1 // 用户主动停止
						if err := h.Repo.CompleteOrderByPort(ctx, devID, protocolPortNo, endTime, endReason); err == nil {
							completeLog := fmt.Sprintf("✅订单已完成: %s (插孔%d, 原因:用户主动停止)", chargingOrder.OrderNo, portNo)
							_ = h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), 0x0015, getDirection(f.IsUplink()), []byte(completeLog), true)

							if h.EventQueue != nil {
								h.pushChargingCompletedEvent(ctx, devicePhyID, chargingOrder.OrderNo, protocolPortNo, endReason, nil)
							}
						} else {
							errorLog := fmt.Sprintf("❌完成订单失败: %s err=%v", chargingOrder.OrderNo, err)
							_ = h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), 0x0015, getDirection(f.IsUplink()), []byte(errorLog), false)
						}
					} else {
						// 检查是否有pending订单（启动充电）
						pendingOrder, err := h.Repo.GetPendingOrderByPort(ctx, devID, protocolPortNo)
						if err != nil {
							errorLog := fmt.Sprintf("❌查询pending订单失败: port=%d err=%v", protocolPortNo, err)
							_ = h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), 0x0015, getDirection(f.IsUplink()), []byte(errorLog), false)
						} else if pendingOrder != nil {
							startTime := time.Now()
							updateErr := h.Repo.UpdateOrderToCharging(ctx, pendingOrder.OrderNo, startTime)
							if updateErr == nil {
								updateLog := fmt.Sprintf("✅订单状态已更新: %s -> charging (插孔%d, start_time=%d)", pendingOrder.OrderNo, portNo, startTime.Unix())
								_ = h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), 0x0015, getDirection(f.IsUplink()), []byte(updateLog), true)

								if h.EventQueue != nil {
									h.pushChargingStartedEvent(ctx, devicePhyID, pendingOrder.OrderNo, protocolPortNo, nil)
								}
							} else {
								errorLog := fmt.Sprintf("❌更新订单状态失败: %s err=%v", pendingOrder.OrderNo, updateErr)
								_ = h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), 0x0015, getDirection(f.IsUplink()), []byte(errorLog), false)
							}
						} else {
							// 无pending/charging订单，可能是重复ACK或异常
							warnLog := fmt.Sprintf("⚠️收到控制成功ACK但无订单: 插孔%d, device_id=%d", portNo, devID)
							_ = h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), 0x0015, getDirection(f.IsUplink()), []byte(warnLog), false)
						}
					}
				} else {
					// 设备拒绝了充电请求 - 需要取消对应的订单
					failLog := fmt.Sprintf("❌设备拒绝充电: 插座=%d 插孔=%d 原因=未知", socketNo, portNo)
					_ = h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), 0x0015, getDirection(f.IsUplink()), []byte(failLog), false)

					if err := h.Repo.CancelOrderByPort(ctx, devID, protocolPortNo); err != nil {
						_ = h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), 0x0015, getDirection(f.IsUplink()),
							[]byte(fmt.Sprintf("❌取消订单失败: port=%d err=%v", protocolPortNo, err)), false)
					} else {
						_ = h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), 0x0015, getDirection(f.IsUplink()),
							[]byte(fmt.Sprintf("✅已自动取消pending订单: 插孔%d", portNo)), true)
					}
				}
			}
		} else if len(f.Data) >= 15 {
			// 长数据：充电结束上报
			endReport, err := ParseBKVChargingEnd(f.Data)
			if err == nil {
				// 处理充电结束
				orderHex := fmt.Sprintf("%04X", endReport.BusinessNo)

				// 计算实际充电时长和用电量
				durationSec := int(endReport.ChargingTime) * 60 // 分钟转秒
				kwhUsed := int(endReport.EnergyUsed)            // 已经是0.01kWh单位

				// 映射结束原因到平台统一原因码
				var platformReason int = 0 // 默认正常结束
				if h.Reason != nil {
					if reason, ok := h.Reason.Translate(int(endReport.EndReason)); ok {
						platformReason = reason
					}
				}

				// 结算订单
				if err := h.Repo.SettleOrder(ctx, devID, int(endReport.Port), orderHex, durationSec, kwhUsed, platformReason); err != nil {
					success = false
				}

				// 更新端口状态为空闲
				idleStatus := 0
				powerW := int(endReport.InstantPower) / 10 // 转换为实际瓦数
				if err := h.Repo.UpsertPortState(ctx, devID, int(endReport.Port), idleStatus, &powerW); err != nil {
					success = false
				}
			}
		}
	}

	// 记录控制指令日志
	return h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), getDirection(f.IsUplink()), f.Data, success)
}

// HandleChargingEnd 处理充电结束上报 (cmd=0x0015 上行，特定格式)
func (h *Handlers) HandleChargingEnd(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	devID, err := h.Repo.EnsureDevice(ctx, devicePhyID)
	if err != nil {
		return err
	}

	success := true

	// 只处理上行的充电结束上报
	if f.IsUplink() && len(f.Data) >= 10 {
		// 解析基础充电结束上报格式 (协议文档 2.2.9)
		// data[0-1]: 帧长 (0011)
		// data[2]: 命令 (02)
		// data[3]: 插座号
		// data[4-5]: 插座版本
		// data[6]: 插座温度
		// data[7]: RSSI
		// data[8]: 插孔号
		// data[9]: 插座状态
		// data[10-11]: 业务号
		// data[12-13]: 瞬时功率
		// data[14-15]: 瞬时电流
		// data[16-17]: 用电量
		// data[18-19]: 充电时间

		if f.Data[2] == 0x02 && len(f.Data) >= 20 { // 确认是充电结束命令
			portNo := int(f.Data[8]) // 插孔号

			// 解析业务号（16位）
			orderID := int(f.Data[10])<<8 | int(f.Data[11])
			orderHex := fmt.Sprintf("%04X", orderID)

			// 解析充电数据
			power := int(f.Data[12])<<8 | int(f.Data[13])       // 瞬时功率（0.1W）
			current := int(f.Data[14])<<8 | int(f.Data[15])     // 瞬时电流（0.001A）
			kwh01 := int(f.Data[16])<<8 | int(f.Data[17])       // 用电量（0.01kWh）
			durationMin := int(f.Data[18])<<8 | int(f.Data[19]) // 充电时间（分钟）
			durationSec := durationMin * 60

			// 从插座状态中提取结束原因（简化版本）
			status := f.Data[9]
			reason := extractEndReason(status)

			// 如果有结束原因映射，进行转换
			if h.Reason != nil {
				if mappedReason, ok := h.Reason.Translate(reason); ok {
					reason = mappedReason
				}
			}

			// 📊 采集充电上报指标（2025-10-31新增）
			if h.Metrics != nil {
				deviceIDStr := fmt.Sprintf("%d", devID)
				portNoStr := fmt.Sprintf("%d", portNo+1) // API端口=协议插孔+1

				// 状态统计
				statusLabel := "idle" // 充电结束=空闲
				if status&0x10 != 0 {
					statusLabel = "charging" // bit4=1表示充电中
				}
				if status&0x04 == 0 || status&0x02 == 0 {
					statusLabel = "abnormal" // 温度或电流异常
				}
				h.Metrics.GetChargeReportTotal().WithLabelValues(deviceIDStr, portNoStr, statusLabel).Inc()

				// 实时功率（W）
				powerW := float64(power) / 10.0
				h.Metrics.GetChargeReportPowerGauge().WithLabelValues(deviceIDStr, portNoStr).Set(powerW)

				// 实时电流（A）
				currentA := float64(current) / 1000.0
				h.Metrics.GetChargeReportCurrentGauge().WithLabelValues(deviceIDStr, portNoStr).Set(currentA)

				// 累计电量（Wh）
				energyWh := float64(kwh01) * 10.0 // 0.01kWh = 10Wh
				h.Metrics.GetChargeReportEnergyTotal().WithLabelValues(deviceIDStr, portNoStr).Add(energyWh)
			}

			// 结算订单
			if err := h.Repo.SettleOrder(ctx, devID, portNo, orderHex, durationSec, kwh01, reason); err != nil {
				success = false
			} else {
				// 更新端口状态为空闲
				idleStatus := 0 // 0=空闲
				if err := h.Repo.UpsertPortState(ctx, devID, portNo, idleStatus, nil); err != nil {
					success = false
				}
			}
		}
	}

	// 记录充电结束日志
	return h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), getDirection(f.IsUplink()), f.Data, success)
}

// extractEndReason 从插座状态中提取结束原因（简化版本）
func extractEndReason(status uint8) int {
	// 根据协议文档中的状态位解析结束原因
	// 这里使用简化的逻辑，实际可能需要更复杂的位操作
	if status&0x08 != 0 { // 检查空载位
		return 1 // 空载结束
	}
	if status&0x04 != 0 { // 检查其他状态位
		return 2 // 其他原因
	}
	return 0 // 正常结束
}

// HandleGeneric 通用处理器，记录所有其他指令
func (h *Handlers) HandleGeneric(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	devID, err := h.Repo.EnsureDevice(ctx, devicePhyID)
	if err != nil {
		return err
	}

	// 记录通用指令日志
	success := true
	return h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), getDirection(f.IsUplink()), f.Data, success)
}

// getDirection 获取数据方向标识
func getDirection(isUplink bool) int16 {
	if isUplink {
		return 1 // 上行
	}
	return 0 // 下行
}

// HandleParam 处理参数读写指令 (完整的写入→回读校验实现)
func (h *Handlers) HandleParam(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	devID, err := h.Repo.EnsureDevice(ctx, devicePhyID)
	if err != nil {
		return err
	}

	success := true

	switch f.Cmd {
	case 0x83, 0x84: // 参数写入
		if !f.IsUplink() {
			// 下行参数写入：存储待验证的参数值
			if len(f.Data) > 0 {
				param := DecodeParamWrite(f.Data)
				if err := h.Repo.StoreParamWrite(ctx, devID, param.ParamID, param.Value, int(f.MsgID)); err != nil {
					success = false
				}
			} else {
				success = false
			}
		} else {
			// 上行参数写入响应：仅确认收到
			if err := h.Repo.AckOutboundByMsgID(ctx, devID, int(f.MsgID), len(f.Data) > 0, nil); err != nil {
				success = false
			}
		}

	case 0x85: // 参数回读
		if f.IsUplink() {
			// 上行参数回读：验证值是否与写入一致
			if len(f.Data) > 0 {
				readback := DecodeParamReadback(f.Data)

				// 获取之前写入的参数值进行比较
				expectedValue, msgID, err := h.Repo.GetParamWritePending(ctx, devID, readback.ParamID)
				if err == nil && expectedValue != nil {
					// 比较回读值与期望值
					if len(readback.Value) == len(expectedValue) {
						match := true
						for i, v := range readback.Value {
							if v != expectedValue[i] {
								match = false
								break
							}
						}

						if match {
							// 校验成功：确认参数写入完成
							if err := h.Repo.AckOutboundByMsgID(ctx, devID, msgID, true, nil); err != nil {
								success = false
							}
						} else {
							// 校验失败：参数值不匹配
							errCode := 1 // 参数校验失败
							if err := h.Repo.AckOutboundByMsgID(ctx, devID, msgID, false, &errCode); err != nil {
								success = false
							}
							success = false
						}
					} else {
						// 校验失败：长度不匹配
						errCode := 2 // 参数长度错误
						if err := h.Repo.AckOutboundByMsgID(ctx, devID, msgID, false, &errCode); err != nil {
							success = false
						}
						success = false
					}
				}
			} else {
				success = false
			}
		}

	default:
		// 其他参数相关命令
		success = len(f.Data) > 0
	}

	return h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), getDirection(f.IsUplink()), f.Data, success)
}

// handleExceptionEvent 处理异常事件上报
func (h *Handlers) handleExceptionEvent(ctx context.Context, deviceID int64, payload *BKVPayload) error {
	event, err := ParseBKVExceptionEvent(payload)
	if err != nil {
		return fmt.Errorf("failed to parse exception event: %w", err)
	}

	// 这里可以根据异常类型进行不同的处理
	// 例如：更新设备状态、发送告警、记录异常日志等

	// 记录异常事件到日志（可以扩展为专门的异常事件表）
	success := true
	return h.Repo.InsertCmdLog(ctx, deviceID, 0, int(payload.Cmd), 1, []byte(fmt.Sprintf("Exception: Socket=%d, Reason=%d", event.SocketNo, event.SocketEventReason)), success)
}

// handleParameterQuery 处理参数查询
func (h *Handlers) handleParameterQuery(ctx context.Context, deviceID int64, payload *BKVPayload) error {
	param, err := ParseBKVParameterQuery(payload)
	if err != nil {
		return fmt.Errorf("failed to parse parameter query: %w", err)
	}

	// 这里可以保存设备参数信息到数据库
	// 或者与之前设置的参数进行比较验证

	// 记录参数查询结果
	success := true
	return h.Repo.InsertCmdLog(ctx, deviceID, 0, int(payload.Cmd), 1, []byte(fmt.Sprintf("Params: Socket=%d, Power=%d, Temp=%d", param.SocketNo, param.PowerLimit, param.HighTempThreshold)), success)
}

// handleBKVControlCommand 处理BKV控制命令
func (h *Handlers) handleBKVControlCommand(ctx context.Context, deviceID int64, payload *BKVPayload) error {
	// BKV控制命令可能包含刷卡充电、远程控制等
	// 这里实现基础的控制逻辑

	// 检查是否为刷卡充电相关
	if payload.IsCardCharging() {
		return h.handleCardCharging(ctx, deviceID, payload)
	}

	// 其他控制命令的通用处理
	success := true
	return h.Repo.InsertCmdLog(ctx, deviceID, 0, int(payload.Cmd), 1, []byte("BKV Control Command"), success)
}

// handleCardCharging 处理刷卡充电
func (h *Handlers) handleCardCharging(ctx context.Context, deviceID int64, payload *BKVPayload) error {
	// 解析刷卡相关信息
	// 这里可以实现刷卡充电的完整流程：
	// 1. 验证卡片有效性
	// 2. 检查余额
	// 3. 创建充电订单
	// 4. 更新端口状态

	success := true
	return h.Repo.InsertCmdLog(ctx, deviceID, 0, int(payload.Cmd), 1, []byte("Card Charging"), success)
}

// ============ Week4: 刷卡充电处理函数 ============

// HandleCardSwipe 处理刷卡上报 (0x0B)
func (h *Handlers) HandleCardSwipe(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}

	// 上行：设备刷卡上报
	if f.IsUplink() {
		return h.handleCardSwipeUplink(ctx, f)
	}

	// 下行：下发充电指令（通常由业务层触发，这里记录日志）
	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		return fmt.Errorf("missing gateway ID")
	}

	devID, err := h.Repo.EnsureDevice(ctx, devicePhyID)
	if err != nil {
		return err
	}

	return h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), getDirection(f.IsUplink()), f.Data, true)
}

// handleCardSwipeUplink 处理刷卡上报上行
func (h *Handlers) handleCardSwipeUplink(ctx context.Context, f *Frame) error {
	// 解析刷卡数据
	req, err := ParseCardSwipeRequest(f.Data)
	if err != nil {
		return fmt.Errorf("failed to parse card swipe: %w", err)
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = req.PhyID
	}

	devID, err := h.Repo.EnsureDevice(ctx, devicePhyID)
	if err != nil {
		return err
	}

	// 记录刷卡日志
	logData := []byte(fmt.Sprintf("CardNo=%s, PhyID=%s, Balance=%d", req.CardNo, req.PhyID, req.Balance))
	err = h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 1, logData, true)
	if err != nil {
		return err
	}

	// Week4: 调用CardService处理刷卡业务
	if h.CardService != nil {
		cmd, err := h.CardService.HandleCardSwipe(ctx, req)
		if err != nil {
			// 业务处理失败，记录错误日志
			errLog := []byte(fmt.Sprintf("CardSwipe failed: %v", err))
			h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 1, errLog, false)
			return fmt.Errorf("card service error: %w", err)
		}

		// Week5: 下发充电指令到设备
		if err := h.sendChargeCommand(f.GatewayID, f.MsgID, cmd); err != nil {
			// 发送失败，记录错误
			errLog := []byte(fmt.Sprintf("Send charge command failed: %v", err))
			h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 0, errLog, false)
			return fmt.Errorf("send charge command error: %w", err)
		}

		// v2.1: 推送订单创建事件
		if cmd != nil && h.EventQueue != nil {
			h.pushOrderCreatedEvent(
				ctx,
				devicePhyID,
				cmd.OrderNo,
				1, // portNo - 从订单中获取，暂时使用默认值
				string(cmd.ChargeMode),
				int(cmd.Duration),
				float64(cmd.PricePerKwh)/100.0, // 转换为元/kWh
				nil,                            // logger可选
			)
		}
	}

	return nil
}

// HandleOrderConfirm 处理订单确认 (0x0F)
func (h *Handlers) HandleOrderConfirm(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}

	// 上行：设备确认订单
	if f.IsUplink() {
		return h.handleOrderConfirmUplink(ctx, f)
	}

	// 下行：平台回复确认（记录日志）
	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		return fmt.Errorf("missing gateway ID")
	}

	devID, err := h.Repo.EnsureDevice(ctx, devicePhyID)
	if err != nil {
		return err
	}

	return h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), getDirection(f.IsUplink()), f.Data, true)
}

// handleOrderConfirmUplink 处理订单确认上行
func (h *Handlers) handleOrderConfirmUplink(ctx context.Context, f *Frame) error {
	// 解析订单确认
	conf, err := ParseOrderConfirmation(f.Data)
	if err != nil {
		return fmt.Errorf("failed to parse order confirmation: %w", err)
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		return fmt.Errorf("missing gateway ID")
	}

	devID, err := h.Repo.EnsureDevice(ctx, devicePhyID)
	if err != nil {
		return err
	}

	// 记录订单确认日志
	logData := []byte(fmt.Sprintf("OrderNo=%s, Status=%d, Reason=%s", conf.OrderNo, conf.Status, conf.Reason))
	err = h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 1, logData, true)
	if err != nil {
		return err
	}

	// Week4: 调用CardService更新订单状态
	if h.CardService != nil {
		err = h.CardService.HandleOrderConfirmation(ctx, conf)
		if err != nil {
			// 更新订单失败，记录错误
			errLog := []byte(fmt.Sprintf("OrderConfirm failed: %v", err))
			h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 1, errLog, false)
			return fmt.Errorf("order confirmation error: %w", err)
		}

		// Week5: 下发确认回复到设备
		result := uint8(0) // 0=成功
		if err := h.sendOrderConfirmReply(f.GatewayID, f.MsgID, conf.OrderNo, result); err != nil {
			// 发送失败，记录错误（但不影响业务流程）
			errLog := []byte(fmt.Sprintf("Send order confirm reply failed: %v", err))
			h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 0, errLog, false)
			// 不返回错误，因为订单已更新成功
		}

		// v2.1: 推送订单确认事件
		if h.EventQueue != nil {
			resultStr := "success"
			failReason := conf.Reason
			if conf.Status != 0 {
				resultStr = "failed"
			}
			h.pushOrderConfirmedEvent(
				ctx,
				devicePhyID,
				conf.OrderNo,
				0, // portNo从订单中获取，这里简化
				resultStr,
				failReason,
				nil, // logger可选
			)

			// v2.1.2: 如果订单确认成功，推送充电开始事件
			if conf.Status == 0 {
				h.pushChargingStartedEvent(
					ctx,
					devicePhyID,
					conf.OrderNo,
					0,   // portNo从订单中获取，这里简化
					nil, // logger可选
				)
			}
		}
	}

	return nil
}

// HandleChargeEnd 处理充电结束 (0x0C)
func (h *Handlers) HandleChargeEnd(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}

	// 上行：设备上报充电结束
	if f.IsUplink() {
		return h.handleChargeEndUplink(ctx, f)
	}

	// 下行：平台确认（记录日志）
	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		return fmt.Errorf("missing gateway ID")
	}

	devID, err := h.Repo.EnsureDevice(ctx, devicePhyID)
	if err != nil {
		return err
	}

	return h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), getDirection(f.IsUplink()), f.Data, true)
}

// handleChargeEndUplink 处理充电结束上行
func (h *Handlers) handleChargeEndUplink(ctx context.Context, f *Frame) error {
	// 解析充电结束数据
	report, err := ParseChargeEndReport(f.Data)
	if err != nil {
		return fmt.Errorf("failed to parse charge end: %w", err)
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		return fmt.Errorf("missing gateway ID")
	}

	devID, err := h.Repo.EnsureDevice(ctx, devicePhyID)
	if err != nil {
		return err
	}

	// 记录充电结束日志
	logData := []byte(fmt.Sprintf("OrderNo=%s, CardNo=%s, Duration=%d, Energy=%d, Amount=%d",
		report.OrderNo, report.CardNo, report.Duration, report.Energy, report.Amount))
	err = h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 1, logData, true)
	if err != nil {
		return err
	}

	// Week4: 调用CardService完成订单和扣款
	if h.CardService != nil {
		err = h.CardService.HandleChargeEnd(ctx, report)
		if err != nil {
			// 扣款失败，记录错误
			errLog := []byte(fmt.Sprintf("ChargeEnd failed: %v", err))
			h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 1, errLog, false)
			return fmt.Errorf("charge end error: %w", err)
		}

		// Week5: 下发结束确认到设备
		result := uint8(0) // 0=成功
		if err := h.sendChargeEndReply(f.GatewayID, f.MsgID, report.OrderNo, result); err != nil {
			// 发送失败，记录错误（但不影响业务流程）
			errLog := []byte(fmt.Sprintf("Send charge end reply failed: %v", err))
			h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 0, errLog, false)
			// 不返回错误，因为订单已完成
		}

		// v2.1: 推送订单完成事件
		if h.EventQueue != nil {
			totalKwh := float64(report.Energy) / 100.0    // 转换为kWh
			totalAmount := float64(report.Amount) / 100.0 // 转换为元
			h.pushOrderCompletedEvent(
				ctx,
				devicePhyID,
				report.OrderNo,
				0, // portNo简化
				int(report.Duration),
				totalKwh,
				0, // peakPower
				0, // avgPower
				totalAmount,
				"normal", // endReason
				"充电完成",   // endReasonMsg
				nil,      // logger可选
			)

			// 同时推送充电结束事件
			h.pushChargingEndedEvent(
				ctx,
				devicePhyID,
				report.OrderNo,
				0, // portNo简化
				int(report.Duration),
				totalKwh,
				"normal",
				"充电完成",
				nil, // logger可选
			)
		}
	}

	return nil
}

// HandleBalanceQuery 处理余额查询 (0x1A)
func (h *Handlers) HandleBalanceQuery(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}

	// 上行：设备查询余额
	if f.IsUplink() {
		return h.handleBalanceQueryUplink(ctx, f)
	}

	// 下行：平台响应余额（记录日志）
	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		return fmt.Errorf("missing gateway ID")
	}

	devID, err := h.Repo.EnsureDevice(ctx, devicePhyID)
	if err != nil {
		return err
	}

	return h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), getDirection(f.IsUplink()), f.Data, true)
}

// handleBalanceQueryUplink 处理余额查询上行
func (h *Handlers) handleBalanceQueryUplink(ctx context.Context, f *Frame) error {
	// 解析余额查询
	query, err := ParseBalanceQuery(f.Data)
	if err != nil {
		return fmt.Errorf("failed to parse balance query: %w", err)
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		return fmt.Errorf("missing gateway ID")
	}

	devID, err := h.Repo.EnsureDevice(ctx, devicePhyID)
	if err != nil {
		return err
	}

	// 记录余额查询日志
	logData := []byte(fmt.Sprintf("CardNo=%s", query.CardNo))
	err = h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 1, logData, true)
	if err != nil {
		return err
	}

	// Week4: 调用CardService查询余额
	if h.CardService != nil {
		resp, err := h.CardService.HandleBalanceQuery(ctx, query)
		if err != nil {
			// 查询失败，记录错误
			errLog := []byte(fmt.Sprintf("BalanceQuery failed: %v", err))
			h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 1, errLog, false)
			return fmt.Errorf("balance query error: %w", err)
		}

		// Week5: 下发余额响应到设备
		if err := h.sendBalanceResponse(f.GatewayID, f.MsgID, resp); err != nil {
			// 发送失败，记录错误
			errLog := []byte(fmt.Sprintf("Send balance response failed: %v", err))
			h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 0, errLog, false)
			return fmt.Errorf("send balance response error: %w", err)
		}
	}

	return nil
}

// ===== Week 6: 组网管理处理器 =====

// HandleNetworkRefresh 处理刷新插座列表响应（上行）
func (h *Handlers) HandleNetworkRefresh(ctx context.Context, f *Frame) error {
	// 解析刷新响应
	resp, err := ParseNetworkRefreshResponse(f.Data)
	if err != nil {
		return fmt.Errorf("parse refresh response: %w", err)
	}

	// 更新数据库中的插座列表
	now := time.Now()
	for _, socket := range resp.Sockets {
		signal := int(socket.SignalStrength)
		lastSeen := now

		gatewaySocket := &pgstorage.GatewaySocket{
			GatewayID:      f.GatewayID,
			SocketNo:       int(socket.SocketNo),
			SocketMAC:      socket.SocketMAC,
			SocketUID:      socket.SocketUID,
			Channel:        int(socket.Channel),
			Status:         int(socket.Status),
			SignalStrength: &signal,
			LastSeenAt:     &lastSeen,
		}

		if err := h.Repo.UpsertGatewaySocket(ctx, gatewaySocket); err != nil {
			return fmt.Errorf("upsert socket %d: %w", socket.SocketNo, err)
		}
	}

	return nil
}

// HandleNetworkAddNode 处理添加插座响应（上行）
func (h *Handlers) HandleNetworkAddNode(ctx context.Context, f *Frame) error {
	// 解析添加响应
	resp, err := ParseNetworkAddNodeResponse(f.Data)
	if err != nil {
		return fmt.Errorf("parse add node response: %w", err)
	}

	// 根据结果更新插座状态
	if resp.Result == 0 {
		// 成功：插座应该已经在刷新列表时更新了
		// 这里可以记录日志或发送通知
		return nil
	} else {
		// 失败：记录错误原因
		return fmt.Errorf("add socket %d failed: %s", resp.SocketNo, resp.Reason)
	}
}

// HandleNetworkDeleteNode 处理删除插座响应（上行）
func (h *Handlers) HandleNetworkDeleteNode(ctx context.Context, f *Frame) error {
	// 解析删除响应
	resp, err := ParseNetworkDeleteNodeResponse(f.Data)
	if err != nil {
		return fmt.Errorf("parse delete node response: %w", err)
	}

	// 根据结果处理
	if resp.Result == 0 {
		// 成功：从数据库删除插座
		if err := h.Repo.DeleteGatewaySocket(ctx, f.GatewayID, int(resp.SocketNo)); err != nil {
			return fmt.Errorf("delete socket %d: %w", resp.SocketNo, err)
		}
		return nil
	} else {
		// 失败：记录错误原因
		return fmt.Errorf("delete socket %d failed: %s", resp.SocketNo, resp.Reason)
	}
}

// ===== Week 7: OTA升级处理器 =====

// HandleOTAResponse 处理OTA升级响应（上行）
func (h *Handlers) HandleOTAResponse(ctx context.Context, f *Frame) error {
	// 解析OTA响应
	resp, err := ParseOTAResponse(f.Data)
	if err != nil {
		return fmt.Errorf("parse OTA response: %w", err)
	}

	// TODO: 根据响应结果更新任务状态
	// 这里需要通过MsgID关联到对应的OTA任务
	// 暂时只记录日志
	devID, _ := h.Repo.EnsureDevice(ctx, f.GatewayID)

	logData := []byte(fmt.Sprintf("OTA Response: target=%d, socket=%d, result=%d, reason=%s",
		resp.TargetType, resp.SocketNo, resp.Result, resp.Reason))
	h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 1, logData, resp.Result == 0)

	return nil
}

// HandleOTAProgress 处理OTA升级进度上报（上行）
func (h *Handlers) HandleOTAProgress(ctx context.Context, f *Frame) error {
	// 解析OTA进度
	progress, err := ParseOTAProgress(f.Data)
	if err != nil {
		return fmt.Errorf("parse OTA progress: %w", err)
	}

	// TODO: 更新任务进度
	// 这里需要找到对应的OTA任务并更新进度
	// 暂时只记录日志
	devID, _ := h.Repo.EnsureDevice(ctx, f.GatewayID)

	logData := []byte(fmt.Sprintf("OTA Progress: target=%d, socket=%d, progress=%d%%, status=%d",
		progress.TargetType, progress.SocketNo, progress.Progress, progress.Status))
	h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 1, logData, true)

	// v2.1: 推送OTA进度事件
	if h.EventQueue != nil {
		status := "in_progress"
		statusMsg := "OTA升级进行中"
		errorMsg := ""
		if progress.Status == 2 {
			status = "completed"
			statusMsg = "OTA升级完成"
		} else if progress.Status == 3 {
			status = "failed"
			statusMsg = "OTA升级失败"
			errorMsg = "设备上报失败"
		}
		h.pushOTAProgressEvent(
			ctx,
			f.GatewayID,
			0,  // taskID需要从数据库查询获取
			"", // version - 从任务中获取
			int(progress.Progress),
			status,
			statusMsg,
			errorMsg,
			nil, // logger可选
		)
	}

	return nil
}

// ===== Week 8: 按功率分档充电处理器 =====

// HandlePowerLevelEnd 处理按功率充电结束上报（上行）
func (h *Handlers) HandlePowerLevelEnd(ctx context.Context, f *Frame) error {
	// 解析充电结束上报
	report, err := ParsePowerLevelEndReport(f.Data)
	if err != nil {
		return fmt.Errorf("parse power level end report: %w", err)
	}

	devID, _ := h.Repo.EnsureDevice(ctx, f.GatewayID)

	// 记录充电结束日志
	logData := []byte(fmt.Sprintf("PowerLevelEnd: port=%d, duration=%dm, energy=%.2fkWh, amount=%.2f元, reason=%d",
		report.PortNo, report.TotalDuration, float64(report.TotalEnergy)/100, float64(report.TotalAmount)/100, report.EndReason))
	h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 1, logData, true)

	// TODO: 更新订单信息，记录各档位使用情况
	// 目前先返回确认
	reply := EncodePowerLevelEndReply(report.PortNo, 0) // 0=确认成功

	// 发送确认回复（下行）
	// TODO: 通过Outbound发送回复
	_ = reply

	return nil
}

// ===== Week 9: 参数管理处理器 =====

// HandleParamReadResponse 处理批量读取参数响应（上行）
func (h *Handlers) HandleParamReadResponse(ctx context.Context, f *Frame) error {
	resp, err := ParseParamReadResponse(f.Data)
	if err != nil {
		return fmt.Errorf("parse param read response: %w", err)
	}

	devID, _ := h.Repo.EnsureDevice(ctx, f.GatewayID)

	// 记录参数读取日志
	logData := []byte(fmt.Sprintf("ParamReadResponse: %d params", len(resp.Params)))
	h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 1, logData, true)

	// TODO: 存储参数到数据库或缓存
	for _, param := range resp.Params {
		_ = param // 暂时忽略
	}

	return nil
}

// HandleParamWriteResponse 处理批量写入参数响应（上行）
func (h *Handlers) HandleParamWriteResponse(ctx context.Context, f *Frame) error {
	resp, err := ParseParamWriteResponse(f.Data)
	if err != nil {
		return fmt.Errorf("parse param write response: %w", err)
	}

	devID, _ := h.Repo.EnsureDevice(ctx, f.GatewayID)

	// 记录参数写入日志
	successCount := 0
	for _, result := range resp.Results {
		if result.Result == 0 {
			successCount++
		}
	}

	logData := []byte(fmt.Sprintf("ParamWriteResponse: %d/%d success", successCount, len(resp.Results)))
	h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 1, logData, true)

	return nil
}

// HandleParamSyncResponse 处理参数同步响应（上行）
func (h *Handlers) HandleParamSyncResponse(ctx context.Context, f *Frame) error {
	resp, err := ParseParamSyncResponse(f.Data)
	if err != nil {
		return fmt.Errorf("parse param sync response: %w", err)
	}

	devID, _ := h.Repo.EnsureDevice(ctx, f.GatewayID)

	// 记录同步状态
	logData := []byte(fmt.Sprintf("ParamSyncResponse: result=%s, progress=%d%%",
		GetParamSyncResultDescription(resp.Result), resp.Progress))
	h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 1, logData, true)

	return nil
}

// HandleParamResetResponse 处理参数重置响应（上行）
func (h *Handlers) HandleParamResetResponse(ctx context.Context, f *Frame) error {
	resp, err := ParseParamResetResponse(f.Data)
	if err != nil {
		return fmt.Errorf("parse param reset response: %w", err)
	}

	devID, _ := h.Repo.EnsureDevice(ctx, f.GatewayID)

	// 记录重置结果
	status := "成功"
	if resp.Result != 0 {
		status = "失败"
	}
	logData := []byte(fmt.Sprintf("ParamResetResponse: %s, message=%s", status, resp.Message))
	h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 1, logData, true)

	return nil
}

// ===== Week 10: 扩展功能处理器 =====

// HandleVoiceConfigResponse 处理语音配置响应（上行）
func (h *Handlers) HandleVoiceConfigResponse(ctx context.Context, f *Frame) error {
	resp, err := ParseVoiceConfigResponse(f.Data)
	if err != nil {
		return fmt.Errorf("parse voice config response: %w", err)
	}

	devID, _ := h.Repo.EnsureDevice(ctx, f.GatewayID)

	status := "成功"
	if resp.Result != 0 {
		status = "失败"
	}
	logData := []byte(fmt.Sprintf("VoiceConfig: %s, message=%s", status, resp.Message))
	h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 1, logData, true)

	return nil
}

// HandleSocketStateResponse 处理插座状态响应（上行）
func (h *Handlers) HandleSocketStateResponse(ctx context.Context, f *Frame) error {
	resp, err := ParseSocketStateResponse(f.Data)
	if err != nil {
		return fmt.Errorf("parse socket state response: %w", err)
	}

	devID, _ := h.Repo.EnsureDevice(ctx, f.GatewayID)

	logData := []byte(fmt.Sprintf("SocketState: socket=%d, status=%s, voltage=%.1fV, current=%dmA, power=%dW",
		resp.SocketNo, GetSocketStatusDescription(resp.Status),
		float64(resp.Voltage)/10, resp.Current, resp.Power))
	h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 1, logData, true)

	// 更新插座状态到数据库
	dbStatus := int(resp.Status) // 0=空闲, 1=充电中, 2=故障
	power := int(resp.Power)     // W
	if err := h.Repo.UpsertPortState(ctx, devID, int(resp.SocketNo), dbStatus, &power); err != nil {
		// 记录错误但不中断处理流程
		errLog := []byte(fmt.Sprintf("❌failed to update port state: socket=%d err=%v", resp.SocketNo, err))
		_ = h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), 0xFFFF, 0, errLog, false)
	}

	// 更新指标
	if h.Metrics != nil {
		h.Metrics.GetPortStatusQueryResponseTotal().WithLabelValues(
			f.GatewayID,
			GetSocketStatusDescription(resp.Status),
		).Inc()
	}

	return nil
}

// HandleServiceFeeEnd 处理服务费充电结束上报（上行）
func (h *Handlers) HandleServiceFeeEnd(ctx context.Context, f *Frame) error {
	report, err := ParseServiceFeeEndReport(f.Data)
	if err != nil {
		return fmt.Errorf("parse service fee end report: %w", err)
	}

	devID, _ := h.Repo.EnsureDevice(ctx, f.GatewayID)

	logData := []byte(fmt.Sprintf("ServiceFeeEnd: port=%d, energy=%.2fkWh, electric=%.2f元, service=%.2f元, total=%.2f元",
		report.PortNo, float64(report.TotalEnergy)/100,
		float64(report.ElectricFee)/100, float64(report.ServiceFee)/100,
		float64(report.TotalAmount)/100))
	h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 1, logData, true)

	// TODO: 更新订单信息
	reply := EncodeServiceFeeEndReply(report.PortNo, 0)
	_ = reply

	return nil
}

// ===== P0修复: 充电事件推送适配方法 =====

// pushChargingStartedEventWithPort 推送充电开始事件（带端口详情）
// P0修复: 增强版本，包含电压、功率等详细信息
func (h *Handlers) pushChargingStartedEventWithPort(
	ctx context.Context,
	devicePhyID string,
	orderNo string,
	port *PortStatus,
	startTime time.Time,
) {
	// 使用已有的pushChargingStartedEvent方法，但需要先存储额外信息到data中
	// 由于event_helpers.go中的方法签名较简单，这里直接构造完整事件
	if h.EventQueue == nil {
		return
	}

	eventData := map[string]interface{}{
		"order_no":   orderNo,
		"port_no":    int(port.PortNo),
		"started_at": startTime.Unix(),
		// P0修复: 新增详细充电参数
		"voltage_v": float64(port.Voltage) / 10.0,   // 0.1V → V
		"power_w":   float64(port.Power) / 10.0,     // 0.1W → W
		"current_a": float64(port.Current) / 1000.0, // 0.001A → A
	}

	event := thirdparty.NewEvent(
		thirdparty.EventChargingStarted,
		devicePhyID,
		eventData,
	)

	// 使用pushEvent统一推送（包含去重逻辑）
	h.pushEvent(ctx, event, nil)
}

// pushChargingProgressEvent 推送充电进度事件
// P0修复: 新增方法，用于推送充电进度更新
func (h *Handlers) pushChargingProgressEvent(
	ctx context.Context,
	devicePhyID string,
	orderNo string,
	port *PortStatus,
) {
	if h.EventQueue == nil {
		return
	}

	eventData := map[string]interface{}{
		"order_no":     orderNo,
		"port_no":      int(port.PortNo),
		"duration_min": int(port.ChargingTime),         // 分钟
		"energy_kwh":   float64(port.Energy) / 100.0,   // 0.01kWh → kWh
		"power_w":      float64(port.Power) / 10.0,     // 0.1W → W
		"current_a":    float64(port.Current) / 1000.0, // 0.001A → A
		"voltage_v":    float64(port.Voltage) / 10.0,   // 0.1V → V
	}

	event := thirdparty.NewEvent(
		thirdparty.EventChargingProgress,
		devicePhyID,
		eventData,
	)

	// 使用pushEvent统一推送（包含去重逻辑）
	h.pushEvent(ctx, event, nil)
}

// P0-2修复: 检查interrupted订单恢复
// 当设备心跳恢复时,检查是否有interrupted状态的订单需要恢复为charging
func (h *Handlers) checkInterruptedOrdersRecovery(ctx context.Context, devicePhyID string, deviceID int64) error {
	// 查询该设备的interrupted订单
	orders, err := h.Repo.GetInterruptedOrders(ctx, deviceID)
	if err != nil {
		return fmt.Errorf("get interrupted orders failed: %w", err)
	}

	if len(orders) == 0 {
		return nil
	}

	// 遍历处理每个interrupted订单
	for _, order := range orders {
		// 检查订单更新时间,超过60秒未恢复则标记为failed
		if time.Since(*order.StartTime) > 60*time.Second {
			if err := h.Repo.FailOrder(ctx, order.OrderNo, "device_offline_timeout"); err != nil {
				continue
			}

			// 推送订单失败事件
			if h.EventQueue != nil {
				eventData := map[string]interface{}{
					"order_no":       order.OrderNo,
					"port_no":        order.PortNo,
					"failure_reason": "device_offline_timeout",
					"interrupted_at": order.StartTime.Unix(),
				}
				event := thirdparty.NewEvent(thirdparty.EventOrderFailed, devicePhyID, eventData)
				h.pushEvent(ctx, event, nil)
			}
			continue
		}

		// TODO: 查询端口实时状态(0x1012命令)
		// 简化实现: 假设设备恢复后端口仍在充电,直接恢复订单
		// 完整实现需要等待P1-4端口状态查询功能完成

		if err := h.Repo.RecoverOrder(ctx, order.OrderNo); err != nil {
			continue
		}

		// 推送订单恢复事件
		if h.EventQueue != nil {
			eventData := map[string]interface{}{
				"order_no":       order.OrderNo,
				"port_no":        order.PortNo,
				"interrupted_at": order.StartTime.Unix(),
				"recovered_at":   time.Now().Unix(),
			}
			event := thirdparty.NewEvent("order.recovered", devicePhyID, eventData)
			h.pushEvent(ctx, event, nil)
		}
	}

	return nil
}
