package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/taoyao-code/iot-server/internal/coremodel"
	"github.com/taoyao-code/iot-server/internal/storage"
	"github.com/taoyao-code/iot-server/internal/thirdparty"
	"go.uber.org/zap"
)

// DriverCore 实现 driverapi.EventSink，将协议驱动上报的规范化事件
// 映射到 CoreRepo / 一致性视图上。当前实现聚焦充电结束事件。
type DriverCore struct {
	core   storage.CoreRepo
	events *thirdparty.EventQueue
	log    *zap.Logger
}

const (
	orderStatusPending     = 0
	orderStatusConfirmed   = 1
	orderStatusCharging    = 2
	orderStatusCompleted   = 3
	orderStatusCancelled   = 5
	orderStatusFailed      = 6
	orderStatusStopping    = 9
	orderStatusInterrupted = 10
)

func NewDriverCore(core storage.CoreRepo, events *thirdparty.EventQueue, log *zap.Logger) *DriverCore {
	return &DriverCore{
		core:   core,
		events: events,
		log:    log,
	}
}

// HandleCoreEvent 处理驱动上报的规范化事件。
func (d *DriverCore) HandleCoreEvent(ctx context.Context, ev *coremodel.CoreEvent) error {
	if ev == nil || d.core == nil {
		return nil
	}

	switch ev.Type {
	case coremodel.EventDeviceHeartbeat:
		return d.handleDeviceHeartbeat(ctx, ev)
	case coremodel.EventSessionStarted:
		return d.handleSessionStarted(ctx, ev)
	case coremodel.EventSessionProgress:
		return d.handleSessionProgress(ctx, ev)
	case coremodel.EventSessionEnded:
		return d.handleSessionEnded(ctx, ev)
	case coremodel.EventPortSnapshot:
		return d.handlePortSnapshot(ctx, ev)
	case coremodel.EventExceptionReported:
		return d.handleException(ctx, ev)
	case coremodel.EventParamResult:
		return d.handleParamResult(ctx, ev)
	case coremodel.EventParamSync:
		return d.handleParamSync(ctx, ev)
	case coremodel.EventOTAProgress:
		return d.handleOTAProgress(ctx, ev)
	case coremodel.EventNetworkTopology:
		return d.handleNetworkTopology(ctx, ev)
	default:
		// 其他事件类型后续按需接入
		return nil
	}
}

func (d *DriverCore) handleSessionStarted(ctx context.Context, ev *coremodel.CoreEvent) error {
	payload := ev.SessionStarted
	if payload == nil || d.core == nil {
		return nil
	}

	phyID := pickDeviceID(ev.DeviceID, payload.DeviceID)
	if phyID == "" {
		return nil
	}

	orderNo, bizPtr := parseBusiness(ev.BusinessNo, payload.BusinessNo)
	portNo := int32(payload.PortNo)
	startedAt := defaultTime(payload.StartedAt)
	status := defaultChargingStatus(payload.Metadata)

	return d.core.WithTx(ctx, func(repo storage.CoreRepo) error {
		device, err := repo.EnsureDevice(ctx, phyID)
		if err != nil {
			if d.log != nil {
				d.log.Warn("driver core: ensure device failed on session started",
					zap.String("device_phy_id", phyID),
					zap.Error(err))
			}
			return err
		}

		if orderNo == "" && bizPtr != nil {
			orderNo = fmt.Sprintf("%04X", *bizPtr)
		}
		if orderNo == "" {
			orderNo = fmt.Sprintf("AUTO-START-%d-%d-%d", device.ID, portNo, startedAt.Unix())
		}

		if err := repo.UpsertOrderProgress(ctx, device.ID, portNo, orderNo, bizPtr, 0, 0, orderStatusCharging, payload.TargetPowerW); err != nil {
			if d.log != nil {
				d.log.Error("driver core: upsert order progress failed",
					zap.String("device_phy_id", phyID),
					zap.Int32("port_no", portNo),
					zap.String("order_no", orderNo),
					zap.Error(err))
			}
			return err
		}

		if err := repo.UpsertPortSnapshot(ctx, device.ID, portNo, status, payload.TargetPowerW, startedAt); err != nil {
			if d.log != nil {
				d.log.Error("driver core: upsert port snapshot failed on session start",
					zap.String("device_phy_id", phyID),
					zap.Int32("port_no", portNo),
					zap.Error(err))
			}
			return err
		}

		d.pushThirdpartyEvent(coremodel.EventSessionStarted, phyID, map[string]interface{}{
			"order_no": orderNo,
			"port_no":  portNo,
		})
		return nil
	})
}

