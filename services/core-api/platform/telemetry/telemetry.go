package telemetry

import (
	"errors"
	"log/slog"
	"os"
	"strings"
)

// Config is the telemetry configuration for one process.
type Config struct {
	ServiceName string
	Environment string
	// Metrics turns the registry on. Off by default, and the reason is written
	// next to the setting: nothing in this repository needs a collector, and a
	// gate that started needing one would stop being runnable offline.
	Metrics bool
	// MetricsAddr is where the exposition is served when Metrics is on. Loopback
	// by default.
	MetricsAddr string
}

func NewLogger(config Config) *slog.Logger {
	level := slog.LevelInfo
	if config.Environment == "development" {
		level = slog.LevelDebug
	}
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(handler).With("service", config.ServiceName, "environment", config.Environment)
}

// New builds everything one process needs: a logger and a registry. The registry
// is always returned, because a nil registry is a nil check at every call site,
// and a disabled one costs one atomic load per sample.
func New(config Config) (*slog.Logger, *Metrics) {
	return NewLogger(config), NewMetrics(config.ServiceName, config.Metrics)
}

// ConfigFromEnvironment reads APP_ENV, TELEMETRY_METRICS_ENABLED and
// TELEMETRY_METRICS_ADDR.
//
// No credential is read here, and no configuration value is logged. A metrics
// endpoint is an inventory of a service's traffic; the address it is served on is
// configuration and belongs in a startup line, and nothing else about the
// telemetry configuration is interesting enough to print.
func ConfigFromEnvironment(getenv func(string) string) Config {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	read := func(name, fallback string) string {
		if value := strings.TrimSpace(getenv(name)); value != "" {
			return value
		}
		return fallback
	}
	environment := read("APP_ENV", "development")
	return Config{
		ServiceName: read("TELEMETRY_SERVICE_NAME", "dawha"),
		Environment: environment,
		Metrics:     ExporterConfigFromEnvironment(getenv).Enabled,
		MetricsAddr: read("TELEMETRY_METRICS_ADDR", DefaultExporterAddr),
	}
}

// Describe renders the telemetry configuration for a startup log. It reports
// whether metrics are on and where; it does not report anything a caller might
// have put in an environment variable.
func (c Config) Describe() string {
	if c.Metrics {
		return "metrics=enabled addr=" + c.MetricsAddr
	}
	return "metrics=disabled"
}

// ErrExporterUnavailable is a metrics listener that could not bind. It is
// distinct from every other startup error here because the right response is
// specific: the deployment asked for an exporter and did not get one, and a
// service that quietly runs without it is a service whose dashboards are empty
// for reasons nobody is told about.
var ErrExporterUnavailable = errors.New("the metrics exporter could not start")
