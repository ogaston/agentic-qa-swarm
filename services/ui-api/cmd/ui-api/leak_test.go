package main

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/ui-api/inbox"
	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/httpapi"
	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/obs"
)

func mark(s string) string {
	l := strings.ToLower(s)
	for _, m := range []string{"pii-", "falso.test"} {
		if strings.Contains(l, m) {
			return m
		}
	}
	return ""
}

type uiCase struct {
	name       string
	burst      int
	pre        func(t *testing.T, c *chain)
	act        func(c *chain) *httptest.ResponseRecorder
	method     string
	route      string
	status     int
	confirms   string // aqs_inbox_confirmations_total
	pending    string
	confirmed  string
	logMessage string
}

func jsonPost(path, body string, hdr ...string) func(c *chain) *httptest.ResponseRecorder {
	return func(c *chain) *httptest.ResponseRecorder {
		h := append([]string{"Authorization", "Bearer " + tok, "Content-Type", "application/json"}, hdr...)
		return c.do("POST", path, body, h...)
	}
}

const confirmRoute = "/notifications/{id}/confirm"

func uiCases() []uiCase {
	pii := `"PII-CLAVE-XYZ":"pii-correo@falso.test","PII-NOMBRE-XYZ":"PII-VALOR-XYZ"`
	return []uiCase{
		{name: "201 confirmación", act: jsonPost("/notifications/n-1/confirm", `{"flows":["PII-FLUJO-XYZ pii-a@falso.test"],`+pii+`}`),
			method: "POST", route: confirmRoute, status: 201, confirms: "1", pending: "1", confirmed: "1"},
		{name: "400 flows vacío", act: jsonPost("/notifications/n-1/confirm", `{"flows":[],`+pii+`}`),
			method: "POST", route: confirmRoute, status: 400, confirms: "0", pending: "2", confirmed: "0"},
		{name: "400 elemento en blanco", act: jsonPost("/notifications/n-1/confirm", `{"flows":["PII-FLUJO-XYZ"," "],`+pii+`}`),
			method: "POST", route: confirmRoute, status: 400, confirms: "0", pending: "2", confirmed: "0"},
		{name: "400 JSON roto", act: jsonPost("/notifications/n-1/confirm", `{"flows":["PII-FLUJO-XYZ"`),
			method: "POST", route: confirmRoute, status: 400, confirms: "0", pending: "2", confirmed: "0"},
		{name: "400 datos tras el objeto", act: jsonPost("/notifications/n-1/confirm", `{"flows":["a"]} PII-COLA-XYZ pii-b@falso.test`),
			method: "POST", route: confirmRoute, status: 400, confirms: "0", pending: "2", confirmed: "0"},
		{name: "400 state inválido en la consulta", act: func(c *chain) *httptest.ResponseRecorder {
			return c.auth("GET", "/notifications?state=PII-ESTADO-XYZ&x=pii-q@falso.test", "")
		}, method: "GET", route: "/notifications", status: 400, confirms: "0", pending: "2", confirmed: "0"},
		{name: "404 notificación inexistente", act: jsonPost("/notifications/n-PII-ID-XYZ/confirm", `{"flows":["PII-FLUJO-XYZ"],`+pii+`}`),
			method: "POST", route: confirmRoute, status: 404, confirms: "0", pending: "2", confirmed: "0"},
		{name: "404 ruta desconocida", act: func(c *chain) *httptest.ResponseRecorder {
			return c.auth("GET", "/PII-RUTA-XYZ/pii-r@falso.test", "")
		}, method: "GET", route: "unmatched", status: 404, confirms: "0", pending: "2", confirmed: "0"},
		{name: "405 método no permitido", act: func(c *chain) *httptest.ResponseRecorder {
			return c.auth("PUT", "/notifications", `{`+pii+`}`)
		}, method: "PUT", route: "/notifications", status: 405, confirms: "0", pending: "2", confirmed: "0"},
		{name: "409 ya confirmada", pre: func(t *testing.T, c *chain) {
			if _, err := c.store.Confirm("n-1", "u0", []string{"f"}); err != nil {
				t.Fatal(err)
			}
		}, act: jsonPost("/notifications/n-1/confirm", `{"flows":["PII-FLUJO-XYZ"],`+pii+`}`),
			method: "POST", route: confirmRoute, status: 409, confirms: "0", pending: "1", confirmed: "1"},
		{name: "413 cuerpo grande", act: jsonPost("/notifications/n-1/confirm", `{`+pii+`,"pad":"`+strings.Repeat("a", httpapi.MaxBodyBytes)+`"}`),
			method: "POST", route: confirmRoute, status: 413, confirms: "0", pending: "2", confirmed: "0"},
		{name: "415 Content-Type", act: func(c *chain) *httptest.ResponseRecorder {
			return c.do("POST", "/notifications/n-1/confirm", `{"flows":["PII-FLUJO-XYZ"],`+pii+`}`, "Authorization", "Bearer "+tok, "Content-Type", "text/plain; x=PII-CT-XYZ")
		}, method: "POST", route: confirmRoute, status: 415, confirms: "0", pending: "2", confirmed: "0"},
		{name: "401 sin token", act: func(c *chain) *httptest.ResponseRecorder {
			return c.do("POST", "/notifications/n-1/confirm", `{`+pii+`}`, "Content-Type", "application/json")
		}, method: "POST", route: confirmRoute, status: 401, confirms: "0", pending: "2", confirmed: "0"},
		{name: "401 token inválido con marcas", act: func(c *chain) *httptest.ResponseRecorder {
			return c.do("GET", "/notifications", "", "Authorization", "Bearer PII-TOKEN-XYZ.pii-t@falso.test")
		}, method: "GET", route: "/notifications", status: 401, confirms: "0", pending: "2", confirmed: "0"},
		{name: "401 doble Authorization", act: func(c *chain) *httptest.ResponseRecorder {
			r := httptest.NewRequest("GET", "/notifications", nil)
			r.Header.Add("Authorization", "Bearer "+tok)
			r.Header.Add("Authorization", "Bearer PII-OTRO-XYZ")
			rec := httptest.NewRecorder()
			c.h.ServeHTTP(rec, r)
			return rec
		}, method: "GET", route: "/notifications", status: 401, confirms: "0", pending: "2", confirmed: "0"},
		{name: "200 listado", act: func(c *chain) *httptest.ResponseRecorder { return c.auth("GET", "/notifications", "") },
			method: "GET", route: "/notifications", status: 200, confirms: "0", pending: "2", confirmed: "0"},
		{name: "429 límite por IP", burst: 1, act: func(c *chain) *httptest.ResponseRecorder {
			c.auth("GET", "/notifications", "")
			return c.do("POST", "/notifications/n-1/confirm", `{"flows":["PII-FLUJO-XYZ"],`+pii+`}`, "Authorization", "Bearer "+tok,
				"Content-Type", "application/json", "X-Forwarded-For", "PII-XFF-XYZ, pii-ip@falso.test")
		}, method: "POST", route: confirmRoute, status: 429, confirms: "0", pending: "2", confirmed: "0"},
		{name: "500 almacén roto", pre: func(t *testing.T, c *chain) {
			if err := os.RemoveAll(c.dataDir); err != nil {
				t.Fatal(err)
			}
		}, act: jsonPost("/notifications/n-1/confirm", `{"flows":["PII-FLUJO-XYZ pii-a@falso.test"],`+pii+`}`),
			method: "POST", route: confirmRoute, status: 500, confirms: "0", pending: "2", confirmed: "0", logMessage: "confirmando la notificación"},
	}
}

