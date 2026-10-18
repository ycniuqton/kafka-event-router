package apitoken

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/store"
)

// Action names the audited API-token operations (registered in access-control's catalog at wiring).
const (
	ActionCreated audit.Action = "api_token.created"
	ActionRevoked audit.Action = "api_token.revoked"
)

// Field is the JSON Pointer of a create-request member.
type Field string

const (
	FieldName    Field = "/name"
	FieldScopes  Field = "/scopes"
	FieldExpires Field = "/expires_in_days"
)

// ErrorCode is the machine-readable code of a field error.
type ErrorCode string

const (
	CodeRequired ErrorCode = "required"
	CodeTooLong  ErrorCode = "too_long"
	CodeUnknown  ErrorCode = "unknown_scope"
	CodeRange    ErrorCode = "out_of_range"
	CodeLimit    ErrorCode = "limit_reached"
)

// Policy bounds token lifetimes and counts.
type Policy struct {
	DefaultTTLDays int
	MaxTTLDays     int
	MaxActive      int
	MaxNameRunes   int
}

// PolicyDefaults groups the default policy values.
type PolicyDefaults int

const (
	DefaultTTLDays      PolicyDefaults = 90
	DefaultMaxTTLDays   PolicyDefaults = 365
	DefaultMaxActive    PolicyDefaults = 50
	DefaultMaxNameRunes PolicyDefaults = 100
	HoursPerDay         PolicyDefaults = 24
)

// DefaultPolicy returns 90-day tokens (max 365), at most 50 active per user.
func DefaultPolicy() Policy {
	return Policy{DefaultTTLDays: int(DefaultTTLDays), MaxTTLDays: int(DefaultMaxTTLDays),
		MaxActive: int(DefaultMaxActive), MaxNameRunes: int(DefaultMaxNameRunes)}
}

var (
	// ErrInteractiveOnly forbids managing tokens with a token (no self-escalation).
	ErrInteractiveOnly = problem.NewError(problem.KindForbidden, "API tokens can only be managed from a signed-in browser session.")
	// ErrNotFound is returned for unknown, foreign or already revoked token ids.
	ErrNotFound = problem.NewError(problem.KindNotFound, "This API token does not exist or was already revoked.")
)

// CreateRequest is the body of POST /api/v1/auth/tokens.
type CreateRequest struct {
	Name          string            `json:"name"`
	Scopes        []auth.TokenScope `json:"scopes"`
	ExpiresInDays *int              `json:"expires_in_days"`
}

// Created is returned once at creation; Token holds the secret and is never retrievable again.
type Created struct {
	View
	Token string `json:"token"`
}

// View is a token as listed by the API.
type View struct {
	Token
	Status Status `json:"status"`
}

// Manager is the business entry point for API tokens.
type Manager struct {
	repo   Repository
	tx     store.TxManager
	scopes *auth.ScopeCatalog
	policy Policy
	codec  SecretCodec
	now    auth.Clock
}

// ManagerDeps groups the Manager's collaborators.
type ManagerDeps struct {
	Repo   Repository
	Tx     store.TxManager
	Scopes *auth.ScopeCatalog
	Policy Policy
	Now    auth.Clock
}

// NewManager builds the manager.
func NewManager(d ManagerDeps) *Manager {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Manager{repo: d.Repo, tx: d.Tx, scopes: d.Scopes, policy: d.Policy, now: d.Now}
}

// List returns the caller's active (non-revoked) tokens.
func (m *Manager) List(ctx context.Context, p auth.Principal) ([]View, error) {
	if err := m.requireInteractive(p); err != nil {
		return nil, err
	}
	tokens, err := m.repo.ListActive(ctx, p.UserID)
	if err != nil {
		return nil, err
	}
	views := make([]View, 0, len(tokens))
	for _, t := range tokens {
		views = append(views, m.view(t))
	}
	return views, nil
}

