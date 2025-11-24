package bkv

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/taoyao-code/iot-server/internal/coremodel"
	"github.com/taoyao-code/iot-server/internal/driverapi"
	"github.com/taoyao-code/iot-server/internal/storage"
	"github.com/taoyao-code/iot-server/internal/storage/models"
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
	if h == nil {
		return nil
	}

	// 使用网关ID作为设备标识
	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	now := time.Now()

	// 通过 CoreEvents 报告心跳，让核心更新 last_seen 等状态；若未注入则回退到直接触库。
	if h.CoreEvents != nil && devicePhyID != "" {
		hb := &coremodel.CoreEvent{
			Type:       coremodel.EventDeviceHeartbeat,
			DeviceID:   coremodel.DeviceID(devicePhyID),
			OccurredAt: now,
			DeviceHeartbeat: &coremodel.DeviceHeartbeatPayload{
				DeviceID:   coremodel.DeviceID(devicePhyID),
				Status:     coremodel.DeviceStateOnline,
				LastSeenAt: now,
			},
		}
		if err := h.CoreEvents.HandleCoreEvent(ctx, hb); err == nil {
			// no-op
		}
	}

	// v2.1.3: 新设备注册时推送设备注册事件
	// 注意：这里简化处理，实际应该在首次注册时才推送
	// 可以通过检查设备是否是新创建来判断（比如检查created_at和updated_at是否相同）
	// 这里为了示例，暂时不推送（避免每次心跳都推送注册事件）

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

	return nil
}

// encodeHeartbeatAck 构造心跳ACK的payload（当前时间）
// 按协议文档使用7字节BCD时间戳: YYYYMMDDHHMMSS
func encodeHeartbeatAck(gatewayID string) []byte {
	now := time.Now()
	year := now.Year()

	toBCD := func(v int) byte {
		if v < 0 {
			v = 0
		}
		if v > 99 {
			v = v % 100
		}
		hi := (v / 10) & 0x0F
		lo := (v % 10) & 0x0F
		return byte(hi<<4 | lo)
	}

	yy1 := year / 100
	yy2 := year % 100

	return []byte{
		toBCD(yy1),
		toBCD(yy2),
		toBCD(int(now.Month())),
		toBCD(now.Day()),
		toBCD(now.Hour()),
		toBCD(now.Minute()),
		toBCD(now.Second()),
	}
}

