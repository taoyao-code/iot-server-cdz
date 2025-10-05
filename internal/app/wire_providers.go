//go:build wireinject
// +build wireinject

package app

import (
	"context"

	"github.com/google/wire"
	"github.com/jackc/pgx/v5/pgxpool"
	cfgpkg "github.com/taoyao-code/iot-server/internal/config"
	"github.com/taoyao-code/iot-server/internal/health"
	"github.com/taoyao-code/iot-server/internal/httpserver"
	"github.com/taoyao-code/iot-server/internal/metrics"
	"github.com/taoyao-code/iot-server/internal/protocol/ap3000"
	"github.com/taoyao-code/iot-server/internal/protocol/bkv"
	"github.com/taoyao-code/iot-server/internal/session"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	redisstorage "github.com/taoyao-code/iot-server/internal/storage/redis"
	"github.com/taoyao-code/iot-server/internal/tcpserver"
	"github.com/taoyao-code/iot-server/internal/thirdparty"
	"go.uber.org/zap"
)

// ProvideLogger 提供Logger (Week3: Wire)
func ProvideLogger(cfg *cfgpkg.Config) (*zap.Logger, error) {
	return NewLogger(cfg.Logging)
}

// ProvideMetrics 提供Metrics (Week3: Wire)
func ProvideMetrics() (*metrics.Registry, *metrics.AppMetrics) {
	return NewMetrics()
}

// ProvideSessionManager 提供SessionManager (Week3: Wire)
func ProvideSessionManager(cfg *cfgpkg.Config) (*session.Manager, session.WeightedPolicy) {
	return NewSessionAndPolicy(cfg.Session)
}

// ProvideDBPool 提供数据库连接池 (Week3: Wire)
func ProvideDBPool(ctx context.Context, cfg *cfgpkg.Config, log *zap.Logger) (*pgxpool.Pool, error) {
	return ConnectDBAndMigrate(ctx, cfg.Database, "db/migrations", log)
}

// ProvideRepository 提供Repository (Week3: Wire)
func ProvideRepository(pool *pgxpool.Pool) *pgstorage.Repository {
	return &pgstorage.Repository{Pool: pool}
}

// ProvideRedisClient 提供Redis客户端 (Week3: Wire)
func ProvideRedisClient(cfg *cfgpkg.Config, log *zap.Logger) (*redisstorage.Client, error) {
	return NewRedisClient(cfg.Redis, log)
}

// ProvidePusher 提供Pusher (Week3: Wire)
func ProvidePusher(cfg *cfgpkg.Config) (*thirdparty.Pusher, string) {
	return NewPusherIfEnabled(cfg.Thirdparty.Push.WebhookURL, cfg.Thirdparty.Push.Secret)
}

// ProvideBKVReasonMap 提供BKV原因码映射 (Week3: Wire)
func ProvideBKVReasonMap(cfg *cfgpkg.Config, log *zap.Logger) (*bkv.ReasonMap, error) {
	return NewBKVReasonMap(cfg.Protocols.BKV.ReasonMapPath, log)
}

// ProvideAP3000Handlers 提供AP3000处理器 (Week3: Wire)
func ProvideAP3000Handlers(
	repo *pgstorage.Repository,
	pusher *thirdparty.Pusher,
	pushURL string,
	appm *metrics.AppMetrics,
) *ap3000.Handlers {
	return &ap3000.Handlers{
		Repo:    repo,
		Pusher:  pusher,
		PushURL: pushURL,
		Metrics: appm,
	}
}

// ProvideBKVHandlers 提供BKV处理器 (Week3: Wire)
func ProvideBKVHandlers(repo *pgstorage.Repository, reasonMap *bkv.ReasonMap) *bkv.Handlers {
	return bkv.NewHandlers(repo, reasonMap)
}

// ProvideHealthAggregator 提供健康检查聚合器 (Week3: Wire)
func ProvideHealthAggregator(pool *pgxpool.Pool) *health.Aggregator {
	return NewHealthAggregator(pool)
}

// ProvideHTTPServer 提供HTTP服务器 (Week3: Wire)
func ProvideHTTPServer(
	cfg *cfgpkg.Config,
	metricsHandler *metrics.Registry,
	readyFn func() bool,
) *httpserver.Server {
	return NewHTTPServer(cfg.HTTP, cfg.Metrics.Path, metricsHandler, readyFn)
}

// ProvideTCPServer 提供TCP服务器 (Week3: Wire)
func ProvideTCPServer(cfg *cfgpkg.Config, log *zap.Logger) *tcpserver.Server {
	return NewTCPServer(cfg.TCP, log)
}

// ProviderSet Wire Provider集合
var ProviderSet = wire.NewSet(
	ProvideLogger,
	ProvideMetrics,
	ProvideSessionManager,
	ProvideDBPool,
	ProvideRepository,
	ProvideRedisClient,
	ProvidePusher,
	ProvideBKVReasonMap,
	ProvideAP3000Handlers,
	ProvideBKVHandlers,
	ProvideHealthAggregator,
	ProvideHTTPServer,
	ProvideTCPServer,
)
