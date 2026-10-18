package audit

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// ExportEnvKey names the environment keys of the continuous audit export (read by cmd/api only).
type ExportEnvKey string

const (
	EnvExportSink     ExportEnvKey = "ROUTER_AUDIT_EXPORT_SINK"
	EnvExportTarget   ExportEnvKey = "ROUTER_AUDIT_EXPORT_TARGET"
	EnvExportInterval ExportEnvKey = "ROUTER_AUDIT_EXPORT_INTERVAL"
	EnvExportBatch    ExportEnvKey = "ROUTER_AUDIT_EXPORT_BATCH"
	EnvExportTimeout  ExportEnvKey = "ROUTER_AUDIT_EXPORT_TIMEOUT"
)

// ExportDefault holds the defaults of the continuous export (design flow 5.9).
type ExportDefault time.Duration

const (
	DefaultExportInterval ExportDefault = ExportDefault(10 * time.Second)
	DefaultExportTimeout  ExportDefault = ExportDefault(10 * time.Second)
	MaxExportBackoff      ExportDefault = ExportDefault(5 * time.Minute)
)

// ExportBatchSize is how many entries one sink call carries.
type ExportBatchSize int

const DefaultExportBatch ExportBatchSize = 500

// ExportSettings configures the continuous SIEM export.
type ExportSettings struct {
	Sink       ExportSinkKind
	Target     string
	Interval   time.Duration
	Timeout    time.Duration
	MaxBackoff time.Duration
	Batch      int
}

// Enabled reports whether a real sink is configured.
func (s ExportSettings) Enabled() bool {
	return s.Sink != ExportSinkNone && s.Sink != ""
}

// ExportSettingsLoader reads ExportSettings from the environment.
type ExportSettingsLoader struct {
	Lookup func(string) (string, bool)
}

// NewExportSettingsLoader reads from the process environment.
func NewExportSettingsLoader() ExportSettingsLoader {
	return ExportSettingsLoader{Lookup: os.LookupEnv}
}

// Load returns the settings; malformed durations or sizes are errors.
func (l ExportSettingsLoader) Load() (ExportSettings, error) {
	interval, err := l.duration(EnvExportInterval, time.Duration(DefaultExportInterval))
	if err != nil {
		return ExportSettings{}, err
	}
	timeout, err := l.duration(EnvExportTimeout, time.Duration(DefaultExportTimeout))
	if err != nil {
		return ExportSettings{}, err
	}
	batch, err := l.size(EnvExportBatch, int(DefaultExportBatch))
	if err != nil {
		return ExportSettings{}, err
	}
	return ExportSettings{Sink: ExportSinkKind(l.str(EnvExportSink, string(ExportSinkNone))), Target: l.str(EnvExportTarget, ""),
		Interval: interval, Timeout: timeout, MaxBackoff: time.Duration(MaxExportBackoff), Batch: batch}, nil
}

func (l ExportSettingsLoader) str(key ExportEnvKey, def string) string {
	if v, ok := l.Lookup(string(key)); ok && v != "" {
		return v
	}
	return def
}

func (l ExportSettingsLoader) duration(key ExportEnvKey, def time.Duration) (time.Duration, error) {
	raw := l.str(key, "")
	if raw == "" {
		return def, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("audit: %s must be a positive duration", key)
	}
	return d, nil
}

func (l ExportSettingsLoader) size(key ExportEnvKey, def int) (int, error) {
	raw := l.str(key, "")
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("audit: %s must be a positive number", key)
	}
	return n, nil
}