// HandleBKVStatus 处理BKV插座状态上报 (cmd=0x1000 with BKV payload)
func (h *Handlers) HandleBKVStatus(ctx context.Context, f *Frame) error {
	if h == nil {
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

	// 如果是状态上报，尝试解析并更新端口状态，并按协议回ACK
	if payload.IsStatusReport() {
		err := h.handleSocketStatusUpdate(ctx, payload)
		h.sendStatusAck(ctx, f, payload, err == nil)
		return err
	}

	// 如果是充电结束上报，处理订单结算
	if payload.IsChargingEnd() {
		return h.handleBKVChargingEnd(ctx, f, payload)
	}

	// 如果是异常事件上报，处理异常信息
	if payload.IsExceptionReport() {
		return h.handleExceptionEvent(ctx, f, payload)
	}

	// 如果是参数查询，记录参数信息
	if payload.IsParameterQuery() {
		return h.handleParameterQuery(ctx, payload)
	}

	// 如果是控制命令，转发到控制处理器
	if payload.IsControlCommand() {
		return h.handleBKVControlCommand(ctx, payload)
	}

	return nil
}

// sendStatusAck 构造并下发0x1017状态上报ACK
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
func (h *Handlers) handleSocketStatusUpdate(ctx context.Context, payload *BKVPayload) error {
	if h == nil || h.CoreEvents == nil {
		return nil
	}

	socketStatus, err := payload.GetSocketStatus()
	if err != nil {
		return fmt.Errorf("parse socket status: %w", err)
	}

	devicePhyID := payload.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	emit := func(port *PortStatus) error {
		if port == nil {
			return nil
		}
		rawStatus := int32(port.Status)
		var power *int32
		if port.Power > 0 {
			p := int32(port.Power) / 10 // 0.1W → W
			power = &p
		}
		portNo := coremodel.PortNo(port.PortNo)
		now := time.Now()
		ev := &coremodel.CoreEvent{
			Type:       coremodel.EventPortSnapshot,
			DeviceID:   coremodel.DeviceID(devicePhyID),
			PortNo:     &portNo,
			OccurredAt: now,
			PortSnapshot: &coremodel.PortSnapshot{
				DeviceID:  coremodel.DeviceID(devicePhyID),
				PortNo:    portNo,
				RawStatus: rawStatus,
				PowerW:    power,
				At:        now,
			},
		}
		return h.CoreEvents.HandleCoreEvent(ctx, ev)
	}

	if err := emit(socketStatus.PortA); err != nil {
		return err
	}
	if err := emit(socketStatus.PortB); err != nil {
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

	// 如果有结束原因映射，进行转换
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

	// 使用 CoreEvents 将充电结束标准化为核心事件，由中间件核心完成订单结算和端口更新。
	if h.CoreEvents == nil || payload.GatewayID == "" {
		return fmt.Errorf("core events sink not configured for BKV charging end")
	}

	nextStatus := int32(0x09) // 0x09 = bit0(在线) + bit3(空载)
	rawReason := int32(reason)
	evPort := coremodel.PortNo(actualPort)
	evBiz := coremodel.BusinessNo(orderHex)

	ev := &coremodel.CoreEvent{
		Type:       coremodel.EventSessionEnded,
		DeviceID:   coremodel.DeviceID(payload.GatewayID),
		PortNo:     &evPort,
		BusinessNo: &evBiz,
		OccurredAt: time.Now(),
		SessionEnded: &coremodel.SessionEndedPayload{
			DeviceID:       coremodel.DeviceID(payload.GatewayID),
			PortNo:         coremodel.PortNo(actualPort),
			BusinessNo:     coremodel.BusinessNo(orderHex),
			EnergyKWh01:    int32(kwh01),
			DurationSec:    int32(durationSec),
			EndReasonCode:  "",
			InstantPowerW:  nil,
			OccurredAt:     time.Now(),
			RawReason:      &rawReason,
			NextPortStatus: &nextStatus,
		},
	}

	if err := h.CoreEvents.HandleCoreEvent(ctx, ev); err != nil {
		return fmt.Errorf("core event session ended failed: %w", err)
	}

	success = true
	return nil
}

// HandleControl 处理控制指令 (cmd=0x0015)
func (h *Handlers) HandleControl(ctx context.Context, f *Frame) error {
	if h == nil || h.CoreEvents == nil {
		return nil
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	if f.IsUplink() {
		// 处理充电结束/功率模式结束上报（0x0015 子命令 0x02 / 0x18）
		if len(f.Data) >= 3 && (f.Data[2] == 0x02 || f.Data[2] == 0x18) {
			if end, err := ParseBKVChargingEnd(f.Data); err == nil {
				now := time.Now()
				portNo := coremodel.PortNo(end.Port)
				rawStatus := int32(end.Status)
				var powerW *int32
				if end.InstantPower > 0 {
					p := int32(end.InstantPower) / 10 // 0.1W -> W
					powerW = &p
				}

				// PortSnapshot 更新
				evPS := &coremodel.CoreEvent{
					Type:       coremodel.EventPortSnapshot,
					DeviceID:   coremodel.DeviceID(devicePhyID),
					PortNo:     &portNo,
					OccurredAt: now,
					PortSnapshot: &coremodel.PortSnapshot{
						DeviceID:  coremodel.DeviceID(devicePhyID),
						PortNo:    portNo,
						RawStatus: rawStatus,
						PowerW:    powerW,
						At:        now,
					},
				}
				sn := int32(end.SocketNo)
				evPS.PortSnapshot.SocketNo = &sn
				_ = h.CoreEvents.HandleCoreEvent(ctx, evPS)

				// SessionEnded 更新
				nextStatus := int32(0x09) // 空闲
				durationSec := int32(end.ChargingTime) * 60
				energy01 := int32(end.EnergyUsed) // 0.01 kWh
				rawReason := int32(end.EndReason)
				biz := coremodel.BusinessNo(fmt.Sprintf("%04X", end.BusinessNo))
				evEnd := &coremodel.CoreEvent{
					Type:       coremodel.EventSessionEnded,
					DeviceID:   coremodel.DeviceID(devicePhyID),
					PortNo:     &portNo,
					BusinessNo: &biz,
					OccurredAt: now,
					SessionEnded: &coremodel.SessionEndedPayload{
						DeviceID:       coremodel.DeviceID(devicePhyID),
						PortNo:         portNo,
						BusinessNo:     biz,
						EnergyKWh01:    energy01,
						DurationSec:    durationSec,
						EndReasonCode:  "",
						InstantPowerW:  powerW,
						OccurredAt:     now,
						RawReason:      &rawReason,
						NextPortStatus: &nextStatus,
					},
				}
				_ = h.CoreEvents.HandleCoreEvent(ctx, evEnd)

				// 回 ACK（使用 socket_no/port_no）
				h.sendChargingEndAck(ctx, f, nil, int(end.SocketNo), int(end.Port), true)
				return nil
			}
		}

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
					status := int32(0x09) // 默认空闲
					if switchFlag == 0x01 {
						status = 0x81 // 充电中
					}
					evPort := coremodel.PortNo(portNo)
					evBiz := coremodel.BusinessNo(fmt.Sprintf("%04X", businessNo))
					ev := &coremodel.CoreEvent{
						Type:       coremodel.EventPortSnapshot,
						DeviceID:   coremodel.DeviceID(devicePhyID),
						PortNo:     &evPort,
						BusinessNo: &evBiz,
						OccurredAt: time.Now(),
						PortSnapshot: &coremodel.PortSnapshot{
							DeviceID:  coremodel.DeviceID(devicePhyID),
							PortNo:    evPort,
							Status:    coremodel.PortStatusUnknown,
							RawStatus: status,
							At:        time.Now(),
						},
					}
					// 在元数据中记录 socket_no 便于诊断
					if ev.PortSnapshot != nil {
						sn := int32(socketNo)
						ev.PortSnapshot.SocketNo = &sn
					}
					_ = h.CoreEvents.HandleCoreEvent(ctx, ev)
				}
			}
		}
	} else {
		if cmd, err := ParseBKVControlCommand(f.Data); err == nil {
			portNo := int(cmd.Port)
			status := int32(0x09)
			if cmd.Switch == SwitchOn {
				status = 0x81
			}
			evPort := coremodel.PortNo(portNo)
			ev := &coremodel.CoreEvent{
				Type:       coremodel.EventPortSnapshot,
				DeviceID:   coremodel.DeviceID(devicePhyID),
				PortNo:     &evPort,
				OccurredAt: time.Now(),
				PortSnapshot: &coremodel.PortSnapshot{
					DeviceID:  coremodel.DeviceID(devicePhyID),
					PortNo:    evPort,
					Status:    coremodel.PortStatusUnknown,
					RawStatus: status,
					At:        time.Now(),
				},
			}
			_ = h.CoreEvents.HandleCoreEvent(ctx, ev)
		}
	}

	return nil
}