// Create validates the request, stores the hash and audits api_token.created in one transaction.
func (m *Manager) Create(ctx context.Context, p auth.Principal, req CreateRequest) (Created, error) {
	if err := m.requireInteractive(p); err != nil {
		return Created{}, err
	}
	req.Name = strings.TrimSpace(req.Name)
	if err := m.validate(req); err != nil {
		return Created{}, err
	}
	secret := m.codec.Generate()
	var stored Token
	err := m.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := m.checkLimit(ctx, p.UserID); err != nil {
			return err
		}
		var err error
		if stored, err = m.repo.Insert(ctx, m.newToken(p, req, secret)); err != nil {
			return err
		}
		return m.record(ctx, ActionCreated, stored, nil, m.view(stored))
	})
	if err != nil {
		return Created{}, err
	}
	return Created{View: m.view(stored), Token: secret.Value}, nil
}

// Revoke revokes one of the caller's tokens and audits api_token.revoked.
func (m *Manager) Revoke(ctx context.Context, p auth.Principal, id uuid.UUID) error {
	if err := m.requireInteractive(p); err != nil {
		return err
	}
	return m.tx.WithinTx(ctx, func(ctx context.Context) error {
		revoked, err := m.repo.Revoke(ctx, p.UserID, id, m.now())
		if errors.Is(err, store.ErrNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return m.record(ctx, ActionRevoked, revoked, m.view(revoked), nil)
	})
}

// Scopes lists the scopes a token may be created with.
func (m *Manager) Scopes() []auth.TokenScope {
	return m.scopes.Scopes()
}

func (m *Manager) requireInteractive(p auth.Principal) error {
	if p.Kind != auth.PrincipalUser {
		return ErrInteractiveOnly
	}
	return nil
}

func (m *Manager) validate(req CreateRequest) error {
	v := api.NewValidationError()
	if req.Name == "" {
		v.Add(string(FieldName), string(CodeRequired), "Give the token a name, e.g. ci-deploy.")
	}
	if utf8.RuneCountInString(req.Name) > m.policy.MaxNameRunes {
		v.Add(string(FieldName), string(CodeTooLong), fmt.Sprintf("Use a shorter name (at most %d characters).", m.policy.MaxNameRunes))
	}
	if len(req.Scopes) == 0 {
		v.Add(string(FieldScopes), string(CodeRequired), "Pick at least one scope: read, write or admin.")
	}
	for i, s := range req.Scopes {
		if !m.scopes.Known(s) {
			v.Add(string(FieldScopes)+"/"+strconv.Itoa(i), string(CodeUnknown), "Unknown scope. Use read, write or admin.")
		}
	}
	if req.ExpiresInDays != nil && (*req.ExpiresInDays < 1 || *req.ExpiresInDays > m.policy.MaxTTLDays) {
		v.Add(string(FieldExpires), string(CodeRange), fmt.Sprintf("Choose an expiry between 1 and %d days.", m.policy.MaxTTLDays))
	}
	return v.OrNil()
}

func (m *Manager) checkLimit(ctx context.Context, userID uuid.UUID) error {
	n, err := m.repo.CountActive(ctx, userID)
	if err != nil || n < m.policy.MaxActive {
		return err
	}
	return api.NewValidationError().Add(string(FieldName), string(CodeLimit),
		fmt.Sprintf("You already have %d active tokens. Revoke one you no longer use, then retry.", m.policy.MaxActive))
}

func (m *Manager) newToken(p auth.Principal, req CreateRequest, secret Secret) Token {
	days := m.policy.DefaultTTLDays
	if req.ExpiresInDays != nil {
		days = *req.ExpiresInDays
	}
	expires := m.now().Add(time.Duration(days) * time.Duration(HoursPerDay) * time.Hour).UTC()
	return Token{UserID: p.UserID, Name: req.Name, Prefix: secret.Prefix, Scopes: req.Scopes, ExpiresAt: &expires, Hash: secret.Hash}
}

func (m *Manager) view(t Token) View {
	return View{Token: t, Status: t.StatusAt(m.now())}
}

func (m *Manager) record(ctx context.Context, action audit.Action, t Token, before, after any) error {
	scope, ok := audit.FromContext(ctx)
	if !ok {
		return nil
	}
	return scope.Record(ctx, audit.Change{Action: action, Object: audit.ObjectAPIToken, ObjectID: t.ID.String(), Before: before, After: after})
}
