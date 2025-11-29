package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/taoyao-code/iot-server/internal/coremodel"
	"github.com/taoyao-code/iot-server/internal/storage"
	"github.com/taoyao-code/iot-server/internal/thirdparty"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// DriverCore 实现 driverapi.EventSink，将协议驱动上报的规范化事件
// 映射到 CoreRepo / 一致性视图上。当前实现聚焦充电结束事件。
type DriverCore struct {
	core   storage.CoreRepo
	events *thirdparty.EventQueue
	log    *zap.Logger

	// sessions 跟踪活跃充电会话，用于防止无效状态升级
	// key: "devicePhyID:portNo" -> value: sessionStartTime (time.Time)
	// 当新状态要升级为"充电中"时，检查是否有活跃会话
	sessions sync.Map
}

func NewDriverCore(core storage.CoreRepo, events *thirdparty.EventQueue, log *zap.Logger) *DriverCore {
	return &DriverCore{
		core:   core,
		events: events,
		log:    log,
	}
}

// sessionKey 生成会话跟踪键
func sessionKey(phyID string, portNo int32) string {
	return fmt.Sprintf("%s:%d", phyID, portNo)
}

// TrackSession 记录活跃会话（公开方法，供 API 层调用）
// 在发送充电命令后立即调用，确保状态升级验证通过
func (d *DriverCore) TrackSession(phyID string, portNo int32) {
	d.trackSession(phyID, portNo)
	if d.log != nil {
		d.log.Info("driver core: session tracked",
			zap.String("device_phy_id", phyID),
			zap.Int32("port_no", portNo))
	}
}

// trackSession 记录活跃会话（内部方法）
func (d *DriverCore) trackSession(phyID string, portNo int32) {
	d.sessions.Store(sessionKey(phyID, portNo), time.Now())
}

// ClearSession 清除会话记录（公开方法，供 API 层调用）
// 在停止充电或充电结束后调用
func (d *DriverCore) ClearSession(phyID string, portNo int32) {
	d.clearSession(phyID, portNo)
	if d.log != nil {
		d.log.Info("driver core: session cleared",
			zap.String("device_phy_id", phyID),
			zap.Int32("port_no", portNo))
	}
}

// clearSession 清除会话记录（内部方法）
func (d *DriverCore) clearSession(phyID string, portNo int32) {
	d.sessions.Delete(sessionKey(phyID, portNo))
}

