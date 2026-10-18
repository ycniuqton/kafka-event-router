package auth

import (
	"context"
	"log/slog"
	"net"
	"net/http"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/platform/requestid"
)

// LoginAuditor records authentication events; failures to audit are logged, never shown to users.
type LoginAuditor struct {
	recorder audit.Recorder
	log      *slog.Logger
}

// NewLoginAuditor builds the auditor.
func NewLoginAuditor(recorder audit.Recorder, log *slog.Logger) *LoginAuditor {
	return &LoginAuditor{recorder: recorder, log: log}
}

// LoggedIn records auth.login for the user.
func (a *LoginAuditor) LoggedIn(r *http.Request, u User, mode AuthMode) {
	a.record(r.Context(), r, audit.Entry{Actor: UserActor(u.ID), Action: audit.ActionAuthLogin,
		Object: audit.ObjectSession, ObjectID: string(mode)})
}

// LoginFailed records auth.login_failed (anonymous actor).
func (a *LoginAuditor) LoginFailed(r *http.Request, mode AuthMode) {
	a.record(r.Context(), r, audit.Entry{Actor: audit.Actor{Kind: audit.ActorUser}, Action: audit.ActionAuthLoginFailed,
		Object: audit.ObjectSession, ObjectID: string(mode)})
}

// LoggedOut records auth.logout.
func (a *LoginAuditor) LoggedOut(r *http.Request, p Principal) {
	a.record(r.Context(), r, audit.Entry{Actor: PrincipalActor(p), Action: audit.ActionAuthLogout, Object: audit.ObjectSession})
}

// Record writes any auth-related entry with the request's ip and id.
func (a *LoginAuditor) Record(ctx context.Context, r *http.Request, e audit.Entry) {
	a.record(ctx, r, e)
}

func (a *LoginAuditor) record(ctx context.Context, r *http.Request, e audit.Entry) {
	e.IP = ClientIP(r)
	e.RequestID = requestid.From(r.Context())
	if err := a.recorder.Record(ctx, e); err != nil {
		a.log.Error("audit auth event failed", string(logging.AttrError), err)
	}
}

// UserActor builds an audit actor for a user id.
func UserActor(id uuid.UUID) audit.Actor {
	return audit.Actor{UserID: &id, Kind: audit.ActorUser}
}

// actorKinds maps principal kinds to audit actor kinds.
var actorKinds = map[PrincipalKind]audit.ActorKind{
	PrincipalUser:      audit.ActorUser,
	PrincipalAPIToken:  audit.ActorAPIToken,
	PrincipalWorkload:  audit.ActorWorkload,
	PrincipalAnonymous: audit.ActorSystem,
}

// PrincipalActor converts a principal into an audit actor.
func PrincipalActor(p Principal) audit.Actor {
	actor := audit.Actor{Kind: actorKinds[p.Kind], Name: p.WorkloadName}
	if p.UserID != uuid.Nil {
		id := p.UserID
		actor.UserID = &id
	}
	if actor.Kind == "" {
		actor.Kind = audit.ActorSystem
	}
	return actor
}

// ClientIP returns the request's remote IP without port (RealIP middleware runs first).
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
