package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/eventrouter/router/internal/audit"
)

// RotationPage bounds how many envelopes one rotation query loads.
type RotationPage int

const DefaultRotationPage RotationPage = 100

// KeyRotator re-wraps every envelope still wrapped by an old master key (FR-29); the job and CLI use it.
type KeyRotator interface {
	Run(ctx context.Context) (RotationReport, error)
	Usage(ctx context.Context) ([]KeyUsage, error)
}

// RotationReport is the outcome of one rotation run; Raced counts envelopes replaced concurrently.
type RotationReport struct {
	Rotated    int      `json:"count"`
	Raced      int      `json:"raced,omitempty"`
	FromKeyIDs []string `json:"fromKeyIds"`
	ToKeyID    string   `json:"toKeyId"`
	Failed     string   `json:"failedRef,omitempty"`
}

func (r *RotationReport) count(fromKeyID string, applied bool) {
	if !applied {
		r.Raced++
		return
	}
	r.Rotated++
	if !slices.Contains(r.FromKeyIDs, fromKeyID) {
		r.FromKeyIDs = append(r.FromKeyIDs, fromKeyID)
	}
}

// Rotator unwraps each stale data key with its recorded key id and wraps it with the current master key;
// payload ciphertext is never touched. A failure stops the run; reruns are idempotent.
type Rotator struct {
	provider Provider
	repo     RotationRepository
	recorder audit.Recorder
	actor    audit.Actor
	page     int
	keys     KeyMaterial
}

// RotatorDeps groups the collaborators of a Rotator.
type RotatorDeps struct {
	Provider Provider
	Repo     RotationRepository
	Recorder audit.Recorder
	Actor    audit.Actor
	PageSize int
}

// NewRotator builds the rotator (page size ≤ 0 uses DefaultRotationPage).
func NewRotator(d RotatorDeps) *Rotator {
	page := d.PageSize
	if page <= 0 {
		page = int(DefaultRotationPage)
	}
	return &Rotator{provider: d.Provider, repo: d.Repo, recorder: d.Recorder, actor: d.Actor, page: page}
}

// Run re-wraps all stale envelopes and records secrets.rotated when anything changed.
func (r *Rotator) Run(ctx context.Context) (RotationReport, error) {
	report := RotationReport{ToKeyID: r.provider.KeyID(), FromKeyIDs: []string{}}
	err := r.sweep(ctx, &report)
	if report.Rotated == 0 {
		return report, err
	}
	return report, errors.Join(err, r.record(ctx, report))
}

// Usage lists how many envelopes each master key wraps.
func (r *Rotator) Usage(ctx context.Context) ([]KeyUsage, error) {
	return r.repo.KeyUsage(ctx)
}

func (r *Rotator) sweep(ctx context.Context, report *RotationReport) error {
	after := Ref("")
	for {
		page, err := r.repo.ListStale(ctx, r.provider.Kind(), report.ToKeyID, after, r.page)
		if err != nil || len(page) == 0 {
			return err
		}
		for _, e := range page {
			if err := r.rewrap(ctx, e, report); err != nil {
				report.Failed = string(e.Ref)
				return fmt.Errorf("secrets: rotate %s: %w", e.Ref, err)
			}
		}
		after = page[len(page)-1].Ref
	}
}

func (r *Rotator) rewrap(ctx context.Context, e Envelope, report *RotationReport) error {
	dek, err := r.provider.UnwrapKey(ctx, e.WrappedKey, e.KeyID)
	if err != nil {
		return ErrUnavailable.Wrap(err)
	}
	defer r.keys.Zero(dek)
	wrapped, err := r.provider.WrapKey(ctx, dek)
	if err != nil {
		return ErrUnavailable.Wrap(err)
	}
	applied, err := r.repo.Rewrap(ctx, Rewrap{Ref: e.Ref, FromKeyID: e.KeyID, FromWrapped: e.WrappedKey,
		ToKeyID: report.ToKeyID, ToWrapped: wrapped})
	if err != nil {
		return err
	}
	report.count(e.KeyID, applied)
	return nil
}

func (r *Rotator) record(ctx context.Context, report RotationReport) error {
	diff, err := json.Marshal(report)
	if err != nil {
		return err
	}
	return r.recorder.Record(ctx, audit.Entry{Actor: r.actor, Action: audit.ActionSecretsRotated,
		Object: audit.ObjectSecret, ObjectID: report.ToKeyID, ObjectLabel: report.ToKeyID, Diff: diff})
}
