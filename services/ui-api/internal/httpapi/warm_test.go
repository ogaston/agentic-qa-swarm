package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/auth"
)

// Marcadores: el token de servicio y el de la persona no deben aparecer donde no corresponde.
const (
	warmSvcMarker  = "svc-MARCADOR-UNICO-987"
	warmPersonTok  = "tok-PERSONA-MARCADOR-456"
	warmStateValid = `{"warm_id":"warm-1","state":"ready","reset_verified":true,"baseline_version":"b1"}`
)

// warmFake es un warm-manager simulado: registra cada petición y responde con handler.
type warmFake struct {
	srv   *httptest.Server
	mu    sync.Mutex
	calls int
	hdrs  []http.Header
}

func newWarmFake(t *testing.T, handler http.HandlerFunc) *warmFake {
	t.Helper()
	f := &warmFake{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.calls++
		f.hdrs = append(f.hdrs, r.Header.Clone())
		f.mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *warmFake) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func respond(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

// warmEnv arma la API con un warm-manager simulado y un archivo con el token de servicio.
func warmEnv(t *testing.T, f *warmFake, logs *bytes.Buffer) *env {
	t.Helper()
	tokFile := filepath.Join(t.TempDir(), "warm-token")
	if err := os.WriteFile(tokFile, []byte(warmSvcMarker+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	v, _ := auth.NewFakeTokenVerifier(warmPersonTok + "=u1:user")
	return newEnv(t, v, func(c *Config) {
		c.WarmURL = f.srv.URL
		c.WarmTokenFile = tokFile
		if logs != nil {
			c.Slog = slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
		}
	})
}

func TestWarmOKParaCadaEstado(t *testing.T) {
	for _, st := range []string{"ready", "dirty", "cuarentena", "idle-escalado"} {
		body := `{"warm_id":"warm-1","state":"` + st + `","reset_verified":false,"baseline_version":"b1"}`
		f := newWarmFake(t, respond(http.StatusOK, body))
		e := warmEnv(t, f, nil)
		w := e.do(http.MethodGet, "/warm", warmPersonTok, "")
		if w.Code != http.StatusOK {
			t.Fatalf("estado %s: %d %s", st, w.Code, w.Body.String())
		}
		var got map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got["state"] != st {
			t.Fatalf("estado %s: cuerpo %s (%v)", st, w.Body.String(), err)
		}
	}
}

func TestWarmTokenDeServicioEnAuthorizationNoElDePersona(t *testing.T) {
	f := newWarmFake(t, respond(http.StatusOK, warmStateValid))
	e := warmEnv(t, f, nil)
	if w := e.do(http.MethodGet, "/warm", warmPersonTok, ""); w.Code != http.StatusOK {
		t.Fatalf("%d", w.Code)
	}
	if f.count() != 1 {
		t.Fatalf("llamadas al warm = %d, quiero 1", f.count())
	}
	h := f.hdrs[0]
	if got := h.Get("Authorization"); got != "Bearer "+warmSvcMarker {
		t.Fatalf("Authorization = %q, quiero el token de servicio", got)
	}
	for k, vs := range h {
		for _, v := range vs {
			if strings.Contains(v, warmPersonTok) {
				t.Fatalf("el token de la persona se reenvió en la cabecera %s", k)
			}
		}
	}
}

func TestWarmSinTokenDePersonaDevuelve401SinLlamar(t *testing.T) {
	f := newWarmFake(t, respond(http.StatusOK, warmStateValid))
	e := warmEnv(t, f, nil)
	if w := e.do(http.MethodGet, "/warm", "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("sin token: %d", w.Code)
	}
	if f.count() != 0 {
		t.Fatalf("el warm recibió %d peticiones sin token de persona", f.count())
	}
}

func TestWarmRespuestaInvalidaEs503WarmUnavailable(t *testing.T) {
	for name, body := range map[string]string{
		"estado fuera de enum": `{"warm_id":"w","state":"zombi","reset_verified":true,"baseline_version":"b"}`,
		"campo extra":          `{"warm_id":"w","state":"ready","reset_verified":true,"baseline_version":"b","x":1}`,
		"falta requerido":      `{"warm_id":"w","state":"ready","reset_verified":true}`,
		"no es JSON":           `<html>`,
	} {
		f := newWarmFake(t, respond(http.StatusOK, body))
		e := warmEnv(t, f, nil)
		w := e.do(http.MethodGet, "/warm", warmPersonTok, "")
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), `"warm_unavailable"`) {
			t.Fatalf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
}

func TestWarmErroresDelWarmDevuelven503ConRetryAfter(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"5xx": respond(http.StatusInternalServerError, `{}`),
		"401": respond(http.StatusUnauthorized, `{}`),
		"lento": func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(3 * time.Second):
			}
		},
	} {
		f := newWarmFake(t, handler)
		e := warmEnv(t, f, nil)
		w := e.do(http.MethodGet, "/warm", warmPersonTok, "")
		if w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") == "" ||
			!strings.Contains(w.Body.String(), `"warm_unavailable"`) {
			t.Fatalf("%s: %d retry=%q %s", name, w.Code, w.Header().Get("Retry-After"), w.Body.String())
		}
	}
}

func TestWarmCircuitoTrasCincoFallosDeTransporte(t *testing.T) {
	// El warm cierra la conexión sin responder: fallo de transporte.
	f := newWarmFake(t, func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	})
	e := warmEnv(t, f, nil)
	for i := 0; i < 5; i++ {
		if w := e.do(http.MethodGet, "/warm", warmPersonTok, ""); w.Code != http.StatusServiceUnavailable {
			t.Fatalf("intento %d: %d", i+1, w.Code)
		}
	}
	if f.count() != 5 {
		t.Fatalf("llamadas = %d, quiero 5", f.count())
	}
	w := e.do(http.MethodGet, "/warm", warmPersonTok, "")
	if w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") == "" {
		t.Fatalf("sexto: %d retry=%q", w.Code, w.Header().Get("Retry-After"))
	}
	if f.count() != 5 {
		t.Fatalf("el circuito abierto dejó salir el sexto: llamadas = %d", f.count())
	}
}

