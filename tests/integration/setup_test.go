package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/taoyao-code/iot-server/tests/testutil"
)

var (
	// 全局测试环境
	testDB      *pgxpool.Pool
	testRedis   *redis.Client
	dockerMgr   *testutil.DockerComposeManager
	skipDocker  bool
	skipCleanup bool
)

// TestMain 控制整个测试套件的生命周期
func TestMain(m *testing.M) {
	// 检查是否跳过 Docker（用于本地调试）
	skipDocker = os.Getenv("SKIP_DOCKER") == "true"
	skipCleanup = os.Getenv("SKIP_CLEANUP") == "true"

	// 设置测试环境
	if err := setupTestEnvironment(); err != nil {
		panic("Failed to setup test environment: " + err.Error())
	}

	// 运行测试
	code := m.Run()

	// 清理测试环境
	if err := teardownTestEnvironment(); err != nil {
		panic("Failed to teardown test environment: " + err.Error())
	}

	os.Exit(code)
}

// setupTestEnvironment 初始化测试环境
func setupTestEnvironment() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if !skipDocker {
		// 启动 Docker Compose
		composeFile := filepath.Join("..", "..", "docker-compose.test.yml")
		dockerMgr = testutil.NewDockerComposeManager(composeFile, "iot-integration-test")

		// 清理残留容器（忽略错误）
		_ = dockerMgr.Down(ctx)

		if err := dockerMgr.Up(ctx); err != nil {
			return err
		}
	}

	// 连接数据库
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		if dockerMgr != nil {
			dsn = dockerMgr.GetPostgresDSN()
		} else {
			dsn = "postgres://postgres:postgres@localhost:15433/iot_test?sslmode=disable"
		}
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	if err := pool.Ping(ctx); err != nil {
		return err
	}
	testDB = pool

	// 连接 Redis
	redisAddr := os.Getenv("TEST_REDIS_ADDR")
	if redisAddr == "" {
		if dockerMgr != nil {
			redisAddr = dockerMgr.GetRedisAddr()
		} else {
			redisAddr = "localhost:6381"
		}
	}

	testRedis = redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	if err := testRedis.Ping(ctx).Err(); err != nil {
		return err
	}

	return nil
}

// teardownTestEnvironment 清理测试环境
func teardownTestEnvironment() error {
	if testDB != nil {
		testDB.Close()
	}

	if testRedis != nil {
		testRedis.Close()
	}

	if !skipDocker && !skipCleanup && dockerMgr != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return dockerMgr.Down(ctx)
	}

	return nil
}

// getTestDB 获取测试数据库连接
func getTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testDB == nil {
		t.Fatal("test db not initialized")
	}
	return testDB
}

// getTestRedis 获取测试 Redis 客户端
func getTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	if testRedis == nil {
		t.Fatal("test redis not initialized")
	}
	return testRedis
}

// cleanupTest 在每个测试后清理数据
func cleanupTest(t *testing.T) {
	t.Helper()
	testutil.CleanDatabase(t, testDB)
	testutil.CleanRedis(t, testRedis)
}
