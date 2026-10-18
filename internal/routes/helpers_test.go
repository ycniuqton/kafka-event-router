package routes

import "testing"

func TestConfigFingerprintIgnoresKeyOrderAndSpace(t *testing.T) {
	f := ConfigFingerprint{}
	a := []byte(`{"b":1,"a":{"y":[1,2],"x":"s"}}`)
	b := []byte(`{ "a": {"x":"s", "y":[1, 2]}, "b": 1.0 }`)
	if !f.Same(a, b) {
		t.Fatal("expected same fingerprint")
	}
	if f.Same(a, []byte(`{"b":2,"a":{"y":[1,2],"x":"s"}}`)) {
		t.Fatal("expected different fingerprint")
	}
	if f.Same(nil, b) || f.Same(a, []byte(`{`)) {
		t.Fatal("nil or invalid documents never match")
	}
	h, err := f.Of(a)
	if err != nil || len(h) != 64 {
		t.Fatalf("hash %q err %v", h, err)
	}
}

func TestDuplicateNamer(t *testing.T) {
	n := DuplicateNamer{}
	cases := []struct {
		name  string
		taken []string
		want  string
	}{
		{"orders", nil, "orders-copy"},
		{"orders", []string{"orders-copy"}, "orders-copy-2"},
		{"orders", []string{"orders-copy", "orders-copy-2"}, "orders-copy-3"},
		{"orders-copy", []string{"orders-copy"}, "orders-copy-2"},
		{"orders-copy-7", []string{"orders-copy"}, "orders-copy-2"},
	}
	for _, c := range cases {
		taken := map[string]bool{}
		for _, t := range c.taken {
			taken[t] = true
		}
		if got := n.Next(c.name, taken); got != c.want {
			t.Errorf("%s with %v: got %s want %s", c.name, c.taken, got, c.want)
		}
	}
	if p := n.Prefix("orders-copy-3"); p != "orders-copy" {
		t.Fatalf("prefix %s", p)
	}
}

func TestConsumerGroupNamer(t *testing.T) {
	cases := map[string]string{
		"order-paid-to-partner": "router.order-paid-to-partner",
		"  Orders To Partner ":  "router.orders-to-partner",
		"a/b c!":                "router.a-b-c",
	}
	for in, want := range cases {
		if got := (ConsumerGroupNamer{}).For(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}
