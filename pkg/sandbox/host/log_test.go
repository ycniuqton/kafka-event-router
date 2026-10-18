package host

import (
	"strings"
	"sync"
	"testing"
)

func TestLogBufferCaps(t *testing.T) {
	cases := []struct {
		name      string
		max       int64
		lines     []string
		kept      int
		truncated bool
	}{
		{"fits", 10, []string{"abc", "defg"}, 2, false},
		{"exact", 7, []string{"abc", "defg"}, 2, false},
		{"overflow drops line", 6, []string{"abc", "defg", "x"}, 3 - 1, true},
		{"zero cap drops all", 0, []string{"a"}, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := NewLogBuffer(tc.max)
			for _, l := range tc.lines {
				b.Append(l)
			}
			if len(b.Lines()) != tc.kept || b.Truncated() != tc.truncated {
				t.Fatalf("lines=%q truncated=%v", b.Lines(), b.Truncated())
			}
		})
	}
}

func TestLogBufferConcurrentAppend(t *testing.T) {
	b := NewLogBuffer(1000)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); b.Append(strings.Repeat("x", 30)) }()
	}
	wg.Wait()
	if len(b.Lines()) != 33 || !b.Truncated() {
		t.Fatalf("kept %d truncated=%v", len(b.Lines()), b.Truncated())
	}
}
