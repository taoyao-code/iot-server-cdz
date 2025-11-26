package bkv

import (
	"context"
	"fmt"
	"math"
	"time"
)

// handlersHelper 辅助方法集合

// (h *Handlers) emitter 获取或创建 EventEmitter
func (h *Handlers) emitter() *EventEmitter {
	if h.CoreEvents == nil {
		return NewEventEmitter(nil, nil)
	}
	return NewEventEmitter(h.CoreEvents, nil)
}

// (h *Handlers) replyHeartbeatACK 回复心跳ACK
func (h *Handlers) replyHeartbeatACK(deviceID string, msgID uint32) {
	if h.Outbound == nil {
		return
	}
	ackPayload := encodeHeartbeatAck(deviceID)
	_ = h.Outbound.SendDownlink(deviceID, 0x0000, msgID, ackPayload)
}

// (h *Handlers) shouldPushHeartbeat 判断是否应该推送心跳（采样）
func (h *Handlers) shouldPushHeartbeat(msgID uint32) bool {
	return h.EventQueue != nil && msgID%10 == 0
}

// (h *Handlers) sendStatusAck 构造并下发0x1017状态上报ACK
func (h *Handlers) sendStatusAck(ctx context.Context, f *Frame, payload *BKVPayload, success bool) {
	if h == nil || payload == nil {
		return
	}

	data, err := EncodeBKVStatusAck(payload, success)
	if err != nil {
		return
	}

	h.deliverBKVAck(ctx, f, payload, data, "status")
}

// (h *Handlers) sendChargingEndAck 发送充电结束ACK
func (h *Handlers) sendChargingEndAck(ctx context.Context, f *Frame, payload *BKVPayload, socketNo, portNo int, success bool) {
	if h == nil || payload == nil {
		return
	}

	var socketPtr *int
	if socketNo >= 0 {
		s := socketNo
		socketPtr = &s
	}

	var portPtr *int
	if portNo >= 0 {
		p := portNo
		portPtr = &p
	}

	data, err := EncodeBKVChargingEndAck(payload, socketPtr, portPtr, success)
	if err != nil {
		return
	}

	h.deliverBKVAck(ctx, f, payload, data, "charging-end")
}

// (h *Handlers) sendExceptionAck 发送异常ACK
func (h *Handlers) sendExceptionAck(ctx context.Context, f *Frame, payload *BKVPayload, socketNo int, success bool) {
	if h == nil || payload == nil {
		return
	}

	var socketPtr *int
	if socketNo >= 0 {
		s := socketNo
		socketPtr = &s
	}

	data, err := EncodeBKVExceptionAck(payload, socketPtr, success)
	if err != nil {
		return
	}

	h.deliverBKVAck(ctx, f, payload, data, "exception")
}

// (h *Handlers) buildParamResultMetadata 构建参数结果元数据
func buildParamResultMetadata(cmd uint16, payload []byte) map[string]string {
	return map[string]string{
		"cmd":         fmt.Sprintf("0x%04X", cmd),
		"raw_payload": fmt.Sprintf("%x", payload),
	}
}

// (h *Handlers) validateLength 校验 payload 长度是否满足期望
func (h *Handlers) validateLength(data []byte, expected int, label string) error {
	if len(data) < expected {
		return fmt.Errorf("%s payload too short: %d, expected >= %d", label, len(data), expected)
	}
	return nil
}

// (h *Handlers) parseControlPayload 提取 0x0015 长度前缀和子命令，返回子命令和内部数据
func (h *Handlers) parseControlPayload(data []byte) (subCmd byte, inner []byte, err error) {
	if err := h.validateLength(data, 3, "control"); err != nil {
		return 0, nil, err
	}
	innerLen := int(data[0])<<8 | int(data[1])
	total := 2 + innerLen
	if len(data) < total {
		return 0, nil, fmt.Errorf("control payload len mismatch: decl=%d actual=%d", innerLen, len(data)-2)
	}
	inner = data[2:total]
	if len(inner) == 0 {
		return 0, nil, fmt.Errorf("control inner payload empty")
	}
	return inner[0], inner, nil
}

