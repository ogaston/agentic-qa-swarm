package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/ui-api/inbox"
	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/auth"
)

const (
	confTokA = "tok-ua-789"
	confTokB = "tok-ub-790"
	confTokX = "tok-admin-791"
)

func confEnv(t *testing.T) *env {
	t.Helper()
	v, err := auth.NewFakeTokenVerifier(confTokA + "=ua:user," + confTokB + "=ub:user," + confTokX + "=adm:admin")
	if err != nil {
		t.Fatal(err)
	}
	return newEnv(t, v, nil)
}

// confirm crea una notificación y la confirma por principal (el orden de llamada es el orden de llegada).
func confirm(t *testing.T, e *env, id, principal string) {
	t.Helper()
	ev := inbox.NotifyCreated{EventID: "ev-" + id, Type: "notify.created", Version: 1, OccurredAt: time.Now(), TraceID: "t"}
	ev.Data.NotificationID, ev.Data.GithubEvent, ev.Data.Repo, ev.Data.SHA = id, "commit", "acme/shop", sha
	ev.Data.Artifact = &inbox.Artifact{Kind: "build-from-repo", Ref: "acme/shop@" + sha}
	e.store.Apply(ev)
	if _, err := e.store.ConfirmTraced(id, principal, []string{"smoke"}, "trace"); err != nil {
		t.Fatal(err)
	}
}

func decodeReceipts(t *testing.T, body []byte) []string {
	t.Helper()
	var rs []struct {
		NotificationID string `json:"notification_id"`
		ConfirmedBy    string `json:"confirmed_by"`
	}
	if err := json.Unmarshal(body, &rs); err != nil {
		t.Fatalf("cuerpo no es []ConfirmationReceipt: %s (%v)", body, err)
	}
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.NotificationID+"|"+r.ConfirmedBy)
	}
	return out
}

func TestConfirmationsUserVeSoloLosSuyos(t *testing.T) {
	e := confEnv(t)
	confirm(t, e, "n-a1", "ua")
	confirm(t, e, "n-b1", "ub")
	w := e.do(http.MethodGet, "/confirmations", confTokA, "")
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	got := decodeReceipts(t, w.Body.Bytes())
	if len(got) != 1 || got[0] != "n-a1|ua" {
		t.Fatalf("user ua vio %v", got)
	}
}

func TestConfirmationsAdminVeTodos(t *testing.T) {
	e := confEnv(t)
	confirm(t, e, "n-a1", "ua")
	confirm(t, e, "n-b1", "ub")
	got := decodeReceipts(t, e.do(http.MethodGet, "/confirmations", confTokX, "").Body.Bytes())
	if len(got) != 2 {
		t.Fatalf("admin vio %v, quiero 2", got)
	}
}

func TestConfirmationsMasRecientePrimero(t *testing.T) {
	e := confEnv(t)
	for i := 1; i <= 3; i++ {
		confirm(t, e, fmt.Sprintf("n-%d", i), "adm")
	}
	got := decodeReceipts(t, e.do(http.MethodGet, "/confirmations", confTokX, "").Body.Bytes())
	want := []string{"n-3|adm", "n-2|adm", "n-1|adm"}
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Fatalf("orden = %v, quiero %v", got, want)
		}
	}
}

func TestConfirmationsLimitPorDefecto20(t *testing.T) {
	e := confEnv(t)
	for i := 1; i <= 25; i++ {
		confirm(t, e, fmt.Sprintf("n-%02d", i), "adm")
	}
	got := decodeReceipts(t, e.do(http.MethodGet, "/confirmations", confTokX, "").Body.Bytes())
	if len(got) != 20 {
		t.Fatalf("por defecto %d recibos, quiero 20", len(got))
	}
	if got := decodeReceipts(t, e.do(http.MethodGet, "/confirmations?limit=100", confTokX, "").Body.Bytes()); len(got) != 25 {
		t.Fatalf("limit=100: %d", len(got))
	}
}

func TestConfirmationsLimitFueraDeRangoEs400(t *testing.T) {
	e := confEnv(t)
	for _, q := range []string{"limit=0", "limit=101", "limit=abc", "limit=", "limit=-1", "limit=1&limit=2"} {
		if w := e.do(http.MethodGet, "/confirmations?"+q, confTokX, ""); w.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d", q, w.Code)
		}
	}
}

func TestConfirmationsXRoleNoCambiaElResultado(t *testing.T) {
	e := confEnv(t)
	confirm(t, e, "n-a1", "ua")
	confirm(t, e, "n-b1", "ub")
	got := decodeReceipts(t, e.do(http.MethodGet, "/confirmations", confTokA, "", "X-Role", "admin").Body.Bytes())
	if len(got) != 1 || got[0] != "n-a1|ua" {
		t.Fatalf("user con X-Role admin vio %v", got)
	}
}

func TestConfirmationsSinTokenDa401(t *testing.T) {
	e := confEnv(t)
	if w := e.do(http.MethodGet, "/confirmations", "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("sin token: %d", w.Code)
	}
}

// Reabrir el store sobre el mismo directorio (reinicio del servicio) debe conservar /confirmations.
func TestConfirmationsTrasReinicioSigueListando(t *testing.T) {
	e := confEnv(t)
	confirm(t, e, "n-a1", "ua")
	confirm(t, e, "n-x2", "adm")

	st, err := inbox.OpenStore(e.dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := auth.NewFakeTokenVerifier(confTokA + "=ua:user," + confTokX + "=adm:admin")
	h, err := New(Config{Store: st, Verifier: v, AllowedOrigins: []string{"https://app.example"}, RateRPS: 1000, RateBurst: 1000})
	if err != nil {
		t.Fatal(err)
	}
	e2 := &env{h: h, store: st, dir: e.dir}
	if got := decodeReceipts(t, e2.do(http.MethodGet, "/confirmations", confTokX, "").Body.Bytes()); len(got) != 2 {
		t.Fatalf("admin tras reinicio vio %v, quiero 2", got)
	}
	if got := decodeReceipts(t, e2.do(http.MethodGet, "/confirmations", confTokA, "").Body.Bytes()); len(got) != 1 || got[0] != "n-a1|ua" {
		t.Fatalf("user tras reinicio vio %v", got)
	}
}