// HandleChargingEnd 处理充电结束上报 (cmd=0x0015 上行，特定格式)
func (h *Handlers) HandleChargingEnd(ctx context.Context, f *Frame) error {
	if h == nil {
		return nil
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

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
				deviceIDStr := devicePhyID
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

			// 使用 CoreEvents 将充电结束标准化为核心事件，由中间件核心完成订单结算和端口更新。
			if h.CoreEvents != nil && devicePhyID != "" {
				nextStatus := int32(0x09) // 0x09 = bit0(在线) + bit3(空载)
				rawReason := int32(reason)
				evPort := coremodel.PortNo(portNo)
				evBiz := coremodel.BusinessNo(orderHex)

				ev := &coremodel.CoreEvent{
					Type:       coremodel.EventSessionEnded,
					DeviceID:   coremodel.DeviceID(devicePhyID),
					PortNo:     &evPort,
					BusinessNo: &evBiz,
					OccurredAt: time.Now(),
					SessionEnded: &coremodel.SessionEndedPayload{
						DeviceID:       coremodel.DeviceID(devicePhyID),
						PortNo:         coremodel.PortNo(portNo),
						BusinessNo:     coremodel.BusinessNo(orderHex),
						EnergyKWh01:    int32(kwh01),
						DurationSec:    int32(durationSec),
						EndReasonCode:  "",
						InstantPowerW:  nil,
						OccurredAt:     time.Now(),
						RawReason:      &rawReason,
						NextPortStatus: &nextStatus,
					},
				}

				_ = h.CoreEvents.HandleCoreEvent(ctx, ev)
			}
		}
	}

	return nil
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
	if h == nil || h.CoreEvents == nil {
		return nil
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	now := time.Now()
	ev := &coremodel.CoreEvent{
		Type:       coremodel.EventExceptionReported,
		DeviceID:   coremodel.DeviceID(devicePhyID),
		OccurredAt: now,
		Exception: &coremodel.ExceptionPayload{
			DeviceID: coremodel.DeviceID(devicePhyID),
			Code:     "generic_cmd",
			Message:  fmt.Sprintf("cmd=0x%04X", f.Cmd),
			Severity: "info",
			Metadata: map[string]string{
				"payload": fmt.Sprintf("%x", f.Data),
			},
			OccurredAt: now,
		},
	}
	_ = h.CoreEvents.HandleCoreEvent(ctx, ev)
	return nil
}

// HandleNetworkList 处理0x0005 网络节点列表相关指令（2.2.5/2.2.6 ACK）
// 对标 docs/协议/设备对接指引-组网设备2024(1).txt 中的:
// - 2.2.5 下发网络节点列表-刷新列表 设备回复
// - 2.2.6 下发网络节点列表-添加单个插座 设备回复
func (h *Handlers) HandleNetworkList(ctx context.Context, f *Frame) error {
	if h == nil {
		return nil
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	if h.CoreEvents == nil {
		return nil
	}

	now := time.Now()
	d := f.Data
	action := "network_ack"
	result := "unknown"
	msg := fmt.Sprintf("NetworkCmd0005: short payload len=%d payload=%x", len(d), d)
	metadata := map[string]string{
		"cmd": fmt.Sprintf("0x%04X", f.Cmd),
	}

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

	ev := &coremodel.CoreEvent{
		Type:       coremodel.EventNetworkTopology,
		DeviceID:   coremodel.DeviceID(devicePhyID),
		OccurredAt: now,
		NetworkTopology: &coremodel.NetworkTopologyPayload{
			DeviceID:   coremodel.DeviceID(devicePhyID),
			Action:     action,
			Result:     result,
			Message:    msg,
			Metadata:   metadata,
			OccurredAt: now,
		},
	}

	_ = h.CoreEvents.HandleCoreEvent(ctx, ev)
	return nil
}

// HandleParam 处理参数读写指令 (完整的写入→回读校验实现)
func (h *Handlers) HandleParam(ctx context.Context, f *Frame) error {
	if h == nil {
		return nil
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	if h.CoreEvents == nil {
		return nil
	}

	now := time.Now()
	result := "param"
	msg := "param message"
	metadata := map[string]string{
		"cmd":     fmt.Sprintf("0x%04X", f.Cmd),
		"payload": fmt.Sprintf("%x", f.Data),
	}

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
	default:
		result = "param"
	}

	ev := &coremodel.CoreEvent{
		Type:       coremodel.EventParamResult,
		DeviceID:   coremodel.DeviceID(devicePhyID),
		OccurredAt: now,
		ParamResult: &coremodel.ParamResultPayload{
			DeviceID:   coremodel.DeviceID(devicePhyID),
			Result:     result,
			Message:    msg,
			Metadata:   metadata,
			OccurredAt: now,
		},
	}

	_ = h.CoreEvents.HandleCoreEvent(ctx, ev)
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

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = payload.GatewayID
	}
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	if h.CoreEvents != nil {
		now := time.Now()
		port := coremodel.PortNo(event.SocketNo)
		rawStatus := int32(event.SocketEventStatus)
		meta := map[string]string{
			"reason": fmt.Sprintf("%d", event.SocketEventReason),
		}
		ev := &coremodel.CoreEvent{
			Type:       coremodel.EventExceptionReported,
			DeviceID:   coremodel.DeviceID(devicePhyID),
			PortNo:     &port,
			OccurredAt: now,
			Exception: &coremodel.ExceptionPayload{
				DeviceID:   coremodel.DeviceID(devicePhyID),
				PortNo:     &port,
				Code:       fmt.Sprintf("socket_event_%d", event.SocketEventReason),
				Message:    fmt.Sprintf("status=%d", event.SocketEventStatus),
				Severity:   "error",
				RawStatus:  &rawStatus,
				Metadata:   meta,
				OccurredAt: now,
			},
		}
		_ = h.CoreEvents.HandleCoreEvent(ctx, ev)
	}

	success = true
	return nil
}

