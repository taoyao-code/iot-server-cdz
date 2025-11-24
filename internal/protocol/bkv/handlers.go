package bkv

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/taoyao-code/iot-server/internal/driverapi"
	"github.com/taoyao-code/iot-server/internal/storage"
	"github.com/taoyao-code/iot-server/internal/thirdparty"
)

// repoAPI 占位（保持构造函数兼容），驱动侧不直接写库。
type repoAPI interface{}

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
	GetPortStatusQueryResponseTotal() *prometheus.CounterVec
}

// Handlers BKV 协议处理器集合
type Handlers struct {
	Core       storage.CoreRepo
	Reason     *ReasonMap
	Outbound   OutboundSender         // Week5: 下行消息发送器
	EventQueue *thirdparty.EventQueue // v2.1: 事件队列（第三方推送）
	Deduper    *thirdparty.Deduper    // v2.1: 去重器
	Metrics    MetricsAPI             // v2.1: 监控指标（Prometheus）

	// CoreEvents 为驱动 -> 核心 的事件上报入口
	CoreEvents driverapi.EventSink
}

// HandleHeartbeat 处理心跳帧 (cmd=0x0000 或 BKV cmd=0x1017)
func (h *Handlers) HandleHeartbeat(ctx context.Context, f *Frame) error {
	// 1. 提取设备ID
	deviceID := extractDeviceIDOrDefault(f)

	// 2. 发送心跳事件到核心
	event := NewEventBuilder(deviceID).BuildHeartbeat()
	h.emitter().Emit(ctx, event)

	// 3. 采样推送第三方心跳事件（每10次推送1次）
	if h.shouldPushHeartbeat(f.MsgID) {
		h.pushDeviceHeartbeatEvent(
			ctx,
			deviceID,
			220.0, // voltage - 默认值，实际应解析
			-50,   // rssi - 默认值，实际应解析
			25.0,  // temp - 默认值，实际应解析
			nil,   // ports - 可选
			nil,   // logger可选
		)
	}

	// 4. 回复心跳ACK（关键：否则设备会在60秒后断开连接）
	h.replyHeartbeatACK(deviceID, f.MsgID)

	return nil
}

// encodeHeartbeatAck moved to utils.go

// HandleBKVStatus 处理BKV插座状态上报 (cmd=0x1000 with BKV payload)
func (h *Handlers) HandleBKVStatus(ctx context.Context, f *Frame) error {
	// 1. 获取BKV载荷
	payload, err := f.GetBKVPayload()
	if err != nil {
		return fmt.Errorf("failed to parse BKV payload: %w", err)
	}

	// 2. 根据载荷类型分发处理
	if payload.IsStatusReport() {
		err := h.handleSocketStatusUpdate(ctx, payload)
		h.sendStatusAck(ctx, f, payload, err == nil)
		return err
	}

	if payload.IsChargingEnd() {
		return h.handleBKVChargingEnd(ctx, f, payload)
	}

	if payload.IsExceptionReport() {
		return h.handleExceptionEvent(ctx, f, payload)
	}

	if payload.IsParameterQuery() {
		return h.handleParameterQuery(ctx, payload)
	}

	if payload.IsControlCommand() {
		return h.handleBKVControlCommand(ctx, payload)
	}

	return nil
}

// sendStatusAck, sendChargingEndAck, sendExceptionAck, deliverBKVAck moved to handlers_helper.go

