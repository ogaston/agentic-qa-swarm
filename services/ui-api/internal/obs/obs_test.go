package obs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func newWrapped(t *testing.T, ready []Check, app http.Handler) (http.Handler, *bytes.Buffer, *HTTPMetrics, func() string) {
	t.Helper()
	buf := &bytes.Buffer{}
	reg := NewRegistry()
	m := NewHTTPMetrics(reg, "svc-prueba")
	if app == nil {
		app = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	}
	h := Wrap(Config{Service: "svc-prueba", Log: NewLogger(buf, "svc-prueba", slog.LevelDebug), Registry: reg, Metrics: m, Ready: ready,
		Route: func(r *http.Request) string {
			if r.URL.Path == "/known" {
				return "/known"
			}
			return "unmatched"
		}}, app)
	scrape := func() string {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
		return rec.Body.String()
	}
	return h, buf, m, scrape
}

func TestLoggerContractFieldsAndLowercaseLevel(t *testing.T) {
	buf := &bytes.Buffer{}
	l := NewLogger(buf, "svc", slog.LevelDebug)
	ctx := context.WithValue(context.WithValue(context.Background(), keyRequestID, "req-12345678"), keyTraceID, strings.Repeat("a", 32))
	l.DebugContext(ctx, "d")
	l.InfoContext(ctx, "i")
	l.WarnContext(ctx, "w")
	l.ErrorContext(ctx, "e")
	l.Info("sin contexto")
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("líneas: %d", len(lines))
	}
	want := []string{"debug", "info", "warn", "error", "info"}
	for i, ln := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(ln), &m); err != nil {
			t.Fatalf("no es JSON: %v", err)
		}
		for _, k := range []string{"timestamp", "request_id", "trace_id", "level", "message", "service"} {
			if _, ok := m[k]; !ok {
				t.Errorf("línea %d sin %q: %s", i, k, ln)
			}
		}
		for _, k := range []string{"time", "msg"} {
			if _, ok := m[k]; ok {
				t.Errorf("línea %d conserva la clave %q", i, k)
			}
		}
		if m["level"] != want[i] {
			t.Errorf("level %v, esperado %s", m["level"], want[i])
		}
		if _, err := time.Parse(time.RFC3339Nano, m["timestamp"].(string)); err != nil || !strings.HasSuffix(m["timestamp"].(string), "Z") {
			t.Errorf("timestamp no UTC RFC 3339: %v", m["timestamp"])
		}
	}
	if !strings.Contains(lines[1], `"request_id":"req-12345678"`) || !strings.Contains(lines[4], `"request_id":""`) {
		t.Errorf("request_id mal propagado: %s / %s", lines[1], lines[4])
	}
}

func TestLoggerRedactsSensitiveKeys(t *testing.T) {
	buf := &bytes.Buffer{}
	l := NewLogger(buf, "svc", slog.LevelInfo)
	l.Info("x", "password", "hunter2-secreto", "Authorization", "Bearer abc", "service_token", "tkn-123", "otp", "123456", "mfa_secret", "JBSWY3DP", "password_hash", "$argon2id$x")
	out := buf.String()
	for _, s := range []string{"hunter2-secreto", "Bearer abc", "tkn-123", "123456", "JBSWY3DP", "$argon2id$x"} {
		if strings.Contains(out, s) {
			t.Errorf("el log contiene %q: %s", s, out)
		}
	}
	if strings.Count(out, "[redacted]") != 6 {
		t.Errorf("se esperaban 6 redacciones: %s", out)
	}
}