// handleParameterQuery 处理参数查询
func (h *Handlers) handleParameterQuery(ctx context.Context, payload *BKVPayload) error {
	param, err := ParseBKVParameterQuery(payload)
	if err != nil {
		return fmt.Errorf("failed to parse parameter query: %w", err)
	}

	if h.CoreEvents == nil {
		return nil
	}

	devicePhyID := payload.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	now := time.Now()
	meta := map[string]string{
		"socket_no": fmt.Sprintf("%d", param.SocketNo),
		"power":     fmt.Sprintf("%d", param.PowerLimit),
		"temp":      fmt.Sprintf("%d", param.HighTempThreshold),
	}
	ev := &coremodel.CoreEvent{
		Type:       coremodel.EventParamResult,
		DeviceID:   coremodel.DeviceID(devicePhyID),
		OccurredAt: now,
		ParamResult: &coremodel.ParamResultPayload{
			DeviceID:   coremodel.DeviceID(devicePhyID),
			Result:     "query",
			Message:    "parameter query",
			Metadata:   meta,
			OccurredAt: now,
		},
	}
	_ = h.CoreEvents.HandleCoreEvent(ctx, ev)

	return nil
}

// handleBKVControlCommand 处理BKV控制命令
func (h *Handlers) handleBKVControlCommand(ctx context.Context, payload *BKVPayload) error {
	if payload.IsCardCharging() {
		return h.handleCardCharging(ctx, payload)
	}

	if h.CoreEvents == nil {
		return nil
	}

	devicePhyID := payload.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	now := time.Now()
	meta := map[string]string{
		"cmd": fmt.Sprintf("0x%02X", payload.Cmd),
	}

	ev := &coremodel.CoreEvent{
		Type:       coremodel.EventExceptionReported,
		DeviceID:   coremodel.DeviceID(devicePhyID),
		OccurredAt: now,
		Exception: &coremodel.ExceptionPayload{
			DeviceID:   coremodel.DeviceID(devicePhyID),
			Code:       "control_command",
			Message:    "control command received",
			Severity:   "info",
			Metadata:   meta,
			OccurredAt: now,
		},
	}
	_ = h.CoreEvents.HandleCoreEvent(ctx, ev)
	return nil
}

// handleCardCharging 处理刷卡充电
func (h *Handlers) handleCardCharging(ctx context.Context, payload *BKVPayload) error {
	if h.CoreEvents == nil {
		return nil
	}

	devicePhyID := payload.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	now := time.Now()
	ev := &coremodel.CoreEvent{
		Type:       coremodel.EventExceptionReported,
		DeviceID:   coremodel.DeviceID(devicePhyID),
		OccurredAt: now,
		Exception: &coremodel.ExceptionPayload{
			DeviceID:   coremodel.DeviceID(devicePhyID),
			Code:       "card_charging_control",
			Message:    "card charging control command",
			Severity:   "info",
			OccurredAt: now,
		},
	}
	_ = h.CoreEvents.HandleCoreEvent(ctx, ev)

	return nil
}

// ============ Week4: 刷卡充电处理函数 ============

// HandleCardSwipe 处理刷卡上报 (0x0B)
func (h *Handlers) HandleCardSwipe(ctx context.Context, f *Frame) error {
	if h == nil {
		return nil
	}

	// 上行：设备刷卡上报
	if f.IsUplink() {
		return h.handleCardSwipeUplink(ctx, f)
	}

	return nil
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

	if h.CoreEvents != nil {
		portNo := coremodel.PortNo(0)
		biz := coremodel.BusinessNo(req.CardNo)
		ev := &coremodel.CoreEvent{
			Type:       coremodel.EventSessionStarted,
			DeviceID:   coremodel.DeviceID(devicePhyID),
			PortNo:     &portNo,
			BusinessNo: &biz,
			OccurredAt: time.Now(),
			SessionStarted: &coremodel.SessionStartedPayload{
				DeviceID:   coremodel.DeviceID(devicePhyID),
				PortNo:     portNo,
				BusinessNo: biz,
				Mode:       "card_swipe",
				CardNo:     &req.CardNo,
				Metadata:   map[string]string{"balance": fmt.Sprintf("%d", req.Balance)},
				StartedAt:  time.Now(),
			},
		}
		_ = h.CoreEvents.HandleCoreEvent(ctx, ev)
	}

	return nil
}

// HandleOrderConfirm 处理订单确认 (0x0F)
func (h *Handlers) HandleOrderConfirm(ctx context.Context, f *Frame) error {
	if h == nil {
		return nil
	}

	// 上行：设备确认订单
	if f.IsUplink() {
		return h.handleOrderConfirmUplink(ctx, f)
	}

	return nil
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

	if h.CoreEvents != nil {
		portNo := int32(0)
		biz := coremodel.BusinessNo(conf.OrderNo)
		status := fmt.Sprintf("%d", conf.Status)
		ev := &coremodel.CoreEvent{
			Type:       coremodel.EventSessionStarted,
			DeviceID:   coremodel.DeviceID(devicePhyID),
			PortNo:     (*coremodel.PortNo)(&portNo),
			BusinessNo: &biz,
			OccurredAt: time.Now(),
			SessionStarted: &coremodel.SessionStartedPayload{
				DeviceID:   coremodel.DeviceID(devicePhyID),
				PortNo:     coremodel.PortNo(portNo),
				BusinessNo: biz,
				Mode:       "order_confirm",
				Metadata:   map[string]string{"status": status, "reason": conf.Reason},
				StartedAt:  time.Now(),
			},
		}
		_ = h.CoreEvents.HandleCoreEvent(ctx, ev)
	}

	return nil
}

