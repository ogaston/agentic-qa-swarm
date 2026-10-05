package intake

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/githubsig"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const secret = "s3cret"

type memPublisher struct {
	events []Event
	fail   bool
}

func (m *memPublisher) Publish(_ context.Context, ev Event) error {
	if m.fail {
		return errors.New("transporte caido")
	}
	m.events = append(m.events, ev)
	return nil
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "github", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type env struct {
	srv   http.Handler
	store *JSONLStore
	pub   *memPublisher
	dir   string
}

func newEnv(t *testing.T, v githubsig.Verifier) *env {
	t.Helper()
	dir := t.TempDir()
	st, err := OpenJSONLStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{store: st, pub: &memPublisher{}, dir: dir}
	e.srv = e.handler(t, st, v)
	return e
}

func (e *env) handler(t *testing.T, st NotificationStore, v githubsig.Verifier) http.Handler {
	t.Helper()
	if v == nil {
		v = githubsig.HMACVerifier{}
	}
	h, err := NewHandler(Deps{
		Secret: []byte(secret), Verifier: v, Store: st, Publisher: e.pub, Resolver: StubResolver{},
		Now: func() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

type req struct {
	event, delivery, ctype, sig, trace string
	body                               []byte
	noSig                              bool
}

func signed(event, delivery string, body []byte) req {
	return req{event: event, delivery: delivery, ctype: "application/json", sig: githubsig.Sign([]byte(secret), body), body: body}
}

func do(h http.Handler, r req) *httptest.ResponseRecorder {
	hr := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(r.body))
	if r.ctype != "" {
		hr.Header.Set("Content-Type", r.ctype)
	}
	if r.event != "" {
		hr.Header.Set("X-GitHub-Event", r.event)
	}
	if r.delivery != "" {
		hr.Header.Set("X-GitHub-Delivery", r.delivery)
	}
	if !r.noSig {
		hr.Header.Set("X-Hub-Signature-256", r.sig)
	}
	if r.trace != "" {
		hr.Header.Set("traceparent", r.trace)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, hr)
	return w
}

func errCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var e struct{ Code, Message string }
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatalf("cuerpo de error no es JSON: %q", w.Body.String())
	}
	return e.Code
}

func TestWebhook(t *testing.T) {
	pr := fixture(t, "pull-request-opened.json")
	big := append([]byte(`{"x":"`), bytes.Repeat([]byte("a"), MaxBodyBytes)...)
	big = append(big, []byte(`"}`)...)
	broken := []byte(`{"action":`)

	cases := []struct {
		name   string
		mutate func(r *req)
		req    req
		status int
		code   string
	}{
		{name: "firma valida", req: signed("pull_request", "d1", pr), status: 202},
		{name: "firma invalida", req: func() req {
			r := signed("pull_request", "d1", pr)
			r.sig = "sha256=" + strings.Repeat("0", 64)
			return r
		}(), status: 401, code: "invalid_signature"},
		{name: "sin firma", req: func() req { r := signed("pull_request", "d1", pr); r.noSig = true; return r }(), status: 401, code: "invalid_signature"},
		{name: "firma valida sobre cuerpo alterado", req: func() req {
			r := signed("pull_request", "d1", pr)
			r.body = bytes.Replace(pr, []byte("opened"), []byte("openeD"), 1)
			return r
		}(), status: 401, code: "invalid_signature"},
		{name: "firma sin prefijo sha256", req: func() req {
			r := signed("pull_request", "d1", pr)
			r.sig = strings.TrimPrefix(r.sig, "sha256=")
			return r
		}(), status: 401, code: "invalid_signature"},
		{name: "JSON roto", req: signed("pull_request", "d1", broken), status: 400, code: "invalid_json"},
		{name: "evento desconocido", req: signed("issues", "d1", pr), status: 400, code: "unsupported_event"},
		{name: "accion no soportada", req: signed("pull_request", "d1", []byte(`{"action":"closed","repository":{"full_name":"a/b"}}`)), status: 400, code: "unsupported_event"},
		{name: "cuerpo mayor a 1 MiB", req: signed("push", "d1", big), status: 413, code: "payload_too_large"},
		{name: "Content-Type incorrecto", req: func() req { r := signed("pull_request", "d1", pr); r.ctype = "text/plain"; return r }(), status: 415, code: "unsupported_media_type"},
		{name: "sin X-GitHub-Delivery", req: signed("pull_request", "", pr), status: 400, code: "missing_delivery"},
		{name: "payload sin sha", req: signed("push", "d1", []byte(`{"ref":"refs/heads/main","repository":{"full_name":"a/b"}}`)), status: 400, code: "invalid_payload"},
		{name: "release con target_commitish no SHA", req: signed("release", "d1", fixture(t, "release-published.json")), status: 400, code: "invalid_payload"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t, nil)
			w := do(e.srv, c.req)
			if w.Code != c.status {
				t.Fatalf("status=%d, quiero %d (%s)", w.Code, c.status, w.Body.String())
			}
			if c.code != "" && errCode(t, w) != c.code {
				t.Fatalf("code=%q, quiero %q", errCode(t, w), c.code)
			}
			if c.status != 202 {
				if _, ok := e.store.GetByDelivery("d1"); ok || len(e.pub.events) != 0 {
					t.Fatal("un rechazo no debe persistir ni publicar")
				}
			}
		})
	}
}

