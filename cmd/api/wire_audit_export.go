package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventrouter/router/internal/audit"
	audithttp "github.com/eventrouter/router/internal/audit/http"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/logging"
)

// AuditExportLog names the log messages of the audit export wiring.
type AuditExportLog string

const (
	LogAuditExportDisabled AuditExportLog = "audit export: continuous export disabled by a configuration error"
	LogAuditExportStarted  AuditExportLog = "audit export: continuous export started"
	LogAttrExportSink      AuditExportLog = "sink"
)

// auditExportWiring builds the audit file export, its status endpoint and the leader-only SIEM job once.
type auditExportWiring struct {
	once   sync.Once
	module api.Module
}

var auditExportFeature = &auditExportWiring{}

// auditExportModule returns GET /audit-log/export and /audit-log/export/status and starts the SIEM export job.
func (s *apiService) auditExportModule() api.Module {
	auditExportFeature.once.Do(func() { auditExportFeature.module = auditExportFeature.build(s) })
	return auditExportFeature.module
}

func (a *auditExportWiring) build(s *apiService) api.Module {
	w := s.wiring
	settings, err := audit.NewExportSettingsLoader().Load()
	if err != nil {
		w.Deps.Log.Error(string(LogAuditExportDisabled), string(logging.AttrError), err)
		settings = audit.ExportSettings{Sink: audit.ExportSinkNone}
	}
	reader := audit.NewPostgresReader(w.Deps.DB)
	checkpoints := audit.NewPostgresCheckpointStore(w.Deps.DB)
	if settings.Enabled() {
		a.start(s, settings, reader, checkpoints)
	}
	monitor := audit.ExportMonitor{Settings: settings, Checkpoints: checkpoints, Source: reader}
	return audithttp.NewExportModule(audithttp.Deps{Audit: w.Access.Audit, Responder: w.Responder, Pages: w.Pages,
		Recorder: w.Deps.Audit, Export: monitor, Now: time.Now})
}

// start builds the configured sink and runs the job on the scheduler leader until SIGINT/SIGTERM.
func (a *auditExportWiring) start(s *apiService, st audit.ExportSettings, reader *audit.PostgresReader, cps *audit.PostgresCheckpointStore) {
	w := s.wiring
	sink, err := audit.NewExportSinkRegistry().Build(st.Sink, audit.ExportSinkDeps{Target: st.Target, Timeout: st.Timeout,
		Client: &http.Client{Timeout: st.Timeout}})
	if err != nil {
		w.Deps.Log.Error(string(LogAuditExportDisabled), string(logging.AttrError), err)
		return
	}
	metrics := audit.NewPromExportMetrics()
	for _, col := range metrics.Collectors() {
		var dup prometheus.AlreadyRegisteredError
		if err := s.registry.Registerer().Register(col); err != nil && !errors.As(err, &dup) {
			w.Deps.Log.Error(string(LogAuditExportDisabled), string(logging.AttrError), err)
		}
	}
	job := &audit.AuditExportJob{Leader: s.runtimeLeaderGate(), Sink: sink, Checkpoints: cps, Source: reader, Metrics: metrics,
		Interval: st.Interval, Batch: st.Batch, Backoff: &audit.Backoff{Base: st.Interval, Max: st.MaxBackoff}, Now: time.Now,
		Log: w.Deps.Log}
	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go job.Run(ctx)
	w.Deps.Log.Info(string(LogAuditExportStarted), string(LogAttrExportSink), string(st.Sink))
}
