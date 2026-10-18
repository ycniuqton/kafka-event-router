package controlapi

import (
	"errors"

	"github.com/eventrouter/router/internal/store"
)

// RouteErr maps lookup outcomes of the route to the API error.
type RouteErr struct{}

// Or returns err, or ErrRouteNotFound when the lookup succeeded but found nothing.
func (RouteErr) Or(err error) error {
	if err != nil {
		return err
	}
	return ErrRouteNotFound
}

// Missing reports whether err is a store miss.
func (RouteErr) Missing(err error) bool {
	return errors.Is(err, store.ErrNotFound)
}
