package httpapi

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/ui-api/inbox"
	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/auth"
)

// Tokens de persona de estas pruebas. El de la persona dueña es el que viaja al controlador.
const (
	repTokU1  = "tok-rep-u1-901"
	repTokU2  = "tok-rep-u2-902"
	repTokAdm = "tok-rep-adm-903"
)

// reportURI es una URI de reporte con la forma que escribe S3ReportStore.put.
func reportURI(runID string) string {
	return "s3://evidence/runs/" + runID + "/report-" + strings.Repeat("0", 64) + ".json"
}

// repEnv confirma n-1 por u1 y deja la API con controlador y reporter simulados (nil = sin URL).
// Devuelve el env y el run_id del recibo.
func repEnv(t *testing.T, ctl, rep *warmFake, mod ...func(*Config)) (*env, string) {
	t.Helper()
	v, err := auth.NewFakeTokenVerifier(repTokU1 + "=u1:user," + repTokU2 + "=u2:user," + repTokAdm + "=a1:admin")
	if err != nil {
		t.Fatal(err)
	}
	e := newEnv(t, v, func(c *Config) {
		if ctl != nil {
			c.RunControllerURL = ctl.srv.URL
		}
		if rep != nil {
			c.ReporterURL = rep.srv.URL
		}
		c.UpstreamTimeout = time.Second
		for _, m := range mod {
			m(c)
		}
	})
	confirm(t, e, "n-1", "u1")
	rs := e.store.Receipts(1, func(inbox.Receipt) bool { return true })
	if len(rs) != 1 {
		t.Fatalf("recibos = %d, quiero 1", len(rs))
	}
	return e, rs[0].RunID
}

// ctlRun responde GET /runs/{id} como go-run-controller: 200 con RunView (report_uri si no es vacío),
// 404 si la ruta no es de esa corrida.
func ctlRun(runID, reportURIValue string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/runs/"+runID {
			respond(http.StatusNotFound, `{"code":"not_found","message":"x"}`)(w, r)
			return
		}
		b := `{"id":"` + runID + `","state":"done","verdict":"bug"`
		if reportURIValue != "" {
			b += `,"report_uri":"` + reportURIValue + `"`
		}
		respond(http.StatusOK, b+`}`)(w, r)
	}
}

// reporterOK responde GET /v1/reports con un reporte de runID y exige la URI pedida.
func reporterOK(runID string) http.HandlerFunc {
	return respond(http.StatusOK, `{"run_id":"`+runID+`","verdict":"bug","summary":"s","findings":[]}`)
}

// dyn devuelve un handler que lee el run_id cuando llega la petición (el recibo se crea después de montar los fakes).
func dyn(ref *string, mk func(id string) http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { mk(*ref)(w, r) }
}

func TestReportOKDevuelveElReporteDelReporter(t *testing.T) {
	var runID string
	ctl := newWarmFake(t, dyn(&runID, func(id string) http.HandlerFunc { return ctlRun(id, reportURI(id)) }))
	rep := newWarmFake(t, dyn(&runID, func(id string) http.HandlerFunc { return reporterOK(id) }))
	e, id := repEnv(t, ctl, rep)
	runID = id
	w := e.do(http.MethodGet, "/reports/"+id, repTokU1, "")
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); !strings.Contains(got, `"run_id":"`+id+`"`) || !strings.Contains(got, `"verdict":"bug"`) {
		t.Fatalf("cuerpo = %s", got)
	}
	if h := ctl.hdrs[0].Get("Authorization"); h != "Bearer "+repTokU1 {
		t.Fatalf("el controlador recibió Authorization = %q, quiero el de la persona dueña", h)
	}
	if h := rep.hdrs[0].Get("Authorization"); h != "" {
		t.Fatalf("el reporter no debe recibir credenciales de persona: %q", h)
	}
	if q := rep.query(0).Get("uri"); q != reportURI(id) {
		t.Fatalf("uri pedida al reporter = %q", q)
	}
}

