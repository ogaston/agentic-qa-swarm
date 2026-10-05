package obs

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDirWritable(t *testing.T) {
	dir := t.TempDir()
	if err := DirWritable(dir); err != nil {
		t.Fatalf("directorio escribible: %v", err)
	}
	if es, _ := os.ReadDir(dir); len(es) != 0 {
		t.Errorf("el chequeo debe borrar su archivo temporal: %v", es)
	}
	if DirWritable(filepath.Join(dir, "no-existe")) == nil {
		t.Error("un directorio inexistente debe fallar")
	}
	f := filepath.Join(dir, "archivo")
	_ = os.WriteFile(f, nil, 0o600)
	if DirWritable(f) == nil {
		t.Error("un archivo en lugar de directorio debe fallar")
	}
}

func TestReadableEventsFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "e.jsonl")
	if err := ReadableEventsFile(f); err != nil {
		t.Fatalf("aún sin eventos (archivo inexistente) es normal: %v", err)
	}
	_ = os.WriteFile(f, []byte("x\n"), 0o600)
	if err := ReadableEventsFile(f); err != nil {
		t.Fatalf("archivo legible: %v", err)
	}
	if ReadableEventsFile(dir) == nil {
		t.Error("un directorio como archivo de eventos debe fallar (también como root)")
	}
	if ReadableEventsFile(filepath.Join(dir, "no-existe", "e.jsonl")) == nil {
		t.Error("archivo de eventos en directorio inexistente debe fallar")
	}
	if ReadableEventsFile(filepath.Join(f, "e.jsonl")) == nil {
		t.Error("cuyo directorio es un archivo debe fallar")
	}
}

func readyz(t *testing.T, dataDir, events string, verifierSet func() bool) (int, map[string]any) {
	t.Helper()
	reg := NewRegistry()
	h := Wrap(Config{Service: "ui-api", Log: NewLogger(&bytes.Buffer{}, "ui-api", slog.LevelDebug), Registry: reg,
		Metrics: NewHTTPMetrics(reg, "ui-api"), Ready: ReadyChecks(dataDir, events, verifierSet)},
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("cuerpo: %v", err)
	}
	return rec.Code, body
}

// Cada chequeo de /readyz tiene su caso negativo: falla solo él y el resto sigue en ok.
func TestReadyChecksEachOneCanFail(t *testing.T) {
	good := t.TempDir()
	events := filepath.Join(good, "e.jsonl")
	yes := func() bool { return true }
	if code, body := readyz(t, good, events, yes); code != 200 || body["status"] != "ready" {
		t.Fatalf("todo en orden: %d %v", code, body)
	}
	missing := filepath.Join(good, "no-existe")
	cases := []struct {
		name, dataDir, events string
		verifier              func() bool
		failing               string
	}{
		{"directorio de datos inexistente", missing, events, yes, "data_dir"},
		{"archivo de eventos es un directorio", good, good, yes, "events_file"},
		{"archivo de eventos en directorio inexistente", good, filepath.Join(missing, "e.jsonl"), yes, "events_file"},
		{"verificador no configurado", good, events, func() bool { return false }, "token_verifier"},
		{"sin función de verificador", good, events, nil, "token_verifier"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, body := readyz(t, c.dataDir, c.events, c.verifier)
			checks, _ := body["checks"].(map[string]any)
			if code != 503 || body["status"] != "unavailable" {
				t.Fatalf("esperado 503 unavailable: %d %v", code, body)
			}
			for _, n := range []string{"data_dir", "events_file", "token_verifier"} {
				want := "ok"
				if n == c.failing {
					want = "fail"
				}
				if checks[n] != want {
					t.Errorf("chequeo %s = %v, esperado %s", n, checks[n], want)
				}
			}
		})
	}
}