func TestRequestIDAndTraceparent(t *testing.T) {
	h, _, _, _ := newWrapped(t, nil, nil)
	do := func(rid, tp string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/known", nil)
		if rid != "" {
			req.Header.Set("X-Request-Id", rid)
		}
		if tp != "" {
			req.Header.Set("traceparent", tp)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	const tp = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	r := do("req-12345678", tp)
	if r.Header().Get("X-Request-Id") != "req-12345678" {
		t.Errorf("X-Request-Id válido debía conservarse: %q", r.Header().Get("X-Request-Id"))
	}
	if got := r.Header().Get("traceparent"); !strings.HasPrefix(got, "00-4bf92f3577b34da6a3ce929d0e0e4736-") || got == tp {
		t.Errorf("traceparent de respuesta: %q", got)
	}
	for _, bad := range []string{"corto", "con espacios 1234", strings.Repeat("a", 65), "x\r\ny-12345678", "ñandú-12345678"} {
		if got := do(bad, "").Header().Get("X-Request-Id"); got == bad || !ValidRequestID(got) {
			t.Errorf("X-Request-Id %q debía reemplazarse, quedó %q", bad, got)
		}
	}
	for _, badTP := range []string{"zz", "01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		"00-00000000000000000000000000000000-00f067aa0ba902b7-01", "00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01"} {
		got := do("", badTP).Header().Get("traceparent")
		if strings.Contains(got, "4bf92f35") || strings.Contains(got, "-00000000") || !traceparentRe.MatchString(got) {
			t.Errorf("traceparent inválido %q debía regenerarse: %q", badTP, got)
		}
	}
}

func TestOpsEndpointsNeedNoTokenAndSkipApp(t *testing.T) {
	called := false
	h, _, _, _ := newWrapped(t, nil, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	for p, code := range map[string]int{"/healthz": 200, "/readyz": 200, "/metrics": 200} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		if rec.Code != code {
			t.Errorf("%s: %d", p, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/healthz", nil))
	if rec.Code != 405 {
		t.Errorf("POST /healthz: %d", rec.Code)
	}
	if called {
		t.Fatal("los endpoints operativos no deben llegar a la aplicación")
	}
}

func TestReadyzFailsForEachFailingCheck(t *testing.T) {
	names := []string{"uno", "dos", "tres"}
	for failing := range names {
		var cs []Check
		for i, n := range names {
			var err error
			if i == failing {
				err = errors.New("detalle interno /ruta/secreta")
			}
			cs = append(cs, Check{Name: n, Fn: func(context.Context) error { return err }})
		}
		h, _, _, _ := newWrapped(t, cs, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
		var body struct {
			Status string
			Checks map[string]string
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != 503 || body.Status != "unavailable" || body.Checks[names[failing]] != "fail" {
			t.Errorf("fallo de %s: %d %s", names[failing], rec.Code, rec.Body)
		}
		if strings.Contains(rec.Body.String(), "secreta") {
			t.Errorf("el detalle del error no debe salir en la respuesta: %s", rec.Body)
		}
	}
	h, _, _, _ := newWrapped(t, []Check{{Name: "ok", Fn: func(context.Context) error { return nil }}}, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"ready"`) {
		t.Errorf("todo en orden: %d %s", rec.Code, rec.Body)
	}
}

func TestReadyzCheckTimeout(t *testing.T) {
	old := CheckTimeout
	CheckTimeout = 50 * time.Millisecond
	defer func() { CheckTimeout = old }()
	release := make(chan struct{})
	defer close(release)
	h, _, _, _ := newWrapped(t, []Check{{Name: "lento", Fn: func(context.Context) error { <-release; return nil }}}, nil)
	start := time.Now()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != 503 || time.Since(start) > 2*time.Second {
		t.Errorf("un chequeo colgado debía dar 503 pronto: %d en %v", rec.Code, time.Since(start))
	}
}

func TestRouteLabelCardinalityIsBounded(t *testing.T) {
	h, buf, _, scrape := newWrapped(t, nil, nil)
	for i := 0; i < 30; i++ {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/ruta-"+strings.Repeat("x", i), nil))
		req := httptest.NewRequest("FOO", "/known", nil)
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
	out := scrape()
	if strings.Contains(out, "ruta-") {
		t.Error("la ruta cruda no debe aparecer en las métricas")
	}
	if !strings.Contains(out, `route="unmatched"`) || !strings.Contains(out, `route="/known"`) || !strings.Contains(out, `method="OTHER"`) {
		t.Errorf("faltan etiquetas acotadas: %s", out)
	}
	if strings.Contains(buf.String(), "ruta-") {
		t.Error("la ruta cruda no debe aparecer en el log de acceso")
	}
}

func TestAccessLogHasNoRequestBody(t *testing.T) {
	h, buf, _, _ := newWrapped(t, nil, nil)
	req := httptest.NewRequest("POST", "/known?token=abc123", strings.NewReader(`{"password":"CLAVE-SECRETA-XYZ"}`))
	req.Header.Set("Authorization", "Bearer TOKEN-SECRETO-XYZ")
	h.ServeHTTP(httptest.NewRecorder(), req)
	for _, s := range []string{"CLAVE-SECRETA-XYZ", "TOKEN-SECRETO-XYZ", "abc123"} {
		if strings.Contains(buf.String(), s) {
			t.Errorf("el log de acceso contiene %q: %s", s, buf)
		}
	}
	if !strings.Contains(buf.String(), `"route":"/known"`) || !strings.Contains(buf.String(), `"status":204`) {
		t.Errorf("log de acceso incompleto: %s", buf)
	}
}

// A nivel info (el de producción) las sondas dejan su línea de acceso con request_id y trace_id.
func TestProbeAccessLogAtInfoLevel(t *testing.T) {
	buf := &bytes.Buffer{}
	reg := NewRegistry()
	h := Wrap(Config{Service: "svc", Log: NewLogger(buf, "svc", slog.LevelInfo), Registry: reg, Metrics: NewHTTPMetrics(reg, "svc")}, nil)
	req := httptest.NewRequest("GET", "/healthz", nil)
	req.Header.Set("X-Request-Id", "req-12345678")
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	h.ServeHTTP(httptest.NewRecorder(), req)
	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &m); err != nil {
		t.Fatalf("sin línea de acceso de /healthz a nivel info: %q", buf)
	}
	if m["request_id"] != "req-12345678" || m["trace_id"] != "4bf92f3577b34da6a3ce929d0e0e4736" || m["route"] != "/healthz" || m["level"] != "info" {
		t.Errorf("línea de acceso: %s", buf)
	}
}

func TestRequestIDLengthBounds(t *testing.T) {
	h, _, _, _ := newWrapped(t, nil, nil)
	for n, keep := range map[int]bool{7: false, 8: true, 64: true, 65: false} {
		id := strings.Repeat("a", n)
		req := httptest.NewRequest("GET", "/known", nil)
		req.Header.Set("X-Request-Id", id)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if got := rec.Header().Get("X-Request-Id"); (got == id) != keep || !ValidRequestID(got) {
			t.Errorf("largo %d: conservar=%v, quedó %q", n, keep, got)
		}
	}
}

func TestTraceparentUppercaseHexIsRegenerated(t *testing.T) {
	h, _, _, _ := newWrapped(t, nil, nil)
	req := httptest.NewRequest("GET", "/known", nil)
	req.Header.Set("traceparent", "00-4BF92F3577B34DA6A3CE929D0E0E4736-00F067AA0BA902B7-01")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := strings.ToLower(rec.Header().Get("traceparent")); strings.Contains(got, "4bf92f3577b34da6a3ce929d0e0e4736") {
		t.Errorf("un traceparent con hex en mayúsculas (inválido para W3C) debía regenerarse: %q", got)
	}
}

func TestInFlightGaugeRisesAndFalls(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	h, _, _, scrape := newWrapped(t, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
	}))
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/known", nil))
		close(done)
	}()
	<-started
	// la petición lenta + el propio scrape
	if out := scrape(); !strings.Contains(out, `aqs_http_in_flight{service="svc-prueba"} 2`+"\n") {
		t.Errorf("durante la petición lenta in_flight debía ser 2: %s", out)
	}
	close(release)
	<-done
	if out := scrape(); !strings.Contains(out, `aqs_http_in_flight{service="svc-prueba"} 1`+"\n") { // solo el scrape
		t.Errorf("tras terminar in_flight debía bajar a 1 (el scrape): %s", out)
	}
}

func TestDurationHistogramBucketsAndProbeRoutes(t *testing.T) {
	h, _, _, scrape := newWrapped(t, nil, nil)
	for _, p := range []string{"/known", "/healthz", "/readyz"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", p, nil))
	}
	out := scrape()
	for _, le := range []string{"0.005", "0.025", "0.1", "0.5", "1", "5", "10", "+Inf"} {
		if !strings.Contains(out, `aqs_http_request_duration_seconds_bucket{method="GET",route="/known",service="svc-prueba",le="`+le+`"}`) {
			t.Errorf("falta el bucket le=%s (el p95 del dashboard depende de ellos)", le)
		}
	}
	for _, r := range []string{"/healthz", "/readyz", "/metrics"} {
		if r == "/metrics" {
			continue // el propio scrape se contabiliza después de responder
		}
		if !strings.Contains(out, `aqs_http_requests_total{code="`+map[string]string{"/healthz": "200", "/readyz": "200"}[r]+`",method="GET",route="`+r+`",service="svc-prueba"} 1`) {
			t.Errorf("falta la serie de la sonda %s con su route", r)
		}
	}
	if out2 := scrape(); !strings.Contains(out2, `route="/metrics"`) {
		t.Error("la sonda /metrics debe llevar route=\"/metrics\"")
	}
}

func TestProbesMethodNotAllowedSetsAllow(t *testing.T) {
	h, _, _, _ := newWrapped(t, nil, nil)
	for _, p := range []string{"/healthz", "/readyz", "/metrics"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("POST", p, nil))
		if rec.Code != 405 || rec.Header().Get("Allow") != "GET, HEAD" {
			t.Errorf("POST %s: %d Allow=%q", p, rec.Code, rec.Header().Get("Allow"))
		}
	}
}

func TestServerErrorAccessLogIsWarn(t *testing.T) {
	buf := &bytes.Buffer{}
	reg := NewRegistry()
	app := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/boom" {
			w.WriteHeader(503)
		}
	})
	h := Wrap(Config{Service: "svc", Log: NewLogger(buf, "svc", slog.LevelInfo), Registry: reg, Metrics: NewHTTPMetrics(reg, "svc")}, app)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/ok", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/boom", nil))
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"level":"info"`) || !strings.Contains(lines[1], `"level":"warn"`) || !strings.Contains(lines[1], `"status":503`) {
		t.Errorf("200 en info y 5xx en warn: %v", lines)
	}
}

// ---- Ronda 3: semántica de etiquetas y unidades, y bordes del contrato ----

func TestParseLevel(t *testing.T) {
	for in, want := range map[string]slog.Level{"debug": slog.LevelDebug, "info": slog.LevelInfo, "warn": slog.LevelWarn, "error": slog.LevelError,
		"": slog.LevelInfo, "DEBUG": slog.LevelDebug, " Warn ": slog.LevelWarn, "ERROR": slog.LevelError, "inválido": slog.LevelInfo, "trace": slog.LevelInfo} {
		if got := ParseLevel(in); got != want {
			t.Errorf("ParseLevel(%q) = %v, esperado %v", in, got, want)
		}
	}
	buf := &bytes.Buffer{}
	l := NewLogger(buf, "svc", ParseLevel("warn"))
	l.Info("no")
	l.Warn("sí")
	if strings.Contains(buf.String(), `"message":"no"`) || !strings.Contains(buf.String(), `"message":"sí"`) {
		t.Errorf("LOG_LEVEL=warn debe filtrar info: %s", buf)
	}
}

func TestLoggerTimestampIsUTCEvenWithLocalZone(t *testing.T) {
	old := time.Local
	time.Local = time.FixedZone("UTC+3", 3*3600)
	defer func() { time.Local = old }()
	buf := &bytes.Buffer{}
	NewLogger(buf, "svc", slog.LevelInfo).Info("x")
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if ts, _ := m["timestamp"].(string); !strings.HasSuffix(ts, "Z") {
		t.Errorf("timestamp no UTC: %v", m["timestamp"])
	}
}

func TestLoggerRedactsEverySensitiveKey(t *testing.T) {
	for _, k := range []string{"password", "passwd", "secret", "token", "otp", "authorization", "hash", "cookie",
		"Set-Cookie", "X-Authorization", "api_token", "client_secret", "PASSWORD"} {
		buf := &bytes.Buffer{}
		NewLogger(buf, "svc", slog.LevelInfo).Info("x", k, "VALOR-SECRETO-XYZ")
		if strings.Contains(buf.String(), "VALOR-SECRETO-XYZ") || !strings.Contains(buf.String(), "[redacted]") {
			t.Errorf("la clave %q debe redactarse: %s", k, buf)
		}
	}
	buf := &bytes.Buffer{}
	NewLogger(buf, "svc", slog.LevelInfo).Info("x", "repo", "acme/shop")
	if !strings.Contains(buf.String(), "acme/shop") {
		t.Error("una clave no sensible no se redacta")
	}
}

func TestRegistryHasGoAndProcessCollectors(t *testing.T) {
	_, _, _, scrape := newWrapped(t, nil, nil)
	out := scrape()
	if !strings.Contains(out, "go_goroutines") || !strings.Contains(out, "process_cpu_seconds_total") {
		t.Error("el registro debe incluir los colectores de Go y de proceso")
	}
}

func TestCheckTimeoutDefaultIsTwoSeconds(t *testing.T) {
	if CheckTimeout != 2*time.Second {
		t.Errorf("la tarea fija 2 s por chequeo: %v", CheckTimeout)
	}
	old := CheckTimeout
	CheckTimeout = 50 * time.Millisecond
	defer func() { CheckTimeout = old }()
	release := make(chan struct{})
	defer close(release)
	h, _, _, _ := newWrapped(t, []Check{{Name: "lento", Fn: func(context.Context) error { <-release; return nil }}}, nil)
	start := time.Now()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
	if el := time.Since(start); rec.Code != 503 || el < 40*time.Millisecond || el > 400*time.Millisecond {
		t.Errorf("el timeout debía ser ≈50 ms: %d en %v", rec.Code, el)
	}
}

func TestTraceparentStrictShape(t *testing.T) {
	h, _, _, _ := newWrapped(t, nil, nil)
	shape := regexp.MustCompile(`^00-[0-9a-f]{32}-[0-9a-f]{16}-01$`)
	const lower = "4bf92f3577b34da6a3ce929d0e0e4736"
	for name, tc := range map[string]struct {
		in    string
		valid bool
	}{
		"válido flags 01":        {"00-" + lower + "-00f067aa0ba902b7-01", true},
		"válido flags 00":        {"00-" + lower + "-00f067aa0ba902b7-00", true},
		"trace-id en mayúsculas": {"00-" + strings.ToUpper(lower) + "-00f067aa0ba902b7-01", false},
		"span en mayúsculas":     {"00-" + lower + "-00F067AA0BA902B7-01", false},
		"flags en mayúsculas":    {"00-" + lower + "-00f067aa0ba902b7-0A", false},
		"flags de 4 hex":         {"00-" + lower + "-00f067aa0ba902b7-0100", false},
		"flags de 1 hex":         {"00-" + lower + "-00f067aa0ba902b7-1", false},
		"flags de 3 hex":         {"00-" + lower + "-00f067aa0ba902b7-011", false},
		"sufijo tras los flags":  {"00-" + lower + "-00f067aa0ba902b7-01-x", false},
		"span corto":             {"00-" + lower + "-00f067aa0ba902b-01", false},
		"trace-id de 31 hex":     {"00-" + lower[:31] + "-00f067aa0ba902b7-01", false},
		"sin guiones":            {"00" + lower + "00f067aa0ba902b701", false},
		"prefijo basura":         {"x00-" + lower + "-00f067aa0ba902b7-01", false},
		"versión ff":             {"ff-" + lower + "-00f067aa0ba902b7-01", false},
		"flags no hex":           {"00-" + lower + "-00f067aa0ba902b7-zz", false},
	} {
		req := httptest.NewRequest("GET", "/known", nil)
		req.Header.Set("traceparent", tc.in)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		got := rec.Header().Get("traceparent")
		if !shape.MatchString(got) {
			t.Errorf("%s: el traceparent de respuesta debe ser 00-<32>-<16>-01: %q", name, got)
		}
		if kept := strings.Contains(got, "-"+lower+"-"); kept != tc.valid {
			t.Errorf("%s: trace-id conservado=%v, esperado %v (%q)", name, kept, tc.valid, got)
		}
		if strings.HasSuffix(strings.Split(tc.in, "-")[len(strings.Split(tc.in, "-"))-1], "00") && tc.valid && strings.Contains(got, "00f067aa0ba902b7") {
			t.Errorf("%s: el span de la respuesta debe ser nuevo", name)
		}
	}
}

// code real de cada respuesta, valor exacto (un 5xx cuenta en code=~"5..").
func TestHTTPCodeLabelIsTheRealStatus(t *testing.T) {
	codes := []int{200, 202, 204, 400, 401, 404, 413, 415, 422, 429, 500, 503}
	h, _, _, scrape := newWrapped(t, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/c"))
		w.WriteHeader(n)
	}))
	for _, c := range codes {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/c"+strconv.Itoa(c), nil))
	}
	out := scrape()
	for _, c := range codes {
		if !strings.Contains(out, `aqs_http_requests_total{code="`+strconv.Itoa(c)+`",method="GET",route="unmatched",service="svc-prueba"} 1`+"\n") {
			t.Errorf("falta code=%d con valor 1", c)
		}
	}
	// sin WriteHeader explícito el código es 200
	h2, _, _, scrape2 := newWrapped(t, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("x")) }))
	h2.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil))
	if !strings.Contains(scrape2(), `code="200",method="GET",route="unmatched"`) {
		t.Error("una respuesta sin WriteHeader cuenta como 200")
	}
}

