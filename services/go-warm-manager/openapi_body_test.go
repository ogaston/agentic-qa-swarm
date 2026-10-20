package warmmanager_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	wm "github.com/ogaston/agentic-qa-swarm/services/go-warm-manager"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/internal/fakes"
)

// inferir corre InferSurface contra un OpenAPI servido por la app del warm.
func inferir(t *testing.T, doc string) wm.SurfaceArtifact {
	t.Helper()
	r := newRig(ws("ready", true))
	mustDeploy(t, r)
	r.web.Routes["/openapi.json"] = fakes.FakeResponse{Status: 200, Body: doc}
	sa, err := r.svc.InferSurface(context.Background(), "r-1", "t")
	if err != nil {
		t.Fatal(err)
	}
	return sa
}

func hallar(sa wm.SurfaceArtifact, method, path string) (wm.Endpoint, bool) {
	for _, e := range sa.Endpoints {
		if e.Method == method && e.Path == path {
			return e, true
		}
	}
	return wm.Endpoint{}, false
}

// CA-2: la inferencia conserva el requestBody application/json del OpenAPI de demo/target-app.
func TestOpenAPIRequestBodyDeTargetApp(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "demo", "target-app", "openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	sa := inferir(t, string(raw))
	post, ok := hallar(sa, "POST", "/orders")
	if !ok {
		t.Fatalf("falta POST /orders: %+v", sa.Endpoints)
	}
	var schema struct {
		Type     string   `json:"type"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(post.RequestBody, &schema); err != nil {
		t.Fatalf("POST /orders sin request_body usable: %v (%s)", err, post.RequestBody)
	}
	if schema.Type != "object" || strings.Join(schema.Required, ",") != "product_id,qty" {
		t.Fatalf("request_body de POST /orders incorrecto: %s", post.RequestBody)
	}
	if get, _ := hallar(sa, "GET", "/orders"); len(get.RequestBody) != 0 {
		t.Fatalf("GET /orders no debe llevar request_body: %s", get.RequestBody)
	}
}

// CA-2: sin requestBody o con $ref externo la inferencia no falla; el endpoint sale sin cuerpo.
func TestOpenAPIRequestBodyAusenteOExternoNoRompe(t *testing.T) {
	doc := `{"openapi":"3.0.0","paths":{"/a":{"post":{"requestBody":{"content":{"application/json":{"schema":{"$ref":"https://otro.example/x.json"}}}}}},"/b":{"put":{}}}}`
	sa := inferir(t, doc)
	if len(sa.Endpoints) != 2 {
		t.Fatalf("endpoints=%+v", sa.Endpoints)
	}
	for _, e := range sa.Endpoints {
		if len(e.RequestBody) != 0 {
			t.Fatalf("%s %s no debe llevar cuerpo: %s", e.Method, e.Path, e.RequestBody)
		}
	}
}

// U8-T09a F-03: $ref en la RAIZ del esquema del requestBody -> el endpoint sale sin cuerpo.
// $ref ANIDADO -> el esquema se conserva tal cual (no se resuelve); el planner exige body sin comprobarlo.
func TestOpenAPIRequestBodyRefRaizVersusAnidado(t *testing.T) {
	raiz := `{"openapi":"3.0.0","paths":{"/a":{"post":{"requestBody":{"content":{"application/json":{"schema":{"$ref":"#/components/schemas/X"}}}}}}}}`
	anidado := `{"openapi":"3.0.0","paths":{"/a":{"post":{"requestBody":{"content":{"application/json":{"schema":{"type":"object","required":["id"],"properties":{"id":{"$ref":"#/components/schemas/Id"}}}}}}}}}}`
	sa := inferir(t, raiz)
	if e, _ := hallar(sa, "POST", "/a"); len(e.RequestBody) != 0 {
		t.Fatalf("$ref en la raiz debe salir sin cuerpo: %s", e.RequestBody)
	}
	sa = inferir(t, anidado)
	e, ok := hallar(sa, "POST", "/a")
	if !ok || !strings.Contains(string(e.RequestBody), `"$ref":"#/components/schemas/Id"`) {
		t.Fatalf("$ref anidado debe conservarse sin resolver: %s", e.RequestBody)
	}
}
