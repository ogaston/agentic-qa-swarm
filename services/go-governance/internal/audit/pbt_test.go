package audit_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/audit"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/gen"
)

// writeChain escribe las entradas con el log real (reloj inyectado) y devuelve
// los bytes del archivo y las entradas devueltas por Append.
func writeChain(t *rapid.T, dir string, in []audit.Entry) ([]byte, []audit.Entry) {
	path := filepath.Join(dir, "audit.jsonl")
	tick := time.Date(2026, 5, 1, 10, 0, 0, 123456789, time.UTC)
	l, err := audit.Open(path, func() time.Time { tick = tick.Add(time.Second + 7*time.Nanosecond); return tick })
	if err != nil {
		t.Fatal(err)
	}
	var written []audit.Entry
	for _, e := range in {
		w, err := l.Append(e)
		if err != nil {
			t.Fatal(err)
		}
		written = append(written, w)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b, written
}

func sameEntries(a, b []audit.Entry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if len(x.Detail) == 0 && len(y.Detail) == 0 {
			x.Detail, y.Detail = nil, nil
		}
		if !x.At.Equal(y.At) {
			return false
		}
		x.At, y.At = time.Time{}, time.Time{}
		if !reflect.DeepEqual(x, y) {
			return false
		}
	}
	return true
}

func chainGen() *rapid.Generator[[]audit.Entry] {
	return rapid.SliceOfN(gen.AuditEntry(), 1, 8)
}

func lines(b []byte) [][]byte {
	var out [][]byte
	for _, l := range bytes.SplitAfter(b, []byte("\n")) {
		if len(l) > 0 {
			out = append(out, l)
		}
	}
	return out
}

func TestPBT_RoundTrip_AuditChain(t *testing.T) {
	tmp := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		dir, err := os.MkdirTemp(tmp, "chain")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		data, written := writeChain(t, dir, chainGen().Draw(t, "entries"))
		got, err := audit.Verify(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("cadena íntegra rechazada: %v", err)
		}
		if !sameEntries(got, written) {
			t.Fatalf("la relectura difiere de lo escrito:\n%#v\n%#v", got, written)
		}
		ls := lines(data)

		switch rapid.IntRange(0, 3).Draw(t, "tamper") {
		case 0: // alterar un byte de cualquier línea (salvo su salto de línea final)
			i := rapid.IntRange(0, len(ls)-1).Draw(t, "line")
			j := rapid.IntRange(0, len(ls[i])-2).Draw(t, "offset") // el '\n' final se trata aparte (ver TestPBT_Limit_AuditTrailingBytes)
			nb := rapid.Byte().Filter(func(b byte) bool { return b != ls[i][j] }).Draw(t, "newbyte")
			mut := append([]byte{}, data...)
			off := 0
			for k := 0; k < i; k++ {
				off += len(ls[k])
			}
			mut[off+j] = nb
			// Se acepta una alteración solo si no cambia el contenido decodificado
			// (p. ej. mayúsculas en una clave JSON: encoding/json las empareja sin
			// distinguir mayúsculas). Cualquier otra debe romper la cadena.
			if es, err := audit.Verify(bytes.NewReader(mut)); err == nil && !sameEntries(es, written) {
				t.Fatalf("alteración no detectada (línea %d, byte %d -> %#x) y con contenido distinto", i+1, j, nb)
			}
		case 1: // borrar una línea que no es la última
			if len(ls) < 2 {
				t.Skip()
			}
			i := rapid.IntRange(0, len(ls)-2).Draw(t, "line")
			mustFail(t, "borrado", join(append(append([][]byte{}, ls[:i]...), ls[i+1:]...)))
		case 2: // duplicar una línea
			i := rapid.IntRange(0, len(ls)-1).Draw(t, "line")
			dup := append(append(append([][]byte{}, ls[:i+1]...), ls[i]), ls[i+1:]...)
			mustFail(t, "duplicado", join(dup))
		default: // permutar dos líneas
			if len(ls) < 2 {
				t.Skip()
			}
			i := rapid.IntRange(0, len(ls)-2).Draw(t, "i")
			j := rapid.IntRange(i+1, len(ls)-1).Draw(t, "j")
			p := append([][]byte{}, ls...)
			p[i], p[j] = p[j], p[i]
			mustFail(t, "permutación", join(p))
		}
	})
}

func join(ls [][]byte) []byte { return bytes.Join(ls, nil) }

func mustFail(t *rapid.T, what string, data []byte) {
	t.Helper()
	_, err := audit.Verify(bytes.NewReader(data))
	var ve *audit.VerifyError
	if !errors.As(err, &ve) {
		t.Fatalf("%s no detectado por Verify (err=%v)", what, err)
	}
}

// Regresión fija del contraejemplo hallado por rapid: borrar la cola del log (la
// última línea) no la detecta Verify, porque la cabeza de la cadena no está
// anclada fuera del archivo. Es un límite conocido del diseño (candidata), no se
// arregla en esta tarea; la prueba documenta el comportamiento aceptado.
func TestPBT_Limit_AuditTailTruncationUndetected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	l, err := audit.Open(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{"gate.allow", "gate.deny"} {
		if _, err := l.Append(audit.Entry{Actor: "a", Action: a}); err != nil {
			t.Fatal(err)
		}
	}
	l.Close()
	b, _ := os.ReadFile(path)
	ls := lines(b)
	es, err := audit.Verify(bytes.NewReader(join(ls[:1])))
	if err != nil || len(es) != 1 {
		t.Fatalf("el límite conocido cambió (¿ancla de cabeza añadida?): %v, %d entradas", err, len(es))
	}
	// Una clave JSON con otras mayúsculas decodifica igual: alteración sin efecto semántico (regresión fija).
	alt := bytes.Replace(ls[0], []byte(`"actor"`), []byte(`"Actor"`), 1)
	if es, err := audit.Verify(bytes.NewReader(join(append([][]byte{alt}, ls[1:]...)))); err != nil || len(es) != 2 {
		t.Fatalf("el límite de claves sin distinguir mayúsculas cambió: %v", err)
	}
}

// Regresión fija del segundo contraejemplo de rapid (reducido: 2 entradas,
// línea 0, offset = salto de línea, nuevo byte 0x00): Verify decodifica un solo
// valor JSON por línea e ignora lo que sigue, así que sustituir el '\n' por otro
// byte fusiona dos líneas y la segunda entrada desaparece sin error (y se pueden
// añadir bytes no autenticados tras el JSON). Defecto de producción registrado
// como candidata (comprobar que no queda contenido tras el valor, como hace
// policy.strict); no se arregla aquí porque esta tarea no toca producción.
func TestPBT_Limit_AuditTrailingBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	l, err := audit.Open(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{"gate.allow", "gate.deny"} {
		if _, err := l.Append(audit.Entry{Actor: "a", Action: a}); err != nil {
			t.Fatal(err)
		}
	}
	l.Close()
	b, _ := os.ReadFile(path)
	ls := lines(b)
	merged := append(append([]byte{}, ls[0][:len(ls[0])-1]...), 0)
	merged = append(merged, ls[1]...)
	es, err := audit.Verify(bytes.NewReader(merged))
	if err != nil || len(es) != 1 {
		t.Fatalf("el defecto registrado cambió (¿corregido?): err=%v, %d entradas", err, len(es))
	}
}
