//go:build contract

package main

import (
	"errors"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/adapters"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

// Requiere go-governance real en GOVERNANCE_URL con GOVERNANCE_SERVICE_TOKEN; sin ellos se omite.
func realGate(t *testing.T) (*adapters.HTTPGate, string) {
	t.Helper()
	u, tok := os.Getenv("GOVERNANCE_URL"), os.Getenv("GOVERNANCE_SERVICE_TOKEN")
	if u == "" || tok == "" {
		t.Skip("sin GOVERNANCE_URL/GOVERNANCE_SERVICE_TOKEN: prueba de contrato omitida")
	}
	g, err := adapters.NewHTTPGate(u, tok)
	if err != nil {
		t.Fatal(err)
	}
	return g, u
}

func ctl(t *testing.T, gate runctl.GateClient, ns string, r runctl.Run) (*runctl.Controller, *runctl.MemStore) {
	st := runctl.NewMemStore()
	_ = st.Save(r)
	k, err := runctl.New(runctl.Config{Namespace: ns, Gate: gate, Store: st, Publisher: &runctl.FakePublisher{},
		Warm: &runctl.FakeWarm{Fact: runctl.True}, Alerter: &runctl.FakeAlerter{}, Phases: &runctl.FakePhases{}})
	if err != nil {
		t.Fatal(err)
	}
	return k, st
}

func base() runctl.Run {
	return runctl.Run{ID: "r-c", State: runctl.Confirmed, ConfirmedBy: "u-1", Flows: []string{"checkout"}}
}

func TestContractLegalTransitionWithTrueFactsAllowed(t *testing.T) {
	g, _ := realGate(t)
	k, st := ctl(t, g, "", base())
	if err := k.Transition(t.Context(), "r-c", runctl.WarmReady); err != nil {
		t.Fatal(err)
	}
	if r, _ := st.Get("r-c"); r.State != runctl.WarmReady {
		t.Fatal(r.State)
	}
}

func TestContractWithoutConfirmationDenied(t *testing.T) {
	g, _ := realGate(t)
	r := base()
	r.ConfirmedBy = ""
	k, st := ctl(t, g, "", r)
	if err := k.Transition(t.Context(), "r-c", runctl.WarmReady); !errors.Is(err, runctl.ErrDenied) {
		t.Fatal(err)
	}
	if got, _ := st.Get("r-c"); got.State != runctl.Confirmed {
		t.Fatal("avanzó")
	}
}

func TestContractOtherNamespaceDenied(t *testing.T) {
	g, _ := realGate(t)
	k, st := ctl(t, g, "aqs-otro", base())
	if err := k.Transition(t.Context(), "r-c", runctl.WarmReady); !errors.Is(err, runctl.ErrDenied) {
		t.Fatal(err)
	}
	if got, _ := st.Get("r-c"); got.State != runctl.Confirmed {
		t.Fatal("avanzó")
	}
}

func TestContractStoppedGateDoesNotAdvance(t *testing.T) {
	realGate(t) // solo para omitir sin entorno
	srv := httptest.NewServer(nil)
	url := srv.URL
	srv.Close() // gate detenido: nadie escucha
	g, err := adapters.NewHTTPGate(url, "x")
	if err != nil {
		t.Fatal(err)
	}
	k, st := ctl(t, g, "", base())
	if err := k.Transition(t.Context(), "r-c", runctl.WarmReady); !errors.Is(err, runctl.ErrGate) {
		t.Fatal(err)
	}
	if got, _ := st.Get("r-c"); got.State != runctl.Confirmed {
		t.Fatal("avanzó")
	}
}
