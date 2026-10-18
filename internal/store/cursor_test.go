package store

import (
	"errors"
	"strings"
	"testing"
)

func TestCursorRoundTripAndTamper(t *testing.T) {
	codec := NewCursorCodec("secret")
	in := Cursor{SortKey: []string{"2026-01-01T00:00:00Z"}, ID: "abc"}
	token := codec.Encode(in)
	out, err := codec.Decode(token)
	if err != nil || out.ID != in.ID || out.SortKey[0] != in.SortKey[0] {
		t.Fatalf("round trip: %+v %v", out, err)
	}
	payload, sig, _ := strings.Cut(token, ".")
	tampered := []string{
		payload + "x." + sig,
		payload,
		"garbage",
		"",
		payload + "." + sig[:len(sig)-2] + "AA",
	}
	for _, tok := range tampered {
		if _, err := codec.Decode(tok); !errors.Is(err, ErrInvalidCursor) {
			t.Errorf("Decode(%q) = %v, want ErrInvalidCursor", tok, err)
		}
	}
	if _, err := NewCursorCodec("other").Decode(token); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("foreign key accepted: %v", err)
	}
	if ErrInvalidCursor.ProblemPath() != "/cursor" {
		t.Errorf("path = %q", ErrInvalidCursor.ProblemPath())
	}
}

func TestKeysetQueryBuild(t *testing.T) {
	q := KeysetQuery{Select: "SELECT id FROM routes", Where: []string{"team_id = $1"}, Args: []any{"t"},
		SortColumn: "created_at", SortCast: "timestamptz", IDColumn: "id", IDCast: "uuid", Limit: 50,
		After: &Cursor{SortKey: []string{"2026"}, ID: "x"}}
	sql, args := q.Build()
	want := "SELECT id FROM routes WHERE team_id = $1 AND (created_at, id) < ($2::timestamptz, $3::uuid) ORDER BY created_at DESC, id DESC LIMIT 51"
	if sql != want || len(args) != 3 {
		t.Fatalf("got %q %v", sql, args)
	}
	q.After, q.Where, q.Args, q.Direction = nil, nil, nil, SortAsc
	sql, _ = q.Build()
	if sql != "SELECT id FROM routes ORDER BY created_at ASC, id ASC LIMIT 51" {
		t.Fatalf("got %q", sql)
	}
}

func TestPageSlicer(t *testing.T) {
	s := PageSlicer[int]{Limit: 2, KeyOf: func(i int) Cursor { return Cursor{ID: string(rune('a' + i))} }}
	items, next := s.Slice([]int{0, 1, 2})
	if len(items) != 2 || next == nil || next.ID != "b" {
		t.Fatalf("got %v %v", items, next)
	}
	if items, next = s.Slice([]int{0, 1}); len(items) != 2 || next != nil {
		t.Fatalf("last page: %v %v", items, next)
	}
}

func TestRowVersionETagAndErrors(t *testing.T) {
	if RowVersion(7).ETag() != `"7"` {
		t.Fatal(RowVersion(7).ETag())
	}
	err := error(&PreconditionFailedError{Current: 8})
	if !errors.Is(err, ErrPreconditionFailed) {
		t.Fatal("PreconditionFailedError must match sentinel")
	}
	conflict := NewConstraintMap().Translate(errors.New("plain"))
	if conflict.Error() != "plain" {
		t.Fatal("non-pg errors pass through")
	}
}
