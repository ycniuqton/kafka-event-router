package enginebind

import (
	"context"
	"time"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/sandbox/host"
)

// Services adapts a ctxhelpers.Set to host.HostServices.
type Services struct {
	Set ctxhelpers.Set
}

func (s Services) Masker() host.Masker               { return masker{s.Set.Mask} }
func (s Services) Hasher() host.Hasher               { return s.Set.Hash }
func (s Services) DateFormatter() host.DateFormatter { return dateFormatter{s.Set.Date} }
func (s Services) Lookups() host.LookupResolver      { return s.Set.Lookup }

// masker maps sandbox mask styles onto the route-config mask styles.
type masker struct {
	m ctxhelpers.Masker
}

func (m masker) Mask(v any, style host.MaskStyle) (any, error) {
	return m.m.Mask(v, config.MaskStyle(style))
}

// dateFormatter maps layout names onto ctxhelpers date layouts.
type dateFormatter struct {
	f ctxhelpers.DateFormatter
}

func (d dateFormatter) Format(t time.Time, layout, tz string) (string, error) {
	return d.f.Format(t, ctxhelpers.DateLayout(layout), tz)
}

// Binder builds per-team Services from a base helper set; the salt from the sandbox's
// SaltProvider replaces the base hasher so ctx.hash and the hash recipe agree.
type Binder struct {
	Base ctxhelpers.Set
}

func (b Binder) Bind(_ context.Context, teamID string, salt []byte) (host.HostServices, error) {
	set := b.Base.WithDefaults()
	set.Hash = ctxhelpers.NewSaltedHasher(ctxhelpers.StaticSaltProvider{Value: salt}, teamID)
	return Services{Set: set}, nil
}
