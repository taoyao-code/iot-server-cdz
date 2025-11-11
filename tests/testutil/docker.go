package testutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// DockerComposeManager 管理测试环境的 Docker Compose
type DockerComposeManager struct {
	composeFile string
	projectName string
}

// NewDockerComposeManager 创建 Docker Compose 管理器
func NewDockerComposeManager(composeFile, projectName string) *DockerComposeManager {
	return &DockerComposeManager{
		composeFile: composeFile,
		projectName: projectName,
	}
}

// Up 启动测试环境
func (m *DockerComposeManager) Up(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "compose",
		"-f", m.composeFile,
		"-p", m.projectName,
		"up", "-d", "--wait")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose up failed: %w", err)
	}

	// 等待服务完全就绪
	if err := m.waitForServices(ctx); err != nil {
		return err
	}

	return nil
}

// Down 停止并清理测试环境
func (m *DockerComposeManager) Down(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "compose",
		"-f", m.composeFile,
		"-p", m.projectName,
		"down", "-v", "--remove-orphans")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose down failed: %w", err)
	}

	return nil
}

// waitForServices 等待服务健康检查通过
func (m *DockerComposeManager) waitForServices(ctx context.Context) error {
	// 等待 PostgreSQL
	if err := m.waitForPostgres(ctx); err != nil {
		return err
	}

	// 等待 Redis
	if err := m.waitForRedis(ctx); err != nil {
		return err
	}

	return nil
}

// waitForPostgres 等待 PostgreSQL 就绪
func (m *DockerComposeManager) waitForPostgres(ctx context.Context) error {
	maxRetries := 30
	for i := 0; i < maxRetries; i++ {
		cmd := exec.CommandContext(ctx, "docker", "exec",
			"iot-postgres-test",
			"pg_isready", "-U", "postgres")

		if err := cmd.Run(); err == nil {
			return nil // 就绪
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
			// 继续等待
		}
	}

	return fmt.Errorf("postgres not ready after %d retries", maxRetries)
}

// waitForRedis 等待 Redis 就绪
func (m *DockerComposeManager) waitForRedis(ctx context.Context) error {
	maxRetries := 30
	for i := 0; i < maxRetries; i++ {
		cmd := exec.CommandContext(ctx, "docker", "exec",
			"iot-redis-test",
			"redis-cli", "ping")

		output, err := cmd.Output()
		if err == nil && strings.TrimSpace(string(output)) == "PONG" {
			return nil // 就绪
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
			// 继续等待
		}
	}

	return fmt.Errorf("redis not ready after %d retries", maxRetries)
}

// GetPostgresDSN 返回测试 PostgreSQL DSN
func (m *DockerComposeManager) GetPostgresDSN() string {
	return "postgres://postgres:postgres@localhost:15433/iot_test?sslmode=disable"
}

// GetRedisAddr 返回测试 Redis 地址
func (m *DockerComposeManager) GetRedisAddr() string {
	return "localhost:6381"
}
