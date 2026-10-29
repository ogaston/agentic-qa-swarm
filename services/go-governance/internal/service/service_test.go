package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-governance/authz"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/audit"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/policy"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/repos"
)

var errBoom = errors.New("boom")

// failingAudit envuelve un log real y puede fallar al escribir.
type failingAudit struct {
	*audit.Log
	fail bool
}

func (f *failingAudit) Append(e audit.Entry) (audit.Entry, error) {
	if f.fail {
		return audit.Entry{}, errBoom
	}
	return f.Log.Append(e)
}

// failingStore envuelve un almacén real: Get/Put pueden fallar o devolver JSON corrupto.
type failingStore struct {
	policy.Store
	getErr, putErr error
	corrupt        bool
}

func (f *failingStore) Get(ctx context.Context, n string) (policy.Version, bool, error) {
	if f.getErr != nil {
		return policy.Version{}, false, f.getErr
	}
	if f.corrupt {
		return policy.Version{Name: n, Version: 1, Value: json.RawMessage(`{rota`)}, true, nil
	}
	return f.Store.Get(ctx, n)
}

func (f *failingStore) Put(ctx context.Context, n string, v json.RawMessage, a string) (int, bool, error) {
	if f.putErr != nil {
		return 0, false, f.putErr
	}
	return f.Store.Put(ctx, n, v, a)
}

type env struct {
	svc   *Service
	log   *audit.Log
	fa    *failingAudit
	store *failingStore
	clock *time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	e := &env{clock: &now}
	clock := func() time.Time { return *e.clock }
	l, err := audit.Open(filepath.Join(dir, "audit.jsonl"), clock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	ps, err := policy.OpenFileStore(filepath.Join(dir, "policies.jsonl"), clock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ps.Close() })
	e.log, e.fa, e.store = l, &failingAudit{Log: l}, &failingStore{Store: ps}
	e.svc = New(Config{Policies: e.store, Audit: e.fa, TestNamespace: "aqs-test", Now: clock})
	return e
}

func okInput() authz.GateInput {
	return authz.GateInput{RunID: "r1", From: authz.StateConfirmed, To: authz.StateWarmReady, Confirmed: authz.True,
		ResetVerified: authz.True, WorkflowAllowed: authz.True, TargetNamespace: "aqs-test"}
}

func wfInput(wf string, run string) authz.GateInput {
	in := okInput()
	in.RunID, in.From, in.To = run, authz.StateRehearsing, authz.StateRunning
	in.EnsayoPassed, in.Workflow = authz.True, wf
	return in
}

const wfPolicy = `{"workflows":[{"name":"checkout","complexity":"low","max_runs_per_day":2,"timeout_seconds":600,"requires_approval":false}]}`

func TestFailClosedAuditFails(t *testing.T) {
	e := newEnv(t)
	e.fa.fail = true
	d, err := e.svc.Authorize(context.Background(), "svc", okInput())
	if d.Allow || !errors.Is(err, ErrAuditFailed) || d.Reason == "" {
		t.Fatalf("sin auditoría no puede haber allow: %+v %v", d, err)
	}
}

func TestFailClosedPolicyUnreadable(t *testing.T) {
	e := newEnv(t)
	e.store.getErr = errBoom
	d, err := e.svc.Authorize(context.Background(), "svc", wfInput("checkout", "r1"))
	if d.Allow || err == nil || d.Reason == "" {
		t.Fatalf("%+v %v", d, err)
	}
	if es := e.log.Query("r1", 5); len(es) != 1 || es[0].Action != "gate.error" {
		t.Fatalf("el error debe auditarse como gate.error: %+v", es)
	}
}

func TestFailClosedStoreReturnsError(t *testing.T) {
	e := newEnv(t)
	e.store.getErr = errors.New("almacén caído")
	d, err := e.svc.Authorize(context.Background(), "svc", wfInput("checkout", "r1"))
	if d.Allow || err == nil {
		t.Fatalf("%+v %v", d, err)
	}
	e.store.putErr = errBoom
	if _, err := e.svc.SetPolicy(context.Background(), "a1", policy.Events, json.RawMessage(`{"enabled_events":[]}`)); err == nil {
		t.Fatal("PUT debía fallar con el almacén caído")
	}
}

