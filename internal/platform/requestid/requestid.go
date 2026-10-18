// Package requestid carries the per-request ULID through contexts so logs, problems and audit
// rows share it. The api middleware assigns it; every other package only reads it.
package requestid

import "context"

// Header is the HTTP header carrying the request id.
type Header string

const HeaderRequestID Header = "X-Request-ID"

type key struct{}

// With stores id in ctx.
func With(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, key{}, id)
}

// From returns the request id in ctx, or "".
func From(ctx context.Context) string {
	id, _ := ctx.Value(key{}).(string)
	return id
}
