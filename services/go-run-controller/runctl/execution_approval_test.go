package runctl

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// rigWith arma un rig con puertos falsos y una Config a medida (reloj y TTL inyectables).
func rigWith(t *testing.T, cfg Config) *rig {
	t.Helper()
	r := &rig{store: NewMemStore(), gate: AllowAll(), warm: &FakeWarm{Fact: True}, al: &FakeAlerter{}, pub: &FakePublisher{}, ph: &FakePhases{}}
	cfg.Gate, cfg.Store, cfg.Publisher, cfg.Warm, cfg.Alerter, cfg.Phases = r.gate, r.store, r.pub, r.warm, r.al, r.ph
	k, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	r.k = k
	return r
}

// runEnsayoAprobado siembra una corrida en rehearsing con el ensayo ya aprobado y la fase lanzada.
func runEnsayoAprobado() Run {
	return Run{ID: "r-1", State: Rehearsing, ConfirmedBy: "u-1", Flows: []string{"f"},
		EnsayoPassed: True, Launched: map[string]bool{PhaseRehearse: true}}
}

func gateVioTransicion(g *FakeGate, from, to State) bool {
	for _, c := range g.Calls {
		if c.From == from && c.To == to {
			return true
		}
	}
	return false
}

// CA-2: con ensayo aprobado, Drive espera (sin preguntar al gate por rehearsing -> running)
// y fija AwaitingSince en el almacén; solo la aprobación del dueño permite avanzar.
func TestEsperaAprobacionAntesDeRunning(t *testing.T) {
	r := rigWith(t, Config{})
	_ = r.store.Save(runEnsayoAprobado())

	for i := 0; i < 3; i++ {
		if err := r.k.Drive(t.Context(), "r-1"); err != nil {
			t.Fatalf("paso %d: %v", i, err)
		}
	}
	cur, _ := r.store.Get("r-1")
	if cur.State != Rehearsing {
		t.Fatalf("sin aprobación la corrida debía quedarse en rehearsing, está en %s", cur.State)
	}
	if gateVioTransicion(r.gate, Rehearsing, Running) {
		t.Fatalf("el gate recibió rehearsing -> running sin aprobación: %+v", r.gate.Calls)
	}
	if cur.AwaitingSince.IsZero() {
		t.Fatal("AwaitingSince debía quedar fijado en el almacén")
	}

	if err := r.k.ApproveExecution(t.Context(), "r-1", "u-1", false); err != nil {
		t.Fatalf("la aprobación del dueño debía funcionar: %v", err)
	}
	if err := r.k.Drive(t.Context(), "r-1"); err != nil {
		t.Fatalf("Drive tras aprobar: %v", err)
	}
	cur, _ = r.store.Get("r-1")
	if cur.State != Running {
		t.Fatalf("tras la aprobación del dueño la corrida debía avanzar a running, está en %s", cur.State)
	}
}

// CA-3: permisos e idempotencia de ApproveExecution.
func TestApproveExecution(t *testing.T) {
	cases := []struct {
		name      string
		seed      *Run
		principal string
		admin     bool
		want      error
		wantBy    string // ExecutionApprovedBy esperado tras la llamada ("" = sin cambio)
	}{
		{
			name: "no existe", seed: nil, principal: "u-1",
			want: ErrNotFound,
		},
		{
			name: "otro usuario", seed: &Run{ID: "r-1", State: Rehearsing, ConfirmedBy: "u-1", EnsayoPassed: True},
			principal: "u-2",
			want:      ErrForbidden,
		},
		{
			name: "admin ajeno", seed: &Run{ID: "r-1", State: Rehearsing, ConfirmedBy: "u-1", EnsayoPassed: True},
			principal: "admin-9", admin: true,
			want: nil, wantBy: "admin-9",
		},
		{
			name: "antes del ensayo", seed: &Run{ID: "r-1", State: Rehearsing, ConfirmedBy: "u-1"},
			principal: "u-1",
			want:      ErrNotAwaiting,
		},
		{
			name: "en running", seed: &Run{ID: "r-1", State: Running, ConfirmedBy: "u-1", EnsayoPassed: True},
			principal: "u-1",
			want:      ErrNotAwaiting,
		},
		{
			name: "ya aprobada por otra persona", seed: &Run{ID: "r-1", State: Rehearsing, ConfirmedBy: "u-1",
				EnsayoPassed: True, ExecutionApprovedBy: "u-2"},
			principal: "u-1",
			want:      ErrNotAwaiting, wantBy: "u-2",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := rigWith(t, Config{})
			if c.seed != nil {
				_ = r.store.Save(*c.seed)
			}
			err := r.k.ApproveExecution(t.Context(), "r-1", c.principal, c.admin)
			if !errors.Is(err, c.want) {
				t.Fatalf("error = %v, esperado %v", err, c.want)
			}
			if c.seed != nil {
				cur, _ := r.store.Get("r-1")
				if cur.ExecutionApprovedBy != c.wantBy {
					t.Fatalf("ExecutionApprovedBy = %q, esperado %q", cur.ExecutionApprovedBy, c.wantBy)
				}
			}
		})
	}

	t.Run("dueño dos veces es idempotente", func(t *testing.T) {
		r := rigWith(t, Config{})
		_ = r.store.Save(runEnsayoAprobado())
		for i := 0; i < 2; i++ {
			if err := r.k.ApproveExecution(t.Context(), "r-1", "u-1", false); err != nil {
				t.Fatalf("llamada %d: %v", i, err)
			}
		}
		cur, _ := r.store.Get("r-1")
		if cur.ExecutionApprovedBy != "u-1" {
			t.Fatalf("el store debía conservar un solo ExecutionApprovedBy=u-1, tiene %q", cur.ExecutionApprovedBy)
		}
	})
}

// CA-4: sin aprobación, pasados 30 min + 1 s la corrida sale por resetting con FailReason;
// a los 29 min sigue esperando.
func TestAprobacionCaduca(t *testing.T) {
	base := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	now := base
	r := rigWith(t, Config{Now: func() time.Time { return now }, ExecutionApprovalTTL: 30 * time.Minute})
	_ = r.store.Save(runEnsayoAprobado())

	if err := r.k.Drive(t.Context(), "r-1"); err != nil {
		t.Fatal(err)
	}
	cur, _ := r.store.Get("r-1")
	if cur.State != Rehearsing || !cur.AwaitingSince.Equal(base) {
		t.Fatalf("estado %s awaiting_since %v (esperado rehearsing desde %v)", cur.State, cur.AwaitingSince, base)
	}

	now = base.Add(29 * time.Minute)
	if err := r.k.Drive(t.Context(), "r-1"); err != nil {
		t.Fatal(err)
	}
	if cur, _ = r.store.Get("r-1"); cur.State != Rehearsing {
		t.Fatalf("a los 29 min debía seguir esperando, está en %s", cur.State)
	}

	now = base.Add(30*time.Minute + time.Second)
	if err := r.k.Drive(t.Context(), "r-1"); err != nil {
		t.Fatal(err)
	}
	cur, _ = r.store.Get("r-1")
	if cur.State != Resetting {
		t.Fatalf("caducada la aprobación debía salir por resetting, está en %s", cur.State)
	}
	if !strings.Contains(cur.FailReason, "no aprobada") {
		t.Fatalf("FailReason debía registrar la caducidad: %q", cur.FailReason)
	}
}
