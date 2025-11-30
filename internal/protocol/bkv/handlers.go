package bkv

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/taoyao-code/iot-server/internal/coremodel"
	"github.com/taoyao-code/iot-server/internal/driverapi"
	"github.com/taoyao-code/iot-server/internal/ordersession"
	"github.com/taoyao-code/iot-server/internal/storage"
	"github.com/taoyao-code/iot-server/internal/thirdparty"
	"go.uber.org/zap"
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

	OrderTracker *ordersession.Tracker
}

// HandleHeartbeat 处理心跳帧 (仅帧命令 cmd=0x0000)
// 注意：BKV子命令0x1017是"插座状态上报"，不是心跳！
func (h *Handlers) HandleHeartbeat(ctx context.Context, f *Frame) error {
	// 1. 提取设备ID
	deviceID := extractDeviceIDOrDefault(f)

	// 2. 心跳数据严格校验：长度=方向(1)+网关ID(7)+ICCID(20)+软版本(8可选)+信号(1) 等
	meta := map[string]string{}
	if len(f.Data) < 29 {
		return fmt.Errorf("heartbeat payload too short: %d", len(f.Data))
	}
	// 方向1B，网关ID 7B 后，取 ICCID 20B，最后1B 信号强度
	rssi := int8(f.Data[len(f.Data)-1])
	meta["rssi"] = fmt.Sprintf("%d", rssi)
	iccidRaw := strings.TrimSpace(string(f.Data[8 : 8+20])) // 文档示例 ICCID 20 字节
	if iccidRaw != "" {
		meta["iccid"] = iccidRaw
	}

	// 3. 发送心跳事件到核心
	event := NewEventBuilder(deviceID).BuildHeartbeat()
	if hb := event.DeviceHeartbeat; hb != nil {
		if v, ok := meta["rssi"]; ok {
			if n, err := strconv.Atoi(v); err == nil {
				rssi := int32(n)
				hb.RSSIDBm = &rssi
			}
		}
	}
	h.emitter().Emit(ctx, event)

	// 3.1 更新设备 last_seen_at（使用当前时间）
	if h.Core != nil {
		_ = h.Core.TouchDeviceLastSeen(ctx, deviceID, time.Now())
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

	// 1.1 长度校验：BKV最小头部 5+11+10=26 字节
	if err := h.validateLength(f.Data, 26, "bkv payload"); err != nil {
		return err
	}

	// 2. 根据载荷类型分发处理
	// 协议规范：0x1017是状态上报，必须同时包含状态字段(tag 0x65 + value 0x94)
	// 同时检查命令码和字段存在性，确保只处理真正的状态上报
	if payload.IsStatusReport() && payload.HasSocketStatusFields() {
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

	if payload.IsParameterSet() {
		return h.handleParameterSet(ctx, f, payload)
	}

	if payload.IsParameterSetAck() {
		return h.handleParameterSetAck(ctx, payload)
	}

	if payload.IsParameterQuery() {
		return h.handleParameterQuery(ctx, payload)
	}

	if payload.IsControlCommand() {
		return h.handleBKVControlCommand(ctx, payload)
	}

	// 记录未知BKV子命令
	zap.L().Debug("unknown BKV sub_cmd",
		zap.Uint16("bkv_cmd", payload.Cmd),
		zap.Int("fields_count", len(payload.Fields)))
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

		rawStatus := normalizeRawStatusByte(port.Status)
		var power *int32
		if port.Power > 0 {
			roundedW := int32(math.Round(float64(port.Power) / 10.0)) // 0.1W → W(四舍五入)
			power = &roundedW
		}

		event := NewEventBuilder(deviceID).
			WithPort(int(port.PortNo)).
			BuildPortSnapshot(rawStatus, power)
		h.emitter().Emit(ctx, event)

		// 如果正在充电，推送充电进度事件到Webhook
		statusBits := coremodel.RawPortStatus(uint8(rawStatus))
		isCharging := statusBits.IsCharging()
		if isCharging && h.EventQueue != nil {
			powerW := float64(port.Power) / 10.0       // 0.1W -> W
			currentA := float64(port.Current) / 1000.0 // 0.001A -> A
			voltageV := float64(port.Voltage) / 10.0   // 0.1V -> V
			energyKwh := float64(port.Energy) / 100.0  // 0.01kWh -> kWh
			durationS := int(port.ChargingTime) * 60   // min -> sec
			businessNo := fmt.Sprintf("%04X", port.BusinessNo)
			session, lookupSource := h.resolveOrderSession(deviceID, int(port.PortNo), businessNo)
			orderNo := businessNo
			if session != nil && session.OrderNo != "" {
				orderNo = session.OrderNo
			} else if lookupSource == "" {
				lookupSource = "business_no"
			}
			h.pushChargingProgressEvent(ctx, deviceID, int(port.PortNo), orderNo, businessNo, lookupSource, powerW, currentA, voltageV, energyKwh, durationS, nil)
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

	nextStatus := int32(coremodel.RawStatusOnlineNoLoad) // 充电结束后设为在线空载
	rawReason := int32(reason)

	event := h.buildSessionEndedEvent(
		deviceID,
		actualPort,
		orderHex,
		int32(kwh01),
		int32(durationSec),
		&rawReason,
		&nextStatus,
		nil,
		nil,
	)

	if err := h.emitter().EmitWithCheck(ctx, event); err != nil {
		return fmt.Errorf("core event session ended failed: %w", err)
	}

	// 推送充电结束事件到Webhook
	totalKwh := float64(kwh01) / 100.0 // 0.01kWh -> kWh
	durationMin := durationSec / 60
	endReasonMsg := h.getEndReasonDescription(reason)
	h.pushChargingEndedEvent(ctx, deviceID, orderHex, orderHex, actualPort, durationMin, totalKwh, fmt.Sprintf("%d", reason), endReasonMsg, "payload", nil)

	success = true
	return nil
}

// HandleControl 处理控制指令 (cmd=0x0015)
// 协议规范：0x0015帧包含多种子命令，需要根据子命令分发到对应处理器
// 子命令列表：
//   - 0x02: 充电结束上报（按时/按量）- 2.2.9
//   - 0x07: 控制设备（按时/按量）- 2.2.8
//   - 0x0B: 刷卡上报 - 2.2.3进阶
//   - 0x0C: 刷卡充电结束 - 2.2.3进阶
//   - 0x0F: 订单确认 - 2.2.3进阶
//   - 0x17: 按功率下发充电命令 - 2.2.1进阶
//   - 0x18: 按功率充电结束上报 - 2.2.2进阶
//   - 0x1A: 刷卡查询余额 - 2.2.4进阶
//   - 0x1B: 设置语音播报时间 - 2.2.5进阶
//   - 0x1D: 查询插座状态 - 2.2.4
func (h *Handlers) HandleControl(ctx context.Context, f *Frame) error {
	deviceID := extractDeviceIDOrDefault(f)

	// 0x0015 data 长度前缀 + 子命令，最小长度 3（len_hi len_lo sub_cmd）
	subCmd, inner, err := h.parseControlPayload(f.Data)
	if err != nil {
		h.ackControlFailureWithSubCmd(deviceID, f.MsgID, subCmd)
		return err
	}

	// inner[0] 是子命令，inner[1:] 是子命令后的载荷数据
	payload := inner[1:]

	if f.IsUplink() {
		// === 上行命令分发 ===
		switch subCmd {
		case 0x02, 0x18:
			// 充电结束上报（0x02=按时/按量，0x18=按功率）
			return h.handleControlChargingEndUplink(ctx, f, deviceID, subCmd)

		case 0x07, 0x17:
			// 控制命令ACK上行（0x07=按时/按量，0x17=按功率）
			if len(payload) < 5 {
				h.ackControlFailureWithSubCmd(deviceID, f.MsgID, subCmd)
				return fmt.Errorf("control uplink sub_cmd 0x%02x too short: %d", subCmd, len(payload))
			}
			switchFlag := payload[0]
			socketNo := int(payload[1])
			portNo := int(payload[2])
			businessNo := binary.BigEndian.Uint16(payload[3:5])
			h.handleControlUplinkStatus(ctx, deviceID, socketNo, portNo, switchFlag, businessNo)
			return nil

		case 0x0B:
			// 刷卡上报
			return h.handleCardSwipeUplink(ctx, f)

		case 0x0C:
			// 刷卡充电结束
			return h.handleChargeEndUplink(ctx, f)

		case 0x0F:
			// 订单确认
			return h.handleOrderConfirmUplink(ctx, f)

		case 0x1A:
			// 余额查询
			return h.handleBalanceQueryUplink(ctx, f)

		case 0x1B:
			// 语音配置响应
			return h.HandleVoiceConfigResponse(ctx, f)

		case 0x1D:
			// 查询插座状态响应
			return h.HandleSocketStateResponse(ctx, f)

		default:
			zap.L().Debug("unknown control uplink sub_cmd",
				zap.String("device_id", deviceID),
				zap.Uint8("sub_cmd", subCmd),
				zap.Int("payload_len", len(payload)))
			return nil
		}
	} else {
		// === 下行命令分发 ===
		switch subCmd {
		case 0x07, 0x17:
			// 控制命令下行（0x07=按时/按量，0x17=按功率分档）
			// payload是去掉子命令字节后的数据，ParseBKVControlCommand期望从socketNo开始
			// 0x07最小长度6字节: [socket][port][switch][mode][duration:2]
			// 0x17最小长度6字节: [socket][port][switch][amount:2][levelCount]
			minLen := 6
			if len(payload) < minLen {
				h.ackControlFailureWithSubCmd(deviceID, f.MsgID, subCmd)
				return fmt.Errorf("control downlink sub_cmd 0x%02X payload too short: %d (need %d)", subCmd, len(payload), minLen)
			}
			cmd, err := ParseBKVControlCommand(subCmd, payload)
			if err != nil {
				h.ackControlFailureWithSubCmd(deviceID, f.MsgID, subCmd)
				return fmt.Errorf("parse control downlink failed: %w", err)
			}
			if err := h.validateControlStart(cmd); err != nil {
				h.ackControlFailureWithSubCmd(deviceID, f.MsgID, subCmd)
				return err
			}
			h.handleControlDownlinkCommand(ctx, deviceID, cmd)
			return nil

		default:
			zap.L().Debug("unknown control downlink sub_cmd",
				zap.String("device_id", deviceID),
				zap.Uint8("sub_cmd", subCmd),
				zap.Int("payload_len", len(payload)))
			return nil
		}
	}
}

// handleControlChargingEndUplink 处理充电结束上报上行（子命令0x02或0x18）
func (h *Handlers) handleControlChargingEndUplink(ctx context.Context, f *Frame, deviceID string, subCmd byte) error {
	end, err := ParseBKVChargingEnd(f.Data)
	if err != nil {
		return fmt.Errorf("parse charging end failed: %w", err)
	}

	receivedBizNo := fmt.Sprintf("%04X", end.BusinessNo)
	session, lookupSource := h.resolveOrderSession(deviceID, int(end.Port), receivedBizNo)
	if session == nil {
		lookupSource = "business_no"
		zap.L().Warn("charging end without active session, fallback to business_no",
			zap.String("device_id", deviceID),
			zap.Uint8("port", uint8(end.Port)),
			zap.String("business_no", receivedBizNo),
			zap.Uint8("status", end.Status),
			zap.Uint8("sub_cmd", subCmd))
	} else if session.BusinessNo != receivedBizNo {
		zap.L().Warn("charging end ignored: business number mismatch",
			zap.String("device_id", deviceID),
			zap.Uint8("port", uint8(end.Port)),
			zap.String("received_business_no", receivedBizNo),
			zap.String("mapped_business_no", session.BusinessNo),
			zap.String("order_no", session.OrderNo),
			zap.Uint8("status", end.Status))
		return nil
	}

	// 触发充电结束流程（无 session 时使用 fallback order_no=biz）
	h.handleControlChargingEnd(ctx, f, deviceID, end, session, lookupSource)

	if session != nil {
		if tracker := h.tracker(); tracker != nil {
			tracker.Clear(deviceID, int(end.Port))
		}
		zap.L().Info("charging end processed and session cleared",
			zap.String("device_id", deviceID),
			zap.Uint8("port", uint8(end.Port)),
			zap.String("business_no", receivedBizNo),
			zap.String("order_no", session.OrderNo),
			zap.Uint8("sub_cmd", subCmd),
			zap.String("lookup_source", lookupSource))
	} else {
		zap.L().Info("charging end processed without mapped session",
			zap.String("device_id", deviceID),
			zap.Uint8("port", uint8(end.Port)),
			zap.String("business_no", receivedBizNo),
			zap.Uint8("sub_cmd", subCmd),
			zap.String("lookup_source", lookupSource))
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

	end, err := ParseBKVChargingEnd(f.Data)
	if err != nil {
		return fmt.Errorf("parse charging end: %w", err)
	}

	nextStatus := int32(coremodel.RawStatusOnlineNoLoad)
	orderNo := ""
	lookupSource := ""
	if tracker := h.tracker(); tracker != nil {
		biz := fmt.Sprintf("%04X", end.BusinessNo)
		if session, source := h.resolveOrderSession(deviceID, int(end.Port), biz); session != nil {
			orderNo = session.OrderNo
			lookupSource = source
			tracker.Clear(deviceID, int(end.Port))
		}
	}
	h.emitChargingEndEvents(ctx, deviceID, end, orderNo, lookupSource, &nextStatus, true)

	return nil
}

// HandleGeneric 通用处理器，记录所有其他指令
func (h *Handlers) HandleGeneric(ctx context.Context, f *Frame) error {
	deviceID, err := h.resolveDeviceID(f, "generic_cmd")
	if err != nil {
		return err
	}

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

	ack, err := ParseNetworkAck(f.Data)
	action := "network_ack"
	result := "failed"
	msg := fmt.Sprintf("network ack invalid payload len=%d payload=%x", len(f.Data), f.Data)
	metadata := map[string]string{"cmd": fmt.Sprintf("0x%04X", f.Cmd)}

	if err == nil {
		metadata["sub_cmd"] = fmt.Sprintf("0x%02X", ack.SubCmd)
		metadata["raw_result"] = fmt.Sprintf("%d", ack.Result)
		switch ack.SubCmd {
		case 0x08:
			action = "refresh_ack"
			// 若设备返回列表（count + 14*N），解析数量以便上层检查映射
			if ack.Result == 0x01 && len(f.Data) > 4 {
				if entries, e := ParseNetworkRefreshList(f.Data[2:]); e == nil {
					metadata["list_count"] = fmt.Sprintf("%d", len(entries))
					upserted, failed := h.upsertGatewaySocketsFromEntries(ctx, deviceID, entries)
					if upserted > 0 {
						metadata["mapping_upserted"] = fmt.Sprintf("%d", upserted)
					}
					if failed > 0 {
						metadata["mapping_failed"] = fmt.Sprintf("%d", failed)
					}
					if len(entries) == 0 {
						result = "failed"
						msg = "refresh ack without list entries"
					}
				}
			}
		case 0x09:
			action = "add_ack"
		case 0x0A:
			action = "delete_ack"
		default:
			action = "network_ack"
		}
		msg = fmt.Sprintf("%s result=%d", action, ack.Result)
		if ack.Result == 0x01 {
			result = "ok"
		}
	} else {
		metadata["raw_payload"] = fmt.Sprintf("%x", f.Data)
	}

	event := NewEventBuilder(deviceID).BuildNetworkTopology(action, result, msg, metadata)
	h.emitter().Emit(ctx, event)
	return err
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
	if len(payload.Fields) == 0 {
		h.sendExceptionAck(ctx, f, payload, -1, false)
		return fmt.Errorf("exception payload empty")
	}
	// 必要字段校验：插座号、状态、原因至少其一存在
	if event.SocketNo == 0 {
		h.sendExceptionAck(ctx, f, payload, -1, false)
		return fmt.Errorf("exception socket_no missing")
	}
	if event.SocketEventReason == 0 && event.SocketEventStatus == 0 && event.Port1EventReason == 0 && event.Port2EventReason == 0 {
		h.sendExceptionAck(ctx, f, payload, int(event.SocketNo), false)
		return fmt.Errorf("exception reason/status missing")
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
	reasonCode := event.SocketEventReason
	if h.Reason != nil {
		if mapped, ok := h.Reason.Translate(int(reasonCode)); ok {
			reasonCode = uint8(mapped)
		}
	}
	meta := map[string]string{"reason": fmt.Sprintf("%d", reasonCode)}

	ev := NewEventBuilder(deviceID).
		WithPort(int(event.SocketNo)).
		BuildException(
			fmt.Sprintf("socket_event_%d", reasonCode),
			fmt.Sprintf("status=%d", event.SocketEventStatus),
			"error",
			&rawStatus,
			meta,
		)

	h.emitter().Emit(ctx, ev)
	success = true
	return nil
}

// handleParameterSet 处理参数设置命令 (BKV子命令0x1011)
// 协议文档2.2.6：平台下发参数设置
func (h *Handlers) handleParameterSet(ctx context.Context, f *Frame, payload *BKVPayload) error {
	deviceID := extractDeviceIDFromPayload(f, payload)
	zap.L().Debug("parameter set command received",
		zap.String("device_id", deviceID),
		zap.Uint16("bkv_cmd", payload.Cmd),
		zap.Int("fields_count", len(payload.Fields)))
	// TODO: 实现参数设置命令处理
	return nil
}

// handleParameterSetAck 处理参数设置ACK (BKV子命令0x1013)
// 协议文档2.2.6：设备回复参数设置ACK
// 注意：0x1013是ACK响应，不包含状态字段！
func (h *Handlers) handleParameterSetAck(ctx context.Context, payload *BKVPayload) error {
	deviceID := extractDeviceIDFromPayload(nil, payload)

	// 从ACK中提取结果
	success := false
	for _, field := range payload.Fields {
		if field.Tag == 0x0F && len(field.Value) >= 1 {
			success = field.Value[0] == 0x01
			break
		}
	}

	zap.L().Debug("parameter set ACK received",
		zap.String("device_id", deviceID),
		zap.Bool("success", success))
	return nil
}

// handleParameterQuery 处理参数查询响应 (BKV子命令0x1012)
// 协议文档2.2.7：参数查询响应
func (h *Handlers) handleParameterQuery(ctx context.Context, payload *BKVPayload) error {
	deviceID := extractDeviceIDFromPayload(nil, payload)
	zap.L().Debug("parameter query response received",
		zap.String("device_id", deviceID),
		zap.Int("fields_count", len(payload.Fields)))
	// TODO: 实现参数查询响应处理
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

	deviceID, err := h.resolveDeviceID(f, "order_confirm")
	if err != nil {
		return err
	}

	// 发送会话开始事件
	metadata := map[string]string{
		"status": fmt.Sprintf("%d", conf.Status),
		"reason": conf.Reason,
	}

	event := h.buildSessionStartedEvent(deviceID, 0, conf.OrderNo, "order_confirm", nil, metadata)
	h.emitter().Emit(ctx, event)

	if tracker := h.tracker(); tracker != nil {
		tracker.TrackPending(deviceID, 0, 0, conf.OrderNo, "order_confirm")
		if _, err := tracker.Promote(deviceID, 0, conf.OrderNo); err != nil {
			zap.L().Warn("order confirm promote failed", zap.String("device_id", deviceID), zap.String("order_no", conf.OrderNo), zap.Error(err))
		} else {
			zap.L().Info("session started and tracked",
				zap.String("device_id", deviceID),
				zap.Int("port", 0),
				zap.String("business_no", conf.OrderNo),
				zap.Bool("from_order_confirm", true))
		}
	}

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

	deviceID, err := h.resolveDeviceID(f, "charge_end")
	if err != nil {
		return err
	}

	// 发送会话结束事件
	amount := int64(report.Amount)
	rawReason := int32(report.EndReason)

	event := h.buildSessionEndedEvent(
		deviceID,
		0,
		report.OrderNo,
		int32(report.Energy/10),
		int32(report.Duration*60),
		&rawReason,
		nil,
		&amount,
		nil,
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

// HandleNetworkRefresh 处理刷新列表 ACK（上行，cmd=0x0005 sub=0x08）
func (h *Handlers) HandleNetworkRefresh(ctx context.Context, f *Frame) error {
	return h.handleNetworkAckWithExpectation(
		ctx,
		f,
		"refresh",
		0x08,
		"network refresh",
		"device reject refresh",
	)
}

// HandleNetworkAddNode 处理添加插座 ACK（上行，cmd=0x0005 sub=0x09）
func (h *Handlers) HandleNetworkAddNode(ctx context.Context, f *Frame) error {
	return h.handleNetworkAckWithExpectation(
		ctx,
		f,
		"add_node",
		0x09,
		"add socket success",
		"add socket failed",
	)
}

// HandleNetworkDeleteNode 处理删除插座 ACK（上行，cmd=0x0005 sub=0x0A）
func (h *Handlers) HandleNetworkDeleteNode(ctx context.Context, f *Frame) error {
	return h.handleNetworkAckWithExpectation(
		ctx,
		f,
		"delete_node",
		0x0A,
		"delete socket success",
		"delete socket failed",
	)
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
	if err := h.validateLength(f.Data, 20, "power level end"); err != nil {
		h.sendDownlinkReply(extractDeviceIDOrDefault(f), f.Cmd, f.MsgID, EncodePowerLevelEndReply(0, 1)) // result=1 表示失败
		return err
	}
	report, err := ParsePowerLevelEndReport(f.Data)
	if err != nil {
		h.sendDownlinkReply(extractDeviceIDOrDefault(f), f.Cmd, f.MsgID, EncodePowerLevelEndReply(0, 1))
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
	return h.handleSimpleParamResponse(
		ctx,
		f,
		"param reset success",
		"param reset failed",
		func(data []byte) (uint8, string, error) {
			resp, err := ParseParamResetResponse(data)
			if err != nil {
				return 0, "", err
			}
			return resp.Result, resp.Message, nil
		},
	)
}

// ===== Week 10: 扩展功能处理器 =====

// HandleVoiceConfigResponse 处理语音配置响应（上行）
func (h *Handlers) HandleVoiceConfigResponse(ctx context.Context, f *Frame) error {
	return h.handleSimpleParamResponse(
		ctx,
		f,
		"voice config success",
		"voice config failed",
		func(data []byte) (uint8, string, error) {
			resp, err := ParseVoiceConfigResponse(data)
			if err != nil {
				return 0, "", err
			}
			return resp.Result, resp.Message, nil
		},
	)
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