func TestFailClosedCorruptPolicyJSON(t *testing.T) {
	e := newEnv(t)
	e.store.corrupt = true
	d, err := e.svc.Authorize(context.Background(), "svc", wfInput("checkout", "r1"))
	if d.Allow || err == nil || !strings.Contains(d.Reason, "corrupto") {
		t.Fatalf("%+v %v", d, err)
	}
}

func TestFailClosedCancelledContext(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d, err := e.svc.Authorize(ctx, "svc", okInput())
	if d.Allow || err == nil {
		t.Fatalf("%+v %v", d, err)
	}
}

func TestFailClosedAllowNeverWithoutAuditEntry(t *testing.T) {
	e := newEnv(t)
	d, err := e.svc.Authorize(context.Background(), "svc", okInput())
	if err != nil || !d.Allow || d.AuditRef == "" {
		t.Fatalf("%+v %v", d, err)
	}
	es := e.log.Query("r1", 5)
	if len(es) != 1 || es[0].Action != "gate.allow" || es[0].Hash != d.AuditRef || es[0].Actor != "svc" {
		t.Fatalf("la decisión debe quedar auditada y referenciada: %+v", es)
	}
	if es[0].Detail["confirmed"] != "true" || es[0].Detail["target_namespace"] != "aqs-test" {
		t.Fatalf("detalle incompleto: %+v", es[0].Detail)
	}
}

func TestDenyIsAudited(t *testing.T) {
	e := newEnv(t)
	in := okInput()
	in.Confirmed = authz.Unknown
	d, err := e.svc.Authorize(context.Background(), "svc", in)
	if err != nil || d.Allow {
		t.Fatalf("%+v %v", d, err)
	}
	if es := e.log.Query("r1", 5); len(es) != 1 || es[0].Action != "gate.deny" {
		t.Fatalf("%+v", es)
	}
}

func TestWorkflowQuotaWithInjectedClock(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if _, err := e.svc.SetPolicy(ctx, "a1", policy.Workflows, json.RawMessage(wfPolicy)); err != nil {
		t.Fatal(err)
	}
	var got []bool
	for _, r := range []string{"r1", "r2", "r3"} {
		d, err := e.svc.Authorize(ctx, "svc", wfInput("checkout", r))
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, d.Allow)
	}
	if got[0] != true || got[1] != true || got[2] != false {
		t.Fatalf("cuota 2/día: %v", got)
	}
	*e.clock = e.clock.Add(24 * time.Hour) // otro día UTC
	if d, _ := e.svc.Authorize(ctx, "svc", wfInput("checkout", "r4")); !d.Allow {
		t.Fatal("el cambio de día UTC reinicia la cuota")
	}
	if d, _ := e.svc.Authorize(ctx, "svc", wfInput("nada", "r5")); d.Allow {
		t.Fatal("workflow inexistente")
	}
	if es := e.log.Query("r1", 5); es[0].Detail["workflows_policy_version"] != "1" {
		t.Fatalf("la auditoría debe registrar la versión de la política usada: %+v", es[0].Detail)
	}
}

func TestWorkflowPolicyAbsentDenies(t *testing.T) {
	e := newEnv(t)
	if d, err := e.svc.Authorize(context.Background(), "svc", wfInput("checkout", "r1")); err != nil || d.Allow {
		t.Fatalf("%+v %v", d, err)
	}
}

func TestConcurrentGateRespectsQuota(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if _, err := e.svc.SetPolicy(ctx, "a1", policy.Workflows, json.RawMessage(wfPolicy)); err != nil {
		t.Fatal(err)
	}
	res := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(i int) {
			d, _ := e.svc.Authorize(ctx, "svc", wfInput("checkout", fmt.Sprintf("rc%d", i)))
			res <- d.Allow
		}(i)
	}
	n := 0
	for i := 0; i < 10; i++ {
		if <-res {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("permitidos=%d, esperados 2 (cuota atómica)", n)
	}
}

