package totp_test

import (
	"encoding/base32"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/gen"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/totp"
)

func TestPBT_RoundTrip_Base32Secret(t *testing.T) {
	gen.AtLeastChecks(t, 500)
	rapid.Check(t, func(t *rapid.T) {
		b := gen.Secret().Draw(t, "secret")
		s := gen.Base32(t, b)
		got, err := totp.ParseSecret(s)
		if err != nil || string(got) != string(b) {
			t.Fatalf("ParseSecret(%q)=%x, %v; esperado %x", s, got, err, b)
		}
		short := gen.ShortSecret().Draw(t, "short")
		if _, err := totp.ParseSecret(base32.StdEncoding.EncodeToString(short)); err == nil {
			t.Fatalf("secreto de %d bytes aceptado", len(short))
		}
	})
}

func TestPBT_TOTP(t *testing.T) {
	gen.AtLeastChecks(t, 500)
	rapid.Check(t, func(t *rapid.T) {
		key := gen.Secret().Draw(t, "secret")
		at := gen.Instant().Draw(t, "t")
		code := totp.Code(key, at)
		if len(code) != totp.Digits {
			t.Fatalf("código de %d dígitos", len(code))
		}
		// aceptado en t y en t±30 s
		for _, d := range []time.Duration{0, -30 * time.Second, 30 * time.Second} {
			if _, ok := totp.Verify(key, code, at.Add(d), 0); !ok {
				t.Fatalf("código rechazado con desplazamiento %v", d)
			}
		}
		// rechazado en t±90 s (salvo colisión fortuita de 6 dígitos con un contador de la ventana: se descarta el sorteo)
		for _, d := range []time.Duration{-90 * time.Second, 90 * time.Second} {
			now := at.Add(d)
			cur := totp.Counter(now)
			for _, c := range []uint64{cur - 1, cur, cur + 1} {
				if totp.HOTP(key, c, totp.Digits) == code {
					t.Skip()
				}
			}
			if _, ok := totp.Verify(key, code, now, 0); ok {
				t.Fatalf("código aceptado con desplazamiento %v", d)
			}
		}
		// no se acepta dos veces
		acc, ok := totp.Verify(key, code, at, 0)
		if !ok {
			t.Fatal("rechazado")
		}
		for _, d := range []time.Duration{0, -30 * time.Second, 30 * time.Second} {
			if _, ok := totp.Verify(key, code, at.Add(d), acc); ok {
				t.Fatalf("reutilización aceptada con desplazamiento %v", d)
			}
		}
		// formato: solo 6 caracteres
		if _, ok := totp.Verify(key, code+"0", at, 0); ok {
			t.Fatal("código largo aceptado")
		}
	})
}

// Ejemplo fijo: vector de RFC 6238 (SHA-1, T=59 s -> 94287082; 6 dígitos 287082).
func TestPBT_Examples_TOTP(t *testing.T) {
	key := []byte("12345678901234567890")
	if got := totp.Code(key, time.Unix(59, 0)); got != "287082" {
		t.Errorf("vector RFC 6238: %s", got)
	}
	if _, ok := totp.Verify(key, "287082", time.Unix(59+90, 0), 0); ok {
		t.Error("t+90 s aceptado")
	}
	acc, ok := totp.Verify(key, "287082", time.Unix(59, 0), 0)
	if !ok {
		t.Fatal("t rechazado")
	}
	if _, ok := totp.Verify(key, "287082", time.Unix(59, 0), acc); ok {
		t.Error("reutilización aceptada")
	}
}
