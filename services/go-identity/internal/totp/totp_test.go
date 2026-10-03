package totp

import (
	"strings"
	"testing"
	"time"
)

// Secreto de los vectores de RFC 6238 (apéndice B), construido sin literal de credencial.
func rfcKey() []byte { return []byte(strings.Repeat("1234567890", 2)) }

func TestRFC6238Vectors(t *testing.T) {
	for _, v := range []struct {
		unix int64
		want string
	}{
		{59, "94287082"}, {1111111109, "07081804"}, {1111111111, "14050471"},
		{1234567890, "89005924"}, {2000000000, "69279037"}, {20000000000, "65353130"},
	} {
		got := HOTP(rfcKey(), Counter(time.Unix(v.unix, 0)), 8)
		if got != v.want {
			t.Errorf("t=%d: %s != %s", v.unix, got, v.want)
		}
	}
}

func TestCodeHasSixDigits(t *testing.T) {
	c := Code(rfcKey(), time.Unix(59, 0))
	if c != "287082" {
		t.Fatalf("los 6 dígitos del vector t=59 son 287082, no %s", c)
	}
}

func TestVerifyWindow(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	k := rfcKey()
	for name, off := range map[string]time.Duration{"-1": -30 * time.Second, "0": 0, "+1": 30 * time.Second} {
		if _, ok := Verify(k, Code(k, now.Add(off)), now, 0); !ok {
			t.Errorf("ventana %s debía aceptarse", name)
		}
	}
	for name, off := range map[string]time.Duration{"-2": -60 * time.Second, "+2": 60 * time.Second} {
		if _, ok := Verify(k, Code(k, now.Add(off)), now, 0); ok {
			t.Errorf("ventana %s debía rechazarse", name)
		}
	}
}

func TestVerifyRejectsReuse(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	k := rfcKey()
	code := Code(k, now)
	c, ok := Verify(k, code, now, 0)
	if !ok {
		t.Fatal("primer uso debía aceptarse")
	}
	if _, ok := Verify(k, code, now, c); ok {
		t.Fatal("reutilización debía rechazarse")
	}
}

func TestVerifyBadFormat(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	for _, c := range []string{"", "12345", "1234567", "abcdef"} {
		if _, ok := Verify(rfcKey(), c, now, 0); ok {
			t.Errorf("%q debía rechazarse", c)
		}
	}
}

func TestParseSecret(t *testing.T) {
	// 20 bytes de 'A' en base32 = 32 caracteres 'I' (0x41 = 01000001...), usamos un valor válido cualquiera.
	good := strings.Repeat("GEZDGNBVGY3TQOJQ", 2) // 20 bytes
	if _, err := ParseSecret(good); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSecret(strings.ToLower(good) + "===="); err != nil {
		t.Fatalf("minúsculas y relleno deben tolerarse: %v", err)
	}
	if _, err := ParseSecret("GEZDGNBVGY3TQOJQ"); err == nil {
		t.Fatal("80 bits debía rechazarse")
	}
	if _, err := ParseSecret("no es base32 !!"); err == nil {
		t.Fatal("base32 inválido debía rechazarse")
	}
}
