package app

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5/pgxpool"
	cfgpkg "github.com/taoyao-code/iot-server/internal/config"
	"github.com/taoyao-code/iot-server/internal/migrate"
	"github.com/taoyao-code/iot-server/internal/storage"
	"github.com/taoyao-code/iot-server/internal/storage/gormrepo"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ConnectDBAndMigrate 建立数据库连接并按需执行迁移
func ConnectDBAndMigrate(ctx context.Context, cfg cfgpkg.DatabaseConfig, migrateDir string, log *zap.Logger) (*pgxpool.Pool, error) {
	dbpool, err := pgstorage.NewPool(ctx, cfg.DSN, cfg.MaxOpenConns, cfg.MaxIdleConns, cfg.ConnMaxLifetime, cfg.LogLevel, log)
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

// NewCoreRepo 使用 GORM 初始化 CoreRepo，实现与 pgxpool 并行的核心存储访问。
// 说明：
// - 仅依赖标准 GORM 和 postgres Dialector，不引入方言特有类型；
// - 返回的 sqlDB 需要在应用关闭时显式 Close。
func NewCoreRepo(cfg cfgpkg.DatabaseConfig, log *zap.Logger) (storage.CoreRepo, *sql.DB, error) {
	// 配置 GORM 日志级别
	gormConfig := &gorm.Config{}

	switch cfg.LogLevel {
	case "silent":
		gormConfig.Logger = logger.Default.LogMode(logger.Silent)
	case "error":
		gormConfig.Logger = logger.Default.LogMode(logger.Error)
	case "warn":
		gormConfig.Logger = logger.Default.LogMode(logger.Warn)
	case "info":
		gormConfig.Logger = logger.Default.LogMode(logger.Info)
	default:
		// 默认使用 silent 模式，避免生产环境打印大量 SQL 日志
		gormConfig.Logger = logger.Default.LogMode(logger.Silent)
	}

	db, err := gorm.Open(postgres.Open(cfg.DSN), gormConfig)
	if err != nil {
		if log != nil {
			log.Error("gorm core repo initialization failed", zap.Error(err))
		}
		return nil, nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		if log != nil {
			log.Error("gorm sql.DB initialization failed", zap.Error(err))
		}
		return nil, nil, err
	}

	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}

	return gormrepo.New(db), sqlDB, nil
}
