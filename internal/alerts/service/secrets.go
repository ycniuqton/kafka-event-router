package service

import (
	"context"
	"strings"

	"github.com/eventrouter/router/internal/alerts"
	"github.com/eventrouter/router/internal/secrets"
)

// ChannelSecretStore turns channel inputs into stored refs: webhook URLs go to the secrets Manager and
// only their refs are kept on the rule; an input without a secret keeps the previous ref of the same slot.
type ChannelSecretStore struct {
	Secrets secrets.Manager
}

// Resolved is the outcome of Resolve: the refs to store and the refs no longer used.
type Resolved struct {
	Channels []alerts.ChannelRef
	Orphaned []string
}

// Resolve stores new secrets and maps inputs to refs; old holds the rule's current channels (nil on create).
func (s ChannelSecretStore) Resolve(ctx context.Context, inputs []alerts.ChannelInput, old []alerts.ChannelRef) (Resolved, error) {
	out := Resolved{Channels: make([]alerts.ChannelRef, 0, len(inputs))}
	kept := map[string]bool{}
	for i, in := range inputs {
		ref, err := s.one(ctx, i, in, s.previous(old, i, in.Type))
		if err != nil {
			return Resolved{}, err
		}
		kept[ref.SecretRef] = true
		out.Channels = append(out.Channels, ref)
	}
	for _, o := range old {
		if o.SecretRef != "" && !kept[o.SecretRef] {
			out.Orphaned = append(out.Orphaned, o.SecretRef)
		}
	}
	return out, nil
}

func (s ChannelSecretStore) one(ctx context.Context, i int, in alerts.ChannelInput, prev alerts.ChannelRef) (alerts.ChannelRef, error) {
	ref := alerts.ChannelRef{Type: in.Type}
	if !in.Type.UsesSecret() {
		ref.Target = strings.Join(alerts.EmailList(in.Target).Addresses(), string(alerts.RecipientSeparator))
		return ref, nil
	}
	if in.Secret == nil {
		if prev.SecretRef == "" {
			return alerts.ChannelRef{}, alerts.ErrMissingSecret.At(alerts.ChannelPath(i, string(alerts.FieldSecret)))
		}
		ref.SecretRef = prev.SecretRef
		return ref, nil
	}
	plain := []byte(strings.TrimSpace(*in.Secret))
	if prev.SecretRef != "" {
		ref.SecretRef = prev.SecretRef
		return ref, s.Secrets.Replace(ctx, secrets.Ref(prev.SecretRef), plain)
	}
	created, err := s.Secrets.Put(ctx, plain)
	ref.SecretRef = string(created)
	return ref, err
}

func (ChannelSecretStore) previous(old []alerts.ChannelRef, i int, t alerts.ChannelType) alerts.ChannelRef {
	if i < len(old) && old[i].Type == t {
		return old[i]
	}
	return alerts.ChannelRef{}
}

// Forget deletes orphaned secrets (best effort, after the rule change committed).
func (s ChannelSecretStore) Forget(ctx context.Context, refs []string) {
	for _, r := range refs {
		_ = s.Secrets.Delete(ctx, secrets.Ref(r))
	}
}

// Inputs converts stored refs back to inputs that keep every secret (PATCH without channels).
func (ChannelSecretStore) Inputs(refs []alerts.ChannelRef) []alerts.ChannelInput {
	out := make([]alerts.ChannelInput, 0, len(refs))
	for _, r := range refs {
		out = append(out, alerts.ChannelInput{Type: r.Type, Target: r.Target})
	}
	return out
}
