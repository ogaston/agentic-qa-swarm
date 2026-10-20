package main

import (
	"bytes"
	"encoding/json"
	"io"
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

// --- U8-T08b: cuerpo JSON en los pasos e invariant_probe en el ensayo ---

// recibidos guarda lo que el servidor de prueba vio en la última petición.
type recibidos struct {
	body        string
	contentType string
	method      string
	path        string
}

// ordersHandler replica POST /orders de demo/target-app/main.go:170-186 y store.go:244 (sin base de datos):
// validación de cuerpo, stock 2 para el producto 1, y el defecto oversell que omite la comprobación de stock.
func ordersHandler(oversell bool, last *recibidos) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if last != nil {
			b := new(bytes.Buffer)
			_, _ = b.ReadFrom(r.Body)
			*last = recibidos{body: b.String(), contentType: r.Header.Get("Content-Type"), method: r.Method, path: r.URL.Path}
		}
		if r.Method != http.MethodPost || r.URL.Path != "/orders" {
			w.WriteHeader(404)
			return
		}
		var in struct {
			ProductID int64 `json:"product_id"`
			Qty       int   `json:"qty"`
		}
		var raw []byte
		if last != nil {
			raw = []byte(last.body)
		} else {
			raw, _ = io.ReadAll(r.Body)
		}
		if err := json.Unmarshal(raw, &in); err != nil || in.ProductID <= 0 || in.Qty <= 0 {
			w.WriteHeader(400) // invalid_body
			return
		}
		if !oversell && in.Qty > 2 {
			w.WriteHeader(409) // insufficient_stock
			return
		}
		w.WriteHeader(201)
	})
}

// Rojo primero (CA-4): con el contrato actual el paso no puede enviar qty, así que el defecto no se activa.
func TestOversellDefectoAlcanzableConCuerpo(t *testing.T) {
	const flow = `[{"method":"POST","path":"/orders","body":{"product_id":1,"qty":5},"expect_status":409}]`
	for _, tc := range []struct {
		name     string
		oversell bool
		wantExit int
	}{{"sin defecto", false, exitOK}, {"con defecto oversell", true, exitFailed}} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(ordersHandler(tc.oversell, nil))
			defer srv.Close()
			var out bytes.Buffer
			code := run([]string{"http-steps", "--flow", "f-oversell", "--target", srv.URL},
				envOf(map[string]string{"RUNNER_STEPS": flow, "RUNNER_INVARIANT": "el stock nunca es negativo"}),
				&out, &bytes.Buffer{}, newClient(stepTimeout))
			if code != tc.wantExit {
				t.Fatalf("salida %d, quería %d; stdout=%s", code, tc.wantExit, out.String())
			}
			if tc.oversell {
				rs := parseLines(t, out.Bytes())
				if len(rs) != 1 || rs[0].Status != 201 || rs[0].Invariant != "el stock nunca es negativo" {
					t.Fatalf("resultado sin el 201 o sin la invariante citada: %+v", rs)
				}
			}
		})
	}
}

// Rojo primero (CA-2a): con body, el servidor recibe el JSON y Content-Type application/json.
func TestBodyEnviaJSONConContentType(t *testing.T) {
	var last recibidos
	srv := httptest.NewServer(ordersHandler(true, &last))
	defer srv.Close()
	steps := `[{"method":"POST","path":"/orders","body":{"product_id":1,"qty":5},"expect_status":201}]`
	var out bytes.Buffer
	if code := run([]string{"http-steps", "--flow", "f", "--target", srv.URL},
		envOf(map[string]string{"RUNNER_STEPS": steps, "RUNNER_INVARIANT": "i"}), &out, &bytes.Buffer{}, newClient(stepTimeout)); code != exitOK {
		t.Fatalf("salida %d: %s", code, out.String())
	}
	if last.contentType != "application/json" {
		t.Fatalf("Content-Type %q, quería application/json", last.contentType)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(last.body), &got); err != nil || got["qty"] != float64(5) || got["product_id"] != float64(1) {
		t.Fatalf("cuerpo recibido %q no es el JSON del paso (err=%v)", last.body, err)
	}
}

// Rojo primero (CA-2b): sin body, el cuerpo llega vacío y sin Content-Type.
func TestSinBodyNoHayCuerpoNiCabecera(t *testing.T) {
	var last recibidos
	srv := httptest.NewServer(ordersHandler(true, &last))
	defer srv.Close()
	steps := `[{"method":"POST","path":"/orders","expect_status":400}]`
	var out bytes.Buffer
	if code := run([]string{"http-steps", "--flow", "f", "--target", srv.URL},
		envOf(map[string]string{"RUNNER_STEPS": steps, "RUNNER_INVARIANT": "i"}), &out, &bytes.Buffer{}, newClient(stepTimeout)); code != exitOK {
		t.Fatalf("salida %d: %s", code, out.String())
	}
	if last.body != "" || last.contentType != "" {
		t.Fatalf("sin body se esperaba cuerpo vacío y sin Content-Type; llegó body=%q ct=%q", last.body, last.contentType)
	}
}

