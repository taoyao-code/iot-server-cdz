package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/taoyao-code/protocol-validation-test/internal/simulator"
)

const banner = `
╔═══════════════════════════════════════════════════════════════╗
║                IoT协议行为模拟器 v1.0.0                     ║
║             Behavioral Simulator                              ║
║                                                               ║
║  最小连接/调度/错误注入能力，仅用于验证真实时间/并发/连接行为   ║
║  不做真实物理仿真，专注协议行为验证                           ║
╚═══════════════════════════════════════════════════════════════╝
`

type Config struct {
	Mode            string        `json:"mode"`
	DeviceCount     int           `json:"device_count"`
	Duration        time.Duration `json:"duration"`
	ErrorRate       float64       `json:"error_rate"`
	HeartbeatInterval time.Duration `json:"heartbeat_interval"`
	ConcurrentLevel int           `json:"concurrent_level"`
	OutputDir       string        `json:"output_dir"`
	Verbose         bool          `json:"verbose"`
}

func main() {
	var (
		mode            = flag.String("mode", "connection", "模拟模式")
		deviceCount     = flag.Int("devices", 10, "模拟设备数量")
		duration        = flag.Duration("duration", 60*time.Second, "运行时长")
		errorRate       = flag.Float64("error-rate", 0.01, "错误注入率")
		heartbeatInterval = flag.Duration("heartbeat", 30*time.Second, "心跳间隔")
		concurrentLevel = flag.Int("concurrent", 5, "并发级别")
		outputDir       = flag.String("output", "./reports", "输出目录")
		verbose         = flag.Bool("verbose", false, "详细输出")
		help            = flag.Bool("help", false, "显示帮助信息")
	)
	flag.Parse()

	if *help {
		showHelp()
		return
	}

	fmt.Print(banner)

	config := &Config{
		Mode:              *mode,
		DeviceCount:       *deviceCount,
		Duration:          *duration,
		ErrorRate:         *errorRate,
		HeartbeatInterval: *heartbeatInterval,
		ConcurrentLevel:   *concurrentLevel,
		OutputDir:         *outputDir,
		Verbose:           *verbose,
	}

	// 创建模拟器
	simConfig := simulator.DefaultSimulatorConfig()
	simConfig.MaxConnections = config.DeviceCount * 2
	simConfig.HeartbeatInterval = config.HeartbeatInterval
	simConfig.ErrorRate = config.ErrorRate

	sim := simulator.NewMinimalSimulator(simConfig)

	// 设置信号处理
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\n收到终止信号，正在停止模拟器...")
		cancel()
	}()

	// 启动模拟器
	if err := sim.Start(ctx); err != nil {
		log.Fatalf("启动模拟器失败: %v", err)
	}
	defer sim.Close()

	// 运行模拟场景
	switch config.Mode {
	case "connection":
		err := runConnectionSimulation(ctx, sim, config)
		if err != nil {
			log.Fatalf("连接模拟失败: %v", err)
		}
	case "scheduler":
		err := runSchedulerSimulation(ctx, sim, config)
		if err != nil {
			log.Fatalf("调度模拟失败: %v", err)
		}
	case "error":
		err := runErrorInjectionSimulation(ctx, sim, config)
		if err != nil {
			log.Fatalf("错误注入模拟失败: %v", err)
		}
	default:
		log.Fatalf("不支持的模拟模式: %s", config.Mode)
	}

	fmt.Println("模拟完成")
}

func runConnectionSimulation(ctx context.Context, sim *simulator.MinimalSimulator, config *Config) error {
	fmt.Printf("启动连接模拟: %d个设备\n", config.DeviceCount)

	// 连接设备
	for i := 0; i < config.DeviceCount; i++ {
		deviceID := fmt.Sprintf("device_%03d", i+1)
		if err := sim.Connect(ctx, deviceID); err != nil {
			log.Printf("设备 %s 连接失败: %v", deviceID, err)
		} else if config.Verbose {
			fmt.Printf("设备 %s 已连接\n", deviceID)
		}
	}

	// 运行指定时长
	time.Sleep(config.Duration)
	return nil
}

func runSchedulerSimulation(ctx context.Context, sim *simulator.MinimalSimulator, config *Config) error {
	fmt.Printf("启动调度模拟: %d个设备\n", config.DeviceCount)

	// 连接设备并调度心跳
	for i := 0; i < config.DeviceCount; i++ {
		deviceID := fmt.Sprintf("device_%03d", i+1)
		sim.Connect(ctx, deviceID)
		sim.ScheduleHeartbeat(ctx, deviceID, config.HeartbeatInterval)
		
		if config.Verbose {
			fmt.Printf("设备 %s 心跳调度已启动\n", deviceID)
		}
	}

	// 运行指定时长
	time.Sleep(config.Duration)
	return nil
}

func runErrorInjectionSimulation(ctx context.Context, sim *simulator.MinimalSimulator, config *Config) error {
	fmt.Printf("启动错误注入模拟: 错误率 %.2f%%\n", config.ErrorRate*100)

	injector := simulator.NewBasicErrorInjector(nil)

	// 连接少量设备
	deviceCount := 3
	for i := 0; i < deviceCount; i++ {
		deviceID := fmt.Sprintf("device_%03d", i+1)
		sim.Connect(ctx, deviceID)
	}

	// 模拟数据发送与错误注入
	testData := []byte{0xFC, 0xFE, 0x00, 0x18, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x01, 0x82, 0x20, 0x05, 0x20, 0x00, 0x48, 0x69, 0x20, 0x20, 0x07, 0x30, 0x16, 0x45, 0x45, 0xA7, 0xFC, 0xEE}

	endTime := time.Now().Add(config.Duration)
	for time.Now().Before(endTime) {
		for i := 0; i < deviceCount; i++ {
			deviceID := fmt.Sprintf("device_%03d", i+1)
			
			// 注入不同类型的错误
			var corruptedData []byte
			switch i {
			case 0:
				corruptedData = injector.InjectChecksumError(testData)
			case 1:
				corruptedData = injector.InjectHeaderError(testData)
			case 2:
				corruptedData = injector.InjectLengthError(testData)
			}

			sim.Send(ctx, deviceID, corruptedData)
			
			if config.Verbose {
				fmt.Printf("设备 %s 注入错误\n", deviceID)
			}
		}
		
		time.Sleep(2 * time.Second)
	}

	return nil
}

func showHelp() {
	fmt.Print(banner)
	fmt.Println(`
使用方法:
  behavioral-simulator [选项]

模拟模式:
  connection   - 连接模拟
  scheduler    - 调度模拟
  error        - 错误注入

选项:
  -mode string       模拟模式 (default "connection")
  -devices int       设备数量 (default 10)
  -duration duration 运行时长 (default 1m0s)
  -error-rate float  错误率 (default 0.01)
  -heartbeat duration 心跳间隔 (default 30s)
  -concurrent int    并发级别 (default 5)
  -output string     输出目录 (default "./reports")
  -verbose           详细输出
  -help              显示帮助信息

示例:
  behavioral-simulator -mode connection -devices 20 -duration 2m
  behavioral-simulator -mode scheduler -devices 10 -heartbeat 15s
  behavioral-simulator -mode error -error-rate 0.05 -verbose
`)
}