package app

import (
	"context"
	"time"

	"github.com/taoyao-code/iot-server/internal/coremodel"
	"github.com/taoyao-code/iot-server/internal/storage"
	"go.uber.org/zap"
)

// DriverCore 实现 driverapi.EventSink，将协议驱动上报的规范化事件
// 映射到 CoreRepo / 一致性视图上。当前实现聚焦充电结束事件。
type DriverCore struct {
	core storage.CoreRepo
	log  *zap.Logger
}

func NewDriverCore(core storage.CoreRepo, log *zap.Logger) *DriverCore {
	return &DriverCore{
		core: core,
		log:  log,
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
	case coremodel.EventSessionEnded:
		return d.handleSessionEnded(ctx, ev)
	case coremodel.EventPortSnapshot:
		return d.handlePortSnapshot(ctx, ev)
	default:
		// 其他事件类型后续按需接入
		return nil
	}
}

func (d *DriverCore) handleSessionEnded(ctx context.Context, ev *coremodel.CoreEvent) error {
	payload := ev.SessionEnded
	if payload == nil {
		return nil
	}

	phyID := string(ev.DeviceID)
	if phyID == "" {
		return nil
	}

	device, err := d.core.EnsureDevice(ctx, phyID)
	if err != nil {
		if d.log != nil {
			d.log.Warn("driver core: ensure device failed on session ended",
				zap.String("device_phy_id", phyID),
				zap.Error(err),
			)
		}
		return err
	}

	deviceID := device.ID
	portNo := int(payload.PortNo)
	if portNo < 0 {
		portNo = 0
	}

	biz := string(payload.BusinessNo)
	durationSec := int(payload.DurationSec)
	kwh01 := int(payload.EnergyKWh01)

	reasonInt := 0
	if payload.RawReason != nil {
		reasonInt = int(*payload.RawReason)
	}

	if err := d.core.SettleOrder(ctx, deviceID, portNo, biz, durationSec, kwh01, reasonInt); err != nil {
		if d.log != nil {
			d.log.Error("driver core: settle order failed",
				zap.String("device_phy_id", phyID),
				zap.Int("port_no", portNo),
				zap.String("business_no", biz),
				zap.Error(err),
			)
		}
		return err
	}

	// 如果驱动提供了会话结束后的端口状态，则更新快照
	if payload.NextPortStatus != nil {
		if err := d.core.UpsertPortSnapshot(ctx, deviceID, int32(portNo), *payload.NextPortStatus, nil, time.Now()); err != nil {
			if d.log != nil {
				d.log.Error("driver core: upsert port snapshot failed after session ended",
					zap.String("device_phy_id", phyID),
					zap.Int("port_no", portNo),
					zap.Error(err),
				)
			}
			return err
		}
	}

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