// La latencia se publica en SEGUNDOS: una petición de ≈50 ms suma entre 0,04 y 5.
func TestLatencyIsInSeconds(t *testing.T) {
	h, _, _, scrape := newWrapped(t, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/lenta", nil))
	out := scrape()
	var sum, count float64
	for _, ln := range strings.Split(out, "\n") {
		if strings.HasPrefix(ln, `aqs_http_request_duration_seconds_sum{method="GET",route="unmatched"`) {
			sum, _ = strconv.ParseFloat(ln[strings.LastIndex(ln, " ")+1:], 64)
		}
		if strings.HasPrefix(ln, `aqs_http_request_duration_seconds_count{method="GET",route="unmatched"`) {
			count, _ = strconv.ParseFloat(ln[strings.LastIndex(ln, " ")+1:], 64)
		}
	}
	if count != 1 || sum < 0.04 || sum > 5 {
		t.Errorf("la suma del histograma debe estar en segundos (≈0,05): sum=%v count=%v", sum, count)
	}
	if !strings.Contains(out, `route="unmatched",service="svc-prueba",le="0.005"} 0`) || !strings.Contains(out, `route="unmatched",service="svc-prueba",le="10"} 1`) {
		t.Error("una petición de 50 ms no cabe en el bucket de 5 ms y sí en el de 10 s")
	}
	// y el log de acceso lleva duration_ms en milisegundos
}

func TestAccessLogDurationMsIsMilliseconds(t *testing.T) {
	buf := &bytes.Buffer{}
	reg := NewRegistry()
	h := Wrap(Config{Service: "svc", Log: NewLogger(buf, "svc", slog.LevelInfo), Registry: reg, Metrics: NewHTTPMetrics(reg, "svc")},
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(50 * time.Millisecond) }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil))
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if d, _ := m["duration_ms"].(float64); d < 40 || d > 5000 {
		t.Errorf("duration_ms debe estar en milisegundos (≈50): %v", m["duration_ms"])
	}
}

