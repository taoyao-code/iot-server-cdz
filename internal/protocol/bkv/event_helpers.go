package bkv

import (
	"context"
	"fmt"
	"time"

	"github.com/taoyao-code/iot-server/internal/thirdparty"
	"go.uber.org/zap"
)

// pushEvent 推送事件到第三方（内部辅助函数）
// 如果EventQueue或Deduper未配置，静默跳过（不影响业务）
func (h *Handlers) pushEvent(ctx context.Context, event *thirdparty.StandardEvent, logger *zap.Logger) {
	// 检查事件队列是否已配置
	if h == nil || h.EventQueue == nil {
		return // 未配置队列，跳过推送
	}

	// 去重检查（如果配置了去重器）
	if h.Deduper != nil {
		isDup, err := h.Deduper.IsDuplicate(ctx, event.EventID)
		if err != nil {
			if logger != nil {
				logger.Warn("dedup check failed", zap.Error(err), zap.String("event_id", event.EventID))
			}
			// 去重检查失败，继续推送（保守策略）
		} else if isDup {
			// 重复事件，跳过推送
			thirdparty.RecordDedupHit(event.EventType)
			if logger != nil {
				logger.Debug("duplicate event skipped", zap.String("event_id", event.EventID))
			}
			return
		}
	}

	// 入队推送
	err := h.EventQueue.Enqueue(ctx, event)
	if err != nil {
		if logger != nil {
			logger.Error("failed to enqueue event",
				zap.Error(err),
				zap.String("event_id", event.EventID),
				zap.String("event_type", string(event.EventType)))
		}
		thirdparty.RecordEnqueueFailure(event.EventType)
	} else {
		thirdparty.RecordEnqueueSuccess(event.EventType)
		if logger != nil {
			logger.Info("event enqueued successfully",
				zap.String("event_id", event.EventID),
				zap.String("event_type", string(event.EventType)))
		}
	}
}

// pushOrderCreatedEvent 推送订单创建事件
func (h *Handlers) pushOrderCreatedEvent(ctx context.Context, devicePhyID, orderNo string, portNo int, chargeMode string, duration int, pricePerKwh float64, logger *zap.Logger) {
	eventData := &thirdparty.OrderCreatedData{
		OrderNo:     orderNo,
		PortNo:      portNo,
		ChargeMode:  chargeMode,
		Duration:    duration,
		PricePerKwh: pricePerKwh,
		CreatedAt:   time.Now().Unix(),
	}

	event := thirdparty.NewEvent(
		thirdparty.EventOrderCreated,
		devicePhyID,
		eventData.ToMap(),
	)

	h.pushEvent(ctx, event, logger)
}

// pushOrderConfirmedEvent 推送订单确认事件
func (h *Handlers) pushOrderConfirmedEvent(ctx context.Context, devicePhyID, orderNo string, portNo int, result, failReason string, logger *zap.Logger) {
	eventData := &thirdparty.OrderConfirmedData{
		OrderNo:     orderNo,
		PortNo:      portNo,
		Result:      result,
		FailReason:  failReason,
		ConfirmedAt: time.Now().Unix(),
	}

	event := thirdparty.NewEvent(
		thirdparty.EventOrderConfirmed,
		devicePhyID,
		eventData.ToMap(),
	)

	h.pushEvent(ctx, event, logger)
}

// pushOrderCompletedEvent 推送订单完成事件
func (h *Handlers) pushOrderCompletedEvent(ctx context.Context, devicePhyID, orderNo string, portNo int, duration int, totalKwh, peakPower, avgPower, totalAmount float64, endReason, endReasonMsg string, logger *zap.Logger) {
	eventData := &thirdparty.OrderCompletedData{
		OrderNo:      orderNo,
		PortNo:       portNo,
		Duration:     duration,
		TotalKwh:     totalKwh,
		PeakPower:    peakPower,
		AvgPower:     avgPower,
		TotalAmount:  totalAmount,
		EndReason:    endReason,
		EndReasonMsg: endReasonMsg,
		CompletedAt:  time.Now().Unix(),
	}

	event := thirdparty.NewEvent(
		thirdparty.EventOrderCompleted,
		devicePhyID,
		eventData.ToMap(),
	)

	h.pushEvent(ctx, event, logger)
}

// pushChargingStartedEvent 推送充电开始事件
func (h *Handlers) pushChargingStartedEvent(ctx context.Context, devicePhyID, orderNo string, portNo int, logger *zap.Logger) {
	eventData := &thirdparty.ChargingStartedData{
		OrderNo:   orderNo,
		PortNo:    portNo,
		StartedAt: time.Now().Unix(),
	}

	event := thirdparty.NewEvent(
		thirdparty.EventChargingStarted,
		devicePhyID,
		eventData.ToMap(),
	)

	h.pushEvent(ctx, event, logger)
}

