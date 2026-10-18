package routeshttp_test

import (
	"net/http"
	"strings"
)

// checkUnarchive (routes#p2-be-unarchive): an archived route comes back paused; repeating is a no-op.
func (h *harness) checkUnarchive(item string) {
	h.t.Helper()
	h.statusOf(h.expect(h.do(http.MethodPost, item+"/unarchive", nil, ""), http.StatusOK), "paused")
	h.statusOf(h.expect(h.do(http.MethodPost, item+"/unarchive", nil, ""), http.StatusOK), "paused")
	last := h.events.Events[len(h.events.Events)-1]
	if last.Kind != "unarchived" || last.DesiredState != "paused" {
		h.t.Fatalf("event %+v", last)
	}
}

// checkExport (routes#p2-be-portability, routes#p2-be-bulk-export): connection names, formats, versions, bulk.
func (h *harness) checkExport(id, dup, item string) {
	t := h.t
	y := h.expect(h.do(http.MethodGet, item+"/export", nil, ""), http.StatusOK)
	if !strings.HasPrefix(y.media, "application/yaml") || !strings.Contains(y.raw, "connectionId: orders-prod") || !strings.HasPrefix(y.raw, "---\napiVersion: router/v1\nname: orders\n") {
		t.Fatalf("%s\n%s", y.media, y.raw)
	}
	j := h.expect(h.do(http.MethodGet, item+"/export?format=json&version=1", nil, ""), http.StatusOK)
	if j.body["name"] != "orders" || !strings.Contains(j.raw, `"connectionId": "partner"`) {
		t.Fatal(j.raw)
	}
	h.expect(h.do(http.MethodGet, item+"/export?format=xml", nil, ""), http.StatusBadRequest)
	h.expect(h.do(http.MethodGet, item+"/export?version=42", nil, ""), http.StatusNotFound)
	multi := h.expect(h.do(http.MethodPost, base+"/export", map[string]any{"ids": []string{id, dup}}, ""), http.StatusOK)
	if strings.Count(multi.raw, "---\n") != 2 || !strings.Contains(multi.raw, "name: orders-copy") {
		t.Fatal(multi.raw)
	}
	h.expect(h.do(http.MethodPost, base+"/export", map[string]any{"ids": []string{}}, ""), http.StatusUnprocessableEntity)
	bulk := h.expect(h.do(http.MethodPost, base+"/bulk", map[string]any{"action": "export", "ids": []string{id}, "format": "json"}, ""), http.StatusOK)
	export, _ := bulk.body["export"].(map[string]any)
	if export == nil || export["fileName"] != "orders.json" || !strings.Contains(export["content"].(string), "orders-prod") {
		t.Fatal(bulk.raw)
	}
}

// checkImport (routes#p2-be-portability): create/upsert/dry run, name resolution, envelope, malformed input.
func (h *harness) checkImport(item string) {
	t := h.t
	y := h.expect(h.do(http.MethodGet, item+"/export?version=draft", nil, ""), http.StatusOK).raw
	skipped := h.importYAML("", y)
	if skipped.Action != "skipped_exists" {
		t.Fatalf("%+v", skipped)
	}
	before := h.expect(h.do(http.MethodGet, item, nil, ""), http.StatusOK).etag
	dry := h.importYAML("?mode=upsert&dryRun=true", y)
	if dry.Action != "draft_updated" || h.expect(h.do(http.MethodGet, item, nil, ""), http.StatusOK).etag != before {
		t.Fatalf("dry run wrote: %+v", dry)
	}
	created := h.importYAML("", strings.Replace(y, "name: orders\n", "name: orders-imported\n", 1))
	if created.Action != "created" || created.RouteID == "" {
		t.Fatalf("%+v", created)
	}
	got := h.expect(h.do(http.MethodGet, base+"/"+created.RouteID, nil, ""), http.StatusOK)
	if got.body["status"] != "draft" || !strings.Contains(got.raw, `"connectionId":"c1"`) {
		t.Fatal(got.raw)
	}
	unknown := h.importYAML("", strings.Replace(strings.Replace(y, "orders-prod", "nope-conn", 1), "name: orders\n", "name: x\n", 1))
	if unknown.Action != "invalid" || unknown.Codes[0] != "connection_not_found" {
		t.Fatalf("%+v", unknown)
	}
	h.expect(h.send(http.MethodPost, base+"/import", "application/yaml", []byte("a: [\n"), ""), http.StatusBadRequest)
	h.expect(h.send(http.MethodPost, base+"/import?mode=merge", "application/yaml", []byte(y), ""), http.StatusBadRequest)
	env := h.expect(h.do(http.MethodPost, base+"/import?mode=upsert", map[string]any{"content": y, "format": "yaml"}, ""), http.StatusOK)
	if !strings.Contains(env.raw, `"action":"draft_updated"`) {
		t.Fatal(env.raw)
	}
}

// importOutcome is the first result of an import response.
type importOutcome struct {
	Action  string
	RouteID string
	Codes   []string
}

func (h *harness) importYAML(query, body string) importOutcome {
	h.t.Helper()
	res := h.expect(h.send(http.MethodPost, base+"/import"+query, "application/yaml", []byte(body), ""), http.StatusOK)
	first := asList(res.body["results"])[0].(map[string]any)
	out := importOutcome{Action: first["action"].(string)}
	out.RouteID, _ = first["routeId"].(string)
	for _, is := range asList(first["issues"]) {
		out.Codes = append(out.Codes, is.(map[string]any)["code"].(string))
	}
	return out
}
