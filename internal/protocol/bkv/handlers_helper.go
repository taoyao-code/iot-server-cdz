package bkv

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/taoyao-code/iot-server/internal/coremodel"
	"github.com/taoyao-code/iot-server/internal/ordersession"
	"go.uber.org/zap"
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

func (h *Handlers) tracker() *ordersession.Tracker {
	if h == nil {
		return nil
	}
	if h.OrderTracker == nil {
		h.OrderTracker = ordersession.NewTracker()
	}
	return h.OrderTracker
}

func (h *Handlers) resolveOrderSession(deviceID string, port int, businessNo string) (*ordersession.ActiveSession, string) {
	tracker := h.tracker()
	if tracker == nil {
		return nil, ""
	}
	if businessNo != "" {
		if session, ok := tracker.LookupByBusiness(deviceID, businessNo); ok {
			return session, "business_no"
		}
	}
	if session, ok := tracker.Lookup(deviceID, port); ok {
		return session, "port_fallback"
	}
	return nil, ""
}

// resolveDeviceID 确保帧中包含合法的网关ID
func (h *Handlers) resolveDeviceID(f *Frame, action string) (string, error) {
	if f == nil {
		return "", fmt.Errorf("%s: empty frame", action)
	}

	deviceID := extractDeviceIDOrDefault(f)
	if deviceID == "" || deviceID == "BKV-UNKNOWN" {
		if action == "" {
			action = "bkv handler"
		}
		return "", fmt.Errorf("%s: missing gateway ID", action)
	}
	return deviceID, nil
}

// buildSessionStartedEvent 构建会话开始事件，统一端口/业务号填充逻辑
func (h *Handlers) buildSessionStartedEvent(
	deviceID string,
	port int,
	businessNo string,
	mode string,
	cardNo *string,
	metadata map[string]string,
) *coremodel.CoreEvent {
	builder := NewEventBuilder(deviceID)
	if port >= 0 {
		builder = builder.WithPort(port)
	}
	if businessNo != "" {
		builder = builder.WithBusinessNo(businessNo)
	}
	return builder.BuildSessionStarted(mode, cardNo, metadata)
}

// buildSessionEndedEvent 构建会话结束事件，确保端口/业务号处理一致
func (h *Handlers) buildSessionEndedEvent(
	deviceID string,
	port int,
	businessNo string,
	energyKWh01 int32,
	durationSec int32,
	rawReason *int32,
	nextStatus *int32,
	amountCent *int64,
	instantPowerW *int32,
) *coremodel.CoreEvent {
	builder := NewEventBuilder(deviceID)
	if port >= 0 {
		builder = builder.WithPort(port)
	}
	if businessNo != "" {
		builder = builder.WithBusinessNo(businessNo)
	}
	return builder.BuildSessionEnded(energyKWh01, durationSec, rawReason, nextStatus, amountCent, instantPowerW)
}

// sendBKVAck 通用的BKV ACK构造与发送逻辑
func (h *Handlers) sendBKVAck(ctx context.Context, f *Frame, payload *BKVPayload, label string, build func(*BKVPayload) ([]byte, error)) {
	if h == nil || payload == nil {
		return
	}

	data, err := build(payload)
	if err != nil || len(data) == 0 {
		return
	}

	h.deliverBKVAck(ctx, f, payload, data, label)
}

// (h *Handlers) sendStatusAck 构造并下发0x1017状态上报ACK
func (h *Handlers) sendStatusAck(ctx context.Context, f *Frame, payload *BKVPayload, success bool) {
	h.sendBKVAck(ctx, f, payload, "status", func(p *BKVPayload) ([]byte, error) {
		return EncodeBKVStatusAck(p, success)
	})
}

