package config

import "testing"

func TestLoaderStaticKeyValidation(t *testing.T) {
	prod := func(extra map[string]string) map[string]string {
		out := map[string]string{"ROUTER_ENV": "production", "ROUTER_CURSOR_SECRET": "c", "ROUTER_COOKIE_SECRET": "k",
			"ROUTER_SECRETS_PROVIDER": "static_key"}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}
	runValidationCases(t, []validationCase{
		{"static key in production api", ServiceAPI, prod(map[string]string{"ROUTER_SECRETS_MASTER_KEY": "a2V5"}), ""},
		{"static key file in production api", ServiceAPI, prod(map[string]string{"ROUTER_SECRETS_MASTER_KEY_FILE": "/run/k"}), ""},
		{"static key missing in production api", ServiceAPI, prod(nil), "set exactly one of ROUTER_SECRETS_MASTER_KEY or ROUTER_SECRETS_MASTER_KEY_FILE for static_key"},
		{"static key and file both set", ServiceAPI, prod(map[string]string{"ROUTER_SECRETS_MASTER_KEY": "a2V5", "ROUTER_SECRETS_MASTER_KEY_FILE": "/run/k"}), "set exactly one"},
		{"worker static key without key", ServiceWorker, map[string]string{"ROUTER_ENV": "production", "ROUTER_SECRETS_PROVIDER": "static_key"}, ""},
		{"local provider in production still fails", ServiceAPI, prod(map[string]string{"ROUTER_SECRETS_PROVIDER": "local"}), "local is only allowed in local"},
	})
}

func TestLoaderStaticKeyValues(t *testing.T) {
	cfg, err := NewLoaderWithLookup(lookupFrom(map[string]string{
		"ROUTER_SECRETS_PROVIDER": "static_key", "ROUTER_SECRETS_MASTER_KEY_FILE": "/run/master.key",
		"ROUTER_SECRETS_PREVIOUS_MASTER_KEYS": "a, b", "ROUTER_SECRETS_PREVIOUS_MASTER_KEY_FILES": "/old1,/old2",
	})).Load(ServiceAPI)
	if err != nil {
		t.Fatal(err)
	}
	s := cfg.Secrets.Static
	if cfg.Secrets.Provider != SecretsStaticKey || s.KeyFile != "/run/master.key" || s.Key != "" ||
		len(s.PreviousKeys) != 2 || s.PreviousKeys[1] != "b" || len(s.PreviousKeyFiles) != 2 {
		t.Fatalf("static key config: %+v", cfg.Secrets)
	}
}