func (d *DriverCore) handleSessionProgress(ctx context.Context, ev *coremodel.CoreEvent) error {
	payload := ev.SessionProgress
	if payload == nil || d.core == nil {
		return nil
	}

	phyID := pickDeviceID(ev.DeviceID, payload.DeviceID)
	if phyID == "" {
		return nil
	}

	orderNo, bizPtr := parseBusiness(ev.BusinessNo, payload.BusinessNo)
	portNo := int32(payload.PortNo)

	energy := int32(0)
	if payload.EnergyKWh01 != nil {
		energy = *payload.EnergyKWh01
	}
	duration := int32(0)
	if payload.DurationSec != nil {
		duration = *payload.DurationSec
	}
	occurredAt := defaultTime(payload.OccurredAt)
	status := defaultChargingStatus(nil)
	if payload.RawStatus != nil {
		status = *payload.RawStatus
	}

	return d.core.WithTx(ctx, func(repo storage.CoreRepo) error {
		device, err := repo.EnsureDevice(ctx, phyID)
		if err != nil {
			if d.log != nil {
				d.log.Warn("driver core: ensure device failed on session progress",
					zap.String("device_phy_id", phyID),
					zap.Error(err))
			}
			return err
		}

		if orderNo == "" && bizPtr != nil {
			orderNo = fmt.Sprintf("%04X", *bizPtr)
		}
		if orderNo == "" {
			orderNo = fmt.Sprintf("AUTO-PROGRESS-%d-%d", device.ID, portNo)
		}

		if err := repo.UpsertOrderProgress(ctx, device.ID, portNo, orderNo, bizPtr, duration, energy, orderStatusCharging, payload.PowerW); err != nil {
			if d.log != nil {
				d.log.Error("driver core: upsert order progress failed on progress",
					zap.String("device_phy_id", phyID),
					zap.Int32("port_no", portNo),
					zap.String("order_no", orderNo),
					zap.Error(err))
			}
			return err
		}

		if err := repo.UpsertPortSnapshot(ctx, device.ID, portNo, status, payload.PowerW, occurredAt); err != nil {
			if d.log != nil {
				d.log.Error("driver core: upsert port snapshot failed on progress",
					zap.String("device_phy_id", phyID),
					zap.Int32("port_no", portNo),
					zap.Error(err))
			}
			return err
		}

		d.pushThirdpartyEvent(coremodel.EventSessionProgress, phyID, map[string]interface{}{
			"order_no": orderNo,
			"port_no":  portNo,
			"energy":   energy,
			"duration": duration,
		})
		return nil
	})
}

