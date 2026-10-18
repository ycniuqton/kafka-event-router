package store

import (
	"io/fs"
	"strings"
	"testing"
)

func TestEveryUpMigrationHasDown(t *testing.T) {
	entries, err := fs.ReadDir(Migrations, string(MigrationsRoot))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name()] = true
	}
	ups := 0
	for name := range names {
		base, isUp := strings.CutSuffix(name, string(SuffixUp))
		if !isUp {
			continue
		}
		ups++
		if !names[base+string(SuffixDown)] {
			t.Errorf("%s has no down migration", name)
		}
	}
	if ups < 3 {
		t.Fatalf("expected at least 0001..0003, got %d up migrations", ups)
	}
	if latest := NewMigrator(nil, "").Latest(); latest < 3 {
		t.Fatalf("latest = %d", latest)
	}
}

func TestMigratorDriverURL(t *testing.T) {
	m := NewMigrator(nil, "postgres://u:p@h:5432/db?sslmode=disable")
	if got := m.driverURL(); got != "pgx5://u:p@h:5432/db?sslmode=disable" {
		t.Fatal(got)
	}
}