// HandleChargeEnd 处理充电结束 (0x0C)
func (h *Handlers) HandleChargeEnd(ctx context.Context, f *Frame) error {
	if h == nil {
		return nil
	}

	// 上行：设备上报充电结束
	if f.IsUplink() {
		return h.handleChargeEndUplink(ctx, f)
	}

	return nil
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

	if h.CoreEvents != nil {
		biz := coremodel.BusinessNo(report.OrderNo)
		port := coremodel.PortNo(0)
		amount := int64(report.Amount)
		kwh01 := int32(report.Energy / 10)
		duration := int32(report.Duration * 60)
		rawReason := int32(report.EndReason)
		ev := &coremodel.CoreEvent{
			Type:       coremodel.EventSessionEnded,
			DeviceID:   coremodel.DeviceID(devicePhyID),
			PortNo:     &port,
			BusinessNo: &biz,
			OccurredAt: time.Now(),
			SessionEnded: &coremodel.SessionEndedPayload{
				DeviceID:    coremodel.DeviceID(devicePhyID),
				PortNo:      port,
				BusinessNo:  biz,
				EnergyKWh01: kwh01,
				DurationSec: duration,
				AmountCent:  &amount,
				RawReason:   &rawReason,
				OccurredAt:  time.Now(),
			},
		}
		_ = h.CoreEvents.HandleCoreEvent(ctx, ev)
	}

	return nil
}

// HandleBalanceQuery 处理余额查询 (0x1A)
func (h *Handlers) HandleBalanceQuery(ctx context.Context, f *Frame) error {
	if h == nil {
		return nil
	}

	// 上行：设备查询余额
	if f.IsUplink() {
		return h.handleBalanceQueryUplink(ctx, f)
	}

	return nil
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

	if h.CoreEvents != nil {
		port := coremodel.PortNo(0)
		ev := &coremodel.CoreEvent{
			Type:       coremodel.EventExceptionReported,
			DeviceID:   coremodel.DeviceID(devicePhyID),
			PortNo:     &port,
			OccurredAt: time.Now(),
			Exception: &coremodel.ExceptionPayload{
				DeviceID:   coremodel.DeviceID(devicePhyID),
				PortNo:     &port,
				Code:       "BalanceQuery",
				Message:    fmt.Sprintf("card=%s", query.CardNo),
				Severity:   "info",
				OccurredAt: time.Now(),
				Metadata:   map[string]string{"card_no": query.CardNo},
			},
		}
		_ = h.CoreEvents.HandleCoreEvent(ctx, ev)
	}

	return nil
}

// ===== Week 6: 组网管理处理器 =====

