package bkv

import (
	"context"
	"fmt"
)

// repoAPI 抽象（与 ap3000 对齐一部分能力）
// P0修复: 扩展接口支持参数持久化
type repoAPI interface {
	EnsureDevice(ctx context.Context, phyID string) (int64, error)
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
}

// CardServiceAPI 刷卡充电服务接口
type CardServiceAPI interface {
	HandleCardSwipe(ctx context.Context, req *CardSwipeRequest) (*ChargeCommand, error)
	HandleOrderConfirmation(ctx context.Context, conf *OrderConfirmation) error
	HandleChargeEnd(ctx context.Context, report *ChargeEndReport) error
	HandleBalanceQuery(ctx context.Context, query *BalanceQuery) (*BalanceResponse, error)
}

// Handlers BKV 协议处理器集合
type Handlers struct {
	Repo        repoAPI
	Reason      *ReasonMap
	CardService CardServiceAPI // Week4: 刷卡充电服务
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

	// 记录心跳日志
	success := true
	return h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), getDirection(f.IsUplink()), f.Data, success)
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
func (h *Handlers) handleSocketStatusUpdate(ctx context.Context, deviceID int64, payload *BKVPayload) error {
	// 使用GetSocketStatus方法解析完整的插座状态
	socketStatus, err := payload.GetSocketStatus()
	if err != nil {
		// 如果解析失败，回退到简化解析
		return h.handleSocketStatusUpdateSimple(ctx, deviceID, payload)
	}

	// 更新端口A状态
	if socketStatus.PortA != nil {
		portA := socketStatus.PortA
		status := int(portA.Status)
		var powerW *int
		if portA.Power > 0 {
			power := int(portA.Power) / 10 // 从0.1W转换为W
			powerW = &power
		}

		if err := h.Repo.UpsertPortState(ctx, deviceID, int(portA.PortNo), status, powerW); err != nil {
			return fmt.Errorf("failed to update port A state: %w", err)
		}
	}

	// 更新端口B状态
	if socketStatus.PortB != nil {
		portB := socketStatus.PortB
		status := int(portB.Status)
		var powerW *int
		if portB.Power > 0 {
			power := int(portB.Power) / 10 // 从0.1W转换为W
			powerW = &power
		}

		if err := h.Repo.UpsertPortState(ctx, deviceID, int(portB.PortNo), status, powerW); err != nil {
			return fmt.Errorf("failed to update port B state: %w", err)
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
		// 如果是上行（设备回复），可能是充电结束上报
		if len(f.Data) >= 15 {
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

			// 解析用电量（16位，单位：0.01kWh）
			kwh01 := int(f.Data[16])<<8 | int(f.Data[17])

			// 解析充电时间（16位，单位：分钟）
			durationMin := int(f.Data[18])<<8 | int(f.Data[19])
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

		// TODO: 下发充电指令到设备
		// 这需要通过outbound队列发送下行消息
		// SendChargeCommand(f.GatewayID, cmd)
		_ = cmd // 暂时忽略，等待outbound集成
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

		// TODO: 下发确认回复到设备
		// reply := &OrderConfirmReply{OrderNo: conf.OrderNo, Result: 0}
		// SendOrderConfirmReply(f.GatewayID, reply)
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

		// TODO: 下发结束确认到设备
		// reply := &ChargeEndReply{OrderNo: report.OrderNo, Result: 0}
		// SendChargeEndReply(f.GatewayID, reply)
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

		// TODO: 下发余额响应到设备
		// SendBalanceResponse(f.GatewayID, resp)
		_ = resp // 暂时忽略，等待outbound集成
	}

	return nil
}