func TestReportSinTokenDa401SinLlamar(t *testing.T) {
	ctl := newWarmFake(t, respond(http.StatusOK, "{}"))
	rep := newWarmFake(t, respond(http.StatusOK, "{}"))
	e, id := repEnv(t, ctl, rep)
	if w := e.do(http.MethodGet, "/reports/"+id, "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("%d", w.Code)
	}
	if ctl.count() != 0 || rep.count() != 0 {
		t.Fatalf("hubo peticiones salientes sin autenticar")
	}
}

func TestReportCorridaDeOtroUsuarioDa403SinLlamar(t *testing.T) {
	ctl := newWarmFake(t, respond(http.StatusOK, "{}"))
	rep := newWarmFake(t, respond(http.StatusOK, "{}"))
	e, id := repEnv(t, ctl, rep)
	if w := e.do(http.MethodGet, "/reports/"+id, repTokU2, ""); w.Code != http.StatusForbidden {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if ctl.count() != 0 || rep.count() != 0 {
		t.Fatalf("propiedad: hubo %d/%d peticiones salientes", ctl.count(), rep.count())
	}
}

func TestReportCorridaInexistenteDa404SinLlamar(t *testing.T) {
	ctl := newWarmFake(t, respond(http.StatusOK, "{}"))
	rep := newWarmFake(t, respond(http.StatusOK, "{}"))
	e, _ := repEnv(t, ctl, rep)
	if w := e.do(http.MethodGet, "/reports/run-0000000000000000", repTokU1, ""); w.Code != http.StatusNotFound {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if ctl.count() != 0 || rep.count() != 0 {
		t.Fatalf("hubo peticiones salientes para una corrida inexistente")
	}
}

func TestReportAdminVeLaCorridaDeOtro(t *testing.T) {
	var runID string
	ctl := newWarmFake(t, dyn(&runID, func(id string) http.HandlerFunc { return ctlRun(id, reportURI(id)) }))
	rep := newWarmFake(t, dyn(&runID, func(id string) http.HandlerFunc { return reporterOK(id) }))
	e, id := repEnv(t, ctl, rep)
	runID = id
	if w := e.do(http.MethodGet, "/reports/"+id, repTokAdm, ""); w.Code != http.StatusOK {
		t.Fatalf("admin: %d %s", w.Code, w.Body.String())
	}
}

func TestReportSinReportURIDa404ReportNotReady(t *testing.T) {
	var runID string
	ctl := newWarmFake(t, dyn(&runID, func(id string) http.HandlerFunc { return ctlRun(id, "") }))
	rep := newWarmFake(t, respond(http.StatusOK, "{}"))
	e, id := repEnv(t, ctl, rep)
	runID = id
	w := e.do(http.MethodGet, "/reports/"+id, repTokU1, "")
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "report_not_ready") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if rep.count() != 0 {
		t.Fatalf("sin report_uri no debe llamar al reporter")
	}
}

func TestReportSinRunControllerURLDa503ReportsUnavailable(t *testing.T) {
	rep := newWarmFake(t, respond(http.StatusOK, "{}"))
	e, id := repEnv(t, nil, rep)
	w := e.do(http.MethodGet, "/reports/"+id, repTokU1, "")
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "reports_unavailable") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if rep.count() != 0 {
		t.Fatalf("sin controlador no debe llamar al reporter")
	}
}

func TestReportSinReporterURLDa503ReportsUnavailable(t *testing.T) {
	var runID string
	ctl := newWarmFake(t, dyn(&runID, func(id string) http.HandlerFunc { return ctlRun(id, reportURI(id)) }))
	e, id := repEnv(t, ctl, nil)
	runID = id
	w := e.do(http.MethodGet, "/reports/"+id, repTokU1, "")
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "reports_unavailable") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestReportControladorCaidoDa503ConRetryAfter(t *testing.T) {
	ctl := newWarmFake(t, respond(http.StatusInternalServerError, "{}"))
	rep := newWarmFake(t, respond(http.StatusOK, "{}"))
	e, id := repEnv(t, ctl, rep)
	w := e.do(http.MethodGet, "/reports/"+id, repTokU1, "")
	if w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") == "" ||
		!strings.Contains(w.Body.String(), "run_controller_unavailable") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if rep.count() != 0 {
		t.Fatalf("con el controlador caído no debe llamar al reporter")
	}
}