// Rojo primero (CA-3): semántica del ensayo. Solo rehearse con REHEARSAL_INVARIANT_PROBE=true relaja el paso.
func rehearseCase(t *testing.T, do doer, steps string, probe string) int {
	t.Helper()
	m := map[string]string{"REHEARSAL_STEPS": steps, "REHEARSAL_INVARIANT": "i"}
	if probe != "" {
		m["REHEARSAL_INVARIANT_PROBE"] = probe
	}
	return run([]string{"rehearse", "--run", "run-1", "--flow", "f", "--target", "http://warm.test"}, envOf(m), &bytes.Buffer{}, &bytes.Buffer{}, do)
}

type fixedDoer struct {
	status int
	err    error
}

func (f fixedDoer) Do(r *http.Request) (*http.Response, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &http.Response{StatusCode: f.status, Body: http.NoBody, Request: r}, nil
}

func TestRehearseEstrictoFlujoNormalEstadoDistintoSale1(t *testing.T) {
	steps := `[{"method":"GET","path":"/health","expect_status":200}]`
	if code := rehearseCase(t, fixedDoer{status: 409}, steps, ""); code != exitFailed {
		t.Fatalf("salida %d, quería 1", code)
	}
	if code := rehearseCase(t, fixedDoer{status: 409}, steps, "false"); code != exitFailed {
		t.Fatalf("con probe=false salida %d, quería 1", code)
	}
}

func TestRehearseSondeoEstadoDistintoMenorA500Sale0(t *testing.T) {
	steps := `[{"method":"POST","path":"/orders","body":{"product_id":1,"qty":5},"expect_status":409}]`
	if code := rehearseCase(t, fixedDoer{status: 201}, steps, "true"); code != exitOK {
		t.Fatalf("salida %d, quería 0", code)
	}
}

func TestRehearseSondeo5xxSale1(t *testing.T) {
	steps := `[{"method":"POST","path":"/orders","body":{"product_id":1,"qty":5},"expect_status":409}]`
	if code := rehearseCase(t, fixedDoer{status: 500}, steps, "true"); code != exitFailed {
		t.Fatalf("salida %d, quería 1", code)
	}
}

func TestRehearseSondeoErrorDeRedSale1(t *testing.T) {
	steps := `[{"method":"POST","path":"/orders","body":{"product_id":1,"qty":5},"expect_status":409}]`
	if code := rehearseCase(t, fixedDoer{err: http.ErrHandlerTimeout}, steps, "true"); code != exitFailed {
		t.Fatalf("salida %d, quería 1", code)
	}
}

func TestRehearseEstrictoHTTPStepsSinCambioConSondeo(t *testing.T) {
	steps := `[{"method":"POST","path":"/orders","body":{"product_id":1,"qty":5},"expect_status":409}]`
	for _, probe := range []string{"", "true"} {
		var out bytes.Buffer
		m := map[string]string{"RUNNER_STEPS": steps, "RUNNER_INVARIANT": "i"}
		if probe != "" {
			m["RUNNER_INVARIANT_PROBE"] = probe
		}
		code := run([]string{"http-steps", "--flow", "f", "--target", "http://warm.test"}, envOf(m), &out, &bytes.Buffer{}, fixedDoer{status: 201})
		if code != exitFailed {
			t.Fatalf("http-steps con probe=%q salida %d, quería 1", probe, code)
		}
	}
}

// U8-T08b: body que no es objeto y REHEARSAL_INVARIANT_PROBE inválido son configuración inválida, sin peticiones.
func TestBodyNoObjetoYProbeInvalidoRechazanSinPeticiones(t *testing.T) {
	for _, tc := range []struct{ name, steps, probe string }{
		{"body array", `[{"method":"POST","path":"/orders","body":[1,2],"expect_status":201}]`, ""},
		{"body null", `[{"method":"POST","path":"/orders","body":null,"expect_status":201}]`, ""},
		{"probe no booleano", goodSteps, "si"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := map[string]string{"REHEARSAL_STEPS": tc.steps, "REHEARSAL_INVARIANT": "i"}
			if tc.probe != "" {
				m["REHEARSAL_INVARIANT_PROBE"] = tc.probe
			}
			d := &countingDoer{t: t}
			if code := run([]string{"rehearse", "--run", "run-1", "--flow", "f", "--target", "http://warm.test"}, envOf(m), &bytes.Buffer{}, &bytes.Buffer{}, d); code != exitConfig {
				t.Fatalf("salida %d, quería 2", code)
			}
			if d.calls.Load() != 0 {
				t.Fatalf("hubo %d peticiones", d.calls.Load())
			}
		})
	}
}