func (d *DriverCore) handleSessionEnded(ctx context.Context, ev *coremodel.CoreEvent) error {
	payload := ev.SessionEnded
	if payload == nil || d.core == nil {
		return nil
	}

	phyID := pickDeviceID(ev.DeviceID, payload.DeviceID)
	if phyID == "" {
		return nil
	}

	orderNo, bizPtr := parseBusiness(ev.BusinessNo, payload.BusinessNo)
	portNo := int32(payload.PortNo)
	occurredAt := defaultTime(payload.OccurredAt)
	rawReason := 0
	if payload.RawReason != nil {
		rawReason = int(*payload.RawReason)
	}

	return d.core.WithTx(ctx, func(repo storage.CoreRepo) error {
		device, err := repo.EnsureDevice(ctx, phyID)
		if err != nil {
			if d.log != nil {
				d.log.Warn("driver core: ensure device failed on session ended",
					zap.String("device_phy_id", phyID),
					zap.Error(err))
			}
			return err
		}

		if orderNo == "" && bizPtr != nil {
			orderNo = fmt.Sprintf("%04X", *bizPtr)
		}
		if orderNo == "" {
			orderNo = fmt.Sprintf("AUTO-END-%d-%d", device.ID, portNo)
		}

		if err := repo.SettleOrder(ctx, device.ID, int(portNo), orderNo, int(payload.DurationSec), int(payload.EnergyKWh01), rawReason); err != nil {
			if d.log != nil {
				d.log.Error("driver core: settle order failed",
					zap.String("device_phy_id", phyID),
					zap.Int32("port_no", portNo),
					zap.String("order_no", orderNo),
					zap.Error(err))
			}
			return err
		}

		status := payload.NextPortStatus
		if status == nil {
			if payload.RawStatus != nil {
				status = payload.RawStatus
			} else {
				idle := defaultIdleStatus(nil)
				status = &idle
			}
		}

		if status != nil {
			var power *int32
			if payload.InstantPowerW != nil {
				p := *payload.InstantPowerW
				power = &p
			}
			if err := repo.UpsertPortSnapshot(ctx, device.ID, portNo, *status, power, occurredAt); err != nil {
				if d.log != nil {
					d.log.Error("driver core: upsert port snapshot failed after session ended",
						zap.String("device_phy_id", phyID),
						zap.Int32("port_no", portNo),
						zap.Error(err))
				}
				return err
			}
		}

		d.pushThirdpartyEvent(coremodel.EventSessionEnded, phyID, map[string]interface{}{
			"order_no": orderNo,
			"port_no":  portNo,
			"energy":   payload.EnergyKWh01,
			"duration": payload.DurationSec,
			"reason":   rawReason,
		})
		return nil
	})
}

func (d *DriverCore) handleException(ctx context.Context, ev *coremodel.CoreEvent) error {
	payload := ev.Exception
	if payload == nil || d.core == nil {
		return nil
	}

	phyID := pickDeviceID(ev.DeviceID, payload.DeviceID)
	if phyID == "" {
		return nil
	}

	portNo := int32(0)
	if payload.PortNo != nil {
		portNo = int32(*payload.PortNo)
	}

	return d.core.WithTx(ctx, func(repo storage.CoreRepo) error {
		device, err := repo.EnsureDevice(ctx, phyID)
		if err != nil {
			if d.log != nil {
				d.log.Warn("driver core: ensure device failed on exception",
					zap.String("device_phy_id", phyID),
					zap.Error(err))
			}
			return err
		}

		if payload.RawStatus != nil {
			_ = repo.UpsertPortSnapshot(ctx, device.ID, portNo, *payload.RawStatus, nil, defaultTime(payload.OccurredAt))
		}

		d.pushThirdpartyEvent(coremodel.EventExceptionReported, phyID, map[string]interface{}{
			"code":     payload.Code,
			"message":  payload.Message,
			"severity": payload.Severity,
			"port_no":  portNo,
		})
		if d.log != nil {
			d.log.Warn("driver core: protocol exception reported",
				zap.String("device_phy_id", phyID),
				zap.Int32("port_no", portNo),
				zap.String("code", payload.Code),
				zap.String("message", payload.Message),
				zap.String("severity", payload.Severity))
		}
		return nil
	})
}

func (d *DriverCore) handleParamResult(ctx context.Context, ev *coremodel.CoreEvent) error {
	payload := ev.ParamResult
	if payload == nil {
		return nil
	}

	phyID := pickDeviceID(ev.DeviceID, payload.DeviceID)
	if phyID == "" {
		return nil
	}

	if d.core != nil {
		_, _ = d.core.EnsureDevice(ctx, phyID)
	}

	data := map[string]interface{}{
		"result":   payload.Result,
		"message":  payload.Message,
		"metadata": payload.Metadata,
	}
	if payload.PortNo != nil {
		data["port_no"] = *payload.PortNo
	}

	d.pushThirdpartyEvent(coremodel.EventParamResult, phyID, data)
	return nil
}

