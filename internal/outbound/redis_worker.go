package outbound

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	redisstorage "github.com/taoyao-code/iot-server/internal/storage/redis"
	"go.uber.org/zap"
)

// RedisWorker Redis下行队列Worker (Week2.2)
type RedisWorker struct {
	queue      *redisstorage.OutboundQueue
	logger     *zap.Logger
	throttleMs int
	retryMax   int
	stopC      chan struct{}
	getConn    func(phyID string) (interface{}, bool)

	// 统计
	sent      atomic.Int64
	failed    atomic.Int64
	retried   atomic.Int64
	deadCount atomic.Int64
}

// NewRedisWorker 创建Redis Worker
func NewRedisWorker(
	queue *redisstorage.OutboundQueue,
	throttleMs int,
	retryMax int,
	logger *zap.Logger,
) *RedisWorker {
	return &RedisWorker{
		queue:      queue,
		throttleMs: throttleMs,
		retryMax:   retryMax,
		logger:     logger,
		stopC:      make(chan struct{}),
	}
}

// SetGetConn 设置获取连接的函数
func (w *RedisWorker) SetGetConn(fn func(phyID string) (interface{}, bool)) {
	w.getConn = fn
}

// Start 启动Worker
func (w *RedisWorker) Start(ctx context.Context) {
	w.logger.Info("redis outbound worker started")

	ticker := time.NewTicker(time.Duration(w.throttleMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("redis outbound worker stopping")
			return
		case <-w.stopC:
			w.logger.Info("redis outbound worker stopped")
			return
		case <-ticker.C:
			w.processOne(ctx)
		}
	}
}

// Stop 停止Worker
func (w *RedisWorker) Stop() {
	close(w.stopC)
}

// processOne 处理一条消息
func (w *RedisWorker) processOne(ctx context.Context) {
	// 出队
	msg, err := w.queue.Dequeue(ctx)
	if err != nil {
		w.logger.Error("dequeue failed", zap.Error(err))
		return
	}

	if msg == nil {
		return // 队列为空
	}

	// 标记为处理中
	if err := w.queue.MarkProcessing(ctx, msg); err != nil {
		w.logger.Error("mark processing failed",
			zap.String("msg_id", msg.ID),
			zap.Error(err))
		return
	}

	// 获取连接
	if w.getConn == nil {
		w.markFailed(ctx, msg, "getConn function not set")
		return
	}

	// 关键修复点2：获取连接时，Session已经做了心跳超时验证
	conn, ok := w.getConn(msg.PhyID)
	if !ok {
		// 设备不在线或连接已失效（包括心跳超时）
		w.logger.Warn("device connection not available",
			zap.String("msg_id", msg.ID),
			zap.String("phy_id", msg.PhyID),
			zap.String("reason", "not connected or heartbeat timeout"))
		w.markFailed(ctx, msg, fmt.Sprintf("device %s not connected or connection invalid", msg.PhyID))
		return
	}

	// DEBUG: 记录即将发送的命令
	w.logger.Info("DEBUG: worker准备发送命令",
		zap.String("msg_id", msg.ID),
		zap.Int("command_len", len(msg.Command)),
		zap.String("command_hex", fmt.Sprintf("%x", msg.Command)))

	// 发送命令（兼容两类写入接口）：
	// 1) net.Conn:          Write([]byte) (int, error)
	// 2) ConnContext等包装: Write([]byte) error
	if writer1, ok := conn.(interface{ Write([]byte) (int, error) }); ok {
		n, err := writer1.Write(msg.Command)
		if err != nil {
			// 关键修复点3：写入失败时记录详细错误
			w.logger.Error("write to device failed",
				zap.String("msg_id", msg.ID),
				zap.String("phy_id", msg.PhyID),
				zap.Int("expected_bytes", len(msg.Command)),
				zap.Int("written_bytes", n),
				zap.Error(err))
			w.markFailed(ctx, msg, fmt.Sprintf("write failed: %v", err))
			return
		}
		// 验证是否完整发送
		if n != len(msg.Command) {
			w.logger.Warn("partial write to device",
				zap.String("msg_id", msg.ID),
				zap.String("phy_id", msg.PhyID),
				zap.Int("expected", len(msg.Command)),
				zap.Int("actual", n))
		}
	} else if writer2, ok := conn.(interface{ Write([]byte) error }); ok {
		if err := writer2.Write(msg.Command); err != nil {
			w.logger.Error("write to device failed",
				zap.String("msg_id", msg.ID),
				zap.String("phy_id", msg.PhyID),
				zap.Error(err))
			w.markFailed(ctx, msg, fmt.Sprintf("write failed: %v", err))
			return
		}
	} else {
		w.markFailed(ctx, msg, "connection does not support Write")
		return
	}

	// 等待ACK（简化版，实际应该有超时和回调）
	// TODO: 实现ACK等待机制
	time.Sleep(time.Duration(msg.Timeout) * time.Millisecond)

	// 标记成功
	if err := w.queue.MarkSuccess(ctx, msg); err != nil {
		w.logger.Error("mark success failed",
			zap.String("msg_id", msg.ID),
			zap.Error(err))
		return
	}

	w.sent.Add(1)
	w.logger.Info("downlink message sent",
		zap.String("msg_id", msg.ID),
		zap.String("phy_id", msg.PhyID),
		zap.Int("bytes", len(msg.Command)))
}

// markFailed 标记失败
func (w *RedisWorker) markFailed(ctx context.Context, msg *redisstorage.OutboundMessage, errMsg string) {
	if err := w.queue.MarkFailed(ctx, msg, errMsg); err != nil {
		w.logger.Error("mark failed error",
			zap.String("msg_id", msg.ID),
			zap.Error(err))
		return
	}

	if msg.Retries+1 >= msg.MaxRetry {
		w.deadCount.Add(1)
		w.logger.Warn("message moved to dead queue",
			zap.String("msg_id", msg.ID),
			zap.String("phy_id", msg.PhyID),
			zap.String("error", errMsg))
	} else {
		w.retried.Add(1)
		w.logger.Debug("message retrying",
			zap.String("msg_id", msg.ID),
			zap.Int("retry", msg.Retries+1))
	}

	w.failed.Add(1)
}

// Stats 获取统计信息
func (w *RedisWorker) Stats(ctx context.Context) map[string]interface{} {
	queueStats, _ := w.queue.Stats(ctx)

	return map[string]interface{}{
		"sent":       w.sent.Load(),
		"failed":     w.failed.Load(),
		"retried":    w.retried.Load(),
		"dead_count": w.deadCount.Load(),
		"queue":      queueStats,
	}
}
