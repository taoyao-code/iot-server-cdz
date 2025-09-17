package bkv

import (
	"context"
	"fmt"
)

// repoAPI 抽象（与 ap3000 对齐一部分能力）
type repoAPI interface {
	EnsureDevice(ctx context.Context, phyID string) (int64, error)
	InsertCmdLog(ctx context.Context, deviceID int64, msgID int, cmd int, direction int16, payload []byte, success bool) error
	UpsertPortState(ctx context.Context, deviceID int64, portNo int, status int, powerW *int) error
	UpsertOrderProgress(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh01 int, status int, powerW01 *int) error
	SettleOrder(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh01 int, reason int) error
	AckOutboundByMsgID(ctx context.Context, deviceID int64, msgID int, ok bool, errCode *int) error
	
	// 参数相关方法
	StoreParamWrite(ctx context.Context, deviceID int64, paramID int, value []byte, msgID int) error
	GetParamWritePending(ctx context.Context, deviceID int64, paramID int) ([]byte, int, error) // value, msgID, error
}

// Handlers BKV 协议处理器集合
type Handlers struct {
	Repo   repoAPI
	Reason *ReasonMap
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
		// 解析详细控制参数（按协议文档格式）
		if len(f.Data) >= 6 {
			socketNo := int(f.Data[0])     // 插座号
			portNo := int(f.Data[1])       // 插孔号 (0=A孔, 1=B孔)
			switchState := int(f.Data[2])  // 开关状态 (1=开, 0=关)
			// chargeMode := int(f.Data[3])   // 充电模式 (1=按时, 0=按量)
			
			// 充电时长(分钟)或电量(wh) - 16位大端
			duration := int(f.Data[4])<<8 | int(f.Data[5])
			
			if switchState == 1 {
				// 开始充电：创建订单并更新端口状态
				orderHex := fmt.Sprintf("%04X%02X%02X", f.MsgID, socketNo, portNo)
				
				// 创建充电订单（状态1=进行中）
				if err := h.Repo.UpsertOrderProgress(ctx, devID, portNo, orderHex, duration, 0, 1, nil); err != nil {
					success = false
				} else {
					// 更新端口状态为充电中
					chargingStatus := 1 // 1=充电中
					if err := h.Repo.UpsertPortState(ctx, devID, portNo, chargingStatus, nil); err != nil {
						success = false
					}
				}
			} else {
				// 停止充电：更新端口状态为空闲
				idleStatus := 0 // 0=空闲
				if err := h.Repo.UpsertPortState(ctx, devID, portNo, idleStatus, nil); err != nil {
					success = false
				}
			}
		}
	} else {
		// 上行控制响应（设备回复）- 可能需要更新ACK状态
		if err := h.Repo.AckOutboundByMsgID(ctx, devID, int(f.MsgID), true, nil); err != nil {
			success = false
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
			portNo := int(f.Data[8])  // 插孔号
			
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