func TestAccessLogLevelByStatusBoundaries(t *testing.T) {
	for status, want := range map[int]string{200: "info", 301: "info", 399: "info", 400: "info", 404: "info", 429: "info", 499: "info", 500: "warn", 501: "warn", 502: "warn", 503: "warn", 599: "warn"} {
		buf := &bytes.Buffer{}
		reg := NewRegistry()
		s := status
		h := Wrap(Config{Service: "svc", Log: NewLogger(buf, "svc", slog.LevelInfo), Registry: reg, Metrics: NewHTTPMetrics(reg, "svc")},
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(s) }))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil))
		if !strings.Contains(buf.String(), `"level":"`+want+`"`) {
			t.Errorf("status %d: nivel esperado %s: %s", status, want, buf)
		}
	}
}

func TestProbesHeadAndCacheControl(t *testing.T) {
	h, _, _, _ := newWrapped(t, nil, nil)
	for _, p := range []string{"/healthz", "/readyz", "/metrics"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("HEAD", p, nil))
		if rec.Code != 200 {
			t.Errorf("HEAD %s: %d", p, rec.Code)
		}
	}
	for _, p := range []string{"/healthz", "/readyz"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Content-Type") != "application/json" {
			t.Errorf("%s: cabeceras %v", p, rec.Header())
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if strings.TrimSpace(rec.Body.String()) != `{"status":"ok"}` {
		t.Errorf("cuerpo de /healthz: %q", rec.Body)
	}
}

