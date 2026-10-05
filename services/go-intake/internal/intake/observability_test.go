package intake

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
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

func newObsEnv(t *testing.T) *obsEnv { return newObsEnvWith(t, nil, nil, nil) }

// newObsEnvWith permite inyectar el almacén, el publicador o el resolvedor (nil = los de siempre).
func newObsEnvWith(t *testing.T, st NotificationStore, pub EventPublisher, res ArtifactResolver) *obsEnv {
	t.Helper()
	e := newEnv(t, nil)
	if st == nil {
		st = e.store
	}
	if pub == nil {
		pub = e.pub
	}
	if res == nil {
		res = NewArtifactResolver("")
	}
	buf := &bytes.Buffer{}
	reg := obs.NewRegistry()
	app, err := NewHandler(Deps{
		Secret: []byte(secret), Verifier: githubsig.HMACVerifier{}, Store: st, Publisher: pub, Resolver: res,
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

// ---- Ronda 3: la CLASE «ninguna ruta del handler loguea el payload ni PII» y la semántica de las métricas ----

type putFailStore struct {
	NotificationStore
	failOn int // número de Put (1-based) que falla
	puts   int
}

func (s *putFailStore) Put(rec Record) error {
	s.puts++
	if s.puts == s.failOn {
		return errors.New("disco lleno")
	}
	return s.NotificationStore.Put(rec)
}

type failResolver struct{}

func (failResolver) Resolve(context.Context, Classified) (Artifact, error) {
	return Artifact{}, errors.New("registro caído")
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("conexión rota") }

// piiMarks son las marcas distintivas inyectadas en cada campo sensible del payload.
var piiMarks = []string{"pii-", "falso.test"}

// marked devuelve el fixture con nombre, correo y una marca en cada campo sensible (también en una clave).
func marked(t *testing.T, base []byte, mod func(map[string]any)) []byte {
	t.Helper()
	var p map[string]any
	if err := json.Unmarshal(base, &p); err != nil {
		t.Fatal(err)
	}
	p["PII-CLAVE-XYZ"] = "PII-VALOR-XYZ"
	p["sender"] = map[string]any{"login": "PII-LOGIN-XYZ", "email": "pii-sender@falso.test", "name": "PII-NOMBRE-XYZ"}
	p["pusher"] = map[string]any{"name": "PII-PUSHER-XYZ", "email": "pii-pusher@falso.test"}
	p["head_commit"] = map[string]any{"message": "PII-MENSAJE-XYZ", "author": map[string]any{"name": "PII-AUTOR-XYZ", "email": "pii-autor@falso.test"}}
	if mod != nil {
		mod(p)
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func hasMark(s string) string {
	l := strings.ToLower(s)
	for _, m := range piiMarks {
		if strings.Contains(l, m) {
			return m
		}
	}
	return ""
}

type routeCase struct {
	name string
	req  func(t *testing.T) req
	raw  func(h http.Handler) *httptest.ResponseRecorder // alternativa a req
	st   func(NotificationStore) NotificationStore
	pub  bool // el publicador falla
	res  ArtifactResolver
	// esperado
	status  int
	code    string // código de error del cuerpo ("" = aceptada)
	reason  string // motivo de aqs_intake_webhook_rejected_total ("" = ninguno)
	created string
	pubFail string
	msg     string // mensaje de log esperado
}

func routeCases() []routeCase {
	push := func(t *testing.T) []byte { return marked(t, fixture(t, "push-branch.json"), nil) }
	return []routeCase{
		{name: "aceptada", req: func(t *testing.T) req { return signed("push", "d1", push(t)) }, status: 202, created: "1", pubFail: "0", msg: "webhook aceptado"},
		{name: "unsupported_event: evento ignorado", req: func(t *testing.T) req {
			return signed("issues", "d1", marked(t, fixture(t, "pull-request-opened.json"), nil))
		}, status: 400, code: "unsupported_event", reason: "unsupported_event", created: "0", pubFail: "0", msg: "webhook rechazado"},
		{name: "unsupported_event: acción ignorada", req: func(t *testing.T) req {
			return signed("pull_request", "d1", marked(t, fixture(t, "pull-request-opened.json"), func(p map[string]any) { p["action"] = "closed" }))
		}, status: 400, code: "unsupported_event", reason: "unsupported_event", created: "0", pubFail: "0", msg: "webhook rechazado"},
		{name: "invalid_payload", req: func(t *testing.T) req {
			return signed("push", "d1", marked(t, fixture(t, "push-branch.json"), func(p map[string]any) { delete(p, "after") }))
		}, status: 400, code: "invalid_payload", reason: "bad_request", created: "0", pubFail: "0", msg: "webhook rechazado"},
		{name: "invalid_json", req: func(t *testing.T) req {
			return signed("push", "d1", []byte(`{"PII-CLAVE-XYZ":"PII-VALOR-XYZ","sender":{"email":"pii-x@falso.test","name":"PII-NOMBRE-XYZ"`))
		}, status: 400, code: "invalid_json", reason: "bad_request", created: "0", pubFail: "0", msg: "webhook rechazado"},
		{name: "unresolvable_artifact: PR de fork", req: func(t *testing.T) req {
			return signed("pull_request", "d1", marked(t, fixture(t, "pull-request-fork.json"), nil))
		}, status: 422, code: "unresolvable_artifact", reason: "unresolvable_artifact", created: "0", pubFail: "0", msg: "webhook rechazado"},
		{name: "unsupported_media_type", req: func(t *testing.T) req {
			r := signed("push", "d1", push(t))
			r.ctype = "text/plain; x=PII-CT-XYZ"
			return r
		}, status: 415, code: "unsupported_media_type", reason: "bad_request", created: "0", pubFail: "0", msg: "webhook rechazado"},
		{name: "missing_delivery", req: func(t *testing.T) req { return signed("push", "", push(t)) },
			status: 400, code: "missing_delivery", reason: "bad_request", created: "0", pubFail: "0", msg: "webhook rechazado"},
		{name: "too_large", req: func(t *testing.T) req {
			b := append([]byte(`{"PII-CLAVE-XYZ":"PII-VALOR-XYZ","email":"pii-x@falso.test","pad":"`), bytes.Repeat([]byte("a"), MaxBodyBytes)...)
			return signed("push", "d1", append(b, []byte(`"}`)...))
		}, status: 413, code: "payload_too_large", reason: "too_large", created: "0", pubFail: "0", msg: "webhook rechazado"},
		{name: "invalid_body: error de lectura", raw: func(h http.Handler) *httptest.ResponseRecorder {
			hr := httptest.NewRequest(http.MethodPost, "/webhooks/github", errReader{})
			hr.Header.Set("X-Hub-Signature-256", "sha256=00")
			hr.Header.Set("X-GitHub-Event", "push")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, hr)
			return w
		}, status: 400, code: "invalid_body", reason: "bad_request", created: "0", pubFail: "0", msg: "webhook rechazado"},
		{name: "firma inválida", req: func(t *testing.T) req {
			r := signed("push", "d1", push(t))
			r.sig = "sha256=" + strings.Repeat("0", 64)
			return r
		}, status: 401, code: "invalid_signature", reason: "invalid_signature", created: "0", pubFail: "0", msg: "webhook rechazado"},
		{name: "firma ausente", req: func(t *testing.T) req {
			r := signed("push", "d1", push(t))
			r.noSig = true
			return r
		}, status: 401, code: "invalid_signature", reason: "invalid_signature", created: "0", pubFail: "0", msg: "webhook rechazado"},
		{name: "artifact_unavailable (503 del resolvedor)", req: func(t *testing.T) req { return signed("push", "d1", push(t)) },
			res: failResolver{}, status: 503, code: "artifact_unavailable", created: "0", pubFail: "0", msg: "webhook rechazado"},
		{name: "store_unavailable: falla el primer Put", req: func(t *testing.T) req { return signed("push", "d1", push(t)) },
			st:     func(s NotificationStore) NotificationStore { return &putFailStore{NotificationStore: s, failOn: 1} },
			status: 503, code: "store_unavailable", created: "0", pubFail: "0", msg: "no se pudo persistir"},
		{name: "publish_failed: falla la publicación", req: func(t *testing.T) req { return signed("push", "d1", push(t)) },
			pub: true, status: 503, code: "publish_failed", created: "1", pubFail: "1", msg: "no se pudo publicar notify.created"},
		{name: "store_unavailable: falla el segundo Put", req: func(t *testing.T) req { return signed("push", "d1", push(t)) },
			st:     func(s NotificationStore) NotificationStore { return &putFailStore{NotificationStore: s, failOn: 2} },
			status: 503, code: "store_unavailable", created: "1", pubFail: "0", msg: "no se pudo persistir"},
	}
}

// Ninguna ruta del handler (aceptada, cada rechazo, cada 503) deja el payload ni PII en el log
// (nivel debug), en /metrics ni en el cuerpo de la respuesta; y cada una cuenta lo que debe con
// valores exactos: code real, motivo de rechazo, created y publish_failures.
func TestObsEveryRouteLeaksNothingAndCountsExactly(t *testing.T) {
	for _, c := range routeCases() {
		t.Run(c.name, func(t *testing.T) {
			var st NotificationStore
			base := newEnv(t, nil)
			if c.st != nil {
				st = c.st(base.store)
			}
			var pub EventPublisher
			if c.pub {
				pub = &memPublisher{fail: true}
			}
			o := newObsEnvWith(t, st, pub, c.res)
			o.h = withPIIHeaders(o.h)
			var w *httptest.ResponseRecorder
			if c.raw != nil {
				w = c.raw(o.h)
			} else {
				w = do(o.h, c.req(t))
			}
			if w.Code != c.status {
				t.Fatalf("status=%d, esperado %d (%s)", w.Code, c.status, w.Body)
			}
			if c.code != "" && errCode(t, w) != c.code {
				t.Fatalf("code=%q, esperado %q", errCode(t, w), c.code)
			}
			rec := httptest.NewRecorder()
			o.h.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
			for what, text := range map[string]string{"respuesta": w.Body.String(), "log": o.log.String(), "/metrics": rec.Body.String()} {
				if m := hasMark(text); m != "" {
					t.Errorf("%s contiene la marca %q: %s", what, m, text)
				}
			}
			for _, ln := range strings.Split(strings.TrimSpace(o.log.String()), "\n") {
				var l map[string]any
				if err := json.Unmarshal([]byte(ln), &l); err != nil {
					t.Fatalf("no es JSON: %q", ln)
				}
				if l["request_id"] == "" || l["request_id"] == nil || l["trace_id"] == "" || l["trace_id"] == nil {
					t.Errorf("línea de log dentro de una petición sin ids: %s", ln)
				}
			}
			if !strings.Contains(o.log.String(), `"message":"`+c.msg+`"`) {
				t.Errorf("falta la línea de log %q: %s", c.msg, o.log)
			}
			// niveles y campos de los logs de dominio
			wantLvl := map[string]string{"webhook aceptado": "info", "webhook rechazado": "warn",
				"no se pudo publicar notify.created": "error", "no se pudo persistir": "error"}
			for _, ln := range strings.Split(strings.TrimSpace(o.log.String()), "\n") {
				var l map[string]any
				_ = json.Unmarshal([]byte(ln), &l)
				msg, _ := l["message"].(string)
				if want, ok := wantLvl[msg]; ok && l["level"] != want {
					t.Errorf("%q debe salir en %s: %s", msg, want, ln)
				}
				switch msg {
				case "webhook rechazado":
					if l["status"] != float64(c.status) || (c.code != "" && l["code"] != c.code) {
						t.Errorf("el log de rechazo debe llevar el status y el code reales (%d, %s): %s", c.status, c.code, ln)
					}
				case "webhook aceptado":
					if id, _ := l["notification_id"].(string); !strings.HasPrefix(id, "n-") || l["status"] != float64(202) || l["delivery_id"] != "d1" {
						t.Errorf("el log de aceptación debe llevar notification_id, delivery_id y status: %s", ln)
					}
				case "no se pudo publicar notify.created":
					if id, _ := l["notification_id"].(string); !strings.HasPrefix(id, "n-") {
						t.Errorf("el log de publicación fallida lleva notification_id: %s", ln)
					}
				}
			}
			// valores exactos de las métricas
			codeSeries := `aqs_http_requests_total{code="` + strconv.Itoa(c.status) + `",method="POST",route="/webhooks/github",service="go-intake"}`
			if got := o.metric(t, codeSeries); got != "1" {
				t.Errorf("%s = %s, esperado 1", codeSeries, got)
			}
			for _, r := range obs.RejectReasons {
				want := "0"
				if r == c.reason {
					want = "1"
				}
				if got := o.metric(t, rejected(r)); got != want {
					t.Errorf("%s = %s, esperado %s", rejected(r), got, want)
				}
			}
			if got := o.metric(t, "aqs_intake_notifications_created_total"); got != c.created {
				t.Errorf("created = %s, esperado %s", got, c.created)
			}
			if got := o.metric(t, "aqs_intake_publish_failures_total"); got != c.pubFail {
				t.Errorf("publish_failures = %s, esperado %s", got, c.pubFail)
			}
		})
	}
}

// Cada rechazo cuenta una sola vez y solo en su motivo: varias entregas seguidas acumulan exacto.
func TestObsRejectedCountersAccumulate(t *testing.T) {
	o := newObsEnv(t)
	pr := fixture(t, "pull-request-opened.json")
	bad := signed("pull_request", "d1", pr)
	bad.sig = "sha256=" + strings.Repeat("0", 64)
	for i := 0; i < 3; i++ {
		do(o.h, bad)
	}
	do(o.h, signed("issues", "d2", pr))
	if o.metric(t, rejected("invalid_signature")) != "3" || o.metric(t, rejected("unsupported_event")) != "1" {
		t.Errorf("acumulado: firma=%s evento=%s", o.metric(t, rejected("invalid_signature")), o.metric(t, rejected("unsupported_event")))
	}
	// una entrega aceptada y su duplicada: created=1
	do(o.h, signed("pull_request", "d3", pr))
	do(o.h, signed("pull_request", "d3", pr))
	if o.metric(t, "aqs_intake_notifications_created_total") != "1" {
		t.Error("la entrega duplicada no crea otra notificación")
	}
}

// withPIIHeaders añade, a cada petición, marcas de PII en cabeceras (y en Host/RemoteAddr): el log de
// acceso y /metrics no pueden reflejar ninguna, ni siquiera los X-Request-Id/traceparent crudos inválidos.
func withPIIHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range map[string]string{
			"Cookie": "sesion=PII-COOKIE-XYZ", "X-Forwarded-For": "pii-ip.falso.test", "User-Agent": "PII-AGENTE-XYZ/1.0",
			"Origin": "https://pii-origen.falso.test", "Referer": "https://pii-ref.falso.test/x", "Accept-Language": "pii-idioma",
			"X-Request-Id": "PII-REQID XYZ", "traceparent": "PII-TRACE-XYZ", "X-Real-Ip": "pii-real.falso.test",
		} {
			r.Header.Set(k, v)
		}
		if r.Header.Get("Content-Type") == "" {
			r.Header.Set("Content-Type", "application/PII-tipo-xyz")
		}
		r.Host, r.RemoteAddr = "pii-host.falso.test", "pii-addr.falso.test:1234"
		h.ServeHTTP(w, r)
	})
}