// Ninguna ruta de ui-api (éxito, cada 4xx, 429, 500) deja el cuerpo, la consulta, la ruta cruda ni
// los tokens en el log (debug), en /metrics ni en la respuesta, y cada una cuenta lo que debe con
// valores exactos: code real, confirmaciones y gauge por estado.
func TestOpsEndpointsEveryRouteLeaksNothingAndCountsExactly(t *testing.T) {
	for _, tc := range uiCases() {
		t.Run(tc.name, func(t *testing.T) {
			b := tc.burst
			if b == 0 {
				b = 1000
			}
			c := newChain(t, b)
			if tc.pre != nil {
				tc.pre(t, c)
			}
			c.logs.Reset()
			w := tc.act(c)
			if w.Code != tc.status {
				t.Fatalf("status=%d, esperado %d (%s)", w.Code, tc.status, w.Body)
			}
			metrics := c.do("GET", "/metrics", "").Body.String()
			for what, text := range map[string]string{"respuesta": w.Body.String(), "log": c.logs.String(), "/metrics": metrics} {
				if m := mark(text); m != "" {
					t.Errorf("%s contiene la marca %q: %s", what, m, text)
				}
			}
			if strings.Contains(c.logs.String(), tok) || strings.Contains(metrics, tok) {
				t.Error("el token no debe salir en el log ni en /metrics")
			}
			for _, l := range logLines(t, c.logs) {
				if l["request_id"] == "" || l["trace_id"] == "" {
					t.Errorf("línea de log dentro de una petición sin ids: %v", l)
				}
			}
			if tc.logMessage != "" && !strings.Contains(c.logs.String(), `"message":"`+tc.logMessage+`"`) {
				t.Errorf("falta el log %q: %s", tc.logMessage, c.logs)
			}
			want := `aqs_http_requests_total{code="` + strconv.Itoa(tc.status) + `",method="` + tc.method + `",route="` + tc.route + `",service="ui-api"} 1` + "\n"
			if !strings.Contains(metrics, want) {
				t.Errorf("falta la serie exacta %q en:\n%s", strings.TrimSpace(want), grepLines(metrics, "aqs_http_requests_total"))
			}
			for _, w := range []string{"aqs_inbox_confirmations_total " + tc.confirms, `aqs_inbox_notifications{state="pending"} ` + tc.pending,
				`aqs_inbox_notifications{state="confirmed"} ` + tc.confirmed, `aqs_inbox_notifications{state="rejected"} 0`} {
				if !strings.Contains(metrics, w+"\n") {
					t.Errorf("falta %q", w)
				}
			}
		})
	}
}

