package bkv

import (
	"context"

	"github.com/taoyao-code/iot-server/internal/coremodel"
	"github.com/taoyao-code/iot-server/internal/driverapi"
	"go.uber.org/zap"
)

// EventEmitter 事件发送器，统一处理事件发送、日志记录和错误处理
type EventEmitter struct {
	sink   driverapi.EventSink
	logger *zap.Logger
}

// NewEventEmitter 创建事件发送器
func NewEventEmitter(sink driverapi.EventSink, logger *zap.Logger) *EventEmitter {
	return &EventEmitter{
		sink:   sink,
		logger: logger,
	}
}

// Emit 发送事件（记录错误但不返回）
func (e *EventEmitter) Emit(ctx context.Context, event *coremodel.CoreEvent) {
	if e.sink == nil {
		if e.logger != nil {
			e.logger.Warn("event sink not configured, event dropped",
				zap.String("event_type", string(event.Type)),
				zap.String("device_id", string(event.DeviceID)))
		}
		return
	}

	if event == nil {
		return
	}

	err := e.sink.HandleCoreEvent(ctx, event)
	if err != nil && e.logger != nil {
		e.logger.Error("failed to emit core event",
			zap.String("event_type", string(event.Type)),
			zap.String("device_id", string(event.DeviceID)),
			zap.Error(err))
	}
}

// EmitWithCheck 发送事件并返回错误（用于需要错误处理的场景）
func (e *EventEmitter) EmitWithCheck(ctx context.Context, event *coremodel.CoreEvent) error {
	if e.sink == nil {
		return nil
	}

	if event == nil {
		return nil
	}

	return e.sink.HandleCoreEvent(ctx, event)
}

// IsConfigured 检查 EventSink 是否已配置
func (e *EventEmitter) IsConfigured() bool {
	return e.sink != nil
}
