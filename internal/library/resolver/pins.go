package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

// PinKind names the library object a pin loads; its value is the internal path segment.
type PinKind string

const (
	PinTransform   PinKind = "transforms"
	PinLookupTable PinKind = "lookup-tables"
)

// PinPath is the internal (workload) path of one pinned library version.
type PinPath string

const (
	PathPin         PinPath = "/internal/v1/library/{kind}/{name}/versions/{version}"
	ParamPinKind    PinPath = "kind"
	ParamPinName    PinPath = "name"
	ParamPinVersion PinPath = "version"
)

// pinPlaceholder wraps a path parameter name into its pattern placeholder.
type pinPlaceholder struct{}

func (pinPlaceholder) Of(name PinPath) string { return "{" + string(name) + "}" }

// ErrUnknownPinKind reports a pin kind without a library table.
var ErrUnknownPinKind = errors.New("library: unknown pin kind")

// Known reports whether the kind has a library table.
func (k PinKind) Known() bool {
	_, ok := pinSQL[k]
	return ok
}

// pinSQL loads the pinned content of each kind.
var pinSQL = map[PinKind]string{PinTransform: transformSQL, PinLookupTable: lookupSQL}

// Pin is one pinned library version.
type Pin struct {
	Kind    PinKind
	Name    string
	Version int
}

// Path fills PathPin for the pin (escaped).
func (p Pin) Path() string {
	return strings.NewReplacer(
		pinPlaceholder{}.Of(ParamPinKind), url.PathEscape(string(p.Kind)),
		pinPlaceholder{}.Of(ParamPinName), url.PathEscape(p.Name),
		pinPlaceholder{}.Of(ParamPinVersion), strconv.Itoa(p.Version),
	).Replace(string(PathPin))
}

// Content returns the raw pinned content (steps or rows); a missing pin wraps config.ErrLibraryRefMissing.
func (r *StoreLibraryResolver) Content(ctx context.Context, p Pin) (json.RawMessage, error) {
	var raw json.RawMessage
	sql, ok := pinSQL[p.Kind]
	if !ok {
		return nil, ErrUnknownPinKind
	}
	err := r.load(ctx, sql, p.Name, p.Version, &raw)
	return raw, err
}