// pushChargingEndedEvent 推送充电结束事件
func (h *Handlers) pushChargingEndedEvent(ctx context.Context, devicePhyID, orderNo string, portNo int, duration int, totalKwh float64, endReason, endReasonMsg string, logger *zap.Logger) {
	eventData := &thirdparty.ChargingEndedData{
		OrderNo:      orderNo,
		PortNo:       portNo,
		Duration:     duration,
		TotalKwh:     totalKwh,
		EndReason:    endReason,
		EndReasonMsg: endReasonMsg,
		EndedAt:      time.Now().Unix(),
	}

	event := thirdparty.NewEvent(
		thirdparty.EventChargingEnded,
		devicePhyID,
		eventData.ToMap(),
	)

	h.pushEvent(ctx, event, logger)
}

// pushDeviceHeartbeatEvent 推送设备心跳事件
func (h *Handlers) pushDeviceHeartbeatEvent(ctx context.Context, devicePhyID string, voltage float64, rssi int, temp float64, ports []thirdparty.PortStatus, logger *zap.Logger) {
	eventData := &thirdparty.DeviceHeartbeatData{
		Voltage: voltage,
		RSSI:    rssi,
		Temp:    temp,
		Ports:   ports,
	}

	event := thirdparty.NewEvent(
		thirdparty.EventDeviceHeartbeat,
		devicePhyID,
		eventData.ToMap(),
	)

	h.pushEvent(ctx, event, logger)
}

// pushOTAProgressEvent 推送OTA进度事件
func (h *Handlers) pushOTAProgressEvent(ctx context.Context, devicePhyID string, taskID int64, version string, progress int, status, statusMsg, errorMsg string, logger *zap.Logger) {
	eventData := &thirdparty.OTAProgressUpdateData{
		TaskID:    fmt.Sprintf("%d", taskID),
		Version:   version,
		Progress:  progress,
		Status:    status,
		StatusMsg: statusMsg,
		ErrorMsg:  errorMsg,
		UpdatedAt: time.Now().Unix(),
	}

	event := thirdparty.NewEvent(
		thirdparty.EventOTAProgressUpdate,
		devicePhyID,
		eventData.ToMap(),
	)

	h.pushEvent(ctx, event, logger)
}

// v2.1.3: 补全的事件推送辅助函数

// pushDeviceRegisteredEvent 推送设备注册事件
func (h *Handlers) pushDeviceRegisteredEvent(ctx context.Context, devicePhyID, iccid, imei, deviceType, firmware string, portCount int, logger *zap.Logger) {
	eventData := &thirdparty.DeviceRegisteredData{
		ICCID:        iccid,
		IMEI:         imei,
		DeviceType:   deviceType,
		Firmware:     firmware,
		PortCount:    portCount,
		RegisteredAt: time.Now().Unix(),
	}

	event := thirdparty.NewEvent(
		thirdparty.EventDeviceRegistered,
		devicePhyID,
		eventData.ToMap(),
	)

	h.pushEvent(ctx, event, logger)
}

// pushDeviceAlarmEvent 推送设备告警事件
func (h *Handlers) pushDeviceAlarmEvent(ctx context.Context, devicePhyID string, alarmType, level, message string, portNo int, metadata map[string]interface{}, logger *zap.Logger) {
	eventData := &thirdparty.DeviceAlarmData{
		AlarmType: alarmType,
		PortNo:    portNo,
		Level:     level,
		Message:   message,
		Metadata:  metadata,
		AlarmAt:   time.Now().Unix(),
	}

	event := thirdparty.NewEvent(
		thirdparty.EventDeviceAlarm,
		devicePhyID,
		eventData.ToMap(),
	)

	h.pushEvent(ctx, event, logger)
}

// pushSocketStateChangedEvent 推送插座状态变更事件
func (h *Handlers) pushSocketStateChangedEvent(ctx context.Context, devicePhyID string, portNo int, oldState, newState, stateReason string, logger *zap.Logger) {
	eventData := &thirdparty.SocketStateChangedData{
		PortNo:      portNo,
		OldState:    oldState,
		NewState:    newState,
		StateReason: stateReason,
		ChangedAt:   time.Now().Unix(),
	}

	event := thirdparty.NewEvent(
		thirdparty.EventSocketStateChanged,
		devicePhyID,
		eventData.ToMap(),
	)

	h.pushEvent(ctx, event, logger)
}