// (h *Handlers) sendChargingEndAck 发送充电结束ACK
func (h *Handlers) sendChargingEndAck(ctx context.Context, f *Frame, payload *BKVPayload, socketNo, portNo int, success bool) {
	if h == nil {
		return
	}

	// 控制通道(0x0015)的充电结束上行没有 BKV payload，需直接回复 0x0f ACK
	if payload == nil {
		if h.Outbound == nil || f == nil {
			return
		}
		h.send0x0fControlAck(extractDeviceIDOrDefault(f), f.MsgID, success)
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

	h.sendBKVAck(ctx, f, payload, "charging-end", func(p *BKVPayload) ([]byte, error) {
		return EncodeBKVChargingEndAck(p, socketPtr, portPtr, success)
	})
}

// (h *Handlers) sendExceptionAck 发送异常ACK
func (h *Handlers) sendExceptionAck(ctx context.Context, f *Frame, payload *BKVPayload, socketNo int, success bool) {
	var socketPtr *int
	if socketNo >= 0 {
		s := socketNo
		socketPtr = &s
	}

	h.sendBKVAck(ctx, f, payload, "exception", func(p *BKVPayload) ([]byte, error) {
		return EncodeBKVExceptionAck(p, socketPtr, success)
	})
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
	if len(data) > total {
		// 某些设备的长度字段不包含子命令本身，此时多出1字节
		// 也有固件直接把剩余所有字节都视为控制payload
		inner = data[2:]
	}
	if len(inner) == 0 {
		return 0, nil, fmt.Errorf("control inner payload empty")
	}
	return inner[0], inner, nil
}

// ackControlFailure 下行/上行控制失败时下发 ACK=失败（用于 0x0015）
// 已废弃：请使用 ackControlFailureWithSubCmd 指定正确的子命令码
func (h *Handlers) ackControlFailure(deviceID string, msgID uint32) {
	h.ackControlFailureWithSubCmd(deviceID, msgID, 0x07)
}

// ackControlFailureWithSubCmd 下行/上行控制失败时下发 ACK=失败（用于 0x0015）
// 参数 subCmd 为触发失败的实际子命令码；协议期望平台回复子命令0x0F的ACK
func (h *Handlers) ackControlFailureWithSubCmd(deviceID string, msgID uint32, subCmd byte) {
	if h.Outbound == nil {
		return
	}
	// ACK格式：长度(2B) + 子命令(1B=0x0F) + 结果(1B)
	// 长度字段值为2（子命令+结果），结果0x00表示失败
	const ackSubCmd byte = 0x0F
	data := []byte{0x00, 0x02, ackSubCmd, 0x00}
	_ = h.Outbound.SendDownlink(deviceID, 0x0015, msgID, data)
}

// ackControlSuccessWithSubCmd 上行控制类命令处理成功后回复 ACK=成功（用于 0x0015）
func (h *Handlers) ackControlSuccessWithSubCmd(deviceID string, msgID uint32, subCmd byte) {
	if h.Outbound == nil {
		return
	}
	// 长度字段值为2（子命令+结果），子命令按协议固定0x0F，结果0x01表示成功
	const ackSubCmd byte = 0x0F
	data := []byte{0x00, 0x02, ackSubCmd, 0x01}
	_ = h.Outbound.SendDownlink(deviceID, 0x0015, msgID, data)
}

// send0x0fControlAck 发送控制通道 0x0015 的统一 0x0f ACK
// success: true=0x01 成功, false=0x00 失败
func (h *Handlers) send0x0fControlAck(deviceID string, msgID uint32, success bool) {
	if h.Outbound == nil {
		return
	}
	ack := byte(0x00)
	if success {
		ack = 0x01
	}
	// 长度(2B) + 子命令(1B) + ACK(1B)
	data := []byte{0x00, 0x02, 0x0f, ack}
	h.sendDownlinkReply(deviceID, 0x0015, msgID, data)
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
	// 协议规范充电模式校验：
	// 0x00 = 按量充电 (ChargingModeByEnergy)
	// 0x01 = 按时充电 (ChargingModeByTime)
	// 0x03 = 按功率充电 (ChargingModeByLevel)
	// 0x04 = 充满模式/服务费模式 (ChargingModeByFull)
	switch cmd.Mode {
	case ChargingModeByTime, ChargingModeByEnergy:
		// 按时/按量时，Duration 1-900 分钟
		if cmd.Duration == 0 || cmd.Duration > 900 {
			return fmt.Errorf("invalid duration_min: %d (must be 1-900)", cmd.Duration)
		}
		// 按量模式额外要求 Energy >0
		if cmd.Mode == ChargingModeByEnergy && cmd.Energy == 0 {
			return fmt.Errorf("invalid energy_wh: %d (must be > 0 for energy mode)", cmd.Energy)
		}
	case ChargingModeByLevel:
		// 按功率模式检查档位（最多5档）
		if cmd.LevelCount == 0 || cmd.LevelCount > 5 {
			return fmt.Errorf("invalid level_count: %d (must be 1-5)", cmd.LevelCount)
		}
		if len(cmd.PowerLevels) < int(cmd.LevelCount) {
			return fmt.Errorf("power level entries mismatch: have %d, need %d", len(cmd.PowerLevels), cmd.LevelCount)
		}
	case ChargingModeByFull:
		// 充满模式/服务费模式 (协议文档 按电费+服务费章节)
		// 此模式由服务费档位信息控制，无需传统的时长/电量约束
	default:
		return fmt.Errorf("invalid mode: %d (must be 0=energy, 1=time, 3=power, 4=full)", cmd.Mode)
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
	rawStatus := coremodel.RawPortStatus(status)
	statusLabel := "idle" // 充电结束=空闲
	if rawStatus.IsCharging() {
		statusLabel = "charging"
	}
	if rawStatus.HasFault() {
		statusLabel = "abnormal"
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

// emitChargingEndEvents 统一的充电结束事件/指标/推送逻辑
func (h *Handlers) emitChargingEndEvents(
	ctx context.Context,
	deviceID string,
	end *BKVChargingEnd,
	orderNo string,
	lookupSource string,
	nextStatusOverride *int32,
	collectMetrics bool,
) {
	if h == nil || end == nil || deviceID == "" {
		return
	}

	if collectMetrics {
		h.collectChargingMetrics(
			deviceID,
			int(end.Port),
			end.Status,
			int(end.InstantPower),
			int(end.InstantCurrent),
			int(end.EnergyUsed),
		)
	}

	rawReason := int32(end.EndReason)
	if h.Reason != nil {
		if mapped, ok := h.Reason.Translate(int(end.EndReason)); ok {
			rawReason = int32(mapped)
		}
	}

	nextStatus := nextStatusOverride
	if nextStatus == nil {
		ns := int32(end.Status)
		nextStatus = &ns
	}

	bizNo := fmt.Sprintf("%04X", end.BusinessNo)
	finalOrder := orderNo
	if finalOrder == "" {
		finalOrder = bizNo
	}
	var powerW *int32
	if end.InstantPower > 0 {
		p := int32(math.Round(float64(end.InstantPower) / 10.0))
		powerW = &p
	}

	evEnd := h.buildSessionEndedEvent(
		deviceID,
		int(end.Port),
		bizNo,
		int32(end.EnergyUsed),
		int32(end.ChargingTime)*60,
		&rawReason,
		nextStatus,
		nil,
		powerW,
	)
	h.emitter().Emit(ctx, evEnd)

	totalKwh := float64(end.EnergyUsed) / 100.0
	durationMin := int(end.ChargingTime)
	endReasonMsg := h.getEndReasonDescription(int(end.EndReason))
	h.pushChargingEndedEvent(ctx, deviceID, finalOrder, bizNo, int(end.Port), durationMin, totalKwh, fmt.Sprintf("%d", end.EndReason), endReasonMsg, lookupSource, nil)
	zap.L().Debug("charging end order mapping",
		zap.String("device_id", deviceID),
		zap.Uint8("port", uint8(end.Port)),
		zap.String("order_no", finalOrder),
		zap.String("business_no", bizNo),
		zap.String("lookup_source", lookupSource))
}

// (h *Handlers) handleControlChargingEnd 处理控制帧中的充电结束上报
// 规范：端口收敛仅依赖 SessionEnded.NextPortStatus，不在充电结束路径发送 PortSnapshot
func (h *Handlers) handleControlChargingEnd(ctx context.Context, f *Frame, deviceID string, end *BKVChargingEnd, session *ordersession.ActiveSession, lookupSource string) {
	if err := h.validateChargingEnd(end); err != nil {
		// 解析到结束帧但字段非法，回复失败 ACK 并返回错误
		h.sendChargingEndAck(ctx, f, nil, int(end.SocketNo), int(end.Port), false)
		return
	}

	// 【方案二：状态位验证】
	// 检查 status bit5（充电状态位），记录状态信息用于监控和调试
	rawStatus := coremodel.RawPortStatus(end.Status)
	isCharging := rawStatus.IsCharging() // bit5=1

	if !isCharging {
		// status bit5=0 表示未充电，可能是：
		// 1. 充电已经结束的确认（正常）
		// 2. 设备异常状态（需关注）
		// 由于已通过业务号验证，这里记录详细信息供后续分析
		zap.L().Info("charging end with non-charging status",
			zap.String("device_id", deviceID),
			zap.Uint8("port", uint8(end.Port)),
			zap.Uint8("status", end.Status),
			zap.String("status_bits", fmt.Sprintf("0b%08b", end.Status)),
			zap.Bool("is_online", rawStatus.IsOnline()),
			zap.Bool("is_charging", isCharging),
			zap.Bool("is_no_load", rawStatus.IsNoLoad()),
			zap.Bool("has_fault", rawStatus.HasFault()),
			zap.String("note", "Status indicates device not charging, but processing as charging end due to valid business_no match"))
	}

	nextStatus := int32(coremodel.RawStatusOnlineNoLoad)
	orderNo := ""
	if session != nil {
		orderNo = session.OrderNo
	}
	if orderNo == "" {
		lookupSource = "business_no"
	}
	h.emitChargingEndEvents(ctx, deviceID, end, orderNo, lookupSource, &nextStatus, false)

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
	session, lookupSource := h.resolveOrderSession(deviceID, int(end.Port), bizNo)
	orderNo := bizNo
	if session != nil && session.OrderNo != "" {
		orderNo = session.OrderNo
	} else if lookupSource == "" {
		lookupSource = "business_no"
	}

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
		h.pushChargingProgressEvent(ctx, deviceID, int(end.Port), orderNo, bizNo, lookupSource, powerWVal, currentA, 0, energyKwh, durationS, nil)
	}
}

// (h *Handlers) handleControlUplinkStatus 处理控制上行ACK
// 协议规范（文档2.3.1节）：
//
//	07   命令（控制ACK）
//	01   结果：1=成功，0=失败  ← switchFlag 是成功/失败标志，不是状态！
//	02   插座号
//	00   插孔号
//	0068 业务号
//
// 重要：switchFlag=0x01 表示命令执行成功，switchFlag=0x00 表示失败
// 不能将 switchFlag 当作端口状态位图处理！
func (h *Handlers) handleControlUplinkStatus(ctx context.Context, msgID uint32, deviceID string, socketNo, portNo int, switchFlag byte, businessNo uint16) {
	if deviceID == "" {
		return
	}

	if portNo < 0 || portNo > 1 {
		zap.L().Warn("control uplink ack ignored: invalid port range",
			zap.String("device_id", deviceID),
			zap.Int("socket_no", socketNo),
			zap.Int("port_no", portNo),
			zap.Uint8("switch_flag", switchFlag),
			zap.Uint16("business_no", businessNo))
		return
	}

	// switchFlag: 1=命令成功, 0=命令失败
	// 注意：这不是端口状态位图！
	if switchFlag == 0x00 {
		h.send0x0fControlAck(deviceID, msgID, false)
		// 命令执行失败，记录警告日志，不更新状态，不记录会话
		zap.L().Warn("control command failed (ACK result=0)",
			zap.String("device_id", deviceID),
			zap.Int("socket_no", socketNo),
			zap.Int("port_no", portNo),
			zap.Uint16("business_no", businessNo))
		return
	}

	// 命令成功（switchFlag=0x01）
	// 充电命令成功 → 设置端口状态为"充电中"
	rawStatus := int32(coremodel.RawStatusOnlineCharging) // 0xA0 = 在线充电中

	zap.L().Info("control uplink ack success, setting port to charging",
		zap.String("device_id", deviceID),
		zap.Int("socket_no", socketNo),
		zap.Int("port_no", portNo),
		zap.Uint16("business_no", businessNo),
		zap.Uint8("switch_flag", switchFlag),
		zap.Int32("raw_status", rawStatus))

	if event := NewEventBuilder(deviceID).WithPort(portNo).BuildPortSnapshot(rawStatus, nil); event != nil {
		h.emitter().Emit(ctx, event)
	}

	if tracker := h.tracker(); tracker != nil {
		bizHex := fmt.Sprintf("%04X", businessNo)
		if session, err := tracker.Promote(deviceID, portNo, bizHex); err != nil {
			zap.L().Warn("session promote failed", zap.String("device_id", deviceID), zap.Int("port_no", portNo), zap.String("business_no", bizHex), zap.Error(err))
		} else {
			zap.L().Info("session tracked from control uplink (command success)",
				zap.String("device_id", deviceID),
				zap.Int("port_no", portNo),
				zap.String("business_no", bizHex),
				zap.String("order_no", session.OrderNo))
		}
	}

	h.send0x0fControlAck(deviceID, msgID, true)
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

// normalizeRawStatusByte 直接返回协议层的原始状态字节
// 协议规范：状态是位图格式(bit7=在线, bit5=充电中, bit4=空载等)
// 注意：不在此处做值映射，原始状态应由 coremodel.NormalizePortStatus 处理
func normalizeRawStatusByte(statusByte byte) int32 {
	return int32(statusByte)
}

// mapSocketStatusToRaw 映射API状态码到原始状态位图
// API状态码定义（coremodel.StatusCode*）：
//
//	0 = StatusCodeOffline  离线
//	1 = StatusCodeIdle     空闲
//	2 = StatusCodeCharging 充电中
//	3 = StatusCodeFault    故障
//
// 原始状态位图（coremodel.RawStatus*）：
//
//	0x00 = RawStatusOffline        离线
//	0x90 = RawStatusOnlineNoLoad   在线空载 (bit7+bit4)
//	0xA0 = RawStatusOnlineCharging 在线充电中 (bit7+bit5)
//	0xC0 = 在线+故障 (bit7+bit6)
func mapSocketStatusToRaw(status int) int32 {
	switch status {
	case int(coremodel.StatusCodeOffline): // 0
		return int32(coremodel.RawStatusOffline) // 0x00
	case int(coremodel.StatusCodeIdle): // 1
		return int32(coremodel.RawStatusOnlineNoLoad) // 0x90
	case int(coremodel.StatusCodeCharging): // 2
		return int32(coremodel.RawStatusOnlineCharging) // 0xA0
	case int(coremodel.StatusCodeFault): // 3
		return int32(0x80 | 0x40) // 0xC0 在线+故障
	default:
		return int32(coremodel.RawStatusOffline)
	}
}

// handleNetworkAckWithExpectation 统一处理刷新/增删节点的ACK
func (h *Handlers) handleNetworkAckWithExpectation(
	ctx context.Context,
	f *Frame,
	action string,
	expectedSubCmd byte,
	okMsg string,
	rejectMsg string,
) error {
	deviceID, devErr := h.resolveDeviceID(f, action)
	if devErr != nil {
		return devErr
	}
	result := "ok"
	msg := okMsg
	metadata := map[string]string{
		"cmd":     fmt.Sprintf("0x%04X", f.Cmd),
		"sub_cmd": fmt.Sprintf("0x%02X", expectedSubCmd),
	}

	ack, err := ParseNetworkAck(f.Data)
	if err != nil {
		result = "failed"
		msg = err.Error()
		metadata["raw_payload"] = fmt.Sprintf("%x", f.Data)
	} else {
		if ack.SubCmd != expectedSubCmd {
			result = "failed"
			msg = fmt.Sprintf("unexpected sub_cmd: 0x%02X", ack.SubCmd)
		} else if ack.Result != 0x01 {
			result = "failed"
			if rejectMsg != "" {
				msg = rejectMsg
			} else {
				msg = fmt.Sprintf("%s failed", action)
			}
		}
	}

	event := NewEventBuilder(deviceID).BuildNetworkTopology(action, result, msg, metadata)
	h.emitter().Emit(ctx, event)
	return err
}

// handleSimpleParamResponse 统一处理解析结果为 result/message 的参数应答
func (h *Handlers) handleSimpleParamResponse(
	ctx context.Context,
	f *Frame,
	successMsg string,
	failureFallback string,
	parse func([]byte) (uint8, string, error),
) error {
	deviceID := extractDeviceIDOrDefault(f)
	result := "ok"
	msg := successMsg
	metadata := map[string]string{"cmd": fmt.Sprintf("0x%04X", f.Cmd)}

	code, respMsg, err := parse(f.Data)
	if err != nil {
		result = "failed"
		msg = err.Error()
		metadata["raw_payload"] = fmt.Sprintf("%x", f.Data)
	} else {
		metadata["raw_result"] = fmt.Sprintf("%d", code)
		if code != 0 {
			result = "failed"
			if respMsg != "" {
				msg = respMsg
			} else if failureFallback != "" {
				msg = failureFallback
			}
		} else if respMsg != "" {
			msg = respMsg
		}
	}

	event := NewEventBuilder(deviceID).BuildParamResult(result, msg, metadata)
	h.emitter().Emit(ctx, event)

	return err
}

// (h *Handlers) getEndReasonDescription 获取结束原因描述
// 优先使用 ReasonMap 配置，回退到默认描述
func (h *Handlers) getEndReasonDescription(reason int) string {
	if h.Reason != nil {
		return h.Reason.GetReasonDescription(reason)
	}
	// 回退到默认描述（与 ReasonMap.GetReasonDescription 保持一致）
	return DefaultReasonMap().GetReasonDescription(reason)
}
