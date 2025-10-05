package app

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/taoyao-code/iot-server/internal/protocol/bkv"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
)

// OutboundAdapter Week5: 适配器，将PostgreSQL outbound_queue适配到bkv.OutboundSender接口
type OutboundAdapter struct {
	dbpool *pgxpool.Pool
	repo   *pgstorage.Repository
}

// NewOutboundAdapter 创建Outbound适配器
func NewOutboundAdapter(dbpool *pgxpool.Pool, repo *pgstorage.Repository) *OutboundAdapter {
	return &OutboundAdapter{
		dbpool: dbpool,
		repo:   repo,
	}
}

// SendDownlink 发送下行消息
func (a *OutboundAdapter) SendDownlink(gatewayID string, cmd uint16, msgID uint32, data []byte) error {
	if a.dbpool == nil || a.repo == nil {
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

	// 插入到Outbound队列（PostgreSQL）
	_, err = a.dbpool.Exec(ctx, `
		INSERT INTO outbound_queue (device_id, phy_id, cmd, payload, priority, msg_id)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, deviceID, gatewayID, int(cmd), frame, 100, int(msgID))
	if err != nil {
		return fmt.Errorf("insert to outbound queue: %w", err)
	}

	return nil
}
