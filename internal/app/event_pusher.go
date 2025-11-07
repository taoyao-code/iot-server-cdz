package app

import (
	"context"
	"encoding/json"
	"time"

	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	"github.com/taoyao-code/iot-server/internal/thirdparty"
	"go.uber.org/zap"
)

// EventPusher P1-7: 事件推送后台任务
// 定期从events表读取待推送事件，推送到第三方
type EventPusher struct {
	repo       *pgstorage.Repository
	eventQueue *thirdparty.EventQueue
	logger     *zap.Logger

	checkInterval time.Duration // 检查间隔
	batchSize     int           // 每次批量处理数量

	// 统计
	statsChecked int64
	statsPushed  int64
	statsFailed  int64
}

// NewEventPusher 创建事件推送器
func NewEventPusher(repo *pgstorage.Repository, eventQueue *thirdparty.EventQueue, logger *zap.Logger) *EventPusher {
	return &EventPusher{
		repo:          repo,
		eventQueue:    eventQueue,
		logger:        logger,
		checkInterval: 10 * time.Second, // 每10秒检查一次
		batchSize:     50,               // 每次最多处理50个事件
	}
}

// Start 启动事件推送器
func (p *EventPusher) Start(ctx context.Context) {
	p.logger.Info("P1-7: event pusher started",
		zap.Duration("check_interval", p.checkInterval),
		zap.Int("batch_size", p.batchSize))

	ticker := time.NewTicker(p.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("P1-7: event pusher stopped",
				zap.Int64("checked", p.statsChecked),
				zap.Int64("pushed", p.statsPushed),
				zap.Int64("failed", p.statsFailed))
			return
		case <-ticker.C:
			p.pushPendingEvents(ctx)
		}
	}
}

// pushPendingEvents 推送待处理的事件
func (p *EventPusher) pushPendingEvents(ctx context.Context) {
	p.statsChecked++

	// 获取待推送事件
	events, err := p.repo.GetPendingEvents(ctx, p.batchSize)
	if err != nil {
		p.logger.Error("P1-7: failed to get pending events", zap.Error(err))
		return
	}

	if len(events) == 0 {
		return
	}

	p.logger.Info("P1-7: processing pending events", zap.Int("count", len(events)))

	// 按订单分组，确保同一订单的事件按序推送
	orderEvents := make(map[string][]pgstorage.Event)
	for _, event := range events {
		orderEvents[event.OrderNo] = append(orderEvents[event.OrderNo], event)
	}

	// 逐订单推送事件
	for orderNo, evts := range orderEvents {
		for _, event := range evts {
			if err := p.pushEvent(ctx, &event); err != nil {
				p.logger.Warn("P1-7: failed to push event",
					zap.Int64("event_id", event.ID),
					zap.String("order_no", orderNo),
					zap.String("event_type", event.EventType),
					zap.Int("retry_count", event.RetryCount),
					zap.Error(err))
				p.statsFailed++

				// 标记失败
				if markErr := p.repo.MarkEventFailed(ctx, event.ID, err.Error()); markErr != nil {
					p.logger.Error("P1-7: failed to mark event as failed",
						zap.Int64("event_id", event.ID),
						zap.Error(markErr))
				}

				// 重试次数>=3时发送告警
				if event.RetryCount >= 3 {
					p.logger.Warn("⚠️ P1-7: event push retry exhausted",
						zap.Int64("event_id", event.ID),
						zap.String("order_no", orderNo),
						zap.String("event_type", event.EventType),
						zap.Int("retry_count", event.RetryCount))
				}

				// 某个事件失败后，跳过该订单的后续事件，保证顺序
				break
			} else {
				p.statsPushed++
				p.logger.Debug("P1-7: event pushed successfully",
					zap.Int64("event_id", event.ID),
					zap.String("order_no", orderNo),
					zap.String("event_type", event.EventType))
			}
		}
	}
}

// pushEvent 推送单个事件
func (p *EventPusher) pushEvent(ctx context.Context, event *pgstorage.Event) error {
	if p.eventQueue == nil {
		// 如果没有事件队列，直接标记为已推送（测试环境）
		return p.repo.MarkEventPushed(ctx, event.ID)
	}

	// 解析事件数据
	var eventData map[string]interface{}
	if err := json.Unmarshal(event.EventData, &eventData); err != nil {
		return err
	}

	// 从事件数据中获取device_phy_id
	devicePhyID, _ := eventData["device_phy_id"].(string)
	if devicePhyID == "" {
		// 如果事件数据中没有device_phy_id，记录警告
		p.logger.Warn("P1-7: event data missing device_phy_id, using order_no as fallback",
			zap.Int64("event_id", event.ID),
			zap.String("order_no", event.OrderNo),
			zap.String("event_type", event.EventType))
		// 使用order_no作为标识，虽然不是device_phy_id但可用于事件关联
		devicePhyID = "unknown"
	}

	// 创建StandardEvent
	eventType := thirdparty.EventType(event.EventType)
	stdEvent := thirdparty.NewEvent(eventType, devicePhyID, eventData)

	// 推送到Redis队列
	if err := p.eventQueue.Enqueue(ctx, stdEvent); err != nil {
		return err
	}

	// 标记为已推送
	return p.repo.MarkEventPushed(ctx, event.ID)
}

// Stats 获取统计信息
func (p *EventPusher) Stats() map[string]interface{} {
	return map[string]interface{}{
		"checked": p.statsChecked,
		"pushed":  p.statsPushed,
		"failed":  p.statsFailed,
	}
}
