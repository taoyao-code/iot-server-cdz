package pg

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/tracelog"
	"go.uber.org/zap"
)

// NewPool 创建 pgx 连接池
func NewPool(ctx context.Context, dsn string, maxOpen, maxIdle int, maxLifetime time.Duration, logger *zap.Logger) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}

	// 添加 SQL 日志追踪器
	if logger != nil {
		cfg.ConnConfig.Tracer = &tracelog.TraceLog{
			Logger:   &pgxZapLogger{logger: logger},
			LogLevel: tracelog.LogLevelTrace, // 记录所有 SQL 语句
		}
	}

	// Week2: 优化连接池配置
	if maxOpen > 0 {
		cfg.MaxConns = int32(maxOpen)
	} else {
		cfg.MaxConns = 20 // 默认20个连接（提升自10）
	}

	if maxIdle > 0 {
		cfg.MinConns = int32(maxIdle)
	} else {
		cfg.MinConns = 5 // 默认保持5个空闲连接（预热）
	}

	if maxLifetime > 0 {
		cfg.MaxConnLifetime = maxLifetime
	} else {
		cfg.MaxConnLifetime = 1 * time.Hour // 连接最大生命周期1小时
	}

	cfg.MaxConnIdleTime = 30 * time.Minute  // 空闲连接30分钟后关闭
	cfg.HealthCheckPeriod = 1 * time.Minute // 每分钟健康检查

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// 探活
	ctxPing, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := pool.Ping(ctxPing); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}

// pgxZapLogger 实现 tracelog.Logger 接口,将 pgx 日志适配到 zap
type pgxZapLogger struct {
	logger *zap.Logger
}

func (l *pgxZapLogger) Log(ctx context.Context, level tracelog.LogLevel, msg string, data map[string]interface{}) {
	fields := make([]zap.Field, 0, len(data))
	for k, v := range data {
		fields = append(fields, zap.Any(k, v))
	}

	switch level {
	case tracelog.LogLevelTrace:
		l.logger.Debug("[SQL] "+msg, fields...)
	case tracelog.LogLevelDebug:
		l.logger.Debug(msg, fields...)
	case tracelog.LogLevelInfo:
		l.logger.Info(msg, fields...)
	case tracelog.LogLevelWarn:
		l.logger.Warn(msg, fields...)
	case tracelog.LogLevelError:
		l.logger.Error(msg, fields...)
	default:
		l.logger.Info(msg, fields...)
	}
}