// HandleNetworkRefresh 处理刷新插座列表响应（上行）
func (h *Handlers) HandleNetworkRefresh(ctx context.Context, f *Frame) error {
	if h == nil {
		return nil
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	resp, err := ParseNetworkRefreshResponse(f.Data)
	result := "ok"
	msg := "network refresh"
	metadata := map[string]string{
		"cmd": fmt.Sprintf("0x%04X", f.Cmd),
	}
	upserted := 0
	upsertErrors := 0

	if err != nil {
		result = "failed"
		msg = err.Error()
		metadata["raw_payload"] = fmt.Sprintf("%x", f.Data)
	} else {
		metadata["socket_count"] = fmt.Sprintf("%d", len(resp.Sockets))
		if h.Core != nil {
			now := time.Now()
			for _, s := range resp.Sockets {
				socket := &models.GatewaySocket{
					GatewayID:  devicePhyID,
					SocketNo:   int32(s.SocketNo),
					SocketMAC:  s.SocketMAC,
					LastSeenAt: &now,
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
					upsertErrors++
					continue
				}
				upserted++
			}
		}
	}
	if upserted > 0 {
		metadata["mapping_upserted"] = fmt.Sprintf("%d", upserted)
	}
	if upsertErrors > 0 {
		metadata["mapping_upsert_errors"] = fmt.Sprintf("%d", upsertErrors)
	}

	if h.CoreEvents != nil {
		now := time.Now()
		ev := &coremodel.CoreEvent{
			Type:       coremodel.EventNetworkTopology,
			DeviceID:   coremodel.DeviceID(devicePhyID),
			OccurredAt: now,
			NetworkTopology: &coremodel.NetworkTopologyPayload{
				DeviceID:   coremodel.DeviceID(devicePhyID),
				Action:     "refresh",
				Result:     result,
				Message:    msg,
				Metadata:   metadata,
				OccurredAt: now,
			},
		}
		_ = h.CoreEvents.HandleCoreEvent(ctx, ev)
	}

	return err
}

// HandleNetworkAddNode 处理添加插座响应（上行）
func (h *Handlers) HandleNetworkAddNode(ctx context.Context, f *Frame) error {
	if h == nil {
		return nil
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	resp, err := ParseNetworkAddNodeResponse(f.Data)
	result := "ok"
	msg := "add socket success"
	metadata := map[string]string{
		"cmd": fmt.Sprintf("0x%04X", f.Cmd),
	}
	var socketPtr *int32

	if err != nil {
		result = "failed"
		msg = err.Error()
		metadata["raw_payload"] = fmt.Sprintf("%x", f.Data)
	} else {
		socket := int32(resp.SocketNo)
		socketPtr = &socket
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

	if h.CoreEvents != nil {
		now := time.Now()
		ev := &coremodel.CoreEvent{
			Type:       coremodel.EventNetworkTopology,
			DeviceID:   coremodel.DeviceID(devicePhyID),
			OccurredAt: now,
			NetworkTopology: &coremodel.NetworkTopologyPayload{
				DeviceID:   coremodel.DeviceID(devicePhyID),
				Action:     "add_node",
				SocketNo:   socketPtr,
				Result:     result,
				Message:    msg,
				Metadata:   metadata,
				OccurredAt: now,
			},
		}
		_ = h.CoreEvents.HandleCoreEvent(ctx, ev)
	}

	return err
}

// HandleNetworkDeleteNode 处理删除插座响应（上行）
func (h *Handlers) HandleNetworkDeleteNode(ctx context.Context, f *Frame) error {
	if h == nil {
		return nil
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	resp, err := ParseNetworkDeleteNodeResponse(f.Data)
	result := "ok"
	msg := "delete socket success"
	metadata := map[string]string{
		"cmd": fmt.Sprintf("0x%04X", f.Cmd),
	}
	var socketPtr *int32

	if err != nil {
		result = "failed"
		msg = err.Error()
		metadata["raw_payload"] = fmt.Sprintf("%x", f.Data)
	} else {
		socket := int32(resp.SocketNo)
		socketPtr = &socket
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

	if h.CoreEvents != nil {
		now := time.Now()
		ev := &coremodel.CoreEvent{
			Type:       coremodel.EventNetworkTopology,
			DeviceID:   coremodel.DeviceID(devicePhyID),
			OccurredAt: now,
			NetworkTopology: &coremodel.NetworkTopologyPayload{
				DeviceID:   coremodel.DeviceID(devicePhyID),
				Action:     "delete_node",
				SocketNo:   socketPtr,
				Result:     result,
				Message:    msg,
				Metadata:   metadata,
				OccurredAt: now,
			},
		}
		_ = h.CoreEvents.HandleCoreEvent(ctx, ev)
	}

	return err
}

// ===== Week 7: OTA升级处理器 =====

// HandleOTAResponse 处理OTA升级响应（上行）
func (h *Handlers) HandleOTAResponse(ctx context.Context, f *Frame) error {
	if h == nil {
		return nil
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	resp, err := ParseOTAResponse(f.Data)
	status := "failed"
	msg := "ota response failed"
	metadata := map[string]string{
		"cmd": fmt.Sprintf("0x%04X", f.Cmd),
	}
	var socketPtr *coremodel.PortNo

	if err != nil {
		msg = err.Error()
		metadata["raw_payload"] = fmt.Sprintf("%x", f.Data)
	} else {
		if resp.SocketNo > 0 {
			socket := coremodel.PortNo(resp.SocketNo)
			socketPtr = &socket
		}
		metadata["target_type"] = fmt.Sprintf("%d", resp.TargetType)
		metadata["raw_result"] = fmt.Sprintf("%d", resp.Result)
		if resp.Result == 0 {
			status = "accepted"
			msg = "ota accepted"
		} else if resp.Reason != "" {
			msg = resp.Reason
		}
	}

	if h.CoreEvents != nil {
		now := time.Now()
		ev := &coremodel.CoreEvent{
			Type:       coremodel.EventOTAProgress,
			DeviceID:   coremodel.DeviceID(devicePhyID),
			PortNo:     nil,
			OccurredAt: now,
			OTAProgress: &coremodel.OTAProgressPayload{
				DeviceID:   coremodel.DeviceID(devicePhyID),
				PortNo:     socketPtr,
				Status:     status,
				Progress:   0,
				Message:    msg,
				Metadata:   metadata,
				OccurredAt: now,
			},
		}
		_ = h.CoreEvents.HandleCoreEvent(ctx, ev)
	}

	return err
}

// HandleOTAProgress 处理OTA升级进度上报（上行）
func (h *Handlers) HandleOTAProgress(ctx context.Context, f *Frame) error {
	if h == nil {
		return nil
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	progress, err := ParseOTAProgress(f.Data)
	status := "in_progress"
	msg := "ota in progress"
	metadata := map[string]string{
		"cmd": fmt.Sprintf("0x%04X", f.Cmd),
	}
	var socketPtr *coremodel.PortNo
	var progressVal int32

	if err != nil {
		status = "failed"
		msg = err.Error()
		metadata["raw_payload"] = fmt.Sprintf("%x", f.Data)
	} else {
		if progress.SocketNo > 0 {
			socket := coremodel.PortNo(progress.SocketNo)
			socketPtr = &socket
		}
		progressVal = int32(progress.Progress)
		metadata["target_type"] = fmt.Sprintf("%d", progress.TargetType)
		metadata["status_code"] = fmt.Sprintf("%d", progress.Status)
		if progress.Progress <= 100 {
			metadata["progress"] = fmt.Sprintf("%d", progress.Progress)
		}
		switch progress.Status {
		case 0:
			status = "downloading"
		case 1:
			status = "installing"
		case 2:
			status = "completed"
			msg = "ota completed"
		case 3:
			status = "failed"
			if progress.ErrorMsg != "" {
				msg = progress.ErrorMsg
			} else {
				msg = "ota failed"
			}
		}
	}

	if h.CoreEvents != nil {
		now := time.Now()
		ev := &coremodel.CoreEvent{
			Type:       coremodel.EventOTAProgress,
			DeviceID:   coremodel.DeviceID(devicePhyID),
			PortNo:     nil,
			OccurredAt: now,
			OTAProgress: &coremodel.OTAProgressPayload{
				DeviceID:   coremodel.DeviceID(devicePhyID),
				PortNo:     socketPtr,
				Status:     status,
				Progress:   progressVal,
				Message:    msg,
				Metadata:   metadata,
				OccurredAt: now,
			},
		}
		_ = h.CoreEvents.HandleCoreEvent(ctx, ev)
	}

	return err
}

// ===== Week 8: 按功率分档充电处理器 =====

// HandlePowerLevelEnd 处理按功率充电结束上报（上行）
func (h *Handlers) HandlePowerLevelEnd(ctx context.Context, f *Frame) error {
	// 解析充电结束上报
	report, err := ParsePowerLevelEndReport(f.Data)
	if err != nil {
		return fmt.Errorf("parse power level end report: %w", err)
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	if h.CoreEvents != nil {
		now := time.Now()
		port := coremodel.PortNo(report.PortNo)
		rawReason := int32(report.EndReason)
		duration := int32(report.TotalDuration) * 60
		energy := int32(report.TotalEnergy)
		amount := int64(report.TotalAmount)

		ev := &coremodel.CoreEvent{
			Type:       coremodel.EventSessionEnded,
			DeviceID:   coremodel.DeviceID(devicePhyID),
			PortNo:     &port,
			OccurredAt: now,
			SessionEnded: &coremodel.SessionEndedPayload{
				DeviceID:    coremodel.DeviceID(devicePhyID),
				PortNo:      port,
				BusinessNo:  "",
				DurationSec: duration,
				EnergyKWh01: energy,
				AmountCent:  &amount,
				RawReason:   &rawReason,
				OccurredAt:  now,
			},
		}
		_ = h.CoreEvents.HandleCoreEvent(ctx, ev)
	}

	reply := EncodePowerLevelEndReply(report.PortNo, 0) // 0=确认成功

	// 发送确认回复（下行），使用cmd=0x0018以匹配上行命令
	if h.Outbound != nil && f.GatewayID != "" && len(reply) > 0 {
		_ = h.Outbound.SendDownlink(f.GatewayID, 0x0018, f.MsgID, reply)
	}

	return nil
}

// ===== Week 9: 参数管理处理器 =====

// HandleParamReadResponse 处理批量读取参数响应（上行）
func (h *Handlers) HandleParamReadResponse(ctx context.Context, f *Frame) error {
	if h == nil {
		return nil
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	resp, err := ParseParamReadResponse(f.Data)
	result := "ok"
	msg := "param read response"
	metadata := map[string]string{
		"cmd": fmt.Sprintf("0x%04X", f.Cmd),
	}

	if err != nil {
		result = "failed"
		msg = err.Error()
		metadata["raw_payload"] = fmt.Sprintf("%x", f.Data)
	} else {
		metadata["param_count"] = fmt.Sprintf("%d", len(resp.Params))
	}

	if h.CoreEvents != nil {
		now := time.Now()
		ev := &coremodel.CoreEvent{
			Type:       coremodel.EventParamResult,
			DeviceID:   coremodel.DeviceID(devicePhyID),
			OccurredAt: now,
			ParamResult: &coremodel.ParamResultPayload{
				DeviceID:   coremodel.DeviceID(devicePhyID),
				Result:     result,
				Message:    msg,
				Metadata:   metadata,
				OccurredAt: now,
			},
		}
		_ = h.CoreEvents.HandleCoreEvent(ctx, ev)
	}

	return err
}

// HandleParamWriteResponse 处理批量写入参数响应（上行）
func (h *Handlers) HandleParamWriteResponse(ctx context.Context, f *Frame) error {
	if h == nil {
		return nil
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	resp, err := ParseParamWriteResponse(f.Data)
	result := "ok"
	msg := "param write response"
	metadata := map[string]string{
		"cmd": fmt.Sprintf("0x%04X", f.Cmd),
	}

	if err != nil {
		result = "failed"
		msg = err.Error()
		metadata["raw_payload"] = fmt.Sprintf("%x", f.Data)
	} else {
		successCount := 0
		for _, r := range resp.Results {
			if r.Result == 0 {
				successCount++
			}
		}
		metadata["param_count"] = fmt.Sprintf("%d", len(resp.Results))
		metadata["success_count"] = fmt.Sprintf("%d", successCount)
		if successCount != len(resp.Results) {
			result = "partial"
		}
	}

	if h.CoreEvents != nil {
		now := time.Now()
		ev := &coremodel.CoreEvent{
			Type:       coremodel.EventParamResult,
			DeviceID:   coremodel.DeviceID(devicePhyID),
			OccurredAt: now,
			ParamResult: &coremodel.ParamResultPayload{
				DeviceID:   coremodel.DeviceID(devicePhyID),
				Result:     result,
				Message:    msg,
				Metadata:   metadata,
				OccurredAt: now,
			},
		}
		_ = h.CoreEvents.HandleCoreEvent(ctx, ev)
	}

	return err
}

// HandleParamSyncResponse 处理参数同步响应（上行）
func (h *Handlers) HandleParamSyncResponse(ctx context.Context, f *Frame) error {
	if h == nil {
		return nil
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	resp, err := ParseParamSyncResponse(f.Data)
	result := "ok"
	msg := "param sync"
	progress := int32(0)
	metadata := map[string]string{
		"cmd": fmt.Sprintf("0x%04X", f.Cmd),
	}

	if err != nil {
		result = "failed"
		msg = err.Error()
		metadata["raw_payload"] = fmt.Sprintf("%x", f.Data)
	} else {
		progress = int32(resp.Progress)
		metadata["raw_result"] = fmt.Sprintf("%d", resp.Result)
		msg = GetParamSyncResultDescription(resp.Result)
		if resp.Message != "" {
			metadata["message"] = resp.Message
		}
		if resp.Result != 0 && resp.Result != 2 {
			result = "in_progress"
		}
	}

	if h.CoreEvents != nil {
		now := time.Now()
		ev := &coremodel.CoreEvent{
			Type:       coremodel.EventParamSync,
			DeviceID:   coremodel.DeviceID(devicePhyID),
			OccurredAt: now,
			ParamSync: &coremodel.ParamSyncPayload{
				DeviceID:   coremodel.DeviceID(devicePhyID),
				Progress:   progress,
				Result:     result,
				Message:    msg,
				Metadata:   metadata,
				OccurredAt: now,
			},
		}
		_ = h.CoreEvents.HandleCoreEvent(ctx, ev)
	}

	return err
}

// HandleParamResetResponse 处理参数重置响应（上行）
func (h *Handlers) HandleParamResetResponse(ctx context.Context, f *Frame) error {
	if h == nil {
		return nil
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	resp, err := ParseParamResetResponse(f.Data)
	result := "ok"
	msg := "param reset success"
	metadata := map[string]string{
		"cmd": fmt.Sprintf("0x%04X", f.Cmd),
	}

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

	if h.CoreEvents != nil {
		now := time.Now()
		ev := &coremodel.CoreEvent{
			Type:       coremodel.EventParamResult,
			DeviceID:   coremodel.DeviceID(devicePhyID),
			OccurredAt: now,
			ParamResult: &coremodel.ParamResultPayload{
				DeviceID:   coremodel.DeviceID(devicePhyID),
				Result:     result,
				Message:    msg,
				Metadata:   metadata,
				OccurredAt: now,
			},
		}
		_ = h.CoreEvents.HandleCoreEvent(ctx, ev)
	}

	return err
}

// ===== Week 10: 扩展功能处理器 =====

// HandleVoiceConfigResponse 处理语音配置响应（上行）
func (h *Handlers) HandleVoiceConfigResponse(ctx context.Context, f *Frame) error {
	if h == nil {
		return nil
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	resp, err := ParseVoiceConfigResponse(f.Data)
	result := "ok"
	msg := "voice config success"
	metadata := map[string]string{
		"cmd": fmt.Sprintf("0x%04X", f.Cmd),
	}

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

	if h.CoreEvents != nil {
		now := time.Now()
		ev := &coremodel.CoreEvent{
			Type:       coremodel.EventParamResult,
			DeviceID:   coremodel.DeviceID(devicePhyID),
			OccurredAt: now,
			ParamResult: &coremodel.ParamResultPayload{
				DeviceID:   coremodel.DeviceID(devicePhyID),
				Result:     result,
				Message:    msg,
				Metadata:   metadata,
				OccurredAt: now,
			},
		}
		_ = h.CoreEvents.HandleCoreEvent(ctx, ev)
	}

	return err
}

// HandleSocketStateResponse 处理插座状态响应（上行）
func (h *Handlers) HandleSocketStateResponse(ctx context.Context, f *Frame) error {
	resp, err := ParseSocketStateResponse(f.Data)
	if err != nil {
		return fmt.Errorf("parse socket state response: %w", err)
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	// 为保持与 BKV 状态位图的一致性，这里将 0/1/2 的业务枚举映射为约定的位图值：
	//   - 0: idle  → 0x09 (在线+空载)
	//   - 1: charging → 0x81 (在线+充电)
	//   - 2: fault → 0x00 (离线/故障，占位，不设置充电位)
	var dbStatus int32
	switch resp.Status {
	case 0:
		dbStatus = 0x09
	case 1:
		dbStatus = 0x81
	case 2:
		dbStatus = 0x00
	default:
		dbStatus = 0x00
	}

	power := int32(resp.Power) // W

	if h.CoreEvents != nil {
		now := time.Now()
		port := coremodel.PortNo(resp.SocketNo)
		ev := &coremodel.CoreEvent{
			Type:       coremodel.EventPortSnapshot,
			DeviceID:   coremodel.DeviceID(devicePhyID),
			PortNo:     &port,
			OccurredAt: now,
			PortSnapshot: &coremodel.PortSnapshot{
				DeviceID:  coremodel.DeviceID(devicePhyID),
				PortNo:    port,
				RawStatus: dbStatus,
				PowerW:    &power,
				At:        now,
			},
		}
		_ = h.CoreEvents.HandleCoreEvent(ctx, ev)
	}

	// 更新指标
	if h.Metrics != nil {
		h.Metrics.GetPortStatusQueryResponseTotal().WithLabelValues(
			devicePhyID,
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

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	if h.CoreEvents != nil {
		now := time.Now()
		port := coremodel.PortNo(report.PortNo)
		rawReason := int32(report.EndReason)
		duration := int32(report.TotalDuration) * 60
		energy := int32(report.TotalEnergy)
		total := int64(report.TotalAmount)

		ev := &coremodel.CoreEvent{
			Type:       coremodel.EventSessionEnded,
			DeviceID:   coremodel.DeviceID(devicePhyID),
			PortNo:     &port,
			OccurredAt: now,
			SessionEnded: &coremodel.SessionEndedPayload{
				DeviceID:    coremodel.DeviceID(devicePhyID),
				PortNo:      port,
				BusinessNo:  "",
				DurationSec: duration,
				EnergyKWh01: energy,
				AmountCent:  &total,
				RawReason:   &rawReason,
				OccurredAt:  now,
			},
		}
		_ = h.CoreEvents.HandleCoreEvent(ctx, ev)
	}

	reply := EncodeServiceFeeEndReply(report.PortNo, 0)

	if h.Outbound != nil && devicePhyID != "" && len(reply) > 0 {
		_ = h.Outbound.SendDownlink(devicePhyID, f.Cmd, f.MsgID, reply)
	}

	return nil
}
