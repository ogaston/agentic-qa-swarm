package server

import (
	"strings"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-governance/authz"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/auth"
)

func TestProjectMetadataAndRotationEndpoints(t *testing.T) {
	f := newFix(t, auth.FakeTokenVerifier{Tokens: map[string]authz.Principal{
		"tok-user": {ID: "alice", Role: authz.RoleUser},
		"tok-bob":  {ID: "bob", Role: authz.RoleUser},
	}})
	code, body := f.do("POST", "/repositories", "tok-user", `{"repo":"acme/shop","name":"Tienda","resources":[]}`)
	if code != 201 || !strings.Contains(body, `"name":"Tienda"`) || !strings.Contains(body, `"resources":[]`) || !strings.Contains(body, `"webhook_secret_version":1`) {
		t.Fatalf("creación proyecto: %d %s", code, body)
	}
	if code, _ := f.do("POST", "/repositories", "tok-user", `{"repo":"acme/other","resources":["mysql"]}`); code != 422 {
		t.Fatalf("recurso no soportado: %d", code)
	}
	if code, _ := f.do("POST", "/repositories/acme/shop/webhook-secret/rotate", "tok-bob", ""); code != 403 {
		t.Fatalf("rotación no owner: %d", code)
	}
	code, body = f.do("POST", "/repositories/acme/shop/webhook-secret/rotate", "tok-user", "")
	if code != 200 || !strings.Contains(body, `"webhook_secret_version":2`) || strings.Contains(body, `"webhook_secret":`) {
		t.Fatalf("rotación owner: %d %s", code, body)
	}
}
