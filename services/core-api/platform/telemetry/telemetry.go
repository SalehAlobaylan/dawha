package telemetry

import (
	"log/slog"
	"os"
)

type Config struct {
	ServiceName string
	Environment string
}

func NewLogger(config Config) *slog.Logger {
	level := slog.LevelInfo
	if config.Environment == "development" {
		level = slog.LevelDebug
	}
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(handler).With("service", config.ServiceName, "environment", config.Environment)
}
