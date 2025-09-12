package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// AppConfig 应用基础信息
type AppConfig struct {
	Name string `mapstructure:"name"`
	Env  string `mapstructure:"env"`
}

// HTTPConfig HTTP 服务配置
type HTTPConfig struct {
	Addr         string        `mapstructure:"addr"`
	ReadTimeout  time.Duration `mapstructure:"readTimeout"`
	WriteTimeout time.Duration `mapstructure:"writeTimeout"`
	Pprof        HTTPPprof     `mapstructure:"pprof"`
}

// HTTPPprof HTTP pprof 配置
type HTTPPprof struct {
	Enable bool   `mapstructure:"enable"`
	Prefix string `mapstructure:"prefix"`
}

// TCPConfig TCP 网关配置
type TCPConfig struct {
	Addr              string        `mapstructure:"addr"`
	ReadTimeout       time.Duration `mapstructure:"readTimeout"`
	WriteTimeout      time.Duration `mapstructure:"writeTimeout"`
	MaxConnections    int           `mapstructure:"maxConnections"`
	ConnectionBacklog int           `mapstructure:"connectionBacklog"`
}

// ProtocolsConfig 协议启用配置
type ProtocolsConfig struct {
	EnableAP3000 bool      `mapstructure:"enable_ap3000"`
	EnableBKV    bool      `mapstructure:"enable_bkv"`
	BKV          BKVConfig `mapstructure:"bkv"`
}

// BKVConfig BKV 协议配置
type BKVConfig struct {
	ReasonMapPath string `mapstructure:"reason_map_path"`
}

// GatewayConfig 设备接入网关与出站相关配置
type GatewayConfig struct {
	Listen            string   `mapstructure:"listen"`
	ReadBufferBytes   int      `mapstructure:"read_buffer_bytes"`
	WriteBufferBytes  int      `mapstructure:"write_buffer_bytes"`
	ThrottleMs        int      `mapstructure:"throttle_ms"`
	AckTimeoutMs      int      `mapstructure:"ack_timeout_ms"`
	RetryMax          int      `mapstructure:"retry_max"`
	DeadRetentionDays int      `mapstructure:"dead_retention_days"`
	IPWhitelist       []string `mapstructure:"ip_whitelist"`
	DeviceWhitelist   []string `mapstructure:"device_whitelist"`
}

// LumberjackConfig 日志滚动（lumberjack）配置
type LumberjackConfig struct {
	Filename   string `mapstructure:"filename"`
	MaxSizeMB  int    `mapstructure:"maxSize"`
	MaxBackups int    `mapstructure:"maxBackups"`
	MaxAgeDays int    `mapstructure:"maxAge"`
	Compress   bool   `mapstructure:"compress"`
}

// LoggingConfig 日志级别与输出配置
type LoggingConfig struct {
	Level  string           `mapstructure:"level"`
	Format string           `mapstructure:"format"`
	File   LumberjackConfig `mapstructure:"file"`
}

// MetricsConfig Prometheus 指标暴露配置
type MetricsConfig struct {
	Enable bool   `mapstructure:"enable"`
	Path   string `mapstructure:"path"`
}

