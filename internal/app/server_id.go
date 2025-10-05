package app

import (
	"fmt"
	"os"

	"github.com/google/uuid"
)

// GenerateServerID 生成服务器实例ID
// 优先使用环境变量SERVER_ID，否则生成UUID
func GenerateServerID() string {
	if serverID := os.Getenv("SERVER_ID"); serverID != "" {
		return serverID
	}

	// 生成格式：iot-server-{hostname}-{uuid}
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	shortUUID := uuid.New().String()[:8]
	return fmt.Sprintf("iot-server-%s-%s", hostname, shortUUID)
}
