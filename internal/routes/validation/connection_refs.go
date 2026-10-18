package validation

import "github.com/eventrouter/router/pkg/config"

// ConnectionRef is one connection id used by a route and where it appears.
type ConnectionRef struct {
	ID   string
	Path config.Pointer
}

// ConnectionRefs collects the source, destination and DLQ connection references of a config.
type ConnectionRefs struct{}

// Of returns every non-empty reference in document order.
func (ConnectionRefs) Of(cfg *config.RouteConfig) []ConnectionRef {
	refs := []ConnectionRef{}
	if cfg.Source.ConnectionID != "" {
		refs = append(refs, ConnectionRef{ID: cfg.Source.ConnectionID, Path: config.PointerRoot.Token(config.TokenSource).Token(config.TokenConnection)})
	}
	for i, b := range cfg.Branches {
		if b.Destination.ConnectionID != "" {
			refs = append(refs, ConnectionRef{ID: b.Destination.ConnectionID, Path: config.BranchPointer(i).Token(config.TokenDestination).Token(config.TokenConnection)})
		}
	}
	if cfg.ErrorPolicy != nil && cfg.ErrorPolicy.DLQConnectionID != nil && *cfg.ErrorPolicy.DLQConnectionID != "" {
		refs = append(refs, ConnectionRef{ID: *cfg.ErrorPolicy.DLQConnectionID, Path: DLQConnectionPointer})
	}
	return refs
}

// IDs returns the distinct ids of refs.
func (ConnectionRefs) IDs(refs []ConnectionRef) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, r := range refs {
		if !seen[r.ID] {
			seen[r.ID] = true
			out = append(out, r.ID)
		}
	}
	return out
}

// DocumentMember names route document members without a pkg/config pointer token.
type DocumentMember string

const (
	MemberErrorPolicy   DocumentMember = "errorPolicy"
	MemberDLQConnection DocumentMember = "dlqConnectionId"
	MemberTopic         DocumentMember = "topic"
)

// DLQConnectionPointer addresses errorPolicy.dlqConnectionId.
var DLQConnectionPointer = config.PointerRoot.Key(string(MemberErrorPolicy)).Key(string(MemberDLQConnection))