func (d *DriverCore) handleParamSync(ctx context.Context, ev *coremodel.CoreEvent) error {
	payload := ev.ParamSync
	if payload == nil {
		return nil
	}

	phyID := pickDeviceID(ev.DeviceID, payload.DeviceID)
	if phyID == "" {
		return nil
	}

	if d.core != nil {
		_, _ = d.core.EnsureDevice(ctx, phyID)
	}

	data := map[string]interface{}{
		"progress": payload.Progress,
		"result":   payload.Result,
		"message":  payload.Message,
		"metadata": payload.Metadata,
	}

	d.pushThirdpartyEvent(coremodel.EventParamSync, phyID, data)
	return nil
}

func (d *DriverCore) handleOTAProgress(ctx context.Context, ev *coremodel.CoreEvent) error {
	payload := ev.OTAProgress
	if payload == nil {
		return nil
	}

	phyID := pickDeviceID(ev.DeviceID, payload.DeviceID)
	if phyID == "" {
		return nil
	}

	if d.core != nil {
		_, _ = d.core.EnsureDevice(ctx, phyID)
	}

	data := map[string]interface{}{
		"status":   payload.Status,
		"progress": payload.Progress,
		"message":  payload.Message,
		"metadata": payload.Metadata,
	}
	if payload.PortNo != nil {
		data["port_no"] = *payload.PortNo
	}

	d.pushThirdpartyEvent(coremodel.EventOTAProgress, phyID, data)
	return nil
}

func (d *DriverCore) handleNetworkTopology(ctx context.Context, ev *coremodel.CoreEvent) error {
	payload := ev.NetworkTopology
	if payload == nil {
		return nil
	}

	phyID := pickDeviceID(ev.DeviceID, payload.DeviceID)
	if phyID == "" {
		return nil
	}

	if d.core != nil {
		_, _ = d.core.EnsureDevice(ctx, phyID)
	}

	data := map[string]interface{}{
		"action":   payload.Action,
		"result":   payload.Result,
		"message":  payload.Message,
		"metadata": payload.Metadata,
	}
	if payload.SocketNo != nil {
		data["socket_no"] = *payload.SocketNo
	}

	d.pushThirdpartyEvent(coremodel.EventNetworkTopology, phyID, data)
	return nil
}

func (d *DriverCore) handleDeviceHeartbeat(ctx context.Context, ev *coremodel.CoreEvent) error {
	if d.core == nil {
		return nil
	}

	payload := ev.DeviceHeartbeat
	deviceID := ev.DeviceID
	if payload != nil && payload.DeviceID != "" {
		deviceID = payload.DeviceID
	}

	phyID := string(deviceID)
	if phyID == "" {
		return nil
	}

	if _, err := d.core.EnsureDevice(ctx, phyID); err != nil {
		if d.log != nil {
			d.log.Warn("driver core: ensure device failed on heartbeat",
				zap.String("device_phy_id", phyID),
				zap.Error(err),
			)
		}
		return err
	}

	lastSeen := ev.OccurredAt
	if payload != nil && !payload.LastSeenAt.IsZero() {
		lastSeen = payload.LastSeenAt
	}
	if lastSeen.IsZero() {
		lastSeen = time.Now()
	}

	if err := d.core.TouchDeviceLastSeen(ctx, phyID, lastSeen); err != nil {
		if d.log != nil {
			d.log.Warn("driver core: touch device last seen failed",
				zap.String("device_phy_id", phyID),
				zap.Error(err),
			)
		}
		return err
	}

	return nil
}

func pickDeviceID(eventID coremodel.DeviceID, payloadID coremodel.DeviceID) string {
	if payloadID != "" {
		return string(payloadID)
	}
	if eventID != "" {
		return string(eventID)
	}
	return ""
}

