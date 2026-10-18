package library_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/eventrouter/router/internal/assist"
	"github.com/eventrouter/router/internal/library"
	"github.com/eventrouter/router/internal/platform/api"
)

func kinds(t *testing.T) *library.KindManager {
	t.Helper()
	checker, err := assist.NewStepChecker()
	if err != nil {
		t.Fatal(err)
	}
	return library.DefaultKindManager(checker, library.Limits{MaxSteps: 3, MaxLookupRows: 2, MaxLookupBytes: 200, MaxListItems: 10}, nil)
}

func paths(err error) []string {
	var verr *api.ValidationError
	if !errors.As(err, &verr) {
		return nil
	}
	out := []string{}
	for _, e := range verr.Errors {
		out = append(out, e.Path)
	}
	return out
}

func TestKindManagerSelectsRegisteredKinds(t *testing.T) {
	m := kinds(t)
	for _, k := range []library.ObjectKind{library.KindTransform, library.KindLookupTable} {
		got, err := m.Get(k)
		if err != nil || got.Kind() != k {
			t.Fatalf("Get(%s) = %v, %v", k, got, err)
		}
	}
	if _, err := m.Get("map"); !errors.Is(err, library.ErrUnknownKind) {
		t.Fatalf("unknown kind err = %v", err)
	}
}

func TestTransformKindValidate(t *testing.T) {
	k, _ := kinds(t).Get(library.KindTransform)
	cases := []struct {
		name, steps string
		size        int
		paths       string
	}{
		{"valid", `[{"kind":"mask","field":"value.email","style":"email"},{"kind":"remove","fields":["value.x"]}]`, 2, ""},
		{"empty", `[]`, 0, "/steps"},
		{"too many", `[{"kind":"hash","field":"value.a"},{"kind":"hash","field":"value.b"},{"kind":"hash","field":"value.c"},{"kind":"hash","field":"value.d"}]`, 4, "/steps"},
		{"invalid step", `[{"kind":"hash","field":"value.a"},{"kind":"rename","from":"value.a"}]`, 2, "/steps/1"},
		{"not a list", `{"kind":"hash"}`, 0, "/steps"},
	}
	for _, c := range cases {
		size, err := k.Validate(json.RawMessage(c.steps), "")
		if got := strings.Join(paths(err), ","); got != c.paths || size != c.size {
			t.Errorf("%s: size=%d paths=%q, want %d %q (err %v)", c.name, size, got, c.size, c.paths, err)
		}
	}
}

func TestLookupTableKindValidate(t *testing.T) {
	k, _ := kinds(t).Get(library.KindLookupTable)
	cases := []struct {
		name, rows string
		paths      string
		keyType    library.ValueType
	}{
		{"valid", `[{"key":"VN","value":"APAC"},{"key":"DE","value":{"region":"EMEA"}}]`, "", ""},
		{"number keys", `[{"key":"1.5","value":1},{"key":"x","value":2}]`, "/rows/1/key", library.ValueNumber},
		{"boolean keys", `[{"key":"true","value":1},{"key":"yes","value":2}]`, "/rows/1/key", library.ValueBoolean},
		{"duplicate key", `[{"key":"VN","value":1},{"key":"VN","value":2}]`, "/rows/1/key", ""},
		{"empty key", `[{"key":"","value":1}]`, "/rows/0/key", ""},
		{"too many rows", `[{"key":"a","value":1},{"key":"b","value":2},{"key":"c","value":3}]`, "/rows", ""},
		{"unknown member", `[{"key":"a","val":1}]`, "/rows", ""},
		{"too large", `[{"key":"a","value":"` + strings.Repeat("x", 300) + `"}]`, "/rows", ""},
		{"empty", `[]`, "/rows", ""},
	}
	for _, c := range cases {
		_, err := k.Validate(json.RawMessage(c.rows), c.keyType)
		if got := strings.Join(paths(err), ","); got != c.paths {
			t.Errorf("%s: paths=%q, want %q (err %v)", c.name, got, c.paths, err)
		}
	}
}

func TestNamePattern(t *testing.T) {
	for name, ok := range map[string]bool{"mask-customer-pii": true, "v2": true, "Mask": false, "-x": false, "a b": false, "": false} {
		if library.NamePattern.MatchString(name) != ok {
			t.Errorf("NamePattern(%q) != %v", name, ok)
		}
	}
}

func TestValueTypeAccepts(t *testing.T) {
	cases := []struct {
		t    library.ValueType
		key  string
		want bool
	}{
		{"", "anything", true}, {library.ValueNumber, "-2.5e3", true}, {library.ValueNumber, "two", false},
		{library.ValueBoolean, "false", true}, {library.ValueBoolean, "1", false}, {"date", "x", false},
	}
	for _, c := range cases {
		if got := c.t.Accepts(c.key); got != c.want {
			t.Errorf("%q.Accepts(%q) = %v", c.t, c.key, got)
		}
	}
}

func TestContentHasherIgnoresKeyOrder(t *testing.T) {
	a, err := library.ContentHasher{}.Hash(json.RawMessage(`[{"kind":"hash","field":"value.a"}]`))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := library.ContentHasher{}.Hash(json.RawMessage(`[ {"field":"value.a", "kind":"hash"} ]`))
	c, _ := library.ContentHasher{}.Hash(json.RawMessage(`[{"kind":"hash","field":"value.b"}]`))
	if a != b || a == c || !strings.HasPrefix(a, string(library.HashSHA256)) {
		t.Fatalf("hashes a=%s b=%s c=%s", a, b, c)
	}
}
