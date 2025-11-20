package app

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	cfgpkg "github.com/taoyao-code/iot-server/internal/config"
	"github.com/taoyao-code/iot-server/internal/migrate"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	"go.uber.org/zap"
)

// ConnectDBAndMigrate 建立数据库连接并按需执行迁移
func ConnectDBAndMigrate(ctx context.Context, cfg cfgpkg.DatabaseConfig, migrateDir string, log *zap.Logger) (*pgxpool.Pool, error) {
	dbpool, err := pgstorage.NewPool(ctx, cfg.DSN, cfg.MaxOpenConns, cfg.MaxIdleConns, cfg.ConnMaxLifetime, log)
	if err != nil {
		if log != nil {
			log.Error("db connect error", zap.Error(err))
		}
		return nil, err
	}
	if cfg.AutoMigrate {
		if err = (migrate.Runner{Dir: migrateDir}).Up(ctx, dbpool); err != nil {
			if log != nil {
				log.Error("db migrate error", zap.Error(err))
			}
			return dbpool, err
		}
		if log != nil {
			log.Info("db migrations applied")
		}
	}
	return dbpool, nil
}
