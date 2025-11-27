package bkv

import (
	"sync"

	"github.com/taoyao-code/iot-server/internal/driverapi"
	"github.com/taoyao-code/iot-server/internal/storage"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	"github.com/taoyao-code/iot-server/internal/thirdparty"
)

// NewHandlers 构造 BKV 处理集合。
// P0修复: 删除内存参数存储，直接使用数据库持久化。
func NewHandlers(repo *pgstorage.Repository, core storage.CoreRepo, reason *ReasonMap, events driverapi.EventSink) *Handlers {
	return &Handlers{
		Core:       core,
		Reason:     reason,
		Outbound:   nil,
		EventQueue: nil,
		Deduper:    nil,
		sessions:   &sync.Map{},
	}
}

// NewHandlersWithServices 构造 BKV 处理集合（包含 Outbound 等）Week5。
// v2.1: 添加EventQueue和Deduper支持。
func NewHandlersWithServices(repo *pgstorage.Repository, core storage.CoreRepo, reason *ReasonMap, _ interface{}, outbound OutboundSender, eventQueue *thirdparty.EventQueue, deduper *thirdparty.Deduper, events driverapi.EventSink) *Handlers {
	return &Handlers{
		Core:       core,
		Reason:     reason,
		Outbound:   outbound,
		EventQueue: eventQueue,
		Deduper:    deduper,
		CoreEvents: events,
		sessions:   &sync.Map{},
	}
}
