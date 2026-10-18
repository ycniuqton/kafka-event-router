package connections_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/internal/connections/connectionstest"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/pkg/kafka"
)

// fakeAccess records the grant question access-control is asked.
type fakeAccess struct {
	allowed bool
	asked   []uuid.UUID
}

func (f *fakeAccess) CanUseConnection(_ context.Context, team, _ uuid.UUID) (bool, error) {
	f.asked = append(f.asked, team)
	return f.allowed, nil
}

func TestServiceCanUseDelegatesToAccessControl(t *testing.T) {
	f := newServiceFixture()
	access := &fakeAccess{allowed: true}
	svc := connections.NewService(connections.ServiceDeps{Repo: f.repo, Usage: f.usage, Vault: f.vault, Tx: connectionstest.DirectTx{}, Access: access,
		Validator: connections.DefaultValidator(kafka.DefaultSecurityRegistry(), kafka.DefaultPropertyCatalog())})
	ctx := context.Background()
	d := validDraft()
	d.TeamID = f.team
	c, err := svc.Create(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := svc.CanUse(ctx, f.team, c.ID); !ok || err != nil || len(access.asked) != 0 {
		t.Fatalf("owner must be allowed without asking: %v %v %v", ok, err, access.asked)
	}
	other := uuid.New()
	if ok, _ := svc.CanUse(ctx, other, c.ID); !ok || len(access.asked) != 1 || access.asked[0] != other {
		t.Fatalf("other team must be delegated: %v", access.asked)
	}
	access.allowed = false
	if ok, _ := svc.CanUse(ctx, other, c.ID); ok {
		t.Fatal("denied grant must be denied")
	}
	if _, err := svc.CanUse(ctx, other, uuid.New()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown connection: %v", err)
	}
	if ok, _ := f.svc.CanUse(ctx, other, c.ID); ok {
		t.Fatal("without access control only the owner may use the connection")
	}
	if team, err := svc.TeamOf(ctx, c.ID.String()); err != nil || team != f.team.String() {
		t.Fatalf("team of: %q %v", team, err)
	}
}

// scopeRecorder captures audit changes of one request scope.
type scopeRecorder struct{ entries []audit.Entry }

func (r *scopeRecorder) Record(_ context.Context, e audit.Entry) error {
	r.entries = append(r.entries, e)
	return nil
}

func TestSensitiveToggleIsAuditedAndBumpsVersion(t *testing.T) {
	f := newServiceFixture()
	rec := &scopeRecorder{}
	ctx := audit.WithScope(context.Background(), audit.NewScope(audit.RequestContext{}, rec, audit.NewDiffBuilder(logging.NewRedactor())))
	d := validDraft()
	d.TeamID = f.team
	c, err := f.svc.Create(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	d.Secrets = kafka.SecretBundle{}
	d.Sensitive = true
	updated, err := f.svc.Update(ctx, c.ID, c.Version, d)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Sensitive || updated.Version != c.Version+1 {
		t.Fatalf("updated: sensitive=%v version=%d", updated.Sensitive, updated.Version)
	}
	last := rec.entries[len(rec.entries)-1]
	if last.Action != connections.ActionUpdated || !strings.Contains(string(last.Diff), `"sensitive"`) {
		t.Fatalf("audit: %s %s", last.Action, last.Diff)
	}
}

func TestSchemaRegistrySecretIsStoredAndDroppedWithTheRegistry(t *testing.T) {
	f := newServiceFixture()
	ctx := context.Background()
	d := validDraft()
	d.TeamID = f.team
	d.SchemaRegistry = &kafka.SchemaRegistrySpec{URL: "https://sr:8081", Auth: kafka.RegistryAuthSpec{Mode: kafka.RegistryAuthBearer}}
	d.Secrets.RegistrySecret = "token"
	c, err := f.svc.Create(ctx, d)
	if err != nil || !c.HasSecret(kafka.SecretRegistry) || c.SchemaRegistry == nil {
		t.Fatalf("create: %+v %v", c, err)
	}
	_ = f.repo.UpdateHealth(ctx, c.ID, connections.Health{Status: connections.HealthHealthy})
	d.Secrets = kafka.SecretBundle{}
	d.SchemaRegistry = nil
	updated, err := f.svc.Update(ctx, c.ID, c.Version, d)
	if err != nil {
		t.Fatal(err)
	}
	bundle, _ := f.vault.Load(ctx, updated.SecretRef)
	if updated.HasSecret(kafka.SecretRegistry) || bundle.RegistrySecret != "" || bundle.Password != "pw" {
		t.Fatalf("registry secret must be dropped, password kept: fields %v", updated.SecretFields)
	}
	if updated.Health.Status != connections.HealthUnknown {
		t.Fatalf("removing the registry must reset health: %s", updated.Health.Status)
	}
}

func TestPlanExtensionsAddSchemaRegistryStep(t *testing.T) {
	spec := kafka.ConnectionSpec{}
	if got := connections.DefaultPlanExtensions.Apply(connections.DefaultProbePlan, spec); got.Has(connections.StepSchemaRegistry) {
		t.Fatalf("no registry: %v", got)
	}
	spec.SchemaRegistry = &kafka.SchemaRegistrySpec{URL: "http://sr"}
	got := connections.DefaultPlanExtensions.Apply(connections.DefaultTestPlan, spec)
	if got[len(got)-1] != connections.StepSchemaRegistry || len(got) != len(connections.DefaultTestPlan)+1 {
		t.Fatalf("with registry: %v", got)
	}
	if again := connections.DefaultPlanExtensions.Apply(got, spec); len(again) != len(got) {
		t.Fatalf("step must not repeat: %v", again)
	}
}
