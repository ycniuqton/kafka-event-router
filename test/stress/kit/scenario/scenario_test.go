package scenario

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const sample = `
name: s2-test
description: ramp
stopOnKnee: true
criteria:
  p99Limit: 500ms
defaults:
  partitions: 3
  routes:
    maxInFlight: 2000
    filter: {conditions: 10}
  load:
    size: 10240
    ramp: {from: 1000, to: 3000, step: 1000, duration: 1m}
cases:
  - name: p1
    partitions: 1
  - name: p48
    partitions: 48
    routes: {count: 4, js: identity}
    load:
      steps: [{rate: 500, duration: 30s}]
      ramp: null
    actions:
      - {at: 20s, name: kill, run: "echo kill"}
`

func TestParseMergesDefaults(t *testing.T) {
	s, err := Loader{}.Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if s.Target != "local" || s.SampleInterval != 5*time.Second || !s.StopOnKnee {
		t.Errorf("scenario fields = %+v", s)
	}
	if s.Criteria.P99Limit != 500*time.Millisecond || s.Criteria.LagSlopeRel != 0.01 {
		t.Errorf("criteria not merged with built-in: %+v", s.Criteria)
	}
	p1, p48 := s.Cases[0], s.Cases[1]
	if p1.Partitions != 1 || p1.Routes.MaxInFlight != 2000 || p1.Routes.Filter.Conditions != 10 || p1.Load.Size != 10240 {
		t.Errorf("p1 = %+v", p1)
	}
	if p1.Routes.Delivery != "at_least_once" || p1.Routes.Count != 1 || p1.Load.Lanes != 4 {
		t.Errorf("built-in defaults lost: %+v", p1.Routes)
	}
	if plan := p1.Load.Plan(); len(plan.Steps) != 3 || plan.Steps[2].Rate != 3000 {
		t.Errorf("p1 plan = %+v", plan)
	}
	if p48.Routes.Count != 4 || p48.Routes.JS != JSIdentity || p48.Routes.MaxInFlight != 2000 {
		t.Errorf("p48 routes = %+v", p48.Routes)
	}
	if plan := p48.Load.Plan(); len(plan.Steps) != 1 || plan.Steps[0].Duration != 30*time.Second {
		t.Errorf("p48 plan = %+v", plan)
	}
	if len(p48.Actions) != 1 || p48.Actions[0].At != 20*time.Second {
		t.Errorf("actions = %+v", p48.Actions)
	}
}

func TestValidationReportsProblems(t *testing.T) {
	bad := `
cases:
  - name: a
    routes: {count: 0, topology: mesh}
  - name: a
    load: {steps: []}
`
	_, err := Loader{}.Parse([]byte(bad))
	if err == nil {
		t.Fatal("invalid scenario accepted")
	}
	for _, want := range []string{"name is required", "repeated", "routes.count", "unknown topology", "load needs steps"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
}

func TestShippedScenariosParse(t *testing.T) {
	files, _ := filepath.Glob("../../scenarios/*.yaml")
	if len(files) == 0 {
		t.Skip("no scenario files")
	}
	for _, f := range files {
		if _, err := (Loader{}).Load(f); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}

func TestTargetPlaceholders(t *testing.T) {
	dir := t.TempDir()
	secrets := filepath.Join(dir, "s.env")
	os.WriteFile(secrets, []byte("# c\nSTAGING_HOST=10.0.0.1\nSTAGING_URL=\"http://stg\"\n"), 0o600)
	targets := filepath.Join(dir, "t.yaml")
	t.Setenv("STRESS_TEST_BIN", "/opt/lg")
	os.WriteFile(targets, []byte(`
staging:
  secretsFile: `+secrets+`
  api: {baseURL: "${secret:STAGING_URL}", login: dex}
  loadgen:
    binary: "${env:STRESS_TEST_BIN}"
    launch: [ssh, "root@${secret:STAGING_HOST}"]
broken:
  secretsFile: `+secrets+`
  api: {token: "${secret:NOPE}"}
`), 0o600)
	tg, err := TargetLoader{}.Load(targets, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if tg.API.BaseURL != "http://stg" || tg.Loadgen.Launch[1] != "root@10.0.0.1" || tg.Loadgen.Binary != "/opt/lg" || tg.API.Login != LoginDex {
		t.Errorf("target = %+v", tg)
	}
	if _, err := (TargetLoader{}).Load(targets, "broken"); err == nil || !strings.Contains(err.Error(), "secret:NOPE") {
		t.Errorf("missing secret error = %v", err)
	}
	if _, err := (TargetLoader{}).Load(targets, "absent"); err == nil {
		t.Error("absent target accepted")
	}
}