// (h *Handlers) ackControlFailure 下行/上行控制失败时下发 ACK=失败（用于 0x0015）
func (h *Handlers) ackControlFailure(deviceID string, msgID uint32) {
	if h.Outbound == nil {
		return
	}
	// 子命令 0x07 失败 ACK：长度1（sub=0x07） + result(0x00)
	data := []byte{0x00, 0x02, 0x07, 0x00} // len=2, sub=0x07, result=0x00
	_ = h.Outbound.SendDownlink(deviceID, 0x0015, msgID, data)
}

// (h *Handlers) validateControlStart 校验 0x07 开始/停止控制参数范围
func (h *Handlers) validateControlStart(cmd *BKVControlCommand) error {
	if cmd == nil {
		return fmt.Errorf("control command is nil")
	}
	if cmd.SocketNo == 0 {
		return fmt.Errorf("socket_no must be 1-250")
	}
	if cmd.Port > 1 {
		return fmt.Errorf("invalid port: %d (must be 0 or 1)", cmd.Port)
	}
	if cmd.Mode != ChargingModeByTime && cmd.Mode != ChargingModeByPower && cmd.Mode != ChargingModeByLevel {
		return fmt.Errorf("invalid mode: %d", cmd.Mode)
	}
	// 按时/按量时，Duration 1-900 分钟
	if (cmd.Mode == ChargingModeByTime || cmd.Mode == ChargingModeByPower) && (cmd.Duration == 0 || cmd.Duration > 900) {
		return fmt.Errorf("invalid duration_min: %d", cmd.Duration)
	}
	// 按量模式要求 Energy >0
	if cmd.Mode == ChargingModeByPower && cmd.Energy == 0 {
		return fmt.Errorf("invalid energy_wh: %d", cmd.Energy)
	}
	// 按功率模式检查档位
	if cmd.Mode == ChargingModeByLevel {
		if cmd.LevelCount == 0 || cmd.LevelCount > 5 {
			return fmt.Errorf("invalid level_count: %d", cmd.LevelCount)
		}
		if len(cmd.PowerLevels) < int(cmd.LevelCount) {
			return fmt.Errorf("power level entries mismatch: %d", len(cmd.PowerLevels))
		}
	}
	if cmd.BusinessNo == 0 {
		return fmt.Errorf("business_no is required")
	}
	return nil
}

// (h *Handlers) upsertGatewaySocketsFromEntries 将刷新列表入库（文档 2.2.5）
func (h *Handlers) upsertGatewaySocketsFromEntries(ctx context.Context, gatewayID string, entries []SocketEntry) (int, int) {
	if h.Core == nil {
		return 0, len(entries)
	}
	now := time.Now()
	upserted, failed := 0, 0
	for _, e := range entries {
		if e.SocketNo == 0 { // 文档要求编号 1-250
			failed++
			continue
		}
		if err := h.Core.UpsertGatewaySocket(ctx, e.SocketEntryToModel(gatewayID, now)); err != nil {
			failed++
			continue
		}
		upserted++
	}
	return upserted, failed
}

// (h *Handlers) deliverBKVAck 统一的BKV ACK下发逻辑
func (h *Handlers) deliverBKVAck(ctx context.Context, f *Frame, payload *BKVPayload, data []byte, label string) {
	if h == nil || h.Outbound == nil || payload == nil || len(data) == 0 {
		return
	}

	targetGateway := payload.GatewayID
	if targetGateway == "" {
		targetGateway = f.GatewayID
	}

	if targetGateway == "" {
		return
	}

	if err := h.Outbound.SendDownlink(targetGateway, 0x1000, f.MsgID, data); err != nil {
		_ = err
	}
}

