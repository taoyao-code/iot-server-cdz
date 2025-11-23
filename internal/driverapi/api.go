package driverapi

import (
	"context"

	"github.com/taoyao-code/iot-server/internal/coremodel"
)

// EventSink 接收驱动上报的规范化事件，由中间件核心实现。
type EventSink interface {
	HandleCoreEvent(ctx context.Context, ev *coremodel.CoreEvent) error
}

// CommandSource 向具体协议驱动发出规范化命令，由中间件核心实现调度。
// 在当前进程内实现阶段，可以简单地由协议适配层持有 CommandSource。
type CommandSource interface {
	SendCoreCommand(ctx context.Context, cmd *coremodel.CoreCommand) error
}
