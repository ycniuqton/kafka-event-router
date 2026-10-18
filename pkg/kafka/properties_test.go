package kafka

import "testing"

func TestPropertyCatalogValidate(t *testing.T) {
	cat := DefaultPropertyCatalog()
	cases := []struct {
		name  string
		props map[PropertyKey]string
		want  []string
	}{
		{"empty", nil, nil},
		{"all allowed", map[PropertyKey]string{
			PropClientID: "router", PropRequestTimeoutMs: "30000", PropMetadataMaxAgeMs: "60000",
			PropConnectionsMaxIdleMs: "1000", PropFetchMaxBytes: "1048576", PropSocketSetupTimeoutMs: "5000",
		}, nil},
		{"unknown key", map[PropertyKey]string{"sasl.jaas.config": "x"}, []string{"/properties/sasl.jaas.config"}},
		{"bad number", map[PropertyKey]string{PropRequestTimeoutMs: "-1"}, []string{"/properties/request.timeout.ms"}},
		{"empty client id", map[PropertyKey]string{PropClientID: ""}, []string{"/properties/client.id"}},
		{"escaped key", map[PropertyKey]string{"a/b": "1"}, []string{"/properties/a~1b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := paths(cat.Validate(tc.props))
			if len(got) != len(tc.want) || (len(got) > 0 && got[0] != tc.want[0]) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPropertyCatalogOptions(t *testing.T) {
	cat := DefaultPropertyCatalog()
	opts, err := cat.Options(map[PropertyKey]string{PropClientID: "x", PropFetchMaxBytes: "10"})
	if err != nil || len(opts) != 2 {
		t.Fatalf("opts=%d err=%v", len(opts), err)
	}
	if _, err := cat.Options(map[PropertyKey]string{"nope": "1"}); err == nil {
		t.Fatal("expected unknown-key error")
	}
	if len(cat.Keys()) != 6 {
		t.Fatalf("allow-list size %d", len(cat.Keys()))
	}
}