// (h *Handlers) collectChargingMetrics 采集充电指标
func (h *Handlers) collectChargingMetrics(deviceID string, portNo int, status uint8, power, current, kwh01 int) {
	if h.Metrics == nil {
		return
	}

	portStr := fmt.Sprintf("%d", portNo+1) // API端口=协议插孔+1

	// 状态统计
	statusLabel := "idle" // 充电结束=空闲
	if status&0x10 != 0 {
		statusLabel = "charging" // bit4=1表示充电中
	}
	if status&0x04 == 0 || status&0x02 == 0 {
		statusLabel = "abnormal" // 温度或电流异常
	}
	h.Metrics.GetChargeReportTotal().WithLabelValues(deviceID, portStr, statusLabel).Inc()

	// 实时功率（W）
	powerW := float64(power) / 10.0
	h.Metrics.GetChargeReportPowerGauge().WithLabelValues(deviceID, portStr).Set(powerW)

	// 实时电流（A）
	currentA := float64(current) / 1000.0
	h.Metrics.GetChargeReportCurrentGauge().WithLabelValues(deviceID, portStr).Set(currentA)

	// 累计电量（Wh）
	energyWh := float64(kwh01) * 10.0 // 0.01kWh = 10Wh
	h.Metrics.GetChargeReportEnergyTotal().WithLabelValues(deviceID, portStr).Add(energyWh)
}

// (h *Handlers) handleControlChargingEnd 处理控制帧中的充电结束上报
// 规范：端口收敛仅依赖 SessionEnded.NextPortStatus，不在充电结束路径发送 PortSnapshot
func (h *Handlers) handleControlChargingEnd(ctx context.Context, f *Frame, deviceID string, end *BKVChargingEnd) {
	// 发送 SessionEnded 事件，端口状态收敛由核心通过 NextPortStatus 完成
	nextStatus := int32(end.Status) // 使用设备上报的状态位，避免覆盖真实结束状态
	rawReason := int32(end.EndReason)
	if h.Reason != nil {
		if mapped, ok := h.Reason.Translate(int(end.EndReason)); ok {
			rawReason = int32(mapped)
		}
	}
	bizNo := fmt.Sprintf("%04X", end.BusinessNo)
	var powerW *int32
	if end.InstantPower > 0 {
		p := int32(math.Round(float64(end.InstantPower) / 10.0)) // 0.1W -> W(四舍五入)
		powerW = &p
	}

	if err := h.validateChargingEnd(end); err != nil {
		// 解析到结束帧但字段非法，回复失败 ACK 并返回错误
		h.sendChargingEndAck(ctx, f, nil, int(end.SocketNo), int(end.Port), false)
		return
	}

	evEnd := NewEventBuilder(deviceID).
		WithPort(int(end.Port)).
		WithBusinessNo(bizNo).
		BuildSessionEnded(
			int32(end.EnergyUsed),
			int32(end.ChargingTime)*60,
			&rawReason,
			&nextStatus,
			nil,
			powerW,
		)
	h.emitter().Emit(ctx, evEnd)

	// 推送充电结束事件到Webhook
	totalKwh := float64(end.EnergyUsed) / 100.0 // 0.01kWh -> kWh
	durationMin := int(end.ChargingTime)
	endReasonMsg := "正常结束"
	if end.EndReason == 1 {
		endReasonMsg = "用户停止"
	} else if end.EndReason == 8 {
		endReasonMsg = "空载结束"
	}
	h.pushChargingEndedEvent(ctx, deviceID, bizNo, int(end.Port), durationMin, totalKwh, fmt.Sprintf("%d", end.EndReason), endReasonMsg, nil)

	// 3. 回复 ACK
	h.sendChargingEndAck(ctx, f, nil, int(end.SocketNo), int(end.Port), true)
}