func TestWebhookPing(t *testing.T) {
	ping := fixture(t, "ping.json")
	t.Run("firma valida responde pong sin persistir ni publicar", func(t *testing.T) {
		e := newEnv(t, nil)
		w := do(e.srv, signed("ping", "d-ping", ping))
		if w.Code != 200 {
			t.Fatalf("status=%d, quiero 200 (%s)", w.Code, w.Body.String())
		}
		var b map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil || len(b) != 1 || b["status"] != "pong" {
			t.Fatalf("cuerpo inesperado: %q", w.Body.String())
		}
		if _, ok := e.store.GetByDelivery("d-ping"); ok || len(e.pub.events) != 0 {
			t.Fatal("ping no debe persistir ni publicar")
		}
		if _, err := os.Stat(filepath.Join(e.dir, "notifications.jsonl")); !os.IsNotExist(err) {
			t.Fatalf("ping no debe tocar el almacen: %v", err)
		}
	})
	t.Run("firma invalida sigue en 401", func(t *testing.T) {
		e := newEnv(t, nil)
		r := signed("ping", "d-ping", ping)
		r.sig = "sha256=" + strings.Repeat("0", 64)
		if w := do(e.srv, r); w.Code != 401 || errCode(t, w) != "invalid_signature" {
			t.Fatalf("status=%d %s", w.Code, w.Body.String())
		}
	})
}

