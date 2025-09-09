package logging

import (
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	cfgpkg "github.com/taoyao-code/iot-server/internal/config"
)

// InitLogger 初始化 zap 日志器（支持 lumberjack 滚动文件）
func InitLogger(cfg cfgpkg.LoggingConfig) (*zap.Logger, error) {
	level := zapcore.InfoLevel
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn", "warning":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	}

	encoderCfg := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stack",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     func(t time.Time, enc zapcore.PrimitiveArrayEncoder) { enc.AppendString(t.Format(time.RFC3339Nano)) },
		EncodeDuration: zapcore.MillisDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	var encoder zapcore.Encoder
	if strings.ToLower(cfg.Format) == "console" {
		encoder = zapcore.NewConsoleEncoder(encoderCfg)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderCfg)
	}

	// 文件输出（带滚动）
	lj := &lumberjack.Logger{
		Filename:   cfg.File.Filename,
		MaxSize:    cfg.File.MaxSizeMB,
		MaxBackups: cfg.File.MaxBackups,
		MaxAge:     cfg.File.MaxAgeDays,
		Compress:   cfg.File.Compress,
	}

	// 控制台 + 文件双写
	ws := zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stdout), zapcore.AddSync(lj))
	core := zapcore.NewCore(encoder, ws, level)

	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	return logger, nil
}
