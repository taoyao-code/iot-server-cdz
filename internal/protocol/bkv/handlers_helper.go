package bkv

import (
	"context"
	"fmt"

	"github.com/taoyao-code/iot-server/internal/storage/models"
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

// (h *Handlers) upsertGatewaySockets 批量upsert网关插座
func (h *Handlers) upsertGatewaySockets(ctx context.Context, devicePhyID string, sockets []SocketInfo, logger *zap.Logger) (upserted, errors int) {
	if h.Core == nil {
		return 0, 0
	}

	t := now()
	for _, s := range sockets {
		socket := &models.GatewaySocket{
			GatewayID:  devicePhyID,
			SocketNo:   int32(s.SocketNo),
			SocketMAC:  s.SocketMAC,
			LastSeenAt: &t,
		}
		if s.SocketUID != "" {
			uid := s.SocketUID
			socket.SocketUID = &uid
		}
		if s.Channel > 0 {
			ch := int32(s.Channel)
			socket.Channel = &ch
		}
		status := int32(s.Status)
		socket.Status = &status
		rssi := int32(s.SignalStrength)
		socket.SignalStrength = &rssi

		if e := h.Core.UpsertGatewaySocket(ctx, socket); e != nil {
			errors++
			continue
		}
		upserted++
	}

	return upserted, errors
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
func (h *Handlers) handleControlChargingEnd(ctx context.Context, f *Frame, deviceID string, end *BKVChargingEnd) {
	// 1. 发送 PortSnapshot 事件
	rawStatus := int32(end.Status)
	var powerW *int32
	if end.InstantPower > 0 {
		p := int32(end.InstantPower) / 10 // 0.1W -> W
		powerW = &p
	}

	evPS := NewEventBuilder(deviceID).
		WithPort(int(end.Port)).
		WithSocketNo(int(end.SocketNo)).
		BuildPortSnapshot(rawStatus, powerW)
	h.emitter().Emit(ctx, evPS)

	// 2. 发送 SessionEnded 事件
	nextStatus := int32(0x09) // 空闲
	rawReason := int32(end.EndReason)
	bizNo := fmt.Sprintf("%04X", end.BusinessNo)

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

	// 3. 回复 ACK
	h.sendChargingEndAck(ctx, f, nil, int(end.SocketNo), int(end.Port), true)
}

// (h *Handlers) handleControlUplinkStatus 处理控制上行状态更新
func (h *Handlers) handleControlUplinkStatus(ctx context.Context, deviceID string, socketNo, portNo int, switchFlag byte, businessNo uint16) {
	status := int32(0x09) // 默认空闲
	if switchFlag == 0x01 {
		status = 0x81 // 充电中
	}

	bizNo := fmt.Sprintf("%04X", businessNo)
	ev := NewEventBuilder(deviceID).
		WithPort(portNo).
		WithSocketNo(socketNo).
		WithBusinessNo(bizNo).
		BuildPortSnapshot(status, nil)

	h.emitter().Emit(ctx, ev)
}

// (h *Handlers) handleControlDownlinkCommand 处理控制下行命令
func (h *Handlers) handleControlDownlinkCommand(ctx context.Context, deviceID string, cmd *BKVControlCommand) {
	status := int32(0x09)
	if cmd.Switch == SwitchOn {
		status = 0x81
	}

	ev := NewEventBuilder(deviceID).
		WithPort(int(cmd.Port)).
		BuildPortSnapshot(status, nil)

	h.emitter().Emit(ctx, ev)
}

// (h *Handlers) sendDownlinkReply 发送下行回复（通用）
func (h *Handlers) sendDownlinkReply(deviceID string, cmd uint16, msgID uint32, data []byte) {
	if h.Outbound == nil || deviceID == "" || len(data) == 0 {
		return
	}
	_ = h.Outbound.SendDownlink(deviceID, cmd, msgID, data)
}

// (h *Handlers) mapSocketStatusToRaw 映射插座状态枚举到原始状态位图
func mapSocketStatusToRaw(status int) int32 {
	switch status {
	case 0:
		return 0x09 // idle → 在线+空载
	case 1:
		return 0x81 // charging → 在线+充电
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

// (h *Handlers) buildParamResultMetadata 构建参数结果元数据
func buildParamResultMetadata(cmd uint16, payload []byte) map[string]string {
	return map[string]string{
		"cmd":         fmt.Sprintf("0x%04X", cmd),
		"raw_payload": fmt.Sprintf("%x", payload),
	}
}