// (h *Handlers) handleControlChargingProgress 处理控制帧中的充电进行中状态上报
// 当 subCmd=0x02/0x18 但 Status 的 bit5=1（仍在充电）时调用此函数
// 只更新端口快照，不触发 SessionEnded
func (h *Handlers) handleControlChargingProgress(ctx context.Context, deviceID string, end *BKVChargingEnd) {
	// 1. 发送 PortSnapshot 事件更新端口状态
	rawStatus := int32(end.Status)
	var powerW *int32
	if end.InstantPower > 0 {
		p := int32(math.Round(float64(end.InstantPower) / 10.0)) // 0.1W -> W(四舍五入)
		powerW = &p
	}

	bizNo := fmt.Sprintf("%04X", end.BusinessNo)

	evPS := NewEventBuilder(deviceID).
		WithPort(int(end.Port)).
		WithSocketNo(int(end.SocketNo)).
		WithBusinessNo(bizNo).
		BuildPortSnapshot(rawStatus, powerW)
	h.emitter().Emit(ctx, evPS)

	// 2. 推送充电进度事件到Webhook（如果配置了事件队列）
	if h.EventQueue != nil {
		powerWVal := float64(end.InstantPower) / 10.0    // 0.1W -> W
		currentA := float64(end.InstantCurrent) / 1000.0 // 0.001A -> A
		energyKwh := float64(end.EnergyUsed) / 100.0     // 0.01kWh -> kWh
		durationS := int(end.ChargingTime) * 60          // min -> sec
		h.pushChargingProgressEvent(ctx, deviceID, int(end.Port), bizNo, powerWVal, currentA, 0, energyKwh, durationS, nil)
	}
}

// (h *Handlers) handleControlUplinkStatus 处理控制上行状态更新
// 规范：控制 ACK 路径不写入 PortSnapshot，端口状态由状态上报或 SessionEnded 决定
func (h *Handlers) handleControlUplinkStatus(ctx context.Context, deviceID string, socketNo, portNo int, switchFlag byte, businessNo uint16) {
	// 规范：仅保留日志，不发送 PortSnapshot 事件
	// 端口状态应由状态上报(0x1000/0x1017)或 SessionEnded 事件决定
}

// (h *Handlers) validateChargingEnd 字段校验（插座/端口/时长/能量等基本范围）
func (h *Handlers) validateChargingEnd(end *BKVChargingEnd) error {
	if end == nil {
		return fmt.Errorf("charging end is nil")
	}
	if end.SocketNo == 0 {
		return fmt.Errorf("invalid socket_no: %d", end.SocketNo)
	}
	if end.Port > 1 {
		return fmt.Errorf("invalid port: %d", end.Port)
	}
	// 充电时间分钟不应过大，按文档示例限制 <= 24h
	if end.ChargingTime > 24*60 {
		return fmt.Errorf("invalid charging_time_min: %d", end.ChargingTime)
	}
	return nil
}

// (h *Handlers) handleControlDownlinkCommand 处理控制下行命令
// 规范：控制下行路径不写入 PortSnapshot，端口状态由状态上报或 SessionEnded 决定
func (h *Handlers) handleControlDownlinkCommand(ctx context.Context, deviceID string, cmd *BKVControlCommand) {
	// 规范：仅保留日志，不发送 PortSnapshot 事件
	// 端口状态应由状态上报(0x1000/0x1017)或 SessionEnded 事件决定
}

// (h *Handlers) sendDownlinkReply 发送下行回复（通用）
func (h *Handlers) sendDownlinkReply(deviceID string, cmd uint16, msgID uint32, data []byte) {
	if h.Outbound == nil || deviceID == "" || len(data) == 0 {
		return
	}
	_ = h.Outbound.SendDownlink(deviceID, cmd, msgID, data)
}

// (h *Handlers) mapSocketStatusToRaw 映射插座状态枚举到原始状态位图
// 协议规范：bit7=在线, bit5=充电, bit4=空载
func mapSocketStatusToRaw(status int) int32 {
	switch status {
	case 0:
		return 0x90 // idle → 在线(bit7)+空载(bit4)
	case 1:
		return 0xA0 // charging → 在线(bit7)+充电(bit5)
	case 2:
		return 0x00 // fault → 离线/故障
	default:
		return 0x00
	}
}

// (h *Handlers) buildNetworkTopologyMetadata 构建网络拓扑元数据
func buildNetworkTopologyMetadata(cmd uint16, payload []byte) map[string]string {
	return map[string]string{
		"cmd":         fmt.Sprintf("0x%04X", cmd),
		"raw_payload": fmt.Sprintf("%x", payload),
	}
}