// El 413 conserva «Connection: close» (en net/http real) aunque el handler esté envuelto.
func TestRequestTooLargeKeepsConnectionClose(t *testing.T) {
	reg := NewRegistry()
	app := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 10)); err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(Wrap(Config{Service: "svc", Log: NewLogger(&bytes.Buffer{}, "svc", slog.LevelInfo), Registry: reg, Metrics: NewHTTPMetrics(reg, "svc")}, app))
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/x", "text/plain", strings.NewReader(strings.Repeat("a", 100)))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 413 || !resp.Close {
		t.Errorf("el 413 debe cerrar la conexión: %d close=%v %v", resp.StatusCode, resp.Close, resp.Header)
	}
	resp2, err := http.Post(srv.URL+"/x", "text/plain", strings.NewReader("ok"))
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != 200 || resp2.Close {
		t.Errorf("un 200 no fuerza el cierre: %v", resp2.Header)
	}
}

// El log de un chequeo fallido de /readyz sale con los ids de la petición y el nombre del chequeo.
func TestReadyzFailureLogCarriesIDsAndCheckName(t *testing.T) {
	h, buf, _, _ := newWrapped(t, []Check{{Name: "roto", Fn: func(context.Context) error { return errors.New("detalle") }}}, nil)
	req := httptest.NewRequest("GET", "/readyz", nil)
	req.Header.Set("X-Request-Id", "req-ready-1234")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	tid := strings.Split(rec.Header().Get("traceparent"), "-")[1]
	var found bool
	for _, ln := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var m map[string]any
		_ = json.Unmarshal([]byte(ln), &m)
		if m["message"] == "readyz: chequeo fallido" {
			found = true
			if m["check"] != "roto" || m["level"] != "warn" || m["request_id"] != "req-ready-1234" || m["trace_id"] != tid || m["error"] != "detalle" {
				t.Errorf("línea de readyz: %s", ln)
			}
		}
		if m["message"] == "request" && (m["status"] != float64(503) || m["level"] != "warn") {
			t.Errorf("acceso del 503 de readyz: %s", ln)
		}
	}
	if !found {
		t.Errorf("falta el log del chequeo fallido: %s", buf)
	}
}

