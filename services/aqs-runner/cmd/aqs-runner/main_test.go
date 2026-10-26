package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

// seedStockProducto1 es el stock inicial del producto 1 en el baseline (store.go:24-31 de la app objetivo).
const seedStockProducto1 = 10

// recibidos guarda lo que el servidor de prueba vio en la última petición.
type recibidos struct {
	body        string
	contentType string
	method      string
	path        string
}

// ordersHandler replica POST /orders de main.go:170-186 y store.go:244 de la app objetivo (sin base de datos).
// Stock inicial del producto 1 = 10, el baseline real de store.go:24-31 de la app objetivo (seedProducts: {"cafe", 10}).
// Sin el defecto, PlaceOrder rechaza con 409 si stock < qty (store.go:244); con oversell se omite esa comprobación.
// No descuenta stock entre peticiones: cada prueba necesita solo la primera venta.
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
		if !oversell && in.Qty > seedStockProducto1 {
			w.WriteHeader(409) // insufficient_stock
			return
		}
		w.WriteHeader(201)
	})
}

// Rojo primero (CA-4): con el contrato actual el paso no puede enviar qty, así que el defecto no se activa.
func TestOversellDefectoAlcanzableConCuerpo(t *testing.T) {
	const flow = `[{"method":"POST","path":"/orders","body":{"product_id":1,"qty":11},"expect_status":409}]`
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
	steps := `[{"method":"POST","path":"/orders","body":{"product_id":1,"qty":11},"expect_status":201}]`
	var out bytes.Buffer
	if code := run([]string{"http-steps", "--flow", "f", "--target", srv.URL},
		envOf(map[string]string{"RUNNER_STEPS": steps, "RUNNER_INVARIANT": "i"}), &out, &bytes.Buffer{}, newClient(stepTimeout)); code != exitOK {
		t.Fatalf("salida %d: %s", code, out.String())
	}
	if last.contentType != "application/json" {
		t.Fatalf("Content-Type %q, quería application/json", last.contentType)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(last.body), &got); err != nil || got["qty"] != float64(11) || got["product_id"] != float64(1) {
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
	steps := `[{"method":"POST","path":"/orders","body":{"product_id":1,"qty":11},"expect_status":409}]`
	if code := rehearseCase(t, fixedDoer{status: 201}, steps, "true"); code != exitOK {
		t.Fatalf("salida %d, quería 0", code)
	}
}

func TestRehearseSondeo5xxSale1(t *testing.T) {
	steps := `[{"method":"POST","path":"/orders","body":{"product_id":1,"qty":11},"expect_status":409}]`
	if code := rehearseCase(t, fixedDoer{status: 500}, steps, "true"); code != exitFailed {
		t.Fatalf("salida %d, quería 1", code)
	}
}

func TestRehearseSondeoErrorDeRedSale1(t *testing.T) {
	steps := `[{"method":"POST","path":"/orders","body":{"product_id":1,"qty":11},"expect_status":409}]`
	if code := rehearseCase(t, fixedDoer{err: http.ErrHandlerTimeout}, steps, "true"); code != exitFailed {
		t.Fatalf("salida %d, quería 1", code)
	}
}

func TestRehearseEstrictoHTTPStepsSinCambioConSondeo(t *testing.T) {
	steps := `[{"method":"POST","path":"/orders","body":{"product_id":1,"qty":11},"expect_status":409}]`
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

// repoRaiz es la raiz del monorepo desde services/aqs-runner/cmd/aqs-runner.
func repoRaiz(t *testing.T) string {
	t.Helper()
	r, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// puertoLibre pide un puerto TCP libre en loopback.
func puertoLibre(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
}

// esperarHTTP espera a que la URL responda 200 (arranque de un proceso hijo).
func esperarHTTP(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if resp, err := http.Get(url); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("%s no respondió 200 a tiempo", url)
}

// arrancar lanza un proceso hijo que se detiene al terminar la prueba.
func arrancar(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
}

type endpoint struct {
	Method      string          `json:"method"`
	Path        string          `json:"path"`
	RequestBody json.RawMessage `json:"request_body,omitempty"`
}

// cuerpoMinimo genera el cuerpo valido del request_body inferido: cada propiedad required entera
// toma su minimum. Es la regla que el planner debe cumplir para POST con request_body.
func cuerpoMinimo(t *testing.T, eps []endpoint) map[string]int {
	t.Helper()
	for _, e := range eps {
		if e.Method != "POST" || e.Path != "/orders" {
			continue
		}
		var sch struct {
			Required   []string `json:"required"`
			Properties map[string]struct {
				Type    string `json:"type"`
				Minimum *int   `json:"minimum"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(e.RequestBody, &sch); err != nil {
			t.Fatalf("request_body de POST /orders no interpretable: %v", err)
		}
		body := map[string]int{}
		for _, k := range sch.Required {
			p := sch.Properties[k]
			if p.Type != "integer" || p.Minimum == nil {
				t.Fatalf("propiedad %s no entera con minimum", k)
			}
			body[k] = *p.Minimum
		}
		return body
	}
	t.Fatal("la superficie no tiene POST /orders")
	return nil
}

// CA-4: superficie inferida del OpenAPI servido por la app objetivo (parseOpenAPI), plan del planner
// con LLM falso para esa superficie, y ejecucion del flujo contra el handler REAL de la app objetivo
// (almacen en memoria, TARGET_APP_STORE=memoria). Con el cuerpo del plan: 201 y salida 0. Sin cuerpo,
// el mismo paso recibe 400 y el runner sale 1 (par falla-pasa).
// La app NO vive en este repo (U11-T02): TARGET_APP_DIR apunta al clon del repositorio externo
// ogaston/demo-target-app-qa-swarm. Sin esa variable la prueba se salta con mensaje claro.
func TestOrdersCuerpoFlujoSanoContraTargetApp(t *testing.T) {
	appDir := os.Getenv("TARGET_APP_DIR")
	if appDir == "" {
		t.Skip("CA-4 OMITIDA: define TARGET_APP_DIR con el clon de https://github.com/ogaston/demo-target-app-qa-swarm (SHA b97a719…); la app ya no vive en este repo (U11-T02)")
	}
	py := os.Getenv("AQS_PLANNER_PYTHON")
	if py == "" {
		t.Skip("CA-4 OMITIDA: define AQS_PLANNER_PYTHON con un Python que tenga jsonschema (deps del planner); ver bitacoras/U8-T09a.md")
	}
	chk := exec.Command(py, "-c", "import jsonschema")
	done := make(chan error, 1)
	if err := chk.Start(); err != nil {
		t.Fatalf("AQS_PLANNER_PYTHON=%q no ejecutable: %v", py, err)
	}
	go func() { done <- chk.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("AQS_PLANNER_PYTHON=%q no importa jsonschema: %v", py, err)
		}
	case <-time.After(2 * time.Second):
		_ = chk.Process.Kill()
		t.Fatalf("AQS_PLANNER_PYTHON=%q tardó más de 2 s en importar jsonschema", py)
	}
	raiz := repoRaiz(t)
	dir := t.TempDir()

	// 1. Binario real de la app objetivo (clon de TARGET_APP_DIR).
	bin := filepath.Join(dir, "target-app")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = appDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build de target-app: %v\n%s", err, out)
	}
	tok := filepath.Join(dir, "baseline.tok")
	if err := os.WriteFile(tok, []byte("tok-u8t09a"), 0o600); err != nil {
		t.Fatal(err)
	}
	appPort, basePort := puertoLibre(t), puertoLibre(t)
	app := "http://127.0.0.1:" + appPort
	app0 := exec.Command(bin)
	app0.Env = append(os.Environ(), "TARGET_APP_STORE=memoria", "LISTEN_ADDR=127.0.0.1:"+appPort,
		"BASELINE_LISTEN_ADDR=127.0.0.1:"+basePort, "BASELINE_TOKEN_FILE="+tok)
	arrancar(t, app0)
	esperarHTTP(t, app+"/health")

	// 2. Superficie: el OpenAPI que sirve la app, inferido con parseOpenAPI (go-warm-manager).
	resp, err := http.Get(app + "/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	inf := exec.Command("go", "run", "./cmd/openapi-endpoints")
	inf.Dir = filepath.Join(raiz, "services", "go-warm-manager")
	inf.Stdin = bytes.NewReader(doc)
	var infOut, infErr bytes.Buffer
	inf.Stdout, inf.Stderr = &infOut, &infErr
	if err := inf.Run(); err != nil {
		t.Fatalf("inferencia de superficie: %v: %s", err, infErr.String())
	}
	var eps []endpoint
	if err := json.Unmarshal(infOut.Bytes(), &eps); err != nil {
		t.Fatal(err)
	}
	surface := map[string]any{"run_id": "run-u8t09a", "base_url": app, "endpoints": eps, "source": "openapi"}

	// 3. Plan del planner con LLM falso: el fixture es la respuesta del LLM para esta superficie.
	const workflow = "pedido"
	cuerpo := cuerpoMinimo(t, eps)
	plan := map[string]any{"run_id": "run-u8t09a", "workflow": workflow, "flows": []any{map[string]any{
		"flow_id": "crear-pedido", "name": "crear pedido",
		"steps":     []any{map[string]any{"method": "POST", "path": "/orders", "body": cuerpo, "expect_status": 201}},
		"invariant": "el stock nunca es negativo",
	}}}
	fix, _ := json.Marshal(map[string]any{"surface": surface, "workflow": workflow, "plan": plan})
	fixDir := filepath.Join(dir, "fixtures")
	if err := os.Mkdir(fixDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixDir, "pedido.json"), fix, 0o600); err != nil {
		t.Fatal(err)
	}
	pport := puertoLibre(t)
	pl := exec.Command(py, "-m", "agent_planner")
	pl.Dir = filepath.Join(raiz, "agents", "agent-planner")
	pl.Env = append(os.Environ(), "PYTHONPATH="+filepath.Join(raiz, "agents", "agent-planner", "src"),
		"PLANNER_HOST=127.0.0.1", "PLANNER_PORT="+pport, "LLM_PROVIDER=fake", "PLANNER_ALLOW_FAKE=true",
		"U3_FAKE_SURFACES_DIR="+fixDir)
	arrancar(t, pl)
	planner := "http://127.0.0.1:" + pport
	esperarHTTP(t, planner+"/healthz")
	req, _ := json.Marshal(map[string]any{"surface": surface, "workflow": workflow})
	pr, err := http.Post(planner+"/v1/plan", "application/json", bytes.NewReader(req))
	if err != nil {
		t.Fatal(err)
	}
	planOut, _ := io.ReadAll(pr.Body)
	_ = pr.Body.Close()
	if pr.StatusCode != http.StatusOK {
		t.Fatalf("el planner rechazó el plan: %d %s", pr.StatusCode, planOut)
	}
	var got struct {
		Flows []struct {
			Steps []json.RawMessage `json:"steps"`
		} `json:"flows"`
	}
	if err := json.Unmarshal(planOut, &got); err != nil || len(got.Flows) != 1 || len(got.Flows[0].Steps) != 1 {
		t.Fatalf("plan devuelto inesperado: %s (%v)", planOut, err)
	}
	stepWithBody := string(got.Flows[0].Steps[0])

	// 4. Ejecucion contra el handler real: con el cuerpo del plan, y sin el (sensibilidad).
	stepNoBody := `{"method":"POST","path":"/orders","expect_status":201}`
	for _, tc := range []struct {
		name     string
		steps    string
		wantExit int
		wantCode int
	}{
		{"con body del plan", "[" + stepWithBody + "]", exitOK, 201},
		{"sin body", "[" + stepNoBody + "]", exitFailed, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			code := run([]string{"http-steps", "--flow", "crear-pedido", "--target", app},
				envOf(map[string]string{"RUNNER_STEPS": tc.steps, "RUNNER_INVARIANT": "el stock nunca es negativo"}),
				&out, &bytes.Buffer{}, newClient(stepTimeout))
			if code != tc.wantExit {
				t.Fatalf("salida %d, quería %d; stdout=%s", code, tc.wantExit, out.String())
			}
			rs := parseLines(t, out.Bytes())
			if len(rs) != 1 || rs[0].Status != tc.wantCode {
				t.Fatalf("resultado %+v, quería estado %d", rs, tc.wantCode)
			}
		})
	}
}
