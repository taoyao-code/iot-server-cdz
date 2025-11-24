package bkv

import (
	"time"

	"github.com/taoyao-code/iot-server/internal/coremodel"
)

// EventBuilder CoreEvent 构建器，使用链式调用模式
type EventBuilder struct {
	deviceID   coremodel.DeviceID
	occurredAt time.Time
	portNo     *coremodel.PortNo
	businessNo *coremodel.BusinessNo
	socketNo   *int32
}

// NewEventBuilder 创建事件构建器
func NewEventBuilder(deviceID string) *EventBuilder {
	return &EventBuilder{
		deviceID:   coremodel.DeviceID(deviceID),
		occurredAt: now(),
	}
}

// WithPort 设置端口号
func (b *EventBuilder) WithPort(portNo int) *EventBuilder {
	p := coremodel.PortNo(portNo)
	b.portNo = &p
	return b
}

// WithBusinessNo 设置业务号
func (b *EventBuilder) WithBusinessNo(businessNo string) *EventBuilder {
	bn := coremodel.BusinessNo(businessNo)
	b.businessNo = &bn
	return b
}

// WithSocketNo 设置插座号
func (b *EventBuilder) WithSocketNo(socketNo int) *EventBuilder {
	sn := int32(socketNo)
	b.socketNo = &sn
	return b
}

// BuildHeartbeat 构建心跳事件
func (b *EventBuilder) BuildHeartbeat() *coremodel.CoreEvent {
	return &coremodel.CoreEvent{
		Type:       coremodel.EventDeviceHeartbeat,
		DeviceID:   b.deviceID,
		OccurredAt: b.occurredAt,
		DeviceHeartbeat: &coremodel.DeviceHeartbeatPayload{
			DeviceID:   b.deviceID,
			Status:     coremodel.DeviceStateOnline,
			LastSeenAt: b.occurredAt,
		},
	}
}

// BuildPortSnapshot 构建端口快照事件
func (b *EventBuilder) BuildPortSnapshot(rawStatus int32, powerW *int32) *coremodel.CoreEvent {
	if b.portNo == nil {
		return nil
	}

	ev := &coremodel.CoreEvent{
		Type:       coremodel.EventPortSnapshot,
		DeviceID:   b.deviceID,
		PortNo:     b.portNo,
		OccurredAt: b.occurredAt,
		PortSnapshot: &coremodel.PortSnapshot{
			DeviceID:  b.deviceID,
			PortNo:    *b.portNo,
			RawStatus: rawStatus,
			PowerW:    powerW,
			At:        b.occurredAt,
		},
	}

	// 如果有插座号，添加到元数据
	if b.socketNo != nil {
		ev.PortSnapshot.SocketNo = b.socketNo
	}

	return ev
}

// BuildSessionEnded 构建会话结束事件
func (b *EventBuilder) BuildSessionEnded(
	energyKWh01 int32,
	durationSec int32,
	rawReason *int32,
	nextStatus *int32,
	amountCent *int64,
	instantPowerW *int32,
) *coremodel.CoreEvent {
	if b.portNo == nil {
		return nil
	}

	var biz coremodel.BusinessNo
	if b.businessNo != nil {
		biz = *b.businessNo
	}

	return &coremodel.CoreEvent{
		Type:       coremodel.EventSessionEnded,
		DeviceID:   b.deviceID,
		PortNo:     b.portNo,
		BusinessNo: b.businessNo,
		OccurredAt: b.occurredAt,
		SessionEnded: &coremodel.SessionEndedPayload{
			DeviceID:       b.deviceID,
			PortNo:         *b.portNo,
			BusinessNo:     biz,
			EnergyKWh01:    energyKWh01,
			DurationSec:    durationSec,
			EndReasonCode:  "",
			InstantPowerW:  instantPowerW,
			RawReason:      rawReason,
			NextPortStatus: nextStatus,
			AmountCent:     amountCent,
			OccurredAt:     b.occurredAt,
		},
	}
}

