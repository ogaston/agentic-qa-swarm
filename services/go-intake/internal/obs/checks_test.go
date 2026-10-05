package obs

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestAppendableFile(t *testing.T) {
	dir := t.TempDir()
	reg := filepath.Join(dir, "e.jsonl")
	if err := AppendableFile(reg); err != nil {
		t.Fatalf("archivo aún inexistente en directorio escribible: %v", err)
	}
	if _, err := os.Stat(reg); err == nil {
		t.Error("el chequeo no debe crear el outbox")
	}
	_ = os.WriteFile(reg, []byte("x\n"), 0o600)
	if err := AppendableFile(reg); err != nil {
		t.Fatalf("archivo existente: %v", err)
	}
	if b, _ := os.ReadFile(reg); string(b) != "x\n" {
		t.Errorf("el chequeo no debe escribir en el outbox: %q", b)
	}
	if AppendableFile(dir) == nil {
		t.Error("un directorio como outbox debe fallar (también como root)")
	}
	if AppendableFile(filepath.Join(dir, "no-existe", "e.jsonl")) == nil {
		t.Error("outbox en directorio inexistente debe fallar")
	}
}

// readyz arma Wrap con los chequeos reales de go-intake y devuelve (código, cuerpo).
func readyz(t *testing.T, dataDir, events string, secret []byte) (int, map[string]any) {
	t.Helper()
	reg := NewRegistry()
	h := Wrap(Config{Service: "go-intake", Log: NewLogger(&bytes.Buffer{}, "go-intake", slog.LevelDebug), Registry: reg,
		Metrics: NewHTTPMetrics(reg, "go-intake"), Ready: ReadyChecks(dataDir, events, secret)},
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
	if code, body := readyz(t, good, events, []byte("s")); code != 200 || body["status"] != "ready" {
		t.Fatalf("todo en orden: %d %v", code, body)
	}
	missing := filepath.Join(good, "no-existe")
	cases := []struct {
		name, dataDir, events string
		secret                []byte
		failing               string
	}{
		{"directorio de datos inexistente", missing, events, []byte("s"), "data_dir"},
		{"outbox es un directorio", good, good, []byte("s"), "outbox"},
		{"outbox en directorio inexistente", good, filepath.Join(missing, "e.jsonl"), []byte("s"), "outbox"},
		{"secreto vacío", good, events, nil, "webhook_secret"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, body := readyz(t, c.dataDir, c.events, c.secret)
			checks, _ := body["checks"].(map[string]any)
			if code != 503 || body["status"] != "unavailable" {
				t.Fatalf("esperado 503 unavailable: %d %v", code, body)
			}
			for _, n := range []string{"data_dir", "outbox", "webhook_secret"} {
				want := "ok"
				if n == c.failing {
					want = "fail"
				}
				if checks[n] != want {
					t.Errorf("chequeo %s = %v, esperado %s", n, checks[n], want)
				}
			}
			if strings.Contains(string(mustJSON(body)), good) {
				t.Error("la respuesta no debe filtrar rutas")
			}
		})
	}
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