func TestWarmTokenDeServicioNoAparecePorLogNiCuerpo(t *testing.T) {
	var logs bytes.Buffer
	for _, handler := range []http.HandlerFunc{
		respond(http.StatusOK, warmStateValid),
		respond(http.StatusUnauthorized, `{"error":"bad"}`),
		respond(http.StatusInternalServerError, `{}`),
	} {
		f := newWarmFake(t, handler)
		e := warmEnv(t, f, &logs)
		w := e.do(http.MethodGet, "/warm", warmPersonTok, "")
		if strings.Contains(w.Body.String(), warmSvcMarker) {
			t.Fatalf("el token de servicio aparece en el cuerpo: %s", w.Body.String())
		}
	}
	if strings.Contains(logs.String(), warmSvcMarker) {
		t.Fatalf("el token de servicio aparece en el log: %s", logs.String())
	}
	if !strings.Contains(logs.String(), `"level":"ERROR"`) {
		t.Fatalf("el 401 del warm-manager debe registrarse en error: %s", logs.String())
	}
}

func TestWarmSinWarmURLDevuelve503YNotificationsSigue200(t *testing.T) {
	v, _ := auth.NewFakeTokenVerifier(warmPersonTok + "=u1:user")
	e := newEnv(t, v, nil)
	if w := e.do(http.MethodGet, "/warm", warmPersonTok, ""); w.Code != http.StatusServiceUnavailable ||
		!strings.Contains(w.Body.String(), `"warm_unavailable"`) {
		t.Fatalf("sin WARM_URL: %d %s", w.Code, w.Body.String())
	}
	if w := e.do(http.MethodGet, "/notifications", warmPersonTok, ""); w.Code != http.StatusOK {
		t.Fatalf("/notifications sin WARM_URL: %d", w.Code)
	}
}

func TestWarmSoloGET(t *testing.T) {
	f := newWarmFake(t, respond(http.StatusOK, warmStateValid))
	e := warmEnv(t, f, nil)
	if w := e.do(http.MethodPost, "/warm", warmPersonTok, ""); w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /warm: %d", w.Code)
	}
	if f.count() != 0 {
		t.Fatalf("POST llegó al warm")
	}
}

func TestWarmArranqueRechazaConfigIncompleta(t *testing.T) {
	v, _ := auth.NewFakeTokenVerifier(warmPersonTok + "=u1:user")
	tokFile := filepath.Join(t.TempDir(), "tok")
	_ = os.WriteFile(tokFile, []byte(warmSvcMarker), 0o600)
	for name, cfg := range map[string]Config{
		"WARM_URL sin token file":   {WarmURL: "http://warm.example"},
		"WARM_URL sin esquema":      {WarmURL: "warm.example:8080", WarmTokenFile: tokFile},
		"WARM_URL con esquema ftp":  {WarmURL: "ftp://warm.example", WarmTokenFile: tokFile},
		"WARM_URL sin host":         {WarmURL: "http://", WarmTokenFile: tokFile},
		"WARM_URL con credenciales": {WarmURL: "http://user:pass@warm.example", WarmTokenFile: tokFile},
	} {
		cfg.Store, cfg.Verifier = newEnv(t, v, nil).store, v
		if _, err := New(cfg); err == nil {
			t.Fatalf("%s: New debe fallar", name)
		}
	}
}

