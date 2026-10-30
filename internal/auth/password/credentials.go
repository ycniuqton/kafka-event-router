package password

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/store"
)

// Credentials is the Manager business code uses for passwords: assign (seed, admin) and self-service change.
type Credentials struct {
	store   CredentialStore
	hashers *HasherSet
	policy  Policy
	gen     Generator
	tx      store.TxManager
	trail   *Trail
}

// CredentialsDeps groups the collaborators of Credentials.
type CredentialsDeps struct {
	Store   CredentialStore
	Hashers *HasherSet
	Policy  Policy
	Gen     Generator
	Tx      store.TxManager
	Trail   *Trail
}

// NewCredentials builds the manager.
func NewCredentials(d CredentialsDeps) *Credentials {
	return &Credentials{store: d.Store, hashers: d.Hashers, policy: d.Policy, gen: d.Gen, tx: d.Tx, trail: d.Trail}
}

// Hashers returns the hashing schemes (shared with the login flow).
func (c *Credentials) Hashers() *HasherSet { return c.hashers }

// Policy returns the password policy (shown on the login page).
func (c *Credentials) Policy() Policy { return c.policy }

// Assignment is the result of setting a password: Generated is non-empty only when the server chose it.
type Assignment struct {
	Generated string
}

// Assign sets userID's password to plain, or to a generated one when plain is empty (runs in ctx's tx).
func (c *Credentials) Assign(ctx context.Context, userID uuid.UUID, plain string, mustChange bool) (Assignment, error) {
	var out Assignment
	if plain == "" {
		generated, err := c.gen.New()
		if err != nil {
			return out, err
		}
		plain, out.Generated = generated, generated
	} else if err := c.policy.Check(plain); err != nil {
		return out, ErrWeakPassword.At(PathPassword)
	}
	hash, err := c.hashers.Hash(plain)
	if err != nil {
		return out, err
	}
	if err := c.store.EnsureSubject(ctx, userID); err != nil {
		return out, err
	}
	return out, c.store.Set(ctx, userID, hash, mustChange)
}

// Change replaces the caller's own password after checking the current one; it clears the forced change.
func (c *Credentials) Change(ctx context.Context, userID uuid.UUID, current, next string) error {
	hash, err := c.checkChange(ctx, userID, current, next)
	if err != nil {
		return err
	}
	return c.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := c.store.Set(ctx, userID, hash, false); err != nil {
			return err
		}
		return c.trail.Record(ctx, audit.Change{Action: audit.ActionPasswordChanged, Object: audit.ObjectUser, ObjectID: userID.String()})
	})
}

// checkChange validates the change and returns the new hash.
func (c *Credentials) checkChange(ctx context.Context, userID uuid.UUID, current, next string) (string, error) {
	cred, err := c.store.Get(ctx, userID)
	if errors.Is(err, store.ErrNotFound) {
		return "", ErrInvalidCurrentPassword
	}
	if err != nil {
		return "", err
	}
	ok, err := c.hashers.Verify(cred.Hash, current)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrInvalidCurrentPassword
	}
	if err := c.policy.Check(next); err != nil {
		return "", err
	}
	if next == current {
		return "", ErrPasswordReused
	}
	return c.hashers.Hash(next)
}

// Seeded reports whether any password credential exists.
func (c *Credentials) Seeded(ctx context.Context) (bool, error) {
	return c.store.Any(ctx)
}
