package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/eventrouter/router/internal/platform/controlplane"
	"github.com/eventrouter/router/pkg/config"
)

// fakeFetcher serves fixed documents by path and counts calls.
type fakeFetcher struct {
	docs  map[string]string
	err   error
	calls int
}

func (f *fakeFetcher) GetJSON(_ context.Context, path string, out any) error {
	f.calls++
	if f.err != nil {
		return f.err
	}
	doc, ok := f.docs[path]
	if !ok {
		return &controlplane.StatusError{Status: http.StatusNotFound, Path: path}
	}
	return json.Unmarshal([]byte(doc), out)
}

func TestPinPathEscapes(t *testing.T) {
	got := Pin{Kind: PinLookupTable, Name: "a b/c", Version: 3}.Path()
	if want := "/internal/v1/library/lookup-tables/a%20b%2Fc/versions/3"; got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestControlPlaneResolverCachesPins(t *testing.T) {
	f := &fakeFetcher{docs: map[string]string{
		Pin{Kind: PinTransform, Name: "clean", Version: 1}.Path():       `[{"id":"r","kind":"rename","from":"value.a","to":"value.b"}]`,
		Pin{Kind: PinLookupTable, Name: "countries", Version: 2}.Path(): `[{"key":"DE","value":"Germany"}]`,
	}}
	r := NewControlPlaneResolver(f)
	for range 2 {
		steps, err := r.Transform(context.Background(), "clean", 1)
		if err != nil || len(steps) != 1 || steps[0].Kind != config.StepKindRename {
			t.Fatalf("transform = %+v, %v", steps, err)
		}
	}
	rows, err := r.LookupTable(context.Background(), "countries", 2)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %+v, %v", rows, err)
	}
	if f.calls != 2 {
		t.Fatalf("calls = %d, want 2 (cached)", f.calls)
	}
}

func TestControlPlaneResolverErrors(t *testing.T) {
	_, err := NewControlPlaneResolver(&fakeFetcher{}).Transform(context.Background(), "nope", 1)
	if !errors.Is(err, config.ErrLibraryRefMissing) {
		t.Fatalf("404 err = %v, want ErrLibraryRefMissing", err)
	}
	down := &fakeFetcher{err: errors.New("connection refused")}
	r := NewControlPlaneResolver(down)
	_, err = r.LookupTable(context.Background(), "t", 1)
	if err == nil || errors.Is(err, config.ErrLibraryRefMissing) {
		t.Fatalf("transient err = %v, want non-missing error", err)
	}
	down.err = nil
	down.docs = map[string]string{Pin{Kind: PinLookupTable, Name: "t", Version: 1}.Path(): `[]`}
	if _, err := r.LookupTable(context.Background(), "t", 1); err != nil {
		t.Fatalf("failure must not be cached: %v", err)
	}
}
