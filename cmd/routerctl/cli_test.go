package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goldenDir = "../../pkg/engine/testdata/golden"

func TestRunMatchesGolden(t *testing.T) {
	for _, name := range []string{"filter_only", "mask_rename", "decode_error", "strict_missing", "library_transform"} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(goldenDir, name)
			var stdout, stderr bytes.Buffer
			args := []string{"run", "--config", filepath.Join(dir, "route.json"), "--events", filepath.Join(dir, "input.jsonl"),
				"--now", "2026-01-01T00:00:00Z"}
			if _, err := os.Stat(filepath.Join(dir, "library")); err == nil {
				args = append(args, "--library", filepath.Join(dir, "library"))
			}
			code := NewCLI(&stdout, &stderr).Main(context.Background(), args)
			want, _ := os.ReadFile(filepath.Join(dir, "expected.jsonl"))
			if stdout.String() != string(want) {
				t.Fatalf("stdout mismatch\nwant:\n%s\ngot:\n%s\nstderr:\n%s", want, stdout.String(), stderr.String())
			}
			wantErrs, _ := os.ReadFile(filepath.Join(dir, "expected_errors.jsonl"))
			if stderr.String() != string(wantErrs) {
				t.Fatalf("stderr mismatch\nwant:\n%s\ngot:\n%s", wantErrs, stderr.String())
			}
			wantCode := ExitOK
			if len(wantErrs) > 0 {
				wantCode = ExitEventErrors
			}
			if code != wantCode {
				t.Fatalf("exit code %d, want %d", code, wantCode)
			}
		})
	}
}

func TestRunRejectsInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "route.json")
	events := filepath.Join(dir, "in.jsonl")
	_ = os.WriteFile(cfg, []byte(`{"apiVersion":"router/v1","name":"x","source":{"connectionId":"c","format":{"type":"json"}},"branches":[{"id":"main","filter":{"type":"cond","field":"value.a","op":"bogus","value":1},"destination":{"connectionId":"c","topic":"t"}}]}`), 0o644)
	_ = os.WriteFile(events, []byte("{}\n"), 0o644)
	var stdout, stderr bytes.Buffer
	code := NewCLI(&stdout, &stderr).Main(context.Background(), []string{"run", "--config", cfg, "--events", events})
	if code != ExitInvalidConfig || !strings.Contains(stderr.String(), `"path":"/branches/0/filter/op"`) {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
}

func TestRunUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := NewCLI(&stdout, &stderr).Main(context.Background(), []string{"bogus"}); code != ExitUsage {
		t.Fatalf("code=%d", code)
	}
	if code := NewCLI(&stdout, &stderr).Main(context.Background(), []string{"run", "--config", "x"}); code != ExitUsage {
		t.Fatalf("code=%d", code)
	}
}

func TestRunWithTraceAndStdinMissingFile(t *testing.T) {
	dir := filepath.Join(goldenDir, "filter_only")
	var stdout, stderr bytes.Buffer
	code := NewCLI(&stdout, &stderr).Main(context.Background(), []string{"run",
		"--config", filepath.Join(dir, "route.json"), "--events", filepath.Join(dir, "missing.jsonl")})
	if code != ExitIO {
		t.Fatalf("code=%d", code)
	}
	stdout.Reset()
	stderr.Reset()
	code = NewCLI(&stdout, &stderr).Main(context.Background(), []string{"run", "--trace",
		"--config", filepath.Join(dir, "route.json"), "--events", filepath.Join(dir, "input.jsonl")})
	if code != ExitOK || !strings.Contains(stderr.String(), `"status":"filtered"`) {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
}

func TestRunLibraryMissingVersion(t *testing.T) {
	dir := filepath.Join(goldenDir, "library_transform")
	var stdout, stderr bytes.Buffer
	code := NewCLI(&stdout, &stderr).Main(context.Background(), []string{"run",
		"--config", filepath.Join(dir, "route.json"), "--events", filepath.Join(dir, "input.jsonl"), "--library", t.TempDir()})
	if code != ExitInvalidConfig || !strings.Contains(stderr.String(), `"code":"library_ref_missing"`) {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
}
