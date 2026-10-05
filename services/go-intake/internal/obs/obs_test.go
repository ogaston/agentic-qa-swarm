package obs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
