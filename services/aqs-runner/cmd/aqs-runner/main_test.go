package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const goodSteps = `[{"method":"GET","path":"/health","expect_status":200},{"method":"POST","path":"/orders","expect_status":201}]`

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// countingDoer falla el test si se llama: sirve para probar que una validación rechaza ANTES de cualquier petición.
type countingDoer struct {
	t     *testing.T
	calls atomic.Int32
}

func (c *countingDoer) Do(*http.Request) (*http.Response, error) {
	c.calls.Add(1)
	c.t.Errorf("se hizo una petición y no debía")
	return nil, http.ErrHandlerTimeout
}

func parseLines(t *testing.T, b []byte) []result {
	t.Helper()
	var out []result
	for _, ln := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if ln == "" {
			continue
		}
		var r result
		if err := json.Unmarshal([]byte(ln), &r); err != nil {
			t.Fatalf("línea de stdout no es JSON: %q: %v", ln, err)
		}
		out = append(out, r)
	}
	return out
}

func okServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/health":
			w.WriteHeader(200)
		case r.Method == http.MethodPost && r.URL.Path == "/orders":
			w.WriteHeader(201)
		default:
			w.WriteHeader(404)
		}
	}))
}

func TestRehearseTodosCumplen(t *testing.T) {
	srv := okServer(t)
	defer srv.Close()
	var out, errb bytes.Buffer
	rc := run([]string{"rehearse", "--run", "r-1", "--flow", "f-1", "--target", srv.URL}, envOf(map[string]string{
		"REHEARSAL_STEPS": goodSteps, "REHEARSAL_INVARIANT": "inv"}), &out, &errb, newClient(2*time.Second))
	if rc != 0 {
		t.Fatalf("rc=%d, quiero 0; stderr=%q", rc, errb.String())
	}
	rs := parseLines(t, out.Bytes())
	if len(rs) != 2 {
		t.Fatalf("quiero un JSON por paso (2), salieron %d", len(rs))
	}
	for _, r := range rs {
		if !r.OK || r.FlowID != "f-1" || r.Invariant != "inv" {
			t.Errorf("paso no OK o mal etiquetado: %+v", r)
		}
	}
}

func TestHTTPStepsTodosCumplen(t *testing.T) {
	srv := okServer(t)
	defer srv.Close()
	var out, errb bytes.Buffer
	rc := run([]string{"http-steps", "--flow", "f-1", "--target", srv.URL}, envOf(map[string]string{
		"RUNNER_STEPS": goodSteps, "RUNNER_INVARIANT": "inv"}), &out, &errb, newClient(2*time.Second))
	if rc != 0 {
		t.Fatalf("rc=%d, quiero 0; stderr=%q", rc, errb.String())
	}
}

func TestPasoConEstadoDistintoEsDistintoDeCero(t *testing.T) {
	srv := okServer(t)
	defer srv.Close()
	steps := `[{"method":"GET","path":"/health","expect_status":200},{"method":"GET","path":"/health","expect_status":500}]`
	var out, errb bytes.Buffer
	rc := run([]string{"rehearse", "--run", "r-1", "--flow", "f-1", "--target", srv.URL}, envOf(map[string]string{
		"REHEARSAL_STEPS": steps, "REHEARSAL_INVARIANT": "inv"}), &out, &errb, newClient(2*time.Second))
	if rc == 0 {
		t.Fatalf("rc=0 con un paso que no cumple su expect_status")
	}
	rs := parseLines(t, out.Bytes())
	if len(rs) != 2 || !rs[0].OK || rs[1].OK || rs[1].Status != 200 {
		t.Fatalf("resumen por paso incorrecto: %+v", rs)
	}
}

func TestDestinoCaidoEsDistintoDeCero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	dead := srv.URL
	srv.Close()
	var out, errb bytes.Buffer
	rc := run([]string{"http-steps", "--flow", "f-1", "--target", dead}, envOf(map[string]string{
		"RUNNER_STEPS": goodSteps, "RUNNER_INVARIANT": "inv"}), &out, &errb, newClient(2*time.Second))
	if rc == 0 {
		t.Fatalf("rc=0 con el destino caído")
	}
	rs := parseLines(t, out.Bytes())
	if len(rs) == 0 || rs[0].OK || rs[0].Status != 0 || rs[0].Error == "" {
		t.Fatalf("un paso sin respuesta debe salir con status 0 y error: %+v", rs)
	}
}

func TestTargetInvalidoRechazaSinPeticiones(t *testing.T) {
	srv := okServer(t)
	defer srv.Close()
	for _, target := range []string{"ftp://x:21", "http://", "warm-app:8080", "", "://bad", "http:/solo-ruta"} {
		c := &countingDoer{t: t}
		var out, errb bytes.Buffer
		rc := run([]string{"http-steps", "--flow", "f-1", "--target", target}, envOf(map[string]string{
			"RUNNER_STEPS": goodSteps, "RUNNER_INVARIANT": "inv"}), &out, &errb, c)
		if rc == 0 {
			t.Errorf("target %q: rc=0, quiero distinto de 0", target)
		}
		if c.calls.Load() != 0 || out.Len() != 0 {
			t.Errorf("target %q: hubo peticiones o salida (%d, %q)", target, c.calls.Load(), out.String())
		}
	}
}