func parseBusiness(eventBiz *coremodel.BusinessNo, payloadBiz coremodel.BusinessNo) (string, *int32) {
	raw := strings.TrimSpace(string(payloadBiz))
	if raw == "" && eventBiz != nil {
		raw = strings.TrimSpace(string(*eventBiz))
	}
	if raw == "" {
		return "", nil
	}

	base := 10
	if strings.HasPrefix(raw, "0x") || strings.HasPrefix(raw, "0X") {
		base = 16
		raw = raw[2:]
	}

	val, err := strconv.ParseInt(raw, base, 32)
	if err != nil && base != 16 {
		val, err = strconv.ParseInt(raw, 16, 32)
	}
	if err != nil {
		return strings.ToUpper(raw), nil
	}
	if val == 0 {
		val = 1
	}
	biz := int32(val)
	return strings.ToUpper(fmt.Sprintf("%04X", val)), &biz
}

func defaultTime(ts time.Time) time.Time {
	if ts.IsZero() {
		return time.Now()
	}
	return ts
}

func defaultChargingStatus(meta map[string]string) int32 {
	if meta != nil {
		if raw, ok := meta["raw_status"]; ok {
			if v, err := strconv.ParseInt(raw, 0, 32); err == nil {
				return int32(v)
			}
		}
	}
	return 0xA0 // 充电中: bit7(在线)+bit5(充电)
}

func defaultIdleStatus(meta map[string]string) int32 {
	if meta != nil {
		if raw, ok := meta["raw_status_idle"]; ok {
			if v, err := strconv.ParseInt(raw, 0, 32); err == nil {
				return int32(v)
			}
		}
	}
	return 0x90 // 空闲: bit7(在线)+bit4(空载)
}

func (d *DriverCore) pushThirdpartyEvent(eventType coremodel.CoreEventType, phyID string, data map[string]interface{}) {
	if d.events == nil || phyID == "" {
		return
	}

	var tpType thirdparty.EventType
	switch eventType {
	case coremodel.EventSessionStarted:
		tpType = thirdparty.EventChargingStarted
	case coremodel.EventSessionProgress:
		tpType = thirdparty.EventChargingProgress
	case coremodel.EventSessionEnded:
		tpType = thirdparty.EventChargingEnded
	case coremodel.EventExceptionReported:
		tpType = thirdparty.EventDeviceAlarm
	case coremodel.EventParamResult:
		tpType = thirdparty.EventParamResult
	case coremodel.EventParamSync:
		tpType = thirdparty.EventParamSync
	case coremodel.EventOTAProgress:
		tpType = thirdparty.EventOTAProgressUpdate
	case coremodel.EventNetworkTopology:
		tpType = thirdparty.EventNetworkTopology
	default:
		return
	}

	ev := thirdparty.NewEvent(tpType, phyID, data)
	_ = d.events.Enqueue(context.Background(), ev)
}

func (d *DriverCore) handlePortSnapshot(ctx context.Context, ev *coremodel.CoreEvent) error {
	ps := ev.PortSnapshot
	if ps == nil {
		return nil
	}

	phyID := string(ps.DeviceID)
	if phyID == "" {
		return nil
	}

	device, err := d.core.EnsureDevice(ctx, phyID)
	if err != nil {
		if d.log != nil {
			d.log.Warn("driver core: ensure device failed on port snapshot",
				zap.String("device_phy_id", phyID),
				zap.Error(err),
			)
		}
		return err
	}

	deviceID := device.ID
	portNo := int32(ps.PortNo)
	status := ps.RawStatus
	var power *int32
	if ps.PowerW != nil {
		p := *ps.PowerW
		power = &p
	}

	at := ps.At
	if at.IsZero() {
		at = time.Now()
	}

	if err := d.core.UpsertPortSnapshot(ctx, deviceID, portNo, status, power, at); err != nil {
		if d.log != nil {
			d.log.Error("driver core: upsert port snapshot failed",
				zap.String("device_phy_id", phyID),
				zap.Int32("port_no", portNo),
				zap.Int32("status", status),
				zap.Error(err),
			)
		}
		return err
	}

	return nil
}
