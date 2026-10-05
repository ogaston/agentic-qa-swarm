package intake

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/githubsig"
	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/obs"
)

// obsEnv arma la cadena real de go-intake: obs.Wrap + handler con log y métricas.
type obsEnv struct {
	*env
	h   http.Handler
	log *bytes.Buffer
}

func newObsEnv(t *testing.T) *obsEnv {
	t.Helper()
	e := newEnv(t, nil)
	buf := &bytes.Buffer{}
	reg := obs.NewRegistry()
	app, err := NewHandler(Deps{
		Secret: []byte(secret), Verifier: githubsig.HMACVerifier{}, Store: e.store, Publisher: e.pub, Resolver: NewArtifactResolver(""),
		Now: func() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) },
		Log: obs.NewLogger(buf, "go-intake", slog.LevelDebug), Metrics: obs.NewIntake(reg),
	})
	if err != nil {
		t.Fatal(err)
	}
	h := obs.Wrap(obs.Config{Service: "go-intake", Log: obs.NewLogger(buf, "go-intake", slog.LevelDebug), Registry: reg,
		Metrics: obs.NewHTTPMetrics(reg, "go-intake"), Route: RoutePattern}, app)
	return &obsEnv{env: e, h: h, log: buf}
}

func (o *obsEnv) metric(t *testing.T, series string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	o.h.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	for _, ln := range strings.Split(rec.Body.String(), "\n") {
		if v, ok := strings.CutPrefix(ln, series+" "); ok {
			return v
		}
	}
	t.Fatalf("serie %q ausente", series)
	return ""
}

func rejected(r string) string { return `aqs_intake_webhook_rejected_total{reason="` + r + `"}` }

func TestObsRejectionReasons(t *testing.T) {
	pr := fixture(t, "pull-request-opened.json")
	big := append(append([]byte(`{"x":"`), bytes.Repeat([]byte("a"), MaxBodyBytes)...), []byte(`"}`)...)
	badSig := signed("pull_request", "d1", pr)
	badSig.sig = "sha256=" + strings.Repeat("0", 64)
	noSig := signed("pull_request", "d1", pr)
	noSig.noSig = true
	cases := []struct {
		name   string
		req    req
		reason string
	}{
		{"firma inválida", badSig, "invalid_signature"},
		{"firma ausente", noSig, "invalid_signature"},
		{"evento no soportado", signed("ping", "d1", fixture(t, "ping.json")), "unsupported_event"},
		{"artefacto irresoluble", signed("pull_request", "d1", fixture(t, "pull-request-fork.json")), "unresolvable_artifact"},
		{"JSON roto", signed("pull_request", "d1", []byte(`{"action":`)), "bad_request"},
		{"sin delivery", signed("pull_request", "", pr), "bad_request"},
		{"cuerpo grande", signed("push", "d1", big), "too_large"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := newObsEnv(t)
			do(o.h, c.req)
			for _, r := range obs.RejectReasons {
				want := "0"
				if r == c.reason {
					want = "1"
				}
				if got := o.metric(t, rejected(r)); got != want {
					t.Errorf("%s = %s, esperado %s", rejected(r), got, want)
				}
			}
			if got := o.metric(t, "aqs_intake_notifications_created_total"); got != "0" {
				t.Errorf("un rechazo no crea notificación: %s", got)
			}
		})
	}
}

func TestObsCreatedAndPublishFailures(t *testing.T) {
	o := newObsEnv(t)
	pr := fixture(t, "pull-request-opened.json")
	o.pub.fail = true
	if w := do(o.h, signed("pull_request", "d1", pr)); w.Code != 503 {
		t.Fatalf("publicación caída: %d", w.Code)
	}
	if o.metric(t, "aqs_intake_publish_failures_total") != "1" || o.metric(t, "aqs_intake_notifications_created_total") != "1" {
		t.Error("fallo de publicación: publish_failures=1 y created=1 (el registro quedó persistido)")
	}
	for _, r := range obs.RejectReasons {
		if o.metric(t, rejected(r)) != "0" {
			t.Errorf("un 503 interno no es un rechazo del remitente: %s", r)
		}
	}
	o.pub.fail = false
	if w := do(o.h, signed("pull_request", "d1", pr)); w.Code != 202 {
		t.Fatalf("reintento: %d", w.Code)
	}
	do(o.h, signed("pull_request", "d1", pr)) // entrega duplicada
	if o.metric(t, "aqs_intake_notifications_created_total") != "1" {
		t.Error("el reintento y la entrega duplicada no crean otra notificación")
	}
	if o.metric(t, "aqs_intake_publish_failures_total") != "1" {
		t.Error("el reintento exitoso no cuenta como fallo")
	}
}

func TestObsTraceIDReachesEventAndLog(t *testing.T) {
	const tid = "4bf92f3577b34da6a3ce929d0e0e4736"
	o := newObsEnv(t)
	r := signed("pull_request", "d1", fixture(t, "pull-request-opened.json"))
	r.trace = "00-" + tid + "-00f067aa0ba902b7-01"
	if w := do(o.h, r); w.Code != 202 {
		t.Fatalf("status %d", w.Code)
	}
	if len(o.pub.events) != 1 || o.pub.events[0].TraceID != tid {
		t.Fatalf("trace_id del evento: %+v", o.pub.events)
	}
	// Sin traceparent válido: el trace_id del evento es el que se devuelve en la respuesta y se loguea.
	o2 := newObsEnv(t)
	r2 := signed("pull_request", "d2", fixture(t, "pull-request-opened.json"))
	r2.trace = "basura"
	w := do(o2.h, r2)
	resp := strings.Split(w.Header().Get("traceparent"), "-")
	if len(resp) != 4 || len(o2.pub.events) != 1 || o2.pub.events[0].TraceID != resp[1] {
		t.Fatalf("evento %v vs traceparent de respuesta %q", o2.pub.events, w.Header().Get("traceparent"))
	}
	if !strings.Contains(o2.log.String(), `"trace_id":"`+resp[1]+`"`) {
		t.Error("el log no lleva el mismo trace_id")
	}
}