// hasActiveSession 检查是否有活跃会话
// 会话有效期为 chargingSessionTimeout，超时后视为无效
func (d *DriverCore) hasActiveSession(phyID string, portNo int32) bool {
	val, ok := d.sessions.Load(sessionKey(phyID, portNo))
	if !ok {
		return false
	}
	startTime, ok := val.(time.Time)
	if !ok {
		return false
	}
	// 会话超过24小时视为过期（防止内存泄漏）
	return time.Since(startTime) < 24*time.Hour
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

// handleSessionStarted 处理充电会话开始事件
func (d *DriverCore) handleSessionStarted(ctx context.Context, ev *coremodel.CoreEvent) error {
	payload := ev.SessionStarted
	if payload == nil || d.core == nil {
		return nil
	}

	status := int32(coremodel.NormalizePortStatus(defaultChargingStatus(payload.Metadata)))

	// 记录活跃会话，用于后续状态升级验证
	phyID := pickDeviceID(ev.DeviceID, payload.DeviceID)
	if phyID != "" {
		d.trackSession(phyID, int32(payload.PortNo))
	}

	return d.handleSessionState(ctx, sessionContext{
		Event:         ev,
		PayloadDevice: payload.DeviceID,
		PortNo:        int32(payload.PortNo),
		Business:      payload.BusinessNo,
		FallbackBiz:   ev.BusinessNo,
		Status:        status,
		Power:         payload.TargetPowerW,
		Timestamp:     payload.StartedAt,
		ThirdpartyEvt: coremodel.EventSessionStarted,
		LogMessage:    "driver core: session started",
		LogLevel:      zapcore.InfoLevel,
	})
}

func (d *DriverCore) handleSessionProgress(ctx context.Context, ev *coremodel.CoreEvent) error {
	payload := ev.SessionProgress
	if payload == nil || d.core == nil {
		return nil
	}

	energy := int32(0)
	if payload.EnergyKWh01 != nil {
		energy = *payload.EnergyKWh01
	}
	duration := int32(0)
	if payload.DurationSec != nil {
		duration = *payload.DurationSec
	}
	status := defaultChargingStatus(nil)
	if payload.RawStatus != nil {
		status = *payload.RawStatus
	}
	status = int32(coremodel.NormalizePortStatus(status))

	return d.handleSessionState(ctx, sessionContext{
		Event:         ev,
		PayloadDevice: payload.DeviceID,
		PortNo:        int32(payload.PortNo),
		Business:      payload.BusinessNo,
		FallbackBiz:   ev.BusinessNo,
		Status:        status,
		Power:         payload.PowerW,
		Timestamp:     payload.OccurredAt,
		ThirdpartyEvt: coremodel.EventSessionProgress,
		ExtraPayload: map[string]interface{}{
			"energy":   energy,
			"duration": duration,
		},
		LogMessage: "driver core: session progress",
		LogLevel:   zapcore.DebugLevel,
		LogFields: []zap.Field{
			zap.Int32("energy_kwh01", energy),
			zap.Int32("duration_sec", duration),
		},
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

	_, bizPtr := parseBusiness(ev.BusinessNo, payload.BusinessNo)
	portNo := int32(payload.PortNo)
	occurredAt := defaultTime(payload.OccurredAt)
	rawReason := 0
	if payload.RawReason != nil {
		rawReason = int(*payload.RawReason)
	}

	// 清除活跃会话记录，防止后续状态上报错误升级
	d.clearSession(phyID, portNo)

	// 确定端口终态（统一转换为 API 状态码）
	var (
		terminalStatus int32
		hasStatus      bool
	)
	if payload.NextPortStatus != nil {
		terminalStatus = int32(coremodel.NormalizePortStatus(*payload.NextPortStatus))
		hasStatus = true
	} else if payload.RawStatus != nil {
		terminalStatus = int32(coremodel.NormalizePortStatus(*payload.RawStatus))
		hasStatus = true
	} else {
		terminalStatus = int32(coremodel.NormalizePortStatus(defaultIdleStatus(nil)))
		hasStatus = true
	}

	var power *int32
	if payload.InstantPowerW != nil {
		p := *payload.InstantPowerW
		power = &p
	}

	// 获取设备信息
	device, err := d.core.EnsureDevice(ctx, phyID)
	if err != nil {
		if d.log != nil {
			d.log.Warn("driver core: ensure device failed on session ended",
				zap.String("device_phy_id", phyID),
				zap.Error(err))
		}
		return err
	}

	// 生成业务号标识（用于事件推送）
	businessNo := ""
	if bizPtr != nil {
		businessNo = fmt.Sprintf("%04X", *bizPtr)
	}

	// 持久化端口终态：确保端口状态收敛
	if hasStatus {
		if err := d.core.UpsertPortSnapshot(ctx, device.ID, portNo, terminalStatus, power, occurredAt); err != nil {
			if d.log != nil {
				d.log.Error("driver core: upsert port snapshot failed on session ended",
					zap.String("device_phy_id", phyID),
					zap.Int32("port_no", portNo),
					zap.Error(err))
			}
			return err
		}
	}

	// 推送第三方事件（供业务系统消费）
	d.pushThirdpartyEvent(coremodel.EventSessionEnded, phyID, map[string]interface{}{
		"business_no": businessNo,
		"port_no":     portNo,
		"energy":      payload.EnergyKWh01,
		"duration":    payload.DurationSec,
		"reason":      rawReason,
	})

	if d.log != nil {
		d.log.Info("driver core: session ended",
			zap.String("device_phy_id", phyID),
			zap.Int32("port_no", portNo),
			zap.String("business_no", businessNo),
			zap.Int32("energy_kwh01", payload.EnergyKWh01),
			zap.Int32("duration_sec", payload.DurationSec),
			zap.Int("reason", rawReason))
	}

	return nil
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
			status := int32(coremodel.NormalizePortStatus(*payload.RawStatus))
			_ = repo.UpsertPortSnapshot(ctx, device.ID, portNo, status, nil, defaultTime(payload.OccurredAt))
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
	} else if isBKVHexFormat(raw) {
		// BKV协议层使用fmt.Sprintf("%04X", businessNo)格式化业务号
		// 识别4位十六进制字符串（如"0041"）并强制按十六进制解析
		// 避免"0041"被错误解析为十进制41，导致输出"0029"
		base = 16
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

// isBKVHexFormat 检测字符串是否为BKV协议的4位十六进制格式（无0x前缀）
// 判断逻辑：必须是4位字符串，且满足以下任一条件：
// 1. 包含A-F字母（如"00A3", "FFFF"）
// 2. 以'0'开头且非"0000"（有前导零，如"0041"表示65而非41）
//
// 这样避免将纯数字"1234"误判为十六进制0x1234=4660
// 例如：
//
//	"0041" → true (前导零) → 解析为65
//	"00A3" → true (包含A) → 解析为163
//	"1234" → false (无前导零,无A-F) → 保持十进制解析��1234
//	"FFFF" → true (包含F) → 解析为65535
func isBKVHexFormat(s string) bool {
	if len(s) != 4 {
		return false
	}

	hasHexLetter := false
	hasLeadingZero := s[0] == '0' && s != "0000"

	for _, c := range s {
		if (c >= 'A' && c <= 'F') || (c >= 'a' && c <= 'f') {
			hasHexLetter = true
		} else if !(c >= '0' && c <= '9') {
			return false // 包含非法字符
		}
	}

	return hasHexLetter || hasLeadingZero
}

func defaultTime(ts time.Time) time.Time {
	if ts.IsZero() {
		return time.Now()
	}
	return ts
}

// 默认充电状态为 1（空闲）
func defaultIdleStatus(meta map[string]string) int32 {
	return 1 // 空闲
}

// 默认充电状态为 2（充电中）
func defaultChargingStatus(meta map[string]string) int32 {
	return 2 // 充电中
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

// chargingEstablishmentPeriod 充电建立期时长
// 在此期间内，不允许将端口状态从"充电中"降级为"空闲"
// 这是为了防止充电命令 ACK 后的状态上报立即覆盖充电状态
const chargingEstablishmentPeriod = 30 * time.Second

// handlePortSnapshot 处理端口状态快照事件
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
	newStatus := int32(coremodel.NormalizePortStatus(ps.RawStatus))
	var power *int32
	if ps.PowerW != nil {
		p := *ps.PowerW
		power = &p
	}

	at := ps.At
	if at.IsZero() {
		at = time.Now()
	}

	// 充电建立期保护：防止刚下发充电命令后的充电状态被状态上报立即覆盖
	// 场景：充电命令 ACK 后，设备立即上报空载状态（可能是检测延迟），导致充电状态丢失
	if newStatus == int32(coremodel.StatusCodeIdle) {
		existingPort, getErr := d.core.GetPort(ctx, deviceID, portNo)
		if getErr == nil && existingPort != nil {
			// 当前状态是"充电中"，新状态要降级为"空闲"
			if existingPort.Status == int32(coremodel.StatusCodeCharging) {
				timeSinceUpdate := time.Since(existingPort.UpdatedAt)
				// 在充电建立期内，忽略空闲状态上报
				if timeSinceUpdate < chargingEstablishmentPeriod {
					if d.log != nil {
						d.log.Warn("driver core: port snapshot ignored during charging establishment period",
							zap.String("device_phy_id", phyID),
							zap.Int32("port_no", portNo),
							zap.Int32("current_status", existingPort.Status),
							zap.Int32("new_status", newStatus),
							zap.Duration("time_since_update", timeSinceUpdate),
							zap.Duration("establishment_period", chargingEstablishmentPeriod),
						)
					}
					return nil // 忽略这次状态更新
				}
			}
		}
	}

	// 无效升级保护：防止无活跃会话时将状态错误升级为"充电中"
	// 场景：设备断连重连后上报旧的"充电中"状态，但实际充电已结束
	if newStatus == int32(coremodel.StatusCodeCharging) {
		existingPort, getErr := d.core.GetPort(ctx, deviceID, portNo)
		if getErr == nil && existingPort != nil {
			// 当前状态不是"充电中"，但新状态要升级为"充电中"
			if existingPort.Status != int32(coremodel.StatusCodeCharging) {
				// 检查是否有活跃会话
				if !d.hasActiveSession(phyID, portNo) {
					if d.log != nil {
						d.log.Warn("driver core: port snapshot ignored: no active session for charging upgrade",
							zap.String("device_phy_id", phyID),
							zap.Int32("port_no", portNo),
							zap.Int32("current_status", existingPort.Status),
							zap.Int32("new_status", newStatus),
						)
					}
					return nil // 拒绝无效的充电状态升级
				}
			}
		}
	}

	if err := d.core.UpsertPortSnapshot(ctx, deviceID, portNo, newStatus, power, at); err != nil {
		if d.log != nil {
			d.log.Error("driver core: upsert port snapshot failed",
				zap.String("device_phy_id", phyID),
				zap.Int32("port_no", portNo),
				zap.Int32("status", newStatus),
				zap.Error(err),
			)
		}
		return err
	}

	return nil
}

type sessionContext struct {
	Event         *coremodel.CoreEvent
	PayloadDevice coremodel.DeviceID
	PortNo        int32
	Business      coremodel.BusinessNo
	FallbackBiz   *coremodel.BusinessNo
	Status        int32
	Power         *int32
	Timestamp     time.Time
	ThirdpartyEvt coremodel.CoreEventType
	ExtraPayload  map[string]interface{}
	LogMessage    string
	LogLevel      zapcore.Level
	LogFields     []zap.Field
}

func (d *DriverCore) handleSessionState(ctx context.Context, sc sessionContext) error {
	if d.core == nil || sc.Event == nil {
		return nil
	}

	phyID := pickDeviceID(sc.Event.DeviceID, sc.PayloadDevice)
	if phyID == "" {
		return nil
	}

	device, err := d.core.EnsureDevice(ctx, phyID)
	if err != nil {
		if d.log != nil {
			d.log.Warn("driver core: ensure device failed",
				zap.String("device_phy_id", phyID),
				zap.Error(err))
		}
		return err
	}

	_, bizPtr := parseBusiness(sc.FallbackBiz, sc.Business)
	businessNo := ""
	if bizPtr != nil {
		businessNo = fmt.Sprintf("%04X", *bizPtr)
	}

	status := int32(coremodel.NormalizePortStatus(sc.Status))
	if err := d.core.UpsertPortSnapshot(ctx, device.ID, sc.PortNo, status, sc.Power, defaultTime(sc.Timestamp)); err != nil {
		if d.log != nil {
			d.log.Error("driver core: upsert port snapshot failed",
				zap.String("device_phy_id", phyID),
				zap.Int32("port_no", sc.PortNo),
				zap.Error(err))
		}
		return err
	}

	payload := map[string]interface{}{
		"business_no": businessNo,
		"port_no":     sc.PortNo,
	}
	for k, v := range sc.ExtraPayload {
		payload[k] = v
	}
	d.pushThirdpartyEvent(sc.ThirdpartyEvt, phyID, payload)

	if d.log != nil && sc.LogMessage != "" {
		fields := []zap.Field{
			zap.String("device_phy_id", phyID),
			zap.Int32("port_no", sc.PortNo),
			zap.String("business_no", businessNo),
		}
		if len(sc.LogFields) > 0 {
			fields = append(fields, sc.LogFields...)
		}
		switch sc.LogLevel {
		case zapcore.DebugLevel:
			d.log.Debug(sc.LogMessage, fields...)
		default:
			d.log.Info(sc.LogMessage, fields...)
		}
	}

	return nil
}
