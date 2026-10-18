package enginebind

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/sandbox/boundary"
)

// ScopeFormats lists the record formats of a route so the sandbox picks its boundary marshaller.
type ScopeFormats struct{}

// Of returns the source value/key formats and every branch destination format.
func (ScopeFormats) Of(cfg *config.RouteConfig) []boundary.Format {
	if cfg == nil {
		return nil
	}
	out := []boundary.Format{boundary.Format(cfg.Source.Format.Type)}
	if cfg.Source.KeyFormat != nil {
		out = append(out, boundary.Format(cfg.Source.KeyFormat.Type))
	}
	for _, b := range cfg.Branches {
		if b.Destination.Format != nil {
			out = append(out, boundary.Format(b.Destination.Format.Type))
		}
	}
	return out
}
