package inbox

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Un evento descartado del outbox se registra sin su contenido: ninguna de las líneas
// «evento descartado» (ni las demás del suscriptor) puede incluir nombres, correos ni claves del evento.
func TestFileSubscriberDiscardLogLeaksNoEventContent(t *testing.T) {
	good := readFile(t, filepath.Join(events, "examples/valid/notify.created.json"))
	mut := func(f func(m map[string]any)) string {
		var m map[string]any
		if err := json.Unmarshal(good, &m); err != nil {
			t.Fatal(err)
		}
		f(m)
		b, _ := json.Marshal(m)
		return string(b)
	}
	data := func(m map[string]any) map[string]any { return m["data"].(map[string]any) }
	marks := `"PII-CLAVE-XYZ":"pii-correo@falso.test"`
	lines := []string{
		`{"event_id":"e1","PII-CLAVE-XYZ":1,"PII-CLAVE-XYZ":2}`,                                           // clave duplicada
		mut(func(m map[string]any) { m["PII-CAMPO-XYZ"] = "pii-correo@falso.test" }),                      // clave de más
		mut(func(m map[string]any) { data(m)["PII-CAMPO-XYZ"] = "PII-NOMBRE-XYZ" }),                       // clave de más en data
		mut(func(m map[string]any) { data(m)["sha"] = "PII-SHA-XYZ pii-correo@falso.test" }),              // valor inválido
		mut(func(m map[string]any) { m["occurred_at"] = "PII-FECHA-XYZ pii-correo@falso.test" }),          // valor inválido
		mut(func(m map[string]any) { m["version"] = "PII-VERSION-XYZ" }),                                  // valor inválido
		mut(func(m map[string]any) { m["event_id"] = 12345; data(m)["github_event"] = "PII-EVENTO-XYZ" }), // tipo inválido
		`{"event_id":"PII-ID-XYZ" "type":"notify.created",` + marks + `}`,                                 // sintaxis rota
		`{"event_id":"e2",` + marks + ` PII-BASURA-XYZ`,                                                   // truncado con basura
		strings.TrimSpace(strings.ReplaceAll(mut(func(m map[string]any) {}), "}", "}")) + ` PII-COLA-XYZ`, // datos tras el objeto
		`{"PII-CLAVE-XYZ":` + strings.Repeat("a", MaxLineBytes) + `,` + marks + `}`,                       // línea demasiado larga
		`[` + marks + `]`,
		`"pii-correo@falso.test"`,
	}
	for i, l := range lines { // cada línea debe descartarse por sí sola
		q := filepath.Join(t.TempDir(), "e.jsonl")
		_ = os.WriteFile(q, []byte(l+"\n"), 0o640)
		one := &FileSubscriber{Path: q}
		if n, _ := one.Drain(func(NotifyCreated) {}); n != 0 || one.Discarded() != 1 {
			t.Errorf("la línea %d debía descartarse: %.80s", i, l)
		}
	}
	p := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	fs := &FileSubscriber{Path: p, Logger: log.New(&logs, "", 0)}
	n, err := fs.Drain(func(e NotifyCreated) { t.Errorf("ninguna línea debía entregarse: %+v", e.Data) })
	if err != nil || n != 0 || fs.Discarded() != len(lines) {
		t.Fatalf("n=%d err=%v descartadas=%d, esperadas %d", n, err, fs.Discarded(), len(lines))
	}
	out := strings.ToLower(logs.String())
	if got := strings.Count(out, "descartad"); got != len(lines) {
		t.Fatalf("se esperaba una línea de log por descarte (%d): %d\n%s", len(lines), got, logs.String())
	}
	for _, m := range []string{"pii-", "falso.test", "pii_"} {
		if strings.Contains(out, m) {
			t.Errorf("el log del suscriptor contiene %q:\n%s", m, logs.String())
		}
	}
}
