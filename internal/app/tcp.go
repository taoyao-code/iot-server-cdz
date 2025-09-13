package app

import (
	cfgpkg "github.com/taoyao-code/iot-server/internal/config"
	"github.com/taoyao-code/iot-server/internal/tcpserver"
)

// NewTCPServer 根据配置创建 TCP 服务器
func NewTCPServer(cfg cfgpkg.TCPConfig) *tcpserver.Server { return tcpserver.New(cfg) }