// DatabaseConfig PostgreSQL 连接配置
type DatabaseConfig struct {
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"maxOpenConns"`
	MaxIdleConns    int           `mapstructure:"maxIdleConns"`
	ConnMaxLifetime time.Duration `mapstructure:"connMaxLifetime"`
	AutoMigrate     bool          `mapstructure:"autoMigrate"`
}

// ThirdpartyPushConfig 第三方推送配置
type ThirdpartyPushConfig struct {
	WebhookURL string `mapstructure:"webhook_url"`
	Secret     string `mapstructure:"secret"`
	MaxRetries int    `mapstructure:"max_retries"`
	Backoff    string `mapstructure:"backoff"`
}

// ThirdpartyAuthConfig 第三方鉴权（入站）
type ThirdpartyAuthConfig struct {
	APIKeys []string `mapstructure:"api_keys"`
}

// ThirdpartyConfig 第三方集成配置
type ThirdpartyConfig struct {
	Push        ThirdpartyPushConfig `mapstructure:"push"`
	Auth        ThirdpartyAuthConfig `mapstructure:"auth"`
	IPWhitelist []string             `mapstructure:"ip_whitelist"`
}

// Config 顶层配置结构
type Config struct {
	App        AppConfig        `mapstructure:"app"`
	HTTP       HTTPConfig       `mapstructure:"http"`
	TCP        TCPConfig        `mapstructure:"tcp"`
	Protocols  ProtocolsConfig  `mapstructure:"protocols"`
	Gateway    GatewayConfig    `mapstructure:"gateway"`
	Logging    LoggingConfig    `mapstructure:"logging"`
	Metrics    MetricsConfig    `mapstructure:"metrics"`
	Database   DatabaseConfig   `mapstructure:"database"`
	Thirdparty ThirdpartyConfig `mapstructure:"thirdparty"`
}

// Load 从 YAML/TOML/JSON 文件与环境变量加载配置。
// 若 path 为空，则尝试从环境变量 IOT_CONFIG 读取；否则回退到 configs/example.yaml。
func Load(path string) (*Config, error) {
	v := viper.New()

	if path == "" {
		path = v.GetString("IOT_CONFIG")
	}

	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.AddConfigPath(".")
		v.AddConfigPath("./configs")
		v.SetConfigName("example")
		v.SetConfigType("yaml")
	}

	// 默认值
	setDefaults(v)

	// 环境变量覆盖：前缀 IOT_，并将点号替换为下划线
	v.SetEnvPrefix("IOT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		// 首次运行允许缺少配置文件，依赖默认值与环境变量
		var notFound viper.ConfigFileNotFoundError
		if fmt.Sprintf("%T", err) != fmt.Sprintf("%T", notFound) {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("app.name", "iot-server")
	v.SetDefault("app.env", "dev")

	v.SetDefault("http.addr", ":8080")
	v.SetDefault("http.readTimeout", "5s")
	v.SetDefault("http.writeTimeout", "10s")
	v.SetDefault("http.pprof.enable", false)
	v.SetDefault("http.pprof.prefix", "/debug/pprof")

	v.SetDefault("tcp.addr", ":7000")
	v.SetDefault("tcp.readTimeout", "5s")
	v.SetDefault("tcp.writeTimeout", "10s")
	v.SetDefault("tcp.maxConnections", 5000)
	v.SetDefault("tcp.connectionBacklog", 1024)

	// 协议默认
	v.SetDefault("protocols.enable_ap3000", true)
	v.SetDefault("protocols.enable_bkv", true)
	v.SetDefault("protocols.bkv.reason_map_path", "configs/bkv-reason-map.example.yaml")

	// 网关与出站默认
	v.SetDefault("gateway.listen", ":9000")
	v.SetDefault("gateway.read_buffer_bytes", 32768)
	v.SetDefault("gateway.write_buffer_bytes", 32768)
	v.SetDefault("gateway.throttle_ms", 500)
	v.SetDefault("gateway.ack_timeout_ms", 15000)
	v.SetDefault("gateway.retry_max", 1)
	v.SetDefault("gateway.dead_retention_days", 7)
	v.SetDefault("gateway.ip_whitelist", []string{})
	v.SetDefault("gateway.device_whitelist", []string{})

	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.file.filename", "logs/iot-server.log")
	v.SetDefault("logging.file.maxSize", 100)
	v.SetDefault("logging.file.maxBackups", 7)
	v.SetDefault("logging.file.maxAge", 30)
	v.SetDefault("logging.file.compress", true)

	v.SetDefault("metrics.enable", true)
	v.SetDefault("metrics.path", "/metrics")

	v.SetDefault("database.dsn", "postgres://postgres:postgres@localhost:5432/iot?sslmode=disable")
	v.SetDefault("database.maxOpenConns", 20)
	v.SetDefault("database.maxIdleConns", 10)
	v.SetDefault("database.connMaxLifetime", "1h")
	v.SetDefault("database.autoMigrate", false)

	// thirdparty defaults
	v.SetDefault("thirdparty.push.webhook_url", "")
	v.SetDefault("thirdparty.push.secret", "")
	v.SetDefault("thirdparty.push.max_retries", 5)
	v.SetDefault("thirdparty.push.backoff", "100ms,200ms,500ms,1s,2s")
}
