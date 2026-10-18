package main

import (
	"github.com/eventrouter/router/internal/library/resolver"
	"github.com/eventrouter/router/internal/platform/api"
)

// libraryInternalModule serves pinned library versions to workers (GET /internal/v1/library/...), so
// worker compiles resolve library steps and named lookup tables without database access.
func (s *apiService) libraryInternalModule() api.Module {
	return resolver.NewInternalModule(resolver.New(s.wiring.Deps.DB), s.wiring.Responder)
}
