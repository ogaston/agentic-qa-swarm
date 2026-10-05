// Package obs implementa el contrato de observabilidad de docs/observability.md:
// log JSON con request_id/trace_id, /healthz, /readyz, /metrics y métricas HTTP.
// El mismo contrato se reimplementa en cada módulo (no hay código compartido entre servicios).
package obs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type ctxKey int

const (
	keyRequestID ctxKey = iota + 1
	keyTraceID
)

var (
	requestIDRe   = regexp.MustCompile(`^[A-Za-z0-9._-]{8,64}$`)
	traceparentRe = regexp.MustCompile(`^00-([0-9a-f]{32})-([0-9a-f]{16})-[0-9a-f]{2}$`)
)

// RequestID devuelve el request_id del contexto ("" fuera de una petición).
func RequestID(ctx context.Context) string { s, _ := ctx.Value(keyRequestID).(string); return s }

// TraceID devuelve el trace_id del contexto ("" fuera de una petición).
func TraceID(ctx context.Context) string { s, _ := ctx.Value(keyTraceID).(string); return s }

func randHex(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func allZero(s string) bool { return strings.Trim(s, "0") == "" }

// ParseTraceparent extrae el trace_id de un traceparent W3C versión 00 (ids no nulos).
func ParseTraceparent(h string) (string, bool) {
	m := traceparentRe.FindStringSubmatch(h)
	if m == nil || allZero(m[1]) || allZero(m[2]) {
		return "", false
	}
	return m[1], true
}

// ValidRequestID indica si X-Request-Id cumple ^[A-Za-z0-9._-]{8,64}$.
func ValidRequestID(s string) bool { return requestIDRe.MatchString(s) }

// sensitiveKey detecta claves de atributo que jamás deben salir en un log.
func sensitiveKey(k string) bool {
	k = strings.ToLower(k)
	for _, s := range []string{"password", "passwd", "secret", "token", "otp", "authorization", "hash", "cookie"} {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}

// ctxHandler añade request_id y trace_id (vacíos fuera de una petición) a cada registro.
type ctxHandler struct{ slog.Handler }

func (h ctxHandler) Handle(ctx context.Context, r slog.Record) error {
	r.AddAttrs(slog.String("request_id", RequestID(ctx)), slog.String("trace_id", TraceID(ctx)))
	return h.Handler.Handle(ctx, r)
}
func (h ctxHandler) WithAttrs(a []slog.Attr) slog.Handler { return ctxHandler{h.Handler.WithAttrs(a)} }
func (h ctxHandler) WithGroup(n string) slog.Handler      { return ctxHandler{h.Handler.WithGroup(n)} }

// ParseLevel interpreta LOG_LEVEL (debug|info|warn|error); vacío o desconocido: info.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	}
	return slog.LevelInfo
}

// NewLogger crea el logger JSON del contrato: timestamp (UTC), level en minúsculas,
// message, request_id, trace_id y service. Redacta cualquier atributo con nombre sensible.
func NewLogger(w io.Writer, service string, level slog.Level) *slog.Logger {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level, ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
		if len(groups) == 0 {
			switch a.Key {
			case slog.TimeKey:
				return slog.String("timestamp", a.Value.Time().UTC().Format(time.RFC3339Nano))
			case slog.LevelKey:
				return slog.String("level", strings.ToLower(a.Value.String()))
			case slog.MessageKey:
				return slog.String("message", a.Value.String())
			}
		}
		if sensitiveKey(a.Key) {
			return slog.String(a.Key, "[redacted]")
		}
		return a
	}})
	return slog.New(ctxHandler{h}).With("service", service)
}

// NewRegistry crea un registro con los colectores de Go y de proceso.
func NewRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return reg
}

// HTTPMetrics son las métricas HTTP comunes.
type HTTPMetrics struct {
	service  string
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inFlight prometheus.Gauge
}

// NewHTTPMetrics registra aqs_http_requests_total, aqs_http_request_duration_seconds y aqs_http_in_flight.
func NewHTTPMetrics(reg prometheus.Registerer, service string) *HTTPMetrics {
	m := &HTTPMetrics{
		service: service,
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aqs_http_requests_total",
			Help: "Peticiones HTTP por servicio, patrón de ruta, método y código."}, []string{"service", "route", "method", "code"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "aqs_http_request_duration_seconds",
			Help: "Latencia de las peticiones HTTP.", Buckets: prometheus.DefBuckets}, []string{"service", "route", "method"}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{Name: "aqs_http_in_flight",
			Help: "Peticiones HTTP en curso.", ConstLabels: prometheus.Labels{"service": service}}),
	}
	reg.MustRegister(m.requests, m.duration, m.inFlight)
	return m
}

func methodLabel(m string) string {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return m
	}
	return "OTHER"
}

// Check es un chequeo de /readyz.
type Check struct {
	Name string
	Fn   func(ctx context.Context) error
}

// CheckTimeout es el tope de cada chequeo de /readyz (variable solo para las pruebas).
var CheckTimeout = 2 * time.Second

