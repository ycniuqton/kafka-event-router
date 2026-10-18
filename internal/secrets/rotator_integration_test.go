package secrets_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/secrets"
	"github.com/eventrouter/router/internal/secrets/local"
	"github.com/eventrouter/router/internal/testkit"
)

// TestRotationRewrapsAllEnvelopes (PC-76): every envelope moves to the new master key over several pages,
// payloads stay readable once the old key file is gone, and a rerun is a no-op.
func TestRotationRewrapsAllEnvelopes(t *testing.T) {
	sb := testkit.NewManager().Postgres(t)
	sb.Migrate(t)
	ctx := context.Background()
	dir := t.TempDir()
	oldPath, newPath := filepath.Join(dir, "old.key"), filepath.Join(dir, "new.key")
	oldKey, err := local.Open(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	repo := secrets.NewPostgresRepository(sb.DB)
	before := secrets.NewEnvelopeManager(oldKey, repo, secrets.AESGCMCipher{})
	const total = 230
	plain := map[secrets.Ref]string{}
	for i := range total {
		value := fmt.Sprintf(`{"password":"pw-%d"}`, i)
		ref, err := before.Put(ctx, []byte(value))
		if err != nil {
			t.Fatal(err)
		}
		plain[ref] = value
	}
	ring, err := local.Open(newPath, oldPath)
	if err != nil {
		t.Fatal(err)
	}
	rot := secrets.NewRotator(secrets.RotatorDeps{Provider: ring, Repo: repo, Recorder: audit.NewPostgresRecorder(sb.DB),
		Actor: audit.Actor{Kind: audit.ActorSystem, Name: "test"}, PageSize: 100})
	report, err := rot.Run(ctx)
	if err != nil || report.Rotated != total || report.ToKeyID != ring.KeyID() || len(report.FromKeyIDs) != 1 || report.FromKeyIDs[0] != oldKey.KeyID() {
		t.Fatalf("rotation: %+v %v", report, err)
	}
	if err := os.Remove(oldPath); err != nil {
		t.Fatal(err)
	}
	newOnly, err := local.Open(newPath)
	if err != nil {
		t.Fatal(err)
	}
	after := secrets.NewEnvelopeManager(newOnly, repo, secrets.AESGCMCipher{})
	for ref, want := range plain {
		got, err := after.Get(ctx, ref)
		if err != nil || string(got) != want {
			t.Fatalf("%s after rotation: %q %v", ref, got, err)
		}
	}
	assertRotationState(t, sb, rot, ring.KeyID(), total)
}

func assertRotationState(t *testing.T, sb *testkit.PostgresSandbox, rot *secrets.Rotator, keyID string, total int) {
	t.Helper()
	ctx := context.Background()
	usage, err := rot.Usage(ctx)
	if err != nil || len(usage) != 1 || usage[0].KeyID != keyID || usage[0].Count != total {
		t.Fatalf("usage: %+v %v", usage, err)
	}
	var unrotated int
	_ = sb.DB.Pool().QueryRow(ctx, `SELECT count(*) FROM secrets WHERE rotated_at IS NULL`).Scan(&unrotated)
	if unrotated != 0 {
		t.Fatalf("%d envelopes without rotated_at", unrotated)
	}
	again, err := rot.Run(ctx)
	if err != nil || again.Rotated != 0 {
		t.Fatalf("rerun: %+v %v", again, err)
	}
	rows, err := sb.DB.Pool().Query(ctx, `SELECT diff::text FROM audit_log WHERE action = $1`, string(audit.ActionSecretsRotated))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var diffs []secrets.RotationReport
	for rows.Next() {
		var raw string
		_ = rows.Scan(&raw)
		var r secrets.RotationReport
		_ = json.Unmarshal([]byte(raw), &r)
		diffs = append(diffs, r)
	}
	if len(diffs) != 1 || diffs[0].Rotated != total || diffs[0].ToKeyID != keyID {
		t.Fatalf("secrets.rotated audit rows: %+v", diffs)
	}
}