func TestObsLogContractAndNoSecrets(t *testing.T) {
	o := newObsEnv(t)
	body := []byte(`{"repository":{"full_name":"acme/shop"},"marca":"CUERPO-SECRETO-XYZ"}`)
	r := signed("push", "d1", body)
	r.sig = "sha256=deadbeefcafe"
	hr := httptest.NewRequest("POST", "/webhooks/github", bytes.NewReader(body))
	hr.Header.Set("X-GitHub-Event", "push")
	hr.Header.Set("X-GitHub-Delivery", "d1")
	hr.Header.Set("X-Hub-Signature-256", "sha256=deadbeefcafe")
	hr.Header.Set("Authorization", "Bearer tok-super-secreto")
	hr.Header.Set("X-Request-Id", "req-12345678")
	o.h.ServeHTTP(httptest.NewRecorder(), hr)
	good := signed("pull_request", "d2", fixture(t, "pull-request-opened.json"))
	if do(o.h, good).Code != 202 {
		t.Fatal("entrega válida")
	}
	for _, s := range []string{secret, "deadbeefcafe", "tok-super-secreto", "CUERPO-SECRETO-XYZ", good.sig} {
		if strings.Contains(o.log.String(), s) {
			t.Errorf("el log contiene %q", s)
		}
	}
	lines := strings.Split(strings.TrimSpace(o.log.String()), "\n")
	if len(lines) < 4 {
		t.Fatalf("pocas líneas: %d", len(lines))
	}
	sawRejected, sawAccepted := false, false
	for _, ln := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(ln), &m); err != nil {
			t.Fatalf("no es JSON: %s", ln)
		}
		for _, k := range []string{"timestamp", "request_id", "trace_id", "level", "message"} {
			if _, ok := m[k]; !ok {
				t.Errorf("sin %q: %s", k, ln)
			}
		}
		switch m["message"] {
		case "webhook rechazado":
			sawRejected = true
			if m["code"] != "invalid_signature" || m["request_id"] != "req-12345678" || m["delivery_id"] != "d1" {
				t.Errorf("rechazo mal registrado: %s", ln)
			}
		case "webhook aceptado":
			sawAccepted = true
			if m["delivery_id"] != "d2" || m["notification_id"] == "" || m["repo"] == nil || m["sha"] == nil {
				t.Errorf("aceptación mal registrada: %s", ln)
			}
		}
	}
	if !sawRejected || !sawAccepted {
		t.Errorf("faltan eventos de log: rechazado=%v aceptado=%v", sawRejected, sawAccepted)
	}
}

func TestRoutePatternBoundsCardinality(t *testing.T) {
	for p, want := range map[string]string{"/webhooks/github": "/webhooks/github", "/a": "unmatched", "/webhooks/github/x": "unmatched", "/healthz": "unmatched"} {
		if got := RoutePattern(httptest.NewRequest("GET", p, nil)); got != want {
			t.Errorf("%s -> %s, esperado %s", p, got, want)
		}
	}
}

func TestClipAndRejectReason(t *testing.T) {
	if len(clip(strings.Repeat("x", 500))) != 64 || clip("abc") != "abc" {
		t.Error("clip")
	}
	for code, want := range map[string]string{"payload_too_large": "too_large", "invalid_json": "bad_request", "store_unavailable": "", "publish_failed": "", "artifact_unavailable": ""} {
		if got := rejectReason(code); got != want {
			t.Errorf("%s -> %q, esperado %q", code, got, want)
		}
	}
}

// F-02 de la ronda 1: la ruta ACEPTADA tampoco filtra el contenido del payload (los payloads de
// GitHub traen nombres y correos): ni el log (a nivel debug) ni /metrics.
func TestObsAcceptedPayloadLeaksNothing(t *testing.T) {
	var p map[string]any
	if err := json.Unmarshal(fixture(t, "push-branch.json"), &p); err != nil {
		t.Fatal(err)
	}
	p["sender"] = map[string]any{"login": "PII-LOGIN-XYZ", "email": "pii-sender@falso.test"}
	p["head_commit"] = map[string]any{"message": "PII-MENSAJE-XYZ", "author": map[string]any{"name": "PII-NOMBRE-XYZ", "email": "pii-autor@falso.test"}}
	body, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	o := newObsEnv(t)
	w := do(o.h, signed("push", "d-pii", body))
	if w.Code != 202 {
		t.Fatalf("la entrega válida debía aceptarse: %d %s", w.Code, w.Body)
	}
	rec := httptest.NewRecorder()
	o.h.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(o.log.String(), `"message":"webhook aceptado"`) {
		t.Fatalf("falta la línea de aceptación: %s", o.log)
	}
	for _, s := range []string{"PII-LOGIN-XYZ", "pii-sender@falso.test", "PII-MENSAJE-XYZ", "PII-NOMBRE-XYZ", "pii-autor@falso.test", "falso.test"} {
		if strings.Contains(o.log.String(), s) {
			t.Errorf("el log contiene %q: %s", s, o.log)
		}
		if strings.Contains(rec.Body.String(), s) {
			t.Errorf("/metrics contiene %q", s)
		}
	}
}
