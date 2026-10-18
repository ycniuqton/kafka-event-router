package kms

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/secrets"
)

// fakeAWS speaks the AWS KMS JSON protocol (X-Amz-Target TrentService.Encrypt/Decrypt) with a reversible "cipher".
func fakeAWS(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["EncryptionContext"].(map[string]any)["app"] != "event-router" {
			http.Error(w, `{"__type":"InvalidCiphertextException"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		switch r.Header.Get("X-Amz-Target") {
		case "TrentService.Encrypt":
			pt, _ := base64.StdEncoding.DecodeString(in["Plaintext"].(string))
			_ = json.NewEncoder(w).Encode(map[string]any{"CiphertextBlob": base64.StdEncoding.EncodeToString(append([]byte("aws:"), pt...)), "KeyId": in["KeyId"]})
		case "TrentService.Decrypt":
			ct, _ := base64.StdEncoding.DecodeString(in["CiphertextBlob"].(string))
			_ = json.NewEncoder(w).Encode(map[string]any{"Plaintext": base64.StdEncoding.EncodeToString(bytes.TrimPrefix(ct, []byte("aws:"))), "KeyId": in["KeyId"]})
		default:
			t.Errorf("unexpected target %q", r.Header.Get("X-Amz-Target"))
		}
	}))
}

func roundTrip(t *testing.T, p secrets.Provider) {
	t.Helper()
	dek := bytes.Repeat([]byte{7}, 32)
	wrapped, err := p.WrapKey(context.Background(), dek)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(wrapped, dek) {
		t.Fatal("wrapped key equals plaintext")
	}
	got, err := p.UnwrapKey(context.Background(), wrapped, p.KeyID())
	if err != nil || !bytes.Equal(got, dek) {
		t.Fatalf("unwrap = %x, %v", got, err)
	}
}

func TestAWSProviderThroughSDK(t *testing.T) {
	srv := fakeAWS(t)
	defer srv.Close()
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	reg := secrets.NewRegistry()
	Register(reg)
	p, err := reg.Build(context.Background(), config.SecretsConfig{Provider: config.SecretsAWSKMS,
		KMS: config.KMSConfig{AWSKeyID: "alias/event-router", AWSRegion: "eu-west-1", AWSEndpoint: srv.URL}})
	if err != nil {
		t.Fatal(err)
	}
	if p.KeyID() != "aws_kms:alias/event-router" || p.Kind() != config.SecretsAWSKMS {
		t.Fatalf("key id %q kind %q", p.KeyID(), p.Kind())
	}
	roundTrip(t, p)
}

func TestAWSRequiresKey(t *testing.T) {
	if _, err := NewAWSFromConfig(context.Background(), config.KMSConfig{}); err == nil {
		t.Fatal("want error without key id")
	}
}

func TestGCPProvider(t *testing.T) {
	const key = "projects/p/locations/global/keyRings/r/cryptoKeys/k"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var in map[string]string
		_ = json.Unmarshal(body, &in)
		switch {
		case r.URL.Path == "/v1/"+key+":encrypt":
			_ = json.NewEncoder(w).Encode(map[string]string{"name": key + "/cryptoKeyVersions/1",
				"ciphertext": base64.StdEncoding.EncodeToString([]byte("gcp:" + in["plaintext"]))})
		case r.URL.Path == "/v1/"+key+":decrypt":
			ct, _ := base64.StdEncoding.DecodeString(in["ciphertext"])
			_ = json.NewEncoder(w).Encode(map[string]string{"plaintext": strings.TrimPrefix(string(ct), "gcp:")})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	p := NewGCP(srv.Client(), srv.URL, key)
	if p.KeyID() != "gcp_kms:"+key {
		t.Fatal(p.KeyID())
	}
	roundTrip(t, p)
	if _, err := NewGCP(srv.Client(), srv.URL, "unknown").WrapKey(context.Background(), []byte("x")); err == nil {
		t.Fatal("want error on 404")
	}
}

func TestGCPRequiresKeyName(t *testing.T) {
	if _, err := NewGCPFromConfig(context.Background(), config.KMSConfig{}); err == nil {
		t.Fatal("want error without key name")
	}
}