func TestPathSinBarraOMetodoInvalidoRechazaSinPeticiones(t *testing.T) {
	cases := map[string]string{
		"sin barra":    `[{"method":"GET","path":"health","expect_status":200}]`,
		"metodo raro":  `[{"method":"TRACE","path":"/health","expect_status":200}]`,
		"status fuera": `[{"method":"GET","path":"/health","expect_status":42}]`,
		"vacio":        `[]`,
		"json roto":    `[{"method":`,
	}
	for name, steps := range cases {
		c := &countingDoer{t: t}
		var out, errb bytes.Buffer
		rc := run([]string{"rehearse", "--run", "r-1", "--flow", "f-1", "--target", "http://127.0.0.1:9"}, envOf(map[string]string{
			"REHEARSAL_STEPS": steps, "REHEARSAL_INVARIANT": "inv"}), &out, &errb, c)
		if rc != 2 {
			t.Errorf("%s: rc=%d, quiero 2 (configuración inválida)", name, rc)
		}
		if c.calls.Load() != 0 {
			t.Errorf("%s: hubo peticiones", name)
		}
	}
}

func TestEnvObligatorioAusenteRechaza(t *testing.T) {
	c := &countingDoer{t: t}
	var out, errb bytes.Buffer
	rc := run([]string{"rehearse", "--run", "r-1", "--flow", "f-1", "--target", "http://127.0.0.1:9"},
		envOf(map[string]string{"REHEARSAL_INVARIANT": "inv"}), &out, &errb, c)
	if rc != 2 || c.calls.Load() != 0 {
		t.Fatalf("sin REHEARSAL_STEPS: rc=%d peticiones=%d", rc, c.calls.Load())
	}
}

func TestRehearseSinRunRechaza(t *testing.T) {
	c := &countingDoer{t: t}
	var out, errb bytes.Buffer
	rc := run([]string{"rehearse", "--flow", "f-1", "--target", "http://127.0.0.1:9"}, envOf(map[string]string{
		"REHEARSAL_STEPS": goodSteps, "REHEARSAL_INVARIANT": "inv"}), &out, &errb, c)
	if rc != 2 || c.calls.Load() != 0 {
		t.Fatalf("sin --run: rc=%d peticiones=%d", rc, c.calls.Load())
	}
}

func TestRedireccionNoSeSigue(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("se siguió una redirección fuera del destino")
		w.WriteHeader(200)
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/x", http.StatusFound)
	}))
	defer srv.Close()
	var out, errb bytes.Buffer
	rc := run([]string{"http-steps", "--flow", "f-1", "--target", srv.URL}, envOf(map[string]string{
		"RUNNER_STEPS": `[{"method":"GET","path":"/go","expect_status":302}]`, "RUNNER_INVARIANT": "inv"}),
		&out, &errb, newClient(2*time.Second))
	if rc != 0 {
		t.Fatalf("rc=%d; la 302 debe contarse como estado observado: %q", rc, errb.String())
	}
}

func TestTimeoutDePasoEsFallo(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer srv.Close()
	defer close(block)
	var out, errb bytes.Buffer
	rc := run([]string{"http-steps", "--flow", "f-1", "--target", srv.URL}, envOf(map[string]string{
		"RUNNER_STEPS": `[{"method":"GET","path":"/lento","expect_status":200}]`, "RUNNER_INVARIANT": "inv"}),
		&out, &errb, newClient(100*time.Millisecond))
	if rc == 0 {
		t.Fatalf("rc=0 con un paso que excede el timeout")
	}
}

func TestAyudaSaleCero(t *testing.T) {
	var out, errb bytes.Buffer
	if rc := run([]string{"--help"}, envOf(nil), &out, &errb, &countingDoer{t: t}); rc != 0 {
		t.Fatalf("--help rc=%d", rc)
	}
	if rc := run([]string{"comando-inexistente"}, envOf(nil), &out, &errb, &countingDoer{t: t}); rc != 2 {
		t.Fatalf("subcomando desconocido rc=%d, quiero 2", rc)
	}
}

func TestRehearseHelpSaleCero(t *testing.T) {
	var out, errb bytes.Buffer
	if rc := run([]string{"rehearse", "--help"}, envOf(nil), &out, &errb, &countingDoer{t: t}); rc != 0 {
		t.Fatalf("rehearse --help rc=%d, quiero 0; stderr=%q", rc, errb.String())
	}
	if rc := run([]string{"http-steps", "-h"}, envOf(nil), &out, &errb, &countingDoer{t: t}); rc != 0 {
		t.Fatalf("http-steps -h rc=%d, quiero 0", rc)
	}
}
