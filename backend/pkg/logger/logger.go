package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger is a global logger instance
var Logger *zap.Logger

// Init initializes the global logger
func Init(env string) error {
	var config zap.Config

	if env == "production" {
		config = zap.NewProductionConfig()
		config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	} else {
		config = zap.NewDevelopmentConfig()
		config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	var err error
	Logger, err = config.Build()
	if err != nil {
		return err
	}

	return nil
}

// Sync flushes any buffered log entries
func Sync() {
	if Logger != nil {
		_ = Logger.Sync()
	}
}

// Get returns the global logger instance
func Get() *zap.Logger {
	if Logger == nil {
		// Fallback to a basic logger if not initialized
		logger, _ := zap.NewDevelopment()
		return logger
	}
	return Logger
}

