package cache

import (
	"errors"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/runtime/domain"
)

func TestFileConfigCacheRoundTrip(t *testing.T) {
	c, err := NewFileConfigCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.LoadAssignments(); !errors.Is(err, ErrMiss) {
		t.Fatalf("empty cache: %v", err)
	}
	set := domain.AssignmentSet{ETag: "e1", Assignments: []domain.Assignment{{RouteID: "r1", VersionID: "v1", LeaseExpiresAt: time.Unix(10, 0).UTC()}}}
	if err := c.SaveAssignments(set); err != nil {
		t.Fatal(err)
	}
	got, err := c.LoadAssignments()
	if err != nil || got.ETag != "e1" || len(got.Assignments) != 1 || got.Assignments[0].RouteID != "r1" {
		t.Fatalf("got %+v %v", got, err)
	}
	ref := domain.VersionRef{RouteID: "r1", VersionID: "v1"}
	if err := c.SaveVersion(ref, []byte(`{"a":1}`)); err != nil {
		t.Fatal(err)
	}
	doc, err := c.LoadVersion(ref)
	if err != nil || string(doc) != `{"a":1}` {
		t.Fatalf("doc %s %v", doc, err)
	}
}
