package server

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/guard"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/passhash"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/session"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/totp"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/users"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/principal"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

func randHex(t *testing.T, n int) string {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

type env struct {
	t        *testing.T
	srv      *Server
	h        http.Handler
	clk      *clock
	logs     *bytes.Buffer
	pw, apw  string // generadas en cada ejecución
	totpKey  []byte
	sessions *session.Store
}

func setup(t *testing.T, trust bool) *env {
	t.Helper()
	e := &env{t: t, clk: &clock{t: time.Unix(1_700_000_000, 0)}, logs: &bytes.Buffer{}}
	e.pw, e.apw = randHex(t, 12), randHex(t, 12)
	e.totpKey = make([]byte, 20)
	_, _ = rand.Read(e.totpKey)
	h1, _ := passhash.Hash(e.pw, nil)
	h2, _ := passhash.Hash(e.apw, nil)
	us, err := users.Parse(mustJSON(t, []map[string]string{
		{"username": "marta", "password_hash": h1, "role": "user"},
		{"username": "julian", "password_hash": h2, "role": "admin", "mfa_secret": base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(e.totpKey)},
	}))
	if err != nil {
		t.Fatal(err)
	}
	decoy, err := NewDecoy(us)
	if err != nil {
		t.Fatal(err)
	}
	e.sessions = session.New(session.Config{Clock: e.clk.now})
	e.srv = New(Config{Users: us, Guard: guard.New(5, e.clk.now), Sessions: e.sessions, Clock: e.clk.now, Decoy: decoy,
		TrustProxy: trust, Logger: slog.New(slog.NewJSONHandler(e.logs, nil))})
	e.h = e.srv.Handler()
	return e
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func (e *env) post(path, ctype string, body []byte, hdr map[string]string, remote string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	if ctype != "" {
		r.Header.Set("Content-Type", ctype)
	}
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	if remote != "" {
		r.RemoteAddr = remote
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	return w
}

func (e *env) login(m map[string]string) *httptest.ResponseRecorder {
	return e.post("/auth/login", "application/json", mustJSON(e.t, m), nil, "")
}

func code(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var b errorBody
	_ = json.Unmarshal(w.Body.Bytes(), &b)
	return b.Code
}

func TestLoginOK(t *testing.T) {
	e := setup(t, false)
	w := e.login(map[string]string{"username": "marta", "password": e.pw})
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var b struct{ Token, ExpiresAt string }
	_ = json.Unmarshal(w.Body.Bytes(), &struct {
		Token     *string `json:"token"`
		ExpiresAt *string `json:"expires_at"`
	}{&b.Token, &b.ExpiresAt})
	if len(b.Token) != 43 {
		t.Fatalf("token de %d caracteres", len(b.Token))
	}
	if _, err := time.Parse(time.RFC3339, b.ExpiresAt); err != nil {
		t.Fatal(err)
	}
	if p, ok := e.sessions.Validate(b.Token); !ok || p != (principal.Principal{ID: "marta", Role: principal.RoleUser}) {
		t.Fatalf("la sesión debía existir: %v", p)
	}
}

func TestLoginUsernameIsCaseInsensitive(t *testing.T) {
	e := setup(t, false)
	if w := e.login(map[string]string{"username": "MARTA", "password": e.pw}); w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
}

func TestUnknownUserIndistinguishableFromBadPassword(t *testing.T) {
	e := setup(t, false)
	a := e.login(map[string]string{"username": "nadie", "password": randHex(t, 8)})
	b := e.login(map[string]string{"username": "marta", "password": randHex(t, 8)})
	if a.Code != 401 || b.Code != 401 || a.Body.String() != b.Body.String() || code(t, a) != "invalid_credentials" {
		t.Fatalf("difieren: %d %s | %d %s", a.Code, a.Body, b.Code, b.Body)
	}
}

func TestUnknownUserTimingSameOrderOfMagnitude(t *testing.T) {
	e := setup(t, false)
	measure := func(user string) time.Duration {
		s := time.Now()
		e.login(map[string]string{"username": user, "password": randHex(t, 8)})
		return time.Since(s)
	}
	measure("marta") // calienta
	known, unknown := measure("marta"), measure("nadie-1")
	if unknown < known/10 || unknown > known*10 {
		t.Fatalf("tiempos de distinto orden: conocido=%v inexistente=%v", known, unknown)
	}
}

func TestMFARequiredOnlyAfterGoodPassword(t *testing.T) {
	e := setup(t, false)
	bad := e.login(map[string]string{"username": "julian", "password": randHex(t, 8)})
	if code(t, bad) != "invalid_credentials" {
		t.Fatalf("contraseña mala de admin no debe revelar mfa: %s", bad.Body)
	}
	good := e.login(map[string]string{"username": "julian", "password": e.apw})
	if good.Code != 401 || code(t, good) != "mfa_required" {
		t.Fatalf("%d %s", good.Code, good.Body)
	}
}

func TestAdminLoginWithOTPAndReuseRejected(t *testing.T) {
	e := setup(t, false)
	otp := totp.Code(e.totpKey, e.clk.now())
	if w := e.login(map[string]string{"username": "julian", "password": e.apw, "otp": otp}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w := e.login(map[string]string{"username": "julian", "password": e.apw, "otp": otp})
	if w.Code != 401 || code(t, w) != "invalid_credentials" {
		t.Fatalf("reutilización debía rechazarse: %d %s", w.Code, w.Body)
	}
	e.clk.add(30 * time.Second)
	if w := e.login(map[string]string{"username": "julian", "password": e.apw, "otp": totp.Code(e.totpKey, e.clk.now())}); w.Code != 200 {
		t.Fatalf("el código siguiente debía aceptarse: %d", w.Code)
	}
}

func TestWrongOTPIsInvalidCredentials(t *testing.T) {
	e := setup(t, false)
	otp := totp.Code(e.totpKey, e.clk.now().Add(5*time.Minute))
	w := e.login(map[string]string{"username": "julian", "password": e.apw, "otp": otp})
	if w.Code != 401 || code(t, w) != "invalid_credentials" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestLockoutEvenWithCorrectPassword(t *testing.T) {
	e := setup(t, false)
	for i := 0; i < 5; i++ {
		if w := e.login(map[string]string{"username": "marta", "password": randHex(t, 8)}); w.Code != 401 {
			t.Fatalf("intento %d: %d", i+1, w.Code)
		}
	}
	w := e.login(map[string]string{"username": "marta", "password": e.pw})
	if w.Code != 429 || w.Header().Get("Retry-After") != "30" {
		t.Fatalf("%d retry-after=%q", w.Code, w.Header().Get("Retry-After"))
	}
	e.clk.add(31 * time.Second)
	if w := e.login(map[string]string{"username": "marta", "password": e.pw}); w.Code != 200 {
		t.Fatalf("tras el bloqueo debía entrar: %d", w.Code)
	}
}

func TestLockoutAlsoForUnknownUsers(t *testing.T) {
	e := setup(t, false)
	for i := 0; i < 5; i++ {
		e.login(map[string]string{"username": "Fantasma", "password": randHex(t, 8)})
	}
	w := e.login(map[string]string{"username": "fantasma", "password": randHex(t, 8)})
	if w.Code != 429 {
		t.Fatalf("un usuario inexistente también se bloquea (sin oráculo): %d", w.Code)
	}
}

func TestSuccessfulLoginResetsUserFailuresNotIPFailures(t *testing.T) {
	e := setup(t, false)
	try := func(pw, remote string) int {
		return e.post("/auth/login", "application/json", mustJSON(t, map[string]string{"username": "marta", "password": pw}), nil, remote).Code
	}
	// El contador del usuario se reinicia con cada acierto aunque cambie la IP.
	for round := 0; round < 3; round++ {
		remote := "192.0.2." + string(rune('1'+round)) + ":1000"
		for i := 0; i < 4; i++ {
			try(randHex(t, 8), remote)
		}
		if c := try(e.pw, remote); c != 200 {
			t.Fatalf("ronda %d: %d", round, c)
		}
	}
	// El de la IP no: 4 fallos + acierto + 4 fallos desde la misma IP acaban en 429.
	remote := "198.51.100.7:1000"
	for i := 0; i < 4; i++ {
		e.post("/auth/login", "application/json", mustJSON(t, map[string]string{"username": "otro" + randHex(t, 3), "password": randHex(t, 8)}), nil, remote)
	}
	try(e.pw, remote)
	e.post("/auth/login", "application/json", mustJSON(t, map[string]string{"username": "otro" + randHex(t, 3), "password": randHex(t, 8)}), nil, remote)
	if c := try(e.pw, remote); c != 429 {
		t.Fatalf("5 fallos acumulados por IP debían bloquear aunque haya un acierto intercalado: %d", c)
	}
}

func TestMFARequiredDoesNotResetOrCountFailures(t *testing.T) {
	e := setup(t, false)
	for i := 0; i < 4; i++ {
		e.login(map[string]string{"username": "julian", "password": randHex(t, 8)})
	}
	for i := 0; i < 10; i++ { // contraseña correcta sin otp: ni bloquea ni reinicia
		if w := e.login(map[string]string{"username": "julian", "password": e.apw}); code(t, w) != "mfa_required" {
			t.Fatalf("iteración %d: %d %s", i, w.Code, w.Body)
		}
	}
	e.login(map[string]string{"username": "julian", "password": randHex(t, 8)}) // quinto fallo
	if w := e.login(map[string]string{"username": "julian", "password": e.apw}); w.Code != 429 {
		t.Fatalf("mfa_required no debía reiniciar el contador: %d", w.Code)
	}
}

func TestBadRequests(t *testing.T) {
	e := setup(t, false)
	good := map[string]any{"username": "marta", "password": e.pw}
	with := func(k string, v any) []byte {
		m := map[string]any{}
		for a, b := range good {
			m[a] = b
		}
		m[k] = v
		return mustJSON(t, m)
	}
	cases := []struct {
		name  string
		ctype string
		body  []byte
		want  int
	}{
		{"sin content-type", "", mustJSON(t, good), 415},
		{"content-type incorrecto", "text/plain", mustJSON(t, good), 415},
		{"JSON malformado", "application/json", []byte(`{"username":`), 400},
		{"campos extra", "application/json", with("admin", true), 400},
		{"sin password", "application/json", []byte(`{"username":"marta"}`), 400},
		{"sin username", "application/json", mustJSON(t, map[string]string{"password": e.pw}), 400},
		{"contraseña de 2000 bytes", "application/json", with("password", strings.Repeat("a", 2000)), 400},
		{"otp con letras", "application/json", with("otp", "12ab56"), 400},
		{"otp de 5 dígitos", "application/json", with("otp", "12345"), 400},
		{"otp no es texto", "application/json", with("otp", 123456), 400},
		{"dos documentos JSON", "application/json", append(mustJSON(t, good), []byte(`{}`)...), 400},
		{"cuerpo de más de 8 KiB", "application/json", with("username", strings.Repeat("a", 9000)), 413},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if w := e.post("/auth/login", c.ctype, c.body, nil, ""); w.Code != c.want {
				t.Fatalf("%d %s, esperado %d", w.Code, w.Body, c.want)
			}
		})
	}
}

func TestLongPasswordRejectedWithoutHashing(t *testing.T) {
	e := setup(t, false)
	s := time.Now()
	w := e.login(map[string]string{"username": "marta", "password": strings.Repeat("a", 2000)})
	if w.Code != 400 || time.Since(s) > 20*time.Millisecond {
		t.Fatalf("%d en %v: debía rechazarse antes de hashear", w.Code, time.Since(s))
	}
	if u, _ := e.srv.cfg.Guard.Len(); u != 0 {
		t.Fatal("un 400 no debe contar como intento")
	}
}

func TestLogout(t *testing.T) {
	e := setup(t, false)
	w := e.login(map[string]string{"username": "marta", "password": e.pw})
	var b map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &b)
	auth := map[string]string{"Authorization": "Bearer " + b["token"]}
	if w := e.post("/auth/logout", "", nil, auth, ""); w.Code != 204 {
		t.Fatalf("%d", w.Code)
	}
	if w := e.post("/auth/logout", "", nil, auth, ""); w.Code != 401 {
		t.Fatalf("logout repetido: %d", w.Code)
	}
	for name, h := range map[string]map[string]string{"sin cabecera": nil, "token falso": {"Authorization": "Bearer " + randHex(t, 16)}, "esquema distinto": {"Authorization": "Basic abc"}, "bearer vacío": {"Authorization": "Bearer "}} {
		if w := e.post("/auth/logout", "", nil, h, ""); w.Code != 401 {
			t.Fatalf("%s: %d", name, w.Code)
		}
	}
}

func TestLogoutExpiredSession(t *testing.T) {
	e := setup(t, false)
	w := e.login(map[string]string{"username": "marta", "password": e.pw})
	var b map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &b)
	e.clk.add(time.Hour)
	if w := e.post("/auth/logout", "", nil, map[string]string{"Authorization": "Bearer " + b["token"]}, ""); w.Code != 401 {
		t.Fatalf("%d", w.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	e := setup(t, false)
	r := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	if w.Code != 405 {
		t.Fatalf("%d", w.Code)
	}
}

func TestConcurrentFailedLoginsStopAtLimit(t *testing.T) {
	e := setup(t, false)
	var wg sync.WaitGroup
	codes := make(chan int, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- e.login(map[string]string{"username": "marta", "password": "x" + randHex(t, 6)}).Code
		}()
	}
	wg.Wait()
	close(codes)
	n401, n429 := 0, 0
	for c := range codes {
		switch c {
		case 401:
			n401++
		case 429:
			n429++
		}
	}
	if n401 != 5 || n429 != 95 {
		t.Fatalf("401=%d 429=%d, esperado 5 y 95", n401, n429)
	}
}

func TestClientIPAndProxyTrust(t *testing.T) {
	for _, trust := range []bool{false, true} {
		e := setup(t, trust)
		for i := 0; i < 5; i++ { // 5 fallos desde la misma IP de red, con XFF distintos
			e.post("/auth/login", "application/json", mustJSON(t, map[string]string{"username": "u" + randHex(t, 4), "password": randHex(t, 8)}),
				map[string]string{"X-Forwarded-For": "10.0.0." + string(rune('1'+i))}, "192.0.2.1:1234")
		}
		w := e.post("/auth/login", "application/json", mustJSON(t, map[string]string{"username": "marta", "password": e.pw}),
			map[string]string{"X-Forwarded-For": "10.0.0.9"}, "192.0.2.1:1234")
		want := 429 // sin confianza se ignora XFF: misma IP
		if trust {
			want = 200 // con confianza cuenta el XFF: IP distinta
		}
		if w.Code != want {
			t.Fatalf("trust=%v: %d, esperado %d", trust, w.Code, want)
		}
	}
}

func TestSecretsNeverLoggedNorInErrorBodies(t *testing.T) {
	e := setup(t, false)
	e.login(map[string]string{"username": "marta", "password": randHex(t, 8)})
	w := e.login(map[string]string{"username": "marta", "password": e.pw})
	var b map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &b)
	bad := e.login(map[string]string{"username": "marta", "password": "mala" + randHex(t, 6)})
	otpw := e.login(map[string]string{"username": "julian", "password": e.apw, "otp": totp.Code(e.totpKey, e.clk.now())})
	var ob map[string]string
	_ = json.Unmarshal(otpw.Body.Bytes(), &ob)
	logs := e.logs.String()
	for name, secret := range map[string]string{"token": b["token"], "token admin": ob["token"], "contraseña": e.pw, "contraseña admin": e.apw,
		"hash token": hex.EncodeToString(sha256Sum(b["token"]))} {
		if secret != "" && strings.Contains(logs, secret) {
			t.Errorf("%s aparece en el log", name)
		}
	}
	if strings.Contains(bad.Body.String(), b["token"]) {
		t.Error("token en cuerpo de error")
	}
	if logs == "" {
		t.Fatal("se esperaban registros")
	}
}

func sha256Sum(s string) []byte { h := sha256.Sum256([]byte(s)); return h[:] }
