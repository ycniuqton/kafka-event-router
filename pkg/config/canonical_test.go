package config_test

import (
	"testing"

	"github.com/eventrouter/router/pkg/config"
)

func TestCanonicalStableAcrossKeyOrderAndWhitespace(t *testing.T) {
	a := []byte(`{"b":1,"a":{"y":[1,2,{"q":true,"p":null}],"x":"s"}}`)
	b := []byte("{\n  \"a\" : { \"x\":\"s\", \"y\":[1, 2, {\"p\":null, \"q\":true}] },\n  \"b\": 1.0\n}")
	c := config.Canonicalizer{}
	ha, err := c.Hash(a)
	if err != nil {
		t.Fatal(err)
	}
	hb, _ := c.Hash(b)
	if ha != hb || len(ha) != 64 {
		t.Fatalf("hash mismatch %s %s", ha, hb)
	}
	hc, _ := c.Hash([]byte(`{"b":2,"a":{"y":[1,2,{"q":true,"p":null}],"x":"s"}}`))
	if hc == ha {
		t.Fatal("different docs must hash differently")
	}
}

func TestCanonicalForm(t *testing.T) {
	cases := map[string]string{
		`{"b":1,"a":2}`: `{"a":2,"b":1}`,
		`[1.0, 1e3, 0.000001, 1e-7, 1e21, -0.5, 123456789012345680000]`: `[1,1000,0.000001,1e-7,1e+21,-0.5,123456789012345680000]`,
		`"\u00e9\n\u001f\"\\/<>"`:                               "\"é\\n\\u001f\\\"\\\\/<>\"",
		`{"\u20ac":1,"\r":2,"1":3,"\ud83d\ude00":4,"\u0080":5}`: "{\"\\r\":2,\"1\":3,\"\u0080\":5,\"€\":1,\"😀\":4}",
		` true `: `true`,
	}
	for in, want := range cases {
		got, err := config.Canonicalizer{}.Canonical([]byte(in))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("canonical(%s) = %s, want %s", in, got, want)
		}
	}
	if _, err := (config.Canonicalizer{}).Canonical([]byte(`{`)); err == nil {
		t.Fatal("want error")
	}
}
