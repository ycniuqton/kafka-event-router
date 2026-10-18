package main

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth/oidc"
	"github.com/eventrouter/router/internal/platform/metrics"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/scheduler"
	"github.com/eventrouter/router/internal/secrets"
)

// PlatformP3Wiring holds platform-core's P3 components: master-key rotation, the real-time audit stream
// (other features attach sinks through AuditStream.Add) and the OIDC group → role mapping.
type PlatformP3Wiring struct {
	Rotator     *secrets.Rotator
	AuditStream *audit.Stream
}

// PlatformP3Metric names metrics registered by this wiring.
type PlatformP3Metric string

const MetricRotationLeader PlatformP3Metric = "router_secrets_rotation_leader"

// PlatformP3Actor names the audit actor of the periodic rotation job.
type PlatformP3Actor string

const RotationJobActor PlatformP3Actor = "job:secrets-rotation"

// buildPlatformP3 runs after buildAccess: it wraps access-control's login hook and uses its audit catalog.
func (w *Wiring) buildPlatformP3(reg *metrics.Registry) func(context.Context) error {
	return func(ctx context.Context) error {
		p := &PlatformP3Wiring{}
		if err := p.buildStream(ctx, w, reg); err != nil {
			return err
		}
		if err := p.buildClaimMapping(w); err != nil {
			return err
		}
		p.buildRotation(ctx, w, reg)
		w.PlatformP3 = p
		return nil
	}
}

// buildStream attaches the configured sinks and makes the recorder publish committed entries.
func (p *PlatformP3Wiring) buildStream(ctx context.Context, w *Wiring, reg *metrics.Registry) error {
	cfg := w.Deps.Config.AuditStream
	sinks, err := audit.NewSinkRegistry().Build(cfg, w.Deps.Log)
	if err != nil {
		return err
	}
	p.AuditStream = audit.NewStream(ctx, cfg.Buffer, w.Deps.Log, audit.NewPromStreamObserver(reg.Registerer()))
	for _, s := range sinks {
		p.AuditStream.Add(s)
	}
	if rec, ok := w.Deps.Audit.(*audit.PostgresRecorder); ok {
		rec.StreamTo(p.AuditStream)
	}
	return nil
}

// buildClaimMapping wraps the current login hook with the OIDC claim → role binding mapping when configured.
func (p *PlatformP3Wiring) buildClaimMapping(w *Wiring) error {
	cfg := w.Deps.Config
	if !cfg.Auth.OIDC.RoleClaims.Enabled() {
		return nil
	}
	mapping, err := oidc.ParseRoleMapping(cfg.Auth.OIDC.RoleClaims.Mappings, rbac.Roles{}.Valid)
	if err != nil {
		return err
	}
	assignable, err := rbac.ParseRoleSet(cfg.Access.AssignableRoles)
	if err != nil {
		return err
	}
	db := w.Deps.DB
	bindings := rbac.NewBindingService(rbac.BindingServiceDeps{Tx: w.Tx, Bindings: rbac.NewPostgresBindingStore(db),
		Subjects: rbac.NewPostgresSubjectLoader(db), Guard: rbac.NewAdminGuard(rbac.NewPostgresAdminStore(db)),
		Assignable: assignable, Trail: rbac.NewAuditTrail(w.Deps.Audit)})
	granter := oidc.NewBindingGranter(w.Identity.Teams, bindings, w.Deps.Log)
	w.Logins.SetHook(oidc.NewClaimRoleHook(w.Logins.Hook(), mapping, granter))
	return nil
}

// buildRotation registers secrets.rotated and starts the leader-only rotation job when an interval is set.
func (p *PlatformP3Wiring) buildRotation(ctx context.Context, w *Wiring, reg *metrics.Registry) {
	if w.Access != nil && w.Access.Catalog != nil {
		w.Access.Catalog.Register(audit.ActionSecretsRotated, audit.ActionMeta{Category: audit.CategoryAdmin, Sensitive: true})
	}
	cfg := w.Deps.Config.Secrets.Rotation
	p.Rotator = w.SecretParts.Rotator(w.Deps.Audit, audit.Actor{Kind: audit.ActorSystem, Name: string(RotationJobActor)}, cfg)
	if !cfg.Enabled() {
		return
	}
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{Name: string(MetricRotationLeader)})
	_ = reg.Registerer().Register(gauge)
	leader := scheduler.PgLeaderElector{Pool: w.Deps.DB.Pool(), Key: scheduler.LockKey(secrets.RotationJobLockKey), Gauge: gauge}
	job := secrets.RotationJob{Rotator: p.Rotator, Leader: leader, Interval: cfg.Interval,
		FirstDelay: time.Duration(secrets.RotationFirstDelay), Log: w.Deps.Log}
	go job.Run(ctx)
}