func TestReportReporterCaidoDa502(t *testing.T) {
	var runID string
	ctl := newWarmFake(t, dyn(&runID, func(id string) http.HandlerFunc { return ctlRun(id, reportURI(id)) }))
	rep := newWarmFake(t, respond(http.StatusInternalServerError, "{}"))
	e, id := repEnv(t, ctl, rep)
	runID = id
	w := e.do(http.MethodGet, "/reports/"+id, repTokU1, "")
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "reporter_unavailable") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestReportRunIDDistintoDa502(t *testing.T) {
	var runID string
	ctl := newWarmFake(t, dyn(&runID, func(id string) http.HandlerFunc { return ctlRun(id, reportURI(id)) }))
	rep := newWarmFake(t, respond(http.StatusOK, `{"run_id":"run-otro","verdict":"bug","summary":"s","findings":[]}`))
	e, id := repEnv(t, ctl, rep)
	runID = id
	w := e.do(http.MethodGet, "/reports/"+id, repTokU1, "")
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "report_mismatch") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestReportRunMalFormadoDa400SinPeticiones(t *testing.T) {
	ctl := newWarmFake(t, respond(http.StatusOK, "{}"))
	rep := newWarmFake(t, respond(http.StatusOK, "{}"))
	e, _ := repEnv(t, ctl, rep)
	for _, bad := range []string{"run%24x", "%20", strings.Repeat("a", 129)} {
		if w := e.do(http.MethodGet, "/reports/"+bad, repTokU1, ""); w.Code != http.StatusBadRequest {
			t.Fatalf("%q: %d %s", bad, w.Code, w.Body.String())
		}
	}
	if ctl.count() != 0 || rep.count() != 0 {
		t.Fatalf("run mal formado no debe producir peticiones salientes")
	}
}

func TestReportSoloGET(t *testing.T) {
	ctl := newWarmFake(t, respond(http.StatusOK, "{}"))
	rep := newWarmFake(t, respond(http.StatusOK, "{}"))
	e, id := repEnv(t, ctl, rep)
	if w := e.do(http.MethodPost, "/reports/"+id, repTokU1, ""); w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("%d", w.Code)
	}
}

func TestReportCircuitoTrasFallosRepetidosNoLlamaMas(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	b := newBreaker(WarmCircuitThresh, WarmCircuitOpenFor, func() time.Time { return now })
	for i := 0; i < WarmCircuitThresh; i++ {
		probe, ok := b.admit()
		if !ok {
			t.Fatalf("intento %d no admitido", i)
		}
		b.settle(probe, false)
	}
	if _, ok := b.admit(); ok {
		t.Fatalf("tras %d fallos el circuito debe estar abierto", WarmCircuitThresh)
	}
	now = now.Add(WarmCircuitOpenFor)
	if _, ok := b.admit(); !ok {
		t.Fatalf("pasado el plazo debe admitir una sonda")
	}
}

func TestReportPatronDeRutaAcotado(t *testing.T) {
	r := &http.Request{URL: &url.URL{Path: "/reports/run-1"}}
	if got := RoutePattern(r); got != RouteReport {
		t.Fatalf("patrón = %q, quiero %q", got, RouteReport)
	}
	r = &http.Request{URL: &url.URL{Path: "/reports/a/b"}}
	if got := RoutePattern(r); got != "unmatched" {
		t.Fatalf("patrón = %q, quiero unmatched", got)
	}
}