func TestWebhookClassification(t *testing.T) {
	cases := []struct{ event, file, want, sha string }{
		{"push", "push-branch.json", EventCommit, strings.Repeat("a", 40)},
		{"push", "push-tag.json", EventTag, strings.Repeat("a", 40)},
		{"pull_request", "pull-request-opened.json", EventPullRequest, strings.Repeat("a", 40)},
		{"pull_request", "pull-request-synchronize.json", EventPullRequest, strings.Repeat("c", 40)},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			e := newEnv(t, nil)
			w := do(e.srv, signed(c.event, "d-"+c.file, fixture(t, c.file)))
			if w.Code != 202 {
				t.Fatalf("status=%d %s", w.Code, w.Body.String())
			}
			var n Notification
			if err := json.Unmarshal(w.Body.Bytes(), &n); err != nil {
				t.Fatal(err)
			}
			if n.GithubEvent != c.want || n.SHA != c.sha || n.Repo != "acme/shop" || n.State != StatePending {
				t.Fatalf("notificacion inesperada: %+v", n)
			}
			if n.Artifact == nil || n.Artifact.Kind != "build-from-repo" || n.Artifact.Ref != "acme/shop@"+c.sha {
				t.Fatalf("artefacto inesperado: %+v", n.Artifact)
			}
			if strings.Contains(n.ID, "/") || !strings.HasPrefix(n.ID, "n-") {
				t.Fatalf("id invalido: %q", n.ID)
			}
		})
	}
	t.Run("release con SHA", func(t *testing.T) {
		sha := strings.Repeat("b", 40)
		body := []byte(`{"action":"published","repository":{"full_name":"a/b"},"release":{"target_commitish":"` + sha + `"}}`)
		c, err := Classify("release", body)
		if err != nil || c.GithubEvent != EventTag || c.SHA != sha {
			t.Fatalf("c=%+v err=%v", c, err)
		}
	})
	t.Run("push que borra la rama", func(t *testing.T) {
		body := []byte(`{"ref":"refs/heads/x","after":"` + zeroSHA + `","repository":{"full_name":"a/b"}}`)
		if _, err := Classify("push", body); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestWebhookFakeVerifier(t *testing.T) {
	pr := fixture(t, "pull-request-opened.json")
	r := req{event: "pull_request", delivery: "d1", ctype: "application/json", sig: "cualquiera", body: pr}
	if w := do(newEnv(t, githubsig.NewFakeVerifier().AcceptAll()).srv, r); w.Code != 202 {
		t.Fatalf("AcceptAll: %d", w.Code)
	}
	if w := do(newEnv(t, githubsig.NewFakeVerifier()).srv, r); w.Code != 401 {
		t.Fatalf("por defecto debe rechazar: %d", w.Code)
	}
}

func TestWebhookIdempotentDelivery(t *testing.T) {
	e := newEnv(t, nil)
	r := signed("pull_request", "d-same", fixture(t, "pull-request-opened.json"))
	w1, w2 := do(e.srv, r), do(e.srv, r)
	if w1.Code != 202 || w2.Code != 202 {
		t.Fatalf("status %d %d", w1.Code, w2.Code)
	}
	if w1.Body.String() != w2.Body.String() {
		t.Fatalf("distinta notificacion:\n%s\n%s", w1.Body.String(), w2.Body.String())
	}
	if len(e.pub.events) != 1 {
		t.Fatalf("eventos publicados=%d, quiero 1", len(e.pub.events))
	}
	// otra entrega distinta crea otra notificacion
	r2 := signed("pull_request", "d-other", fixture(t, "pull-request-opened.json"))
	if w3 := do(e.srv, r2); w3.Body.String() == w1.Body.String() {
		t.Fatal("una entrega distinta debe crear otra notificacion")
	}
}

func TestWebhookRestartKeepsDeliveryIndex(t *testing.T) {
	dir := t.TempDir()
	events := filepath.Join(dir, "events.jsonl")
	build := func() http.Handler {
		st, err := OpenJSONLStore(filepath.Join(dir, "data"))
		if err != nil {
			t.Fatal(err)
		}
		h, err := NewHandler(Deps{Secret: []byte(secret), Verifier: githubsig.HMACVerifier{}, Store: st, Publisher: NewOutbox(events), Resolver: StubResolver{}})
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	r := signed("push", "d-restart", fixture(t, "push-branch.json"))
	w1 := do(build(), r)
	w2 := do(build(), r) // "reinicio": nuevo store sobre el mismo directorio
	if w1.Code != 202 || w2.Code != 202 || w1.Body.String() != w2.Body.String() {
		t.Fatalf("w1=%d %s w2=%d %s", w1.Code, w1.Body.String(), w2.Code, w2.Body.String())
	}
	b, _ := os.ReadFile(events)
	if n := bytes.Count(b, []byte("\n")); n != 1 {
		t.Fatalf("lineas de eventos=%d, quiero 1", n)
	}
}

func TestStoreRestartIgnoresBrokenLine(t *testing.T) {
	dir := t.TempDir()
	st, _ := OpenJSONLStore(dir)
	rec := Record{Notification: Notification{ID: "n-1", GithubEvent: EventCommit, Repo: "a/b", SHA: strings.Repeat("a", 40), State: StatePending}, DeliveryID: "d1"}
	if err := st.Put(rec); err != nil {
		t.Fatal(err)
	}
	f, _ := os.OpenFile(filepath.Join(dir, "notifications.jsonl"), os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"id":"n-2","deliv`) // linea truncada a media escritura
	f.Close()

	st2, err := OpenJSONLStore(dir)
	if err != nil {
		t.Fatalf("la linea rota no debe impedir arrancar: %v", err)
	}
	if got, ok := st2.GetByDelivery("d1"); !ok || got.ID != "n-1" {
		t.Fatalf("no recupero n-1: %+v %v", got, ok)
	}
	rec2 := rec
	rec2.ID, rec2.DeliveryID = "n-3", "d3"
	if err := st2.Put(rec2); err != nil {
		t.Fatal(err)
	}
	st3, _ := OpenJSONLStore(dir)
	if got, ok := st3.GetByDelivery("d3"); !ok || got.ID != "n-3" {
		t.Fatalf("la linea nueva se perdio pegada a la rota: %+v %v", got, ok)
	}
}

func TestWebhookPublishFailureThenRetry(t *testing.T) {
	e := newEnv(t, nil)
	r := signed("push", "d-fail", fixture(t, "push-branch.json"))
	e.pub.fail = true
	w1 := do(e.srv, r)
	if w1.Code != 503 {
		t.Fatalf("status=%d", w1.Code)
	}
	rec, ok := e.store.GetByDelivery("d-fail")
	if !ok || !rec.PublishPending {
		t.Fatalf("debe quedar publish_pending: %+v %v", rec, ok)
	}
	e.pub.fail = false
	w2 := do(e.srv, r)
	if w2.Code != 202 || len(e.pub.events) != 1 {
		t.Fatalf("status=%d eventos=%d", w2.Code, len(e.pub.events))
	}
	var n Notification
	_ = json.Unmarshal(w2.Body.Bytes(), &n)
	if n.ID != rec.ID {
		t.Fatalf("el reintento debe conservar la notificacion: %s vs %s", n.ID, rec.ID)
	}
	if rec2, _ := e.store.GetByDelivery("d-fail"); rec2.PublishPending {
		t.Fatal("publish_pending debe limpiarse")
	}
	if w3 := do(e.srv, r); w3.Code != 202 || len(e.pub.events) != 1 {
		t.Fatalf("no debe volver a publicar: %d %d", w3.Code, len(e.pub.events))
	}
}

func TestWebhookEventValidatesAgainstSchema(t *testing.T) {
	e := newEnv(t, nil)
	r := signed("pull_request", "d-schema", fixture(t, "pull-request-opened.json"))
	r.trace = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	if w := do(e.srv, r); w.Code != 202 {
		t.Fatalf("status=%d", w.Code)
	}
	if len(e.pub.events) != 1 {
		t.Fatal("falta el evento")
	}
	raw, _ := json.Marshal(e.pub.events[0])
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	sb, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "contracts", "events", "notify.created.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var sdoc any
	_ = json.Unmarshal(sb, &sdoc)
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.AssertFormat()
	if err := c.AddResource("mem:///notify.created.json", sdoc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("mem:///notify.created.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(doc); err != nil {
		t.Fatalf("el evento no valida: %v\n%s", err, raw)
	}
	if e.pub.events[0].TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("trace_id=%s", e.pub.events[0].TraceID)
	}
	// sin traceparent se genera uno de 32 hex
	r2 := signed("pull_request", "d-schema2", fixture(t, "pull-request-opened.json"))
	do(e.srv, r2)
	if tid := e.pub.events[1].TraceID; len(tid) != 32 {
		t.Fatalf("trace_id generado=%q", tid)
	}
}

func TestNewHandlerRequiresSecret(t *testing.T) {
	_, err := NewHandler(Deps{Verifier: githubsig.HMACVerifier{}, Store: &JSONLStore{}, Publisher: &memPublisher{}, Resolver: StubResolver{}})
	if err == nil {
		t.Fatal("un secreto vacio debe fallar")
	}
}