// Config configura Wrap.
type Config struct {
	Service  string
	Log      *slog.Logger
	Registry *prometheus.Registry
	Metrics  *HTTPMetrics
	Ready    []Check
	// Route devuelve el PATRÓN de la ruta, evaluado después de atender la petición
	// ("unmatched" si no hay patrón). Nunca debe devolver la ruta cruda.
	Route func(r *http.Request) string
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(c int) {
	if w.status == 0 {
		w.status = c
	}
	if c == http.StatusRequestEntityTooLarge {
		// http.MaxBytesReader solo marca «Connection: close» si el writer es el de net/http
		// (interfaz con método no exportado, que este envoltorio no puede satisfacer): se
		// replica aquí para que el 413 siga cerrando la conexión como antes de U1-T06.
		w.ResponseWriter.Header().Set("Connection", "close")
	}
	w.ResponseWriter.WriteHeader(c)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// Wrap sirve /healthz, /readyz y /metrics (sin token, sin pasar por la aplicación) y envuelve el
// resto con identificadores, log de acceso y métricas HTTP.
func Wrap(c Config, app http.Handler) http.Handler {
	if c.Route == nil {
		c.Route = func(*http.Request) string { return "unmatched" }
	}
	metricsH := promhttp.HandlerFor(c.Registry, promhttp.HandlerOpts{})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get("X-Request-Id")
		if !ValidRequestID(rid) {
			rid = randHex(8)
		}
		tid, ok := ParseTraceparent(r.Header.Get("traceparent"))
		if !ok {
			tid = randHex(16)
		}
		w.Header().Set("X-Request-Id", rid)
		w.Header().Set("traceparent", fmt.Sprintf("00-%s-%s-01", tid, randHex(8)))
		ctx := context.WithValue(context.WithValue(r.Context(), keyRequestID, rid), keyTraceID, tid)
		r = r.WithContext(ctx)

		start := time.Now()
		c.Metrics.inFlight.Inc()
		defer c.Metrics.inFlight.Dec()
		sw := &statusWriter{ResponseWriter: w}
		route := ""
		switch r.URL.Path {
		case "/healthz":
			route = "/healthz"
			opsMethod(sw, r, func() { writeJSON(sw, http.StatusOK, map[string]string{"status": "ok"}) })
		case "/readyz":
			route = "/readyz"
			opsMethod(sw, r, func() { c.readyz(sw, r) })
		case "/metrics":
			route = "/metrics"
			opsMethod(sw, r, func() { metricsH.ServeHTTP(sw, r) })
		default:
			app.ServeHTTP(sw, r)
			route = c.Route(r)
		}
		if sw.status == 0 {
			sw.status = http.StatusOK
		}
		dur := time.Since(start)
		m := methodLabel(r.Method)
		c.Metrics.requests.WithLabelValues(c.Service, route, m, fmt.Sprint(sw.status)).Inc()
		c.Metrics.duration.WithLabelValues(c.Service, route, m).Observe(dur.Seconds())
		// Las sondas también se registran en info (CA-3 de U1-T06 exige ver el request_id de /healthz);
		// quien no las quiera en el log sube LOG_LEVEL a warn.
		lvl := slog.LevelInfo
		if sw.status >= 500 {
			lvl = slog.LevelWarn // un 5xx no se pierde con LOG_LEVEL=warn
		}
		c.Log.Log(ctx, lvl, "request", "route", route, "method", m, "status", sw.status,
			"duration_ms", float64(dur.Microseconds())/1000)
	})
}

func opsMethod(w http.ResponseWriter, r *http.Request, h func()) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"code": "method_not_allowed", "message": "método no permitido"})
		return
	}
	h()
}

func (c Config) readyz(w http.ResponseWriter, r *http.Request) {
	res := RunChecks(r.Context(), c.Ready)
	status, code := "ready", http.StatusOK
	checks := map[string]string{}
	for _, cr := range res {
		if cr.Err != nil {
			status, code = "unavailable", http.StatusServiceUnavailable
			checks[cr.Name] = "fail"
			c.Log.WarnContext(r.Context(), "readyz: chequeo fallido", "check", cr.Name, "error", cr.Err.Error())
		} else {
			checks[cr.Name] = "ok"
		}
	}
	writeJSON(w, code, map[string]any{"status": status, "checks": checks})
}

// CheckResult es el resultado de un chequeo.
type CheckResult struct {
	Name string
	Err  error
}

// RunChecks ejecuta los chequeos en paralelo, cada uno con CheckTimeout.
func RunChecks(ctx context.Context, cs []Check) []CheckResult {
	out := make([]CheckResult, len(cs))
	var wg sync.WaitGroup
	for i, ch := range cs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, CheckTimeout)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- ch.Fn(cctx) }()
			select {
			case err := <-done:
				out[i] = CheckResult{ch.Name, err}
			case <-cctx.Done():
				out[i] = CheckResult{ch.Name, fmt.Errorf("tiempo agotado: %w", cctx.Err())}
			}
		}()
	}
	wg.Wait()
	return out
}
