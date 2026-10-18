package ctxhelpers_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
)

func TestMasker(t *testing.T) {
	m := ctxhelpers.NewStyleMasker(ctxhelpers.DefaultMaskPolicy())
	cases := []struct {
		in    any
		style config.MaskStyle
		want  any
	}{
		{"secret", config.MaskStyleFull, "******"},
		{"héllo", config.MaskStyleFull, "*****"},
		{"4111111111111111", config.MaskStyleLast4, "************1111"},
		{"123", config.MaskStyleLast4, "***"},
		{"jane@example.com", config.MaskStyleEmail, "j***@example.com"},
		{"john@mail.com", config.MaskStyleEmail, "j***@mail.com"},
		{"ab@c.de", config.MaskStyleEmail, "a*@c.de"},
		{"a@x.com", config.MaskStyleEmail, "*@x.com"},
		{"@x.com", config.MaskStyleEmail, "******"},
		{int64(12345), config.MaskStyleLast4, "*2345"},
		{nil, config.MaskStyleFull, nil},
	}
	for _, c := range cases {
		got, err := m.Mask(c.in, c.style)
		if err != nil || got != c.want {
			t.Errorf("mask(%v,%s) = %v %v, want %v", c.in, c.style, got, err, c.want)
		}
	}
	if _, err := m.Mask("x", "bogus"); err == nil {
		t.Fatal("unknown style")
	}
}

type countingSalt struct{ calls int }

func (c *countingSalt) Salt(context.Context, string) ([]byte, error) {
	c.calls++
	return []byte("salt"), nil
}

type failingSalt struct{}

func (failingSalt) Salt(context.Context, string) ([]byte, error) {
	return nil, errors.New("vault down")
}

func TestHasherSaltedOncePerTeam(t *testing.T) {
	sp := &countingSalt{}
	h := ctxhelpers.NewSaltedHasher(sp, "team-a")
	a, _ := h.Hash(context.Background(), "v")
	b, _ := h.Hash(context.Background(), "v")
	unsalted, _ := ctxhelpers.NewSaltedHasher(ctxhelpers.StaticSaltProvider{}, "").Hash(context.Background(), "v")
	if a != b || a == unsalted || sp.calls != 1 || len(a) != 64 {
		t.Fatalf("a=%s b=%s calls=%d", a, b, sp.calls)
	}
	if _, err := ctxhelpers.NewSaltedHasher(failingSalt{}, "t").Hash(context.Background(), "v"); err == nil {
		t.Fatal("salt error must propagate")
	}
}

func TestClockDateLogs(t *testing.T) {
	at := time.Date(2026, 3, 4, 5, 6, 7, 8000000, time.UTC)
	if (ctxhelpers.FixedClock{T: at}).Now() != at || (ctxhelpers.SystemClock{}).Now().Location() != time.UTC {
		t.Fatal("clocks")
	}
	f := ctxhelpers.LayoutDateFormatter{}
	for layout, want := range map[ctxhelpers.DateLayout]string{
		ctxhelpers.LayoutRFC3339: "2026-03-04T05:06:07Z", ctxhelpers.LayoutRFC3339Ms: "2026-03-04T05:06:07.008Z",
		ctxhelpers.LayoutDate: "2026-03-04", ctxhelpers.LayoutDateTime: "2026-03-04 05:06:07",
	} {
		if got, err := f.Format(at, layout, ""); err != nil || got != want {
			t.Errorf("%s: %s %v", layout, got, err)
		}
	}
	if got, _ := f.Format(at, ctxhelpers.LayoutDateTime, "Europe/Berlin"); got != "2026-03-04 06:06:07" {
		t.Errorf("tz: %s", got)
	}
	if _, err := f.Format(at, "x", ""); err == nil {
		t.Error("unknown layout")
	}
	if _, err := f.Format(at, ctxhelpers.LayoutDate, "Mars/Base"); err == nil {
		t.Error("unknown tz")
	}
	sink := ctxhelpers.CappedLogSinkFactory{MaxBytes: 5}.New()
	sink.Log("abc")
	sink.Log("defg")
	sink.Log("h")
	if len(sink.Lines()) != 2 || !sink.Truncated() {
		t.Fatalf("lines %v", sink.Lines())
	}
	set := ctxhelpers.Set{}.WithDefaults()
	if set.Mask == nil || set.Hash == nil || set.Clock == nil || set.Date == nil || set.Lookup == nil || set.Logs == nil {
		t.Fatal("defaults")
	}
	if _, ok := set.Lookup.Lookup("t", "k"); ok {
		t.Fatal("empty lookup")
	}
}