func TestSetPolicyAuditsBeforeStoring(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.fa.fail = true
	if _, err := e.svc.SetPolicy(ctx, "a1", policy.Events, json.RawMessage(`{"enabled_events":["tag"]}`)); !errors.Is(err, ErrAuditFailed) {
		t.Fatalf("sin auditoría no hay cambio: %v", err)
	}
	if _, found, _ := e.store.Get(ctx, policy.Events); found {
		t.Fatal("la política no debe guardarse si la auditoría falló")
	}
	e.fa.fail = false
	v, err := e.svc.SetPolicy(ctx, "a1", policy.Events, json.RawMessage(`{"enabled_events":["tag"]}`))
	if err != nil || v != 1 {
		t.Fatal(v, err)
	}
	if v, _ := e.svc.SetPolicy(ctx, "a1", policy.Events, json.RawMessage(`{"enabled_events":["tag"]}`)); v != 1 {
		t.Fatal("PUT idéntico no crea versión")
	}
	_, err = e.svc.SetPolicy(ctx, "a1", policy.ConfirmRequired, json.RawMessage(`{"required":false}`))
	if !errors.Is(err, policy.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := e.svc.SetPolicy(ctx, "a1", "inventada", json.RawMessage(`{}`)); !errors.Is(err, ErrUnknownPolicy) {
		t.Fatal(err)
	}
	var actions []string
	for _, en := range e.log.Query("", 10) {
		actions = append(actions, en.Action)
	}
	want := "policy.set:events@v1,policy.set:events@v1,policy.rejected"
	if strings.Join(actions, ",") != want {
		t.Fatalf("acciones %v, esperadas %s", actions, want)
	}
}

func TestSetPolicyStoreFailureAuditsError(t *testing.T) {
	e := newEnv(t)
	e.store.putErr = errBoom
	_, _ = e.svc.SetPolicy(context.Background(), "a1", policy.Events, json.RawMessage(`{"enabled_events":[]}`))
	es := e.log.Query("", 10)
	if len(es) != 2 || es[1].Action != "policy.error" {
		t.Fatalf("%+v", es)
	}
}

func TestConcurrentSetPolicyConsecutiveVersions(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	res := make(chan int, 20)
	for i := 0; i < 20; i++ {
		go func(i int) {
			v, err := e.svc.SetPolicy(ctx, "a1", policy.WarmQuotas, json.RawMessage(fmt.Sprintf(
				`{"max_runs_per_day":%d,"rebuild_cadence_hours":1,"idle_scale_down_minutes":1,"housekeeping_grace_hours":1}`, i+1)))
			if err != nil {
				t.Error(err)
			}
			res <- v
		}(i)
	}
	seen := map[int]bool{}
	for i := 0; i < 20; i++ {
		seen[<-res] = true
	}
	for i := 1; i <= 20; i++ {
		if !seen[i] {
			t.Fatalf("hueco en la versión %d: %v", i, seen)
		}
	}
	if _, err := audit.VerifyFile(e.log.Path()); err != nil {
		t.Fatal(err)
	}
}

func TestServiceRepositories(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	// 1. Crear repositorios hasta el límite (default 3)
	r1, err := e.svc.CreateRepository(ctx, "u-alice", repos.RepositoryInput{Repo: "acme/shop"})
	if err != nil {
		t.Fatalf("CreateRepository 1: %v", err)
	}
	if r1.Owner != "u-alice" || r1.ImageRepo != "ghcr.io/acme/shop" || r1.Warm.Namespace != "aqs-w-acme-shop" {
		t.Fatalf("r1 inesperado: %+v", r1)
	}

	// Duplicado -> 409 ErrAlreadyExists
	if _, err := e.svc.CreateRepository(ctx, "u-bob", repos.RepositoryInput{Repo: "acme/shop"}); !errors.Is(err, repos.ErrAlreadyExists) {
		t.Fatalf("esperado ErrAlreadyExists, obtenido: %v", err)
	}

	// Repo inválido -> 422 ErrInvalid
	if _, err := e.svc.CreateRepository(ctx, "u-alice", repos.RepositoryInput{Repo: "INVALID REPO"}); !errors.Is(err, repos.ErrInvalid) {
		t.Fatalf("esperado ErrInvalid para repo con mayúsculas/espacios, obtenido: %v", err)
	}

	// Image repo inválida -> 422 ErrInvalid
	if _, err := e.svc.CreateRepository(ctx, "u-alice", repos.RepositoryInput{Repo: "acme/other", ImageRepo: "docker.io/acme/other"}); !errors.Is(err, repos.ErrInvalid) {
		t.Fatalf("esperado ErrInvalid para imagen fuera de ghcr, obtenido: %v", err)
	}

	// Crear repo 2 y 3
	_, err = e.svc.CreateRepository(ctx, "u-bob", repos.RepositoryInput{Repo: "acme/blog"})
	if err != nil {
		t.Fatalf("CreateRepository 2: %v", err)
	}
	_, err = e.svc.CreateRepository(ctx, "u-carol", repos.RepositoryInput{Repo: "acme/docs"})
	if err != nil {
		t.Fatalf("CreateRepository 3: %v", err)
	}

	// Crear repo 4 debe fallar por límite WARM_MAX_ENVIRONMENTS (3) -> 429
	if _, err := e.svc.CreateRepository(ctx, "u-alice", repos.RepositoryInput{Repo: "acme/extra"}); !errors.Is(err, repos.ErrWarmCapExceeded) {
		t.Fatalf("esperado ErrWarmCapExceeded en repo 4, obtenido: %v", err)
	}

	// 2. List y Get
	list, err := e.svc.ListRepositories(ctx)
	if err != nil || len(list) != 3 {
		t.Fatalf("ListRepositories: len=%d, err=%v", len(list), err)
	}
	got, found, err := e.svc.GetRepository(ctx, "acme/shop")
	if err != nil || !found || got.Repo != "acme/shop" {
		t.Fatalf("GetRepository: found=%v, err=%v", found, err)
	}

	// Virtual policy get para repositories
	pol, found, err := e.svc.GetPolicy(ctx, "repositories")
	if err != nil || !found {
		t.Fatalf("GetPolicy(repositories): found=%v, err=%v", found, err)
	}
	var polVal struct {
		Repositories []repos.Repository `json:"repositories"`
	}
	if err := json.Unmarshal(pol.Value, &polVal); err != nil || len(polVal.Repositories) != 3 {
		t.Fatalf("GetPolicy Value inválido: len=%d, err=%v", len(polVal.Repositories), err)
	}

	// 3. Patch
	// Bob no es owner de acme/shop y no es admin -> 403 ErrForbidden
	dis := false
	if _, err := e.svc.PatchRepository(ctx, "u-bob", authz.RoleUser, "acme/shop", repos.RepositoryPatch{Enabled: &dis}); !errors.Is(err, repos.ErrForbidden) {
		t.Fatalf("esperado ErrForbidden cuando un no-owner intenta parchear, obtenido: %v", err)
	}

	// Alice (owner) sí puede
	patched, err := e.svc.PatchRepository(ctx, "u-alice", authz.RoleUser, "acme/shop", repos.RepositoryPatch{Enabled: &dis})
	if err != nil || patched.Enabled != false {
		t.Fatalf("PatchRepository owner falló: %v", err)
	}

	// Admin también puede
	en := true
	patchedAdmin, err := e.svc.PatchRepository(ctx, "u-admin", authz.RoleAdmin, "acme/shop", repos.RepositoryPatch{Enabled: &en})
	if err != nil || patchedAdmin.Enabled != true {
		t.Fatalf("PatchRepository admin falló: %v", err)
	}

	// 4. Delete
	// Bob no es owner ni admin -> 403 ErrForbidden
	if err := e.svc.DeleteRepository(ctx, "u-bob", authz.RoleUser, "acme/shop"); !errors.Is(err, repos.ErrForbidden) {
		t.Fatalf("esperado ErrForbidden en delete por no-owner, obtenido: %v", err)
	}

	// Alice (owner) elimina acme/shop
	if err := e.svc.DeleteRepository(ctx, "u-alice", authz.RoleUser, "acme/shop"); err != nil {
		t.Fatalf("DeleteRepository owner: %v", err)
	}

	// Ya no existe -> 404
	if _, found, _ := e.svc.GetRepository(ctx, "acme/shop"); found {
		t.Fatal("acme/shop no debería existir tras delete")
	}

	// Ahora que hay 2, se puede crear uno nuevo sin exceder el cap
	_, err = e.svc.CreateRepository(ctx, "u-alice", repos.RepositoryInput{Repo: "acme/extra"})
	if err != nil {
		t.Fatalf("CreateRepository tras liberar cupo falló: %v", err)
	}

	// Verificar integridad de la cadena de auditoría
	if _, err := audit.VerifyFile(e.log.Path()); err != nil {
		t.Fatalf("auditoría rota tras operaciones de repositorio: %v", err)
	}
}
