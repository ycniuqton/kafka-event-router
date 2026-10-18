// Package routes owns the route as a living object: drafts, in-context validation, deploys,
// immutable versions, diff, rollback, pause/resume and archive. Managers are the only entry points;
// handlers live in routes/http, Postgres repositories in internal/store/routestore.
package routes
