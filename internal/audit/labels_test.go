package audit

import (
	"strings"
	"testing"
)

func TestObjectLabelerPrefersStoredLabelThenLiveName(t *testing.T) {
	l := NewObjectLabeler()
	l.Register(ObjectAlertRule, `SELECT name FROM alert_rules WHERE id::text = a.object_id`)
	p := l.Projection()
	if !strings.HasPrefix(p, "COALESCE(NULLIF(a.object_label, ''), CASE a.object_type") {
		t.Fatalf("stored label must win: %s", p)
	}
	for _, want := range []string{"WHEN 'route' THEN (SELECT name FROM routes", "WHEN 'alert_rule' THEN", "WHEN 'deployment' THEN"} {
		if !strings.Contains(p, want) {
			t.Fatalf("missing %q in %s", want, p)
		}
	}
	where, _ := WhereBuilder{Search: l.Search()}.Build(Filter{Text: "orders"})
	if len(where) != 1 || !strings.Contains(where[0], "SELECT id::text FROM routes WHERE name ILIKE $1") ||
		!strings.Contains(where[0], "a.object_type IN ('alert_rule', 'sample_session') AND COALESCE(NULLIF(a.object_label") {
		t.Fatalf("text search must find live names (finders) and scan types without one: %v", where)
	}
}
