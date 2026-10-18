package main

import (
	"time"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/platform/ratelimit"
	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/internal/preview/gateway"
	"github.com/eventrouter/router/internal/rbac"
)

// previewModule builds the live-preview gateway (/api/v1/preview/*) forwarding to cmd/preview;
// a configuration error disables the module (logged) instead of failing the API.
func (w *Wiring) previewModule() api.Module {
	settings, err := preview.NewSettingsLoader().Load(w.Deps.Config.Environment)
	if err == nil {
		var deps gateway.Deps
		deps, err = w.previewDeps(settings)
		if err == nil {
			return gateway.NewModule(deps)
		}
	}
	w.Deps.Log.Error(string(preview.LogGatewayDisabled), string(logging.AttrError), err)
	return api.Modules{}
}

// previewDeps composes authorization, masking (access-control SampleVisibility), the per-user rate limit
// and the test-topic audit action.
func (w *Wiring) previewDeps(settings preview.Settings) (gateway.Deps, error) {
	authz, err := w.Authorizers.Build(w.Deps.Config.Auth.Authz)
	if err != nil {
		return gateway.Deps{}, err
	}
	limiter, err := ratelimit.NewRegistry().Build(w.Deps.Config.RateLimit, w.Deps.Redis)
	if err != nil {
		return gateway.Deps{}, err
	}
	var samples rbac.SampleAccess
	if w.Access != nil {
		samples = w.Access.Samples
		w.Access.Catalog.Register(gateway.ActionTestTopicWritten, audit.ActionMeta{Category: audit.CategoryChange})
	}
	rule := ratelimit.Rule{PerMinute: settings.RatePerMinute, Burst: settings.RateBurst}
	return gateway.Deps{
		Client:     gateway.NewPreviewClient(settings.URL, nil, preview.NewDirectiveCodec(settings.DirectiveSecret, time.Now)),
		Authorizer: gateway.PreviewAuthorizer{Authz: authz, Samples: samples},
		Responder:  w.Responder,
		Limiter:    gateway.NewPreviewRateLimiter(limiter, rule, w.Deps.Log),
		TestTopics: gateway.TestTopicAuditor{Recorder: w.Deps.Audit},
	}, nil
}

// previewSampleMasker is the live-preview masking port for AI assist prompts (LP-202): transform-studio
// asks it before any sample value reaches an LLM provider.
func (w *Wiring) previewSampleMasker() (gateway.AssistSampleMasker, error) {
	authz, err := w.Authorizers.Build(w.Deps.Config.Auth.Authz)
	if err != nil {
		return gateway.AssistSampleMasker{}, err
	}
	var samples rbac.SampleAccess
	if w.Access != nil {
		samples = w.Access.Samples
	}
	return gateway.AssistSampleMasker{Authorizer: gateway.PreviewAuthorizer{Authz: authz, Samples: samples}, Values: preview.NewSampleValueMasker()}, nil
}
