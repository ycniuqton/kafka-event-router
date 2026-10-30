package password

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestArgon2idHashVerify(t *testing.T) {
	h := fastHashers()
	a, err := h.Hash("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := h.Hash("correct horse battery")
	if a == b || !strings.HasPrefix(a, "$argon2id$v=19$m=64,t=1,p=1$") {
		t.Fatalf("hashes must be salted PHC strings: %q %q", a, b)
	}
	if ok, err := h.Verify(a, "correct horse battery"); !ok || err != nil {
		t.Fatalf("verify ok=%v err=%v", ok, err)
	}
	if ok, _ := h.Verify(a, "correct horse batterY"); ok {
		t.Fatal("wrong password verified")
	}
}

func TestVerifyRejectsMalformedHashes(t *testing.T) {
	h := fastHashers()
	for _, enc := range []string{"", "plain", "$bcrypt$x", "$argon2id$v=18$m=64,t=1,p=1$AAAA$AAAA", "$argon2id$v=19$m=64,t=1,p=1$***$AAAA"} {
		if _, err := h.Verify(enc, "x"); !errors.Is(err, ErrMalformedHash) {
			t.Errorf("%q: err=%v", enc, err)
		}
	}
}

func TestDefaultParamsVerify(t *testing.T) {
	h := NewHasherSet(NewArgon2idHasher(DefaultArgon2Params))
	enc, err := h.Hash("a-long-enough-password")
	if err != nil || !strings.Contains(enc, "m=19456,t=2,p=1") {
		t.Fatalf("enc=%q err=%v", enc, err)
	}
	if ok, _ := fastHashers().Verify(enc, "a-long-enough-password"); !ok {
		t.Fatal("stored parameters must win over the verifier's own")
	}
}

func TestPolicy(t *testing.T) {
	cases := map[string]bool{strings.Repeat("a", 11): false, strings.Repeat("a", 12): true, strings.Repeat("é", 12): true,
		strings.Repeat("a", 256): true, strings.Repeat("a", 257): false, "": false}
	for pw, ok := range cases {
		if err := DefaultPolicy.Check(pw); (err == nil) != ok {
			t.Errorf("len %d: err=%v want ok=%v", utf8.RuneCountInString(pw), err, ok)
		}
	}
}

func TestGenerator(t *testing.T) {
	seen := map[string]bool{}
	for range 50 {
		pw, err := Generator{}.New()
		if err != nil {
			t.Fatal(err)
		}
		if len(pw) != int(DefaultGeneratedLength) || DefaultPolicy.Check(pw) != nil || seen[pw] {
			t.Fatalf("bad generated password %q", pw)
		}
		for _, r := range pw {
			if !strings.ContainsRune(string(SafeAlphabet), r) {
				t.Fatalf("char %q outside alphabet", r)
			}
		}
		seen[pw] = true
	}
}
