package payload

import (
	"encoding/json"
	"testing"
)

func TestRenderedEventIsValidJSONWithSlots(t *testing.T) {
	tpl, err := Compile([]byte(DefaultTemplate), 0)
	if err != nil {
		t.Fatal(err)
	}
	raw := tpl.Append(nil, Event{Seq: 12345, Stream: "s-7", Bad: true})
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("invalid JSON %s: %v", raw, err)
	}
	if got["seq"] != float64(12345) || got["stream"] != "s-7" || got["bad"] != true || got["type"] != "order" {
		t.Errorf("event = %v", got)
	}
}

func TestPaddingReachesTargetSize(t *testing.T) {
	for _, size := range []int{1024, 10 * 1024, 100 * 1024} {
		tpl, err := Compile([]byte(DefaultTemplate), size)
		if err != nil {
			t.Fatal(err)
		}
		n := len(tpl.Append(nil, Event{Seq: 1, Stream: "s-1"}))
		if n < size-16 || n > size+16 {
			t.Errorf("size %d rendered %d bytes", size, n)
		}
	}
}

func TestTemplateMustBeObject(t *testing.T) {
	if _, err := Compile([]byte(`[1,2]`), 0); err == nil {
		t.Error("array template accepted")
	}
}

func TestStreamsArePartitionedBetweenLanes(t *testing.T) {
	s := Streams{Prefix: "s", Total: 5, Lanes: 2}
	a, b := s.Owned(0), s.Owned(1)
	if len(a) != 3 || len(b) != 2 || a[0] != "s-0" || b[1] != "s-3" {
		t.Errorf("lanes = %v %v", a, b)
	}
	if few := (Streams{Prefix: "s", Total: 1, Lanes: 3}).Owned(2); len(few) != 1 {
		t.Errorf("lane without streams = %v", few)
	}
}

func TestKeyModes(t *testing.T) {
	for _, m := range []KeyMode{KeyRoundRobin, KeyRandom, KeyNone} {
		p, err := KeyManager{}.New(m, 1)
		if err != nil {
			t.Fatal(err)
		}
		if i := p.Pick(3); i < 0 || i >= 3 {
			t.Errorf("%s picked %d", m, i)
		}
		if (p.KeyFor("s-1") == nil) != (m == KeyNone) {
			t.Errorf("%s key = %q", m, p.KeyFor("s-1"))
		}
	}
	if _, err := (KeyManager{}).New("hot", 1); err == nil {
		t.Error("unknown mode accepted")
	}
}