// handleSocketStatusUpdate 处理插座状态更新
func (h *Handlers) handleSocketStatusUpdate(ctx context.Context, payload *BKVPayload) error {
	if h.emitter() == nil || !h.emitter().IsConfigured() {
		return nil
	}

	socketStatus, err := payload.GetSocketStatus()
	if err != nil {
		return fmt.Errorf("parse socket status: %w", err)
	}

	deviceID := extractDeviceIDFromPayload(nil, payload)

	// 发送端口快照事件
	emitPortSnapshot := func(port *PortStatus) error {
		if port == nil {
			return nil
		}

		rawStatus := int32(port.Status)
		var power *int32
		if port.Power > 0 {
			p := int32(port.Power) / 10 // 0.1W → W
			power = &p
		}

		event := NewEventBuilder(deviceID).
			WithPort(int(port.PortNo)).
			BuildPortSnapshot(rawStatus, power)
		h.emitter().Emit(ctx, event)

		// 如果正在充电，推送充电进度事件到Webhook
		isCharging := (port.Status & 0x20) != 0 // bit5=充电
		if isCharging && h.EventQueue != nil {
			powerW := float64(port.Power) / 10.0       // 0.1W -> W
			currentA := float64(port.Current) / 1000.0 // 0.001A -> A
			voltageV := float64(port.Voltage) / 10.0   // 0.1V -> V
			energyKwh := float64(port.Energy) / 100.0  // 0.01kWh -> kWh
			durationS := int(port.ChargingTime) * 60   // min -> sec
			businessNo := fmt.Sprintf("%04X", port.BusinessNo)
			h.pushChargingProgressEvent(ctx, deviceID, int(port.PortNo), businessNo, powerW, currentA, voltageV, energyKwh, durationS, nil)
		}

		return nil
	}

	// 处理两个端口
	if err := emitPortSnapshot(socketStatus.PortA); err != nil {
		return err
	}
	if err := emitPortSnapshot(socketStatus.PortB); err != nil {
		return err
	}

	return nil
}