func TestRequestIDCharset(t *testing.T) {
	h, _, _, _ := newWrapped(t, nil, nil)
	for id, keep := range map[string]bool{
		"ABCDEFGH": true, "abcdefgh": true, "01234567": true, "a.b.c.d.": true, "a_b_c_d_": true, "a-b-c-d-": true, "Req.ID_1-X": true,
		"abcd/efg": false, "abcd efg": false, "abcd:efg": false, "abcd@efg": false, "abcd+efg": false, "abcd=efg": false, "abcd;efg": false, "abcd,efg": false, "abcd\"efg": false,
	} {
		req := httptest.NewRequest("GET", "/known", nil)
		req.Header.Set("X-Request-Id", id)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if got := rec.Header().Get("X-Request-Id"); (got == id) != keep {
			t.Errorf("%q: conservar=%v, quedó %q", id, keep, got)
		}
	}
}

// «Connection: close» es solo del 413: ni 400, 404, 405, 500 ni 200 lo llevan.
func TestConnectionCloseOnlyOn413(t *testing.T) {
	for _, status := range []int{200, 201, 204, 400, 401, 404, 405, 409, 412, 413, 414, 415, 422, 429, 500, 503} {
		s := status
		h, _, _, _ := newWrapped(t, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(s) }))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))
		got := rec.Header().Get("Connection")
		if s == 413 && got != "close" {
			t.Errorf("413 debe llevar Connection: close: %q", got)
		}
		if s != 413 && got != "" {
			t.Errorf("%d no debe llevar Connection: %q", s, got)
		}
	}
	// también las respuestas de las propias sondas (405 de POST /healthz)
	h, _, _, _ := newWrapped(t, nil, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/healthz", nil))
	if rec.Code != 405 || rec.Header().Get("Connection") != "" {
		t.Errorf("405 de sonda: %d %v", rec.Code, rec.Header())
	}
}