func grepLines(s, sub string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, sub) {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// Un panic del verificador (valor con marcas) tampoco deja el valor ni el token en log, métricas o respuesta.
func TestOpsEndpointsPanicLeaksNothing(t *testing.T) {
	root := t.TempDir()
	logs := &bytes.Buffer{}
	st, err := inbox.OpenStore(root+"/d", nil)
	if err != nil {
		t.Fatal(err)
	}
	h, err := newHandler(obs.NewLogger(logs, serviceName, slog.LevelDebug),
		httpapi.Config{Store: st, Verifier: panicVerifier{}}, root+"/d", root+"/e.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/notifications/n-1/confirm", strings.NewReader(`{"PII-CLAVE-XYZ":"pii-c@falso.test"}`))
	r.Header.Set("Authorization", "Bearer PII-TOKEN-XYZ")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("panic -> 500: %d", rec.Code)
	}
	m := httptest.NewRecorder()
	h.ServeHTTP(m, httptest.NewRequest("GET", "/metrics", nil))
	for what, text := range map[string]string{"respuesta": rec.Body.String(), "log": logs.String(), "/metrics": m.Body.String()} {
		if x := mark(text); x != "" || strings.Contains(text, "valor-que-no-debe-salir") {
			t.Errorf("%s contiene %q: %s", what, x, text)
		}
	}
	if !strings.Contains(m.Body.String(), `aqs_http_requests_total{code="500",method="POST",route="/notifications/{id}/confirm",service="ui-api"} 1`) {
		t.Error("el panic cuenta como 500 en la ruta de confirmación")
	}
}