// handleBKVChargingEnd 处理BKV格式的充电结束上报
func (h *Handlers) handleBKVChargingEnd(ctx context.Context, f *Frame, payload *BKVPayload) error {
	var socketNo int = -1
	var portNo int = -1
	var orderID int
	var kwh01 int
	var durationSec int
	var reason int
	success := false

	defer func() {
		h.sendChargingEndAck(ctx, f, payload, socketNo, portNo, success)
	}()

	// 解析BKV字段
	for _, field := range payload.Fields {
		switch field.Tag {
		case 0x4A: // 插座号
			if len(field.Value) >= 1 {
				socketNo = int(field.Value[0])
			}
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

	// 原因码映射
	if h.Reason != nil {
		if mappedReason, ok := h.Reason.Translate(reason); ok {
			reason = mappedReason
		}
	}

	// 生成订单号
	orderHex := fmt.Sprintf("%04X", orderID)
	actualPort := portNo
	if actualPort < 0 {
		actualPort = 0
	}

	// 发送充电结束事件
	deviceID := extractDeviceIDFromPayload(f, payload)
	if h.emitter() == nil || !h.emitter().IsConfigured() || deviceID == "" {
		return fmt.Errorf("core events sink not configured for BKV charging end")
	}

	nextStatus := int32(0x90) // 0x90 = bit7(在线) + bit4(空载)
	rawReason := int32(reason)

	event := NewEventBuilder(deviceID).
		WithPort(actualPort).
		WithBusinessNo(orderHex).
		BuildSessionEnded(
			int32(kwh01),
			int32(durationSec),
			&rawReason,
			&nextStatus,
			nil, // amountCent
			nil, // instantPowerW
		)

	if err := h.emitter().EmitWithCheck(ctx, event); err != nil {
		return fmt.Errorf("core event session ended failed: %w", err)
	}

	// 推送充电结束事件到Webhook
	totalKwh := float64(kwh01) / 100.0 // 0.01kWh -> kWh
	durationMin := durationSec / 60
	endReasonMsg := "正常结束"
	if reason == 1 {
		endReasonMsg = "用户停止"
	} else if reason == 8 {
		endReasonMsg = "空载结束"
	}
	h.pushChargingEndedEvent(ctx, deviceID, orderHex, actualPort, durationMin, totalKwh, fmt.Sprintf("%d", reason), endReasonMsg, nil)

	success = true
	return nil
}

// HandleControl 处理控制指令 (cmd=0x0015)
func (h *Handlers) HandleControl(ctx context.Context, f *Frame) error {
	deviceID := extractDeviceIDOrDefault(f)

	if f.IsUplink() {
		// 1. 处理充电结束/功率模式结束上报（子命令 0x02 / 0x18）
		if len(f.Data) >= 3 && (f.Data[2] == 0x02 || f.Data[2] == 0x18) {
			if end, err := ParseBKVChargingEnd(f.Data); err == nil {
				h.handleControlChargingEnd(ctx, f, deviceID, end)
				return nil
			}
		}

		// 2. 处理其他格式的控制命令上行
		if len(f.Data) >= 2 && len(f.Data) < 64 {
			innerLen := (int(f.Data[0]) << 8) | int(f.Data[1])
			totalLen := 2 + innerLen
			if innerLen >= 5 && len(f.Data) >= totalLen {
				inner := f.Data[2:totalLen]
				if len(inner) >= 5 && inner[0] == 0x07 {
					socketNo := int(inner[1])
					portNo := int(inner[2])
					switchFlag := inner[3]
					var businessNo uint16
					if len(inner) >= 6 {
						businessNo = binary.BigEndian.Uint16(inner[4:6])
					}
					h.handleControlUplinkStatus(ctx, deviceID, socketNo, portNo, switchFlag, businessNo)
				}
			}
		}
	} else {
		// 3. 处理控制下行命令
		if cmd, err := ParseBKVControlCommand(f.Data); err == nil {
			h.handleControlDownlinkCommand(ctx, deviceID, cmd)
		}
	}

	return nil
}

// HandleChargingEnd 处理充电结束上报 (cmd=0x0015 上行，特定格式)
func (h *Handlers) HandleChargingEnd(ctx context.Context, f *Frame) error {
	deviceID := extractDeviceIDOrDefault(f)

	// 只处理上行的充电结束上报
	if !f.IsUplink() || len(f.Data) < 20 {
		return nil
	}

	// 确认是充电结束命令 (data[2] == 0x02)
	if f.Data[2] != 0x02 {
		return nil
	}

	// 解析充电数据
	portNo := int(f.Data[8])                            // 插孔号
	orderID := int(f.Data[10])<<8 | int(f.Data[11])     // 业务号
	power := int(f.Data[12])<<8 | int(f.Data[13])       // 瞬时功率（0.1W）
	current := int(f.Data[14])<<8 | int(f.Data[15])     // 瞬时电流（0.001A）
	kwh01 := int(f.Data[16])<<8 | int(f.Data[17])       // 用电量（0.01kWh）
	durationMin := int(f.Data[18])<<8 | int(f.Data[19]) // 充电时间（分钟）
	status := f.Data[9]                                 // 插座状态

	// 提取并映射结束原因
	reason := extractEndReason(status)
	if h.Reason != nil {
		if mappedReason, ok := h.Reason.Translate(reason); ok {
			reason = mappedReason
		}
	}

	// 采集充电指标
	h.collectChargingMetrics(deviceID, portNo, status, power, current, kwh01)

	// 发送充电结束事件
	if h.emitter().IsConfigured() {
		orderHex := fmt.Sprintf("%04X", orderID)
		nextStatus := int32(0x90) // 0x90 = bit7(在线) + bit4(空载)
		rawReason := int32(reason)

		event := NewEventBuilder(deviceID).
			WithPort(portNo).
			WithBusinessNo(orderHex).
			BuildSessionEnded(
				int32(kwh01),
				int32(durationMin*60),
				&rawReason,
				&nextStatus,
				nil, // amountCent
				nil, // instantPowerW
			)
		h.emitter().Emit(ctx, event)
	}

	return nil
}

// extractEndReason 从插座状态中提取结束原因（简化版本）
// HandleGeneric 通用处理器，记录所有其他指令
func (h *Handlers) HandleGeneric(ctx context.Context, f *Frame) error {
	deviceID := extractDeviceIDOrDefault(f)

	event := NewEventBuilder(deviceID).BuildException(
		"generic_cmd",
		fmt.Sprintf("cmd=0x%04X", f.Cmd),
		"info",
		nil,
		map[string]string{"payload": fmt.Sprintf("%x", f.Data)},
	)

	h.emitter().Emit(ctx, event)
	return nil
}

// HandleNetworkList 处理0x0005 网络节点列表相关指令（2.2.5/2.2.6 ACK）
func (h *Handlers) HandleNetworkList(ctx context.Context, f *Frame) error {
	deviceID := extractDeviceIDOrDefault(f)
	d := f.Data

	// 构建基础元数据
	action := "network_ack"
	result := "unknown"
	msg := fmt.Sprintf("NetworkCmd0005: short payload len=%d payload=%x", len(d), d)
	metadata := map[string]string{"cmd": fmt.Sprintf("0x%04X", f.Cmd)}

	// 解析子命令和结果
	if len(d) >= 4 {
		subCmd := d[2]
		rawResult := d[3]
		metadata["sub_cmd"] = fmt.Sprintf("0x%02X", subCmd)
		metadata["raw_result"] = fmt.Sprintf("%d", rawResult)

		switch subCmd {
		case 0x08:
			action = "refresh_ack"
		case 0x09:
			action = "add_ack"
		default:
			action = "network_ack"
		}

		result = "ok"
		if rawResult != 0x01 {
			result = "failed"
		}
		msg = fmt.Sprintf("%s result=%d", action, rawResult)
	}

	// 发送网络拓扑事件
	event := NewEventBuilder(deviceID).BuildNetworkTopology(action, result, msg, metadata)
	h.emitter().Emit(ctx, event)
	return nil
}

// HandleParam 处理参数读写指令
func (h *Handlers) HandleParam(ctx context.Context, f *Frame) error {
	deviceID := extractDeviceIDOrDefault(f)

	// 构建基础元数据
	result := "param"
	msg := "param message"
	metadata := map[string]string{
		"cmd":     fmt.Sprintf("0x%04X", f.Cmd),
		"payload": fmt.Sprintf("%x", f.Data),
	}

	// 根据命令码判断参数操作类型
	switch f.Cmd {
	case 0x83, 0x84: // 参数写入
		result = "write_ack"
		msg = "param write ack"
	case 0x85: // 参数回读
		result = "readback"
		if len(f.Data) > 0 {
			readback := DecodeParamReadback(f.Data)
			metadata["param_id"] = fmt.Sprintf("%d", readback.ParamID)
			metadata["value_hex"] = fmt.Sprintf("%x", readback.Value)
		}
	}

	// 发送参数结果事件
	event := NewEventBuilder(deviceID).BuildParamResult(result, msg, metadata)
	h.emitter().Emit(ctx, event)
	return nil
}

// handleExceptionEvent 处理异常事件上报
func (h *Handlers) handleExceptionEvent(ctx context.Context, f *Frame, payload *BKVPayload) error {
	event, err := ParseBKVExceptionEvent(payload)
	if err != nil {
		h.sendExceptionAck(ctx, f, payload, -1, false)
		return fmt.Errorf("failed to parse exception event: %w", err)
	}

	success := false
	defer func() {
		socket := -1
		if event != nil {
			socket = int(event.SocketNo)
		}
		h.sendExceptionAck(ctx, f, payload, socket, success)
	}()

	deviceID := extractDeviceIDFromPayload(f, payload)

	// 发送异常事件
	rawStatus := int32(event.SocketEventStatus)
	meta := map[string]string{"reason": fmt.Sprintf("%d", event.SocketEventReason)}

	ev := NewEventBuilder(deviceID).
		WithPort(int(event.SocketNo)).
		BuildException(
			fmt.Sprintf("socket_event_%d", event.SocketEventReason),
			fmt.Sprintf("status=%d", event.SocketEventStatus),
			"error",
			&rawStatus,
			meta,
		)

	h.emitter().Emit(ctx, ev)
	success = true
	return nil
}

// handleParameterQuery 处理参数查询
func (h *Handlers) handleParameterQuery(ctx context.Context, payload *BKVPayload) error {
	// TODO 暂时不需要实现
	return nil
}

// handleBKVControlCommand 处理BKV控制命令
func (h *Handlers) handleBKVControlCommand(ctx context.Context, payload *BKVPayload) error {
	if payload.IsCardCharging() {
		return h.handleCardCharging(ctx, payload)
	}

	deviceID := extractDeviceIDFromPayload(nil, payload)
	meta := map[string]string{"cmd": fmt.Sprintf("0x%02X", payload.Cmd)}

	event := NewEventBuilder(deviceID).BuildException(
		"control_command",
		"control command received",
		"info",
		nil,
		meta,
	)

	h.emitter().Emit(ctx, event)
	return nil
}

// handleCardCharging 处理刷卡充电
func (h *Handlers) handleCardCharging(ctx context.Context, payload *BKVPayload) error {
	// TODO 暂时不需要实现
	return nil
}

// ============ Week4: 刷卡充电处理函数 ============

// HandleCardSwipe 处理刷卡上报 (0x0B)
func (h *Handlers) HandleCardSwipe(ctx context.Context, f *Frame) error {
	// TODO 暂时不需要实现
	return nil
}

// handleCardSwipeUplink 处理刷卡上报上行
func (h *Handlers) handleCardSwipeUplink(ctx context.Context, f *Frame) error {
	// TODO 暂时不需要实现
	return nil
}

// HandleOrderConfirm 处理订单确认 (0x0F)
func (h *Handlers) HandleOrderConfirm(ctx context.Context, f *Frame) error {
	if f.IsUplink() {
		return h.handleOrderConfirmUplink(ctx, f)
	}
	return nil
}

// handleOrderConfirmUplink 处理订单确认上行
func (h *Handlers) handleOrderConfirmUplink(ctx context.Context, f *Frame) error {
	conf, err := ParseOrderConfirmation(f.Data)
	if err != nil {
		return fmt.Errorf("failed to parse order confirmation: %w", err)
	}

	deviceID := extractDeviceIDOrDefault(f)
	if deviceID == "BKV-UNKNOWN" {
		return fmt.Errorf("missing gateway ID")
	}

	// 发送会话开始事件
	metadata := map[string]string{
		"status": fmt.Sprintf("%d", conf.Status),
		"reason": conf.Reason,
	}

	event := NewEventBuilder(deviceID).
		WithPort(0).
		WithBusinessNo(conf.OrderNo).
		BuildSessionStarted("order_confirm", nil, metadata)

	h.emitter().Emit(ctx, event)
	return nil
}

// HandleChargeEnd 处理充电结束 (0x0C)
func (h *Handlers) HandleChargeEnd(ctx context.Context, f *Frame) error {
	if f.IsUplink() {
		return h.handleChargeEndUplink(ctx, f)
	}
	return nil
}

// handleChargeEndUplink 处理充电结束上行
func (h *Handlers) handleChargeEndUplink(ctx context.Context, f *Frame) error {
	report, err := ParseChargeEndReport(f.Data)
	if err != nil {
		return fmt.Errorf("failed to parse charge end: %w", err)
	}

	deviceID := extractDeviceIDOrDefault(f)
	if deviceID == "BKV-UNKNOWN" {
		return fmt.Errorf("missing gateway ID")
	}

	// 发送会话结束事件
	amount := int64(report.Amount)
	rawReason := int32(report.EndReason)

	event := NewEventBuilder(deviceID).
		WithPort(0).
		WithBusinessNo(report.OrderNo).
		BuildSessionEnded(
			int32(report.Energy/10),
			int32(report.Duration*60),
			&rawReason,
			nil, // nextStatus
			&amount,
			nil, // instantPowerW
		)

	h.emitter().Emit(ctx, event)
	return nil
}

// HandleBalanceQuery 处理余额查询 (0x1A)
func (h *Handlers) HandleBalanceQuery(ctx context.Context, f *Frame) error {
	if f.IsUplink() {
		return h.handleBalanceQueryUplink(ctx, f)
	}
	return nil
}

// handleBalanceQueryUplink 处理余额查询上行
func (h *Handlers) handleBalanceQueryUplink(ctx context.Context, f *Frame) error {
	// TODO 暂时不需要实现
	return nil
}

// ===== Week 6: 组网管理处理器 =====

// HandleNetworkRefresh 处理刷新插座列表响应（上行）
func (h *Handlers) HandleNetworkRefresh(ctx context.Context, f *Frame) error {
	deviceID := extractDeviceIDOrDefault(f)

	resp, err := ParseNetworkRefreshResponse(f.Data)
	result := "ok"
	msg := "network refresh"
	metadata := map[string]string{"cmd": fmt.Sprintf("0x%04X", f.Cmd)}

	if err != nil {
		result = "failed"
		msg = err.Error()
		metadata["raw_payload"] = fmt.Sprintf("%x", f.Data)
	} else {
		metadata["socket_count"] = fmt.Sprintf("%d", len(resp.Sockets))
		// 批量更新插座映射
		upserted, upsertErrors := h.upsertGatewaySockets(ctx, deviceID, resp.Sockets, nil)
		if upserted > 0 {
			metadata["mapping_upserted"] = fmt.Sprintf("%d", upserted)
		}
		if upsertErrors > 0 {
			metadata["mapping_upsert_errors"] = fmt.Sprintf("%d", upsertErrors)
		}
	}

	event := NewEventBuilder(deviceID).BuildNetworkTopology("refresh", result, msg, metadata)
	h.emitter().Emit(ctx, event)

	return err
}

// HandleNetworkAddNode 处理添加插座响应（上行）
func (h *Handlers) HandleNetworkAddNode(ctx context.Context, f *Frame) error {
	deviceID := extractDeviceIDOrDefault(f)

	resp, err := ParseNetworkAddNodeResponse(f.Data)
	result := "ok"
	msg := "add socket success"
	metadata := map[string]string{"cmd": fmt.Sprintf("0x%04X", f.Cmd)}
	var socketNo *int32

	if err != nil {
		result = "failed"
		msg = err.Error()
		metadata["raw_payload"] = fmt.Sprintf("%x", f.Data)
	} else {
		sn := int32(resp.SocketNo)
		socketNo = &sn
		metadata["raw_result"] = fmt.Sprintf("%d", resp.Result)
		if resp.Result != 0 {
			result = "failed"
			if resp.Reason != "" {
				msg = resp.Reason
			} else {
				msg = "add socket failed"
			}
		}
	}

	builder := NewEventBuilder(deviceID)
	if socketNo != nil {
		builder.WithSocketNo(int(*socketNo))
	}
	event := builder.BuildNetworkTopology("add_node", result, msg, metadata)
	h.emitter().Emit(ctx, event)

	return err
}

// HandleNetworkDeleteNode 处理删除插座响应（上行）
func (h *Handlers) HandleNetworkDeleteNode(ctx context.Context, f *Frame) error {
	deviceID := extractDeviceIDOrDefault(f)

	resp, err := ParseNetworkDeleteNodeResponse(f.Data)
	result := "ok"
	msg := "delete socket success"
	metadata := map[string]string{"cmd": fmt.Sprintf("0x%04X", f.Cmd)}
	var socketNo *int32

	if err != nil {
		result = "failed"
		msg = err.Error()
		metadata["raw_payload"] = fmt.Sprintf("%x", f.Data)
	} else {
		sn := int32(resp.SocketNo)
		socketNo = &sn
		metadata["raw_result"] = fmt.Sprintf("%d", resp.Result)
		if resp.Result != 0 {
			result = "failed"
			if resp.Reason != "" {
				msg = resp.Reason
			} else {
				msg = "delete socket failed"
			}
		}
	}

	builder := NewEventBuilder(deviceID)
	if socketNo != nil {
		builder.WithSocketNo(int(*socketNo))
	}
	event := builder.BuildNetworkTopology("delete_node", result, msg, metadata)
	h.emitter().Emit(ctx, event)

	return err
}

// ===== Week 7: OTA升级处理器 =====

// HandleOTAResponse 处理OTA升级响应（上行）
func (h *Handlers) HandleOTAResponse(ctx context.Context, f *Frame) error {
	// TODO 暂时不需要实现
	return nil
}

// HandleOTAProgress 处理OTA升级进度上报（上行）
func (h *Handlers) HandleOTAProgress(ctx context.Context, f *Frame) error {
	// TODO 暂时不需要实现
	return nil
}

// ===== Week 8: 按功率分档充电处理器 =====

// HandlePowerLevelEnd 处理按功率充电结束上报（上行）
func (h *Handlers) HandlePowerLevelEnd(ctx context.Context, f *Frame) error {
	report, err := ParsePowerLevelEndReport(f.Data)
	if err != nil {
		return fmt.Errorf("parse power level end report: %w", err)
	}

	deviceID := extractDeviceIDOrDefault(f)

	rawReason := int32(report.EndReason)
	duration := int32(report.TotalDuration) * 60
	energy := int32(report.TotalEnergy)
	amount := int64(report.TotalAmount)

	event := NewEventBuilder(deviceID).
		WithPort(int(report.PortNo)).
		BuildSessionEnded(energy, duration, &rawReason, nil, &amount, nil)
	h.emitter().Emit(ctx, event)

	// 发送确认回复
	reply := EncodePowerLevelEndReply(report.PortNo, 0)
	h.sendDownlinkReply(deviceID, 0x0018, f.MsgID, reply)

	return nil
}

// ===== Week 9: 参数管理处理器 =====

// HandleParamReadResponse 处理批量读取参数响应（上行）
func (h *Handlers) HandleParamReadResponse(ctx context.Context, f *Frame) error {
	// TODO 暂时不需要实现
	return nil
}

// HandleParamWriteResponse 处理批量写入参数响应（上行）
func (h *Handlers) HandleParamWriteResponse(ctx context.Context, f *Frame) error {
	// TODO 暂时不需要实现
	return nil
}

// HandleParamSyncResponse 处理参数同步响应（上行）
func (h *Handlers) HandleParamSyncResponse(ctx context.Context, f *Frame) error {
	// TODO 暂时不需要实现
	return nil
}

// HandleParamResetResponse 处理参数重置响应（上行）
func (h *Handlers) HandleParamResetResponse(ctx context.Context, f *Frame) error {
	deviceID := extractDeviceIDOrDefault(f)

	resp, err := ParseParamResetResponse(f.Data)
	result := "ok"
	msg := "param reset success"
	metadata := map[string]string{"cmd": fmt.Sprintf("0x%04X", f.Cmd)}

	if err != nil {
		result = "failed"
		msg = err.Error()
		metadata["raw_payload"] = fmt.Sprintf("%x", f.Data)
	} else {
		metadata["raw_result"] = fmt.Sprintf("%d", resp.Result)
		if resp.Result != 0 {
			result = "failed"
			if resp.Message != "" {
				msg = resp.Message
			} else {
				msg = "param reset failed"
			}
		} else if resp.Message != "" {
			msg = resp.Message
		}
	}

	event := NewEventBuilder(deviceID).BuildParamResult(result, msg, metadata)
	h.emitter().Emit(ctx, event)

	return err
}

// ===== Week 10: 扩展功能处理器 =====

// HandleVoiceConfigResponse 处理语音配置响应（上行）
func (h *Handlers) HandleVoiceConfigResponse(ctx context.Context, f *Frame) error {
	deviceID := extractDeviceIDOrDefault(f)

	resp, err := ParseVoiceConfigResponse(f.Data)
	result := "ok"
	msg := "voice config success"
	metadata := map[string]string{"cmd": fmt.Sprintf("0x%04X", f.Cmd)}

	if err != nil {
		result = "failed"
		msg = err.Error()
		metadata["raw_payload"] = fmt.Sprintf("%x", f.Data)
	} else {
		metadata["raw_result"] = fmt.Sprintf("%d", resp.Result)
		if resp.Result != 0 {
			result = "failed"
			if resp.Message != "" {
				msg = resp.Message
			} else {
				msg = "voice config failed"
			}
		} else if resp.Message != "" {
			msg = resp.Message
		}
	}

	event := NewEventBuilder(deviceID).BuildParamResult(result, msg, metadata)
	h.emitter().Emit(ctx, event)

	return err
}

// HandleSocketStateResponse 处理插座状态响应（上行）
func (h *Handlers) HandleSocketStateResponse(ctx context.Context, f *Frame) error {
	resp, err := ParseSocketStateResponse(f.Data)
	if err != nil {
		return fmt.Errorf("parse socket state response: %w", err)
	}

	deviceID := extractDeviceIDOrDefault(f)

	// 映射业务枚举到BKV状态位图
	dbStatus := mapSocketStatusToRaw(int(resp.Status))
	power := int32(resp.Power)

	event := NewEventBuilder(deviceID).
		WithPort(int(resp.SocketNo)).
		BuildPortSnapshot(dbStatus, &power)
	h.emitter().Emit(ctx, event)

	// 更新指标
	if h.Metrics != nil {
		h.Metrics.GetPortStatusQueryResponseTotal().WithLabelValues(
			deviceID,
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

	deviceID := extractDeviceIDOrDefault(f)

	rawReason := int32(report.EndReason)
	duration := int32(report.TotalDuration) * 60
	energy := int32(report.TotalEnergy)
	total := int64(report.TotalAmount)

	event := NewEventBuilder(deviceID).
		WithPort(int(report.PortNo)).
		BuildSessionEnded(energy, duration, &rawReason, nil, &total, nil)
	h.emitter().Emit(ctx, event)

	// 发送确认回复
	reply := EncodeServiceFeeEndReply(report.PortNo, 0)
	h.sendDownlinkReply(deviceID, f.Cmd, f.MsgID, reply)

	return nil
}
