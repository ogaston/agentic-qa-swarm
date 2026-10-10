//go:build contract

package adapters

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// Contrato U2↔U3 contra un stub HTTP local del contrato (POST /v1/plan {surface, workflow} → FlowPlan).
// Si U3_URL está definida (U3-T07), la prueba de plan válido corre contra ese servicio en vez del stub.

func TestContractU3ValidFlowPlan(t *testing.T) {
	url := os.Getenv("U3_URL")
	if url == "" {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/v1/plan" || r.Header.Get("Content-Type") != "application/json" {
				js(w, 404, `{"error":"not_found"}`)
				return
			}
			js(w, 200, validPlanJSON)
		}))
		defer s.Close()
		url = s.URL
	}
	fp, err := newU3(t, url).Flows("r1")
	if err != nil || len(fp.Flows) == 0 || fp.RunID != "r1" {
		t.Fatalf("FlowPlan válido rechazado: %+v %v", fp, err)
	}
}

func TestContractU3InvalidResponseFailsPhase(t *testing.T) {
	for name, body := range map[string]string{
		"sin flujos":      `{"run_id":"r1","workflow":"checkout","flows":[]}`,
		"paso fuera de /": `{"run_id":"r1","workflow":"checkout","flows":[{"flow_id":"f","name":"n","invariant":"i","steps":[{"method":"GET","path":"http://evil.example/","expect_status":200}]}]}`,
		"no es un plan":   `{"error":"flows_not_publishable"}`,
	} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { js(w, 200, body) }))
		_, err := newU3(t, s.URL).Flows("r1")
		s.Close()
		if err == nil {
			t.Errorf("%s: respuesta inválida aceptada", name)
		}
	}
}

func TestContractU3StoppedStubFailsPhase(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { js(w, 200, validPlanJSON) }))
	f := newU3(t, s.URL)
	s.Close() // servicio detenido
	if _, err := f.Flows("r1"); err == nil {
		t.Fatal("U3 detenido aceptado")
	}
}
