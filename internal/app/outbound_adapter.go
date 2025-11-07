package app

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/taoyao-code/iot-server/internal/outbound"
	"github.com/taoyao-code/iot-server/internal/protocol/bkv"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	redisstorage "github.com/taoyao-code/iot-server/internal/storage/redis"
)

// OutboundAdapter Week5: 适配器，将Redis队列适配到bkv.OutboundSender接口
type OutboundAdapter struct {
	dbpool *pgxpool.Pool
	repo   *pgstorage.Repository
	queue  *redisstorage.OutboundQueue // 🔥 修复：使用Redis队列而不是PG
}

// NewOutboundAdapter 创建Outbound适配器
func NewOutboundAdapter(dbpool *pgxpool.Pool, repo *pgstorage.Repository, queue *redisstorage.OutboundQueue) *OutboundAdapter {
	return &OutboundAdapter{
		dbpool: dbpool,
		repo:   repo,
		queue:  queue,
	}
}

// SendDownlink 发送下行消息
// 🔥 修复：使用Redis队列，确保心跳ACK能被worker立即处理
func (a *OutboundAdapter) SendDownlink(gatewayID string, cmd uint16, msgID uint32, data []byte) error {
	if a.queue == nil || a.repo == nil {
		return fmt.Errorf("outbound sender not configured")
	}

	ctx := context.Background()

	// 构造BKV下行帧
	frame := bkv.Build(cmd, msgID, gatewayID, data)

	// 通过PhyID获取DeviceID
	deviceID, err := a.repo.EnsureDevice(ctx, gatewayID)
	if err != nil {
		return fmt.Errorf("get device id: %w", err)
	}

	// 插入到Redis队列（立即发送）
	// P1-6修复: 使用标准化优先级
	priority := outbound.GetCommandPriority(cmd)

	err = a.queue.Enqueue(ctx, &redisstorage.OutboundMessage{
		ID:       fmt.Sprintf("bkv_%d_%d", cmd, time.Now().UnixNano()),
		DeviceID: deviceID,
		PhyID:    gatewayID,
		Command:  frame,
		Priority: priority,
		MaxRetry: 1,
	})
	if err != nil {
		return fmt.Errorf("enqueue to redis: %w", err)
	}

	return nil
}