// Cinco 5xx seguidos no abren el circuito: las seis llamadas llegan al warm (interpretación del
// circuito: solo fallos de transporte y plazo).
func TestWarmCincoCincoxxNoAbreElCircuito(t *testing.T) {
	f := newWarmFake(t, respond(http.StatusInternalServerError, `{}`))
	e := warmEnv(t, f, nil)
	for i := 0; i < 6; i++ {
		if w := e.do(http.MethodGet, "/warm", warmPersonTok, ""); w.Code != http.StatusServiceUnavailable {
			t.Fatalf("intento %d: %d", i+1, w.Code)
		}
	}
	if f.count() != 6 {
		t.Fatalf("llamadas = %d, quiero 6 (un 5xx no abre el circuito)", f.count())
	}
}

// Archivo de token ilegible o vacío: 503 warm_unavailable, cero llamadas al warm y error en el log.
func TestWarmTokenIlegibleOVacioDa503SinLlamar(t *testing.T) {
	for name, prepare := range map[string]func(string) string{
		"ilegible": func(dir string) string { return filepath.Join(dir, "no-existe") },
		"vacio": func(dir string) string {
			p := filepath.Join(dir, "vacio")
			_ = os.WriteFile(p, []byte(" \n"), 0o600)
			return p
		},
	} {
		f := newWarmFake(t, respond(http.StatusOK, warmStateValid))
		var logs bytes.Buffer
		v, _ := auth.NewFakeTokenVerifier(warmPersonTok + "=u1:user")
		tokFile := prepare(t.TempDir())
		e := newEnv(t, v, func(c *Config) {
			c.WarmURL = f.srv.URL
			c.WarmTokenFile = tokFile
			c.Slog = slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
		})
		w := e.do(http.MethodGet, "/warm", warmPersonTok, "")
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), `"warm_unavailable"`) {
			t.Fatalf("%s: %d %s", name, w.Code, w.Body.String())
		}
		if f.count() != 0 {
			t.Fatalf("%s: el warm recibió %d llamadas sin token", name, f.count())
		}
		if !strings.Contains(logs.String(), `"level":"ERROR"`) {
			t.Fatalf("%s: falta el log error: %s", name, logs.String())
		}
	}
}

// Medio abierto con reloj inyectable: tras 5 fallos el circuito abre; al vencer la ventana una
// sonda que tiene éxito cierra el circuito; una sonda fallida lo reabre sin dejar pasar más.
func TestWarmCircuitoMedioAbiertoConRelojInyectable(t *testing.T) {
	var mu sync.Mutex
	broken := true
	f := newWarmFake(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		b := broken
		mu.Unlock()
		if b {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
			return
		}
		respond(http.StatusOK, warmStateValid)(w, r)
	})
	e := warmEnv(t, f, nil)
	for i := 0; i < 5; i++ {
		e.do(http.MethodGet, "/warm", warmPersonTok, "")
	}
	if f.count() != 5 {
		t.Fatalf("antes de abrir: %d llamadas", f.count())
	}
	// Ventana abierta: no sale.
	*e.clock = e.clock.Add(9 * time.Second)
	e.do(http.MethodGet, "/warm", warmPersonTok, "")
	if f.count() != 5 {
		t.Fatalf("circuito abierto dejó salir: %d llamadas", f.count())
	}
	// Vence la ventana y la sonda falla: reabre.
	*e.clock = e.clock.Add(2 * time.Second)
	e.do(http.MethodGet, "/warm", warmPersonTok, "")
	if f.count() != 6 {
		t.Fatalf("sonda: %d llamadas, quiero 6", f.count())
	}
	e.do(http.MethodGet, "/warm", warmPersonTok, "")
	if f.count() != 6 {
		t.Fatalf("tras sonda fallida no reabrió: %d llamadas", f.count())
	}
	// Vence otra vez y el warm está sano: la sonda cierra el circuito.
	mu.Lock()
	broken = false
	mu.Unlock()
	*e.clock = e.clock.Add(11 * time.Second)
	if w := e.do(http.MethodGet, "/warm", warmPersonTok, ""); w.Code != http.StatusOK {
		t.Fatalf("sonda sana: %d %s", w.Code, w.Body.String())
	}
	if w := e.do(http.MethodGet, "/warm", warmPersonTok, ""); w.Code != http.StatusOK {
		t.Fatalf("circuito cerrado: %d", w.Code)
	}
}