// BuildSessionStarted 构建会话开始事件
func (b *EventBuilder) BuildSessionStarted(
	mode string,
	cardNo *string,
	metadata map[string]string,
) *coremodel.CoreEvent {
	if b.portNo == nil {
		return nil
	}

	var biz coremodel.BusinessNo
	if b.businessNo != nil {
		biz = *b.businessNo
	}

	return &coremodel.CoreEvent{
		Type:       coremodel.EventSessionStarted,
		DeviceID:   b.deviceID,
		PortNo:     b.portNo,
		BusinessNo: b.businessNo,
		OccurredAt: b.occurredAt,
		SessionStarted: &coremodel.SessionStartedPayload{
			DeviceID:   b.deviceID,
			PortNo:     *b.portNo,
			BusinessNo: biz,
			Mode:       mode,
			CardNo:     cardNo,
			Metadata:   metadata,
			StartedAt:  b.occurredAt,
		},
	}
}

// BuildNetworkTopology 构建网络拓扑事件
func (b *EventBuilder) BuildNetworkTopology(
	action string,
	result string,
	message string,
	metadata map[string]string,
) *coremodel.CoreEvent {
	ev := &coremodel.CoreEvent{
		Type:       coremodel.EventNetworkTopology,
		DeviceID:   b.deviceID,
		OccurredAt: b.occurredAt,
		NetworkTopology: &coremodel.NetworkTopologyPayload{
			DeviceID:   b.deviceID,
			Action:     action,
			Result:     result,
			Message:    message,
			Metadata:   metadata,
			OccurredAt: b.occurredAt,
		},
	}

	// 如果有插座号，添加到 payload
	if b.socketNo != nil {
		ev.NetworkTopology.SocketNo = b.socketNo
	}

	return ev
}

// BuildParamResult 构建参数结果事件
func (b *EventBuilder) BuildParamResult(
	result string,
	message string,
	metadata map[string]string,
) *coremodel.CoreEvent {
	return &coremodel.CoreEvent{
		Type:       coremodel.EventParamResult,
		DeviceID:   b.deviceID,
		OccurredAt: b.occurredAt,
		ParamResult: &coremodel.ParamResultPayload{
			DeviceID:   b.deviceID,
			Result:     result,
			Message:    message,
			Metadata:   metadata,
			OccurredAt: b.occurredAt,
		},
	}
}

// BuildParamSync 构建参数同步事件
func (b *EventBuilder) BuildParamSync(
	progress int32,
	result string,
	message string,
	metadata map[string]string,
) *coremodel.CoreEvent {
	return &coremodel.CoreEvent{
		Type:       coremodel.EventParamSync,
		DeviceID:   b.deviceID,
		OccurredAt: b.occurredAt,
		ParamSync: &coremodel.ParamSyncPayload{
			DeviceID:   b.deviceID,
			Progress:   progress,
			Result:     result,
			Message:    message,
			Metadata:   metadata,
			OccurredAt: b.occurredAt,
		},
	}
}

// BuildOTAProgress 构建OTA进度事件
func (b *EventBuilder) BuildOTAProgress(
	status string,
	progress int32,
	message string,
	metadata map[string]string,
) *coremodel.CoreEvent {
	var socketNo *coremodel.PortNo
	if b.socketNo != nil {
		sn := coremodel.PortNo(*b.socketNo)
		socketNo = &sn
	}

	return &coremodel.CoreEvent{
		Type:       coremodel.EventOTAProgress,
		DeviceID:   b.deviceID,
		PortNo:     nil,
		OccurredAt: b.occurredAt,
		OTAProgress: &coremodel.OTAProgressPayload{
			DeviceID:   b.deviceID,
			PortNo:     socketNo,
			Status:     status,
			Progress:   progress,
			Message:    message,
			Metadata:   metadata,
			OccurredAt: b.occurredAt,
		},
	}
}

// BuildException 构建异常事件
func (b *EventBuilder) BuildException(
	code string,
	message string,
	severity string,
	rawStatus *int32,
	metadata map[string]string,
) *coremodel.CoreEvent {
	return &coremodel.CoreEvent{
		Type:       coremodel.EventExceptionReported,
		DeviceID:   b.deviceID,
		PortNo:     b.portNo,
		OccurredAt: b.occurredAt,
		Exception: &coremodel.ExceptionPayload{
			DeviceID:   b.deviceID,
			PortNo:     b.portNo,
			Code:       code,
			Message:    message,
			Severity:   severity,
			RawStatus:  rawStatus,
			Metadata:   metadata,
			OccurredAt: b.occurredAt,
		},
	}
}
