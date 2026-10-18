package importers_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/eventrouter/router/internal/library"
	"github.com/eventrouter/router/internal/library/importers"
	"github.com/eventrouter/router/internal/platform/api"
)

var limits = library.Limits{MaxLookupRows: 3, MaxLookupBytes: 200, MaxImportIssues: 2}

func lines(issues []library.ImportIssue) []int {
	out := []int{}
	for _, i := range issues {
		out = append(out, i.Line)
	}
	return out
}

func TestCSVRowImporter(t *testing.T) {
	cases := []struct {
		name, body string
		keyType    library.ValueType
		rows       int
		issueLines []int
		firstValue string
	}{
		{"plain", "key,value\nVN,APAC\nDE,EMEA\n", "", 2, []int{}, `"APAC"`},
		{"bom and crlf", "\uFEFFkey,value\r\nVN,APAC\r\n", "", 1, []int{}, `"APAC"`},
		{"quoted comma and newline", "key,value\n\"a\",\"x, \"\"y\"\"\nz\"\n", "", 1, []int{}, `"x, \"y\"\nz"`},
		{"object value", "Key,region,zone\nVN,APAC,7\n", "", 1, []int{}, `{"region":"APAC","zone":"7"}`},
		{"duplicate and empty keys", "key,value\nA,1\nA,2\n ,3\n", "", 1, []int{3, 4}, `"1"`},
		{"number keys", "key,value\n1,a\none,b\n", library.ValueNumber, 1, []int{3}, `"a"`},
		{"missing header key", "id,value\n1,a\n", "", 0, []int{1}, ""},
		{"not utf-8", "key,value\n\xff\xfe,a\n", "", 0, []int{0}, ""},
		{"bad quote", "key,value\nA,\"open\nB,2\n", "", 0, []int{2}, ""},
	}
	for _, c := range cases {
		rows, issues, err := importers.CSVRowImporter{}.Import(strings.NewReader(c.body), c.keyType, limits)
		if err != nil || len(rows) != c.rows {
			t.Fatalf("%s: rows=%v err=%v", c.name, rows, err)
		}
		if got := lines(issues); len(got) != len(c.issueLines) {
			t.Fatalf("%s: issues=%v want lines %v", c.name, issues, c.issueLines)
		}
		for i, l := range c.issueLines {
			if issues[i].Line != l {
				t.Errorf("%s: issue %d line=%d want %d (%s)", c.name, i, issues[i].Line, l, issues[i].Message)
			}
		}
		if c.firstValue != "" && string(rows[0].Value) != c.firstValue {
			t.Errorf("%s: first value=%s want %s", c.name, rows[0].Value, c.firstValue)
		}
	}
}

func TestJSONRowImporter(t *testing.T) {
	cases := []struct {
		name, body string
		keyType    library.ValueType
		keys       string
		issueLines []int
	}{
		{"rows", `[{"key":"VN","value":"APAC"},{"key":2,"value":{"r":1}}]`, "", "VN,2", []int{}},
		{"map keeps order", `{"b":1,"a":2}`, "", "b,a", []int{}},
		{"boolean keys", `[{"key":true,"value":1},{"key":"maybe","value":2}]`, library.ValueBoolean, "true", []int{2}},
		{"duplicate", `{"a":1,"a":2}`, "", "a", []int{2}},
		{"wrong shape", `"text"`, "", "", []int{0}},
		{"bad row", `[{"key":"a","value":1}, 7]`, "", "a", []int{2}},
	}
	for _, c := range cases {
		rows, issues, err := importers.JSONRowImporter{}.Import(strings.NewReader(c.body), c.keyType, limits)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		keys := []string{}
		for _, r := range rows {
			keys = append(keys, r.Key)
		}
		if strings.Join(keys, ",") != c.keys || len(issues) != len(c.issueLines) {
			t.Fatalf("%s: keys=%v issues=%v", c.name, keys, issues)
		}
		for i, l := range c.issueLines {
			if issues[i].Line != l {
				t.Errorf("%s: issue line=%d want %d", c.name, issues[i].Line, l)
			}
		}
	}
}

func TestManagerLimitsAndFormats(t *testing.T) {
	m := importers.Register(limits)
	if _, err := m.Import("text/csv", strings.NewReader("key,value\na,1\nb,2\nc,3\nd,4\n"), ""); !errors.Is(err, library.ErrImportTooLarge) {
		t.Fatalf("too many rows err = %v", err)
	}
	if _, err := m.Import("text/csv", strings.NewReader("key,value\na,"+strings.Repeat("x", 300)+"\n"), ""); !errors.Is(err, library.ErrImportTooLarge) {
		t.Fatalf("too many bytes err = %v", err)
	}
	var verr *api.ValidationError
	if _, err := m.Import("application/xml", strings.NewReader("<x/>"), ""); !errors.As(err, &verr) {
		t.Fatalf("unknown format err = %v", err)
	}
	_, err := m.Import("text/csv", strings.NewReader("key,value\n,1\n,2\n,3\n"), "")
	if !errors.As(err, &verr) || len(verr.Errors) != 3 || verr.Errors[0].Path != "/lines/2" || verr.Errors[2].Path != "/lines" {
		t.Fatalf("issue report = %v", err)
	}
	rows, err := m.Import("application/json", strings.NewReader(`{"a":1}`), "")
	if err != nil || len(rows) != 1 || len(m.Formats()) != 2 {
		t.Fatalf("json import = %v, %v", rows, err)
	}
}

// FuzzCSVRowImporter: random bytes never panic and every row has a non-empty unique key.
func FuzzCSVRowImporter(f *testing.F) {
	for _, seed := range []string{"key,value\na,1\n", "\uFEFFkey,a,b\n\"x\",\"y\n", "key\n", "\xff,\n", "key,value\n\"a\"\"b\",c"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		rows, _, _ := importers.CSVRowImporter{}.Import(strings.NewReader(string(body)), "", library.DefaultLimits)
		seen := map[string]bool{}
		for _, r := range rows {
			if r.Key == "" || seen[r.Key] {
				t.Fatalf("bad key %q in %q", r.Key, body)
			}
			seen[r.Key] = true
		}
	})
}
