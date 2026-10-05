package githubsig

import (
	"errors"
	"strings"
	"testing"
)

// Vector producido por: printf '{"a":1}' | openssl dgst -sha256 -hmac 's3cret'
const signVector = "sha256=5910e62016ef5034272c926c27071992a465c2335cecf41851bda071577f4f6d"

func TestSignVector(t *testing.T) {
	got := Sign([]byte("s3cret"), []byte(`{"a":1}`))
	if got != signVector {
		t.Fatalf("Sign = %s, want %s", got, signVector)
	}
}

func TestSignFormat(t *testing.T) {
	got := Sign([]byte("k"), []byte("x"))
	if !strings.HasPrefix(got, Prefix) || len(got) != len(Prefix)+64 {
		t.Fatalf("formato inesperado: %q", got)
	}
}

func TestFakeVerifierDefaultRejects(t *testing.T) {
	var zero FakeVerifier
	for name, v := range map[string]Verifier{"zero": &zero, "new": NewFakeVerifier()} {
		good := Sign([]byte("s"), []byte("b"))
		if err := v.Verify([]byte("s"), []byte("b"), good); !errors.Is(err, ErrInvalidSignature) {
			t.Fatalf("%s: debe rechazar por defecto, err=%v", name, err)
		}
	}
}

func TestFakeVerifierAcceptAll(t *testing.T) {
	v := NewFakeVerifier().AcceptAll()
	for _, h := range []string{"", "basura", "sha256=zz", Sign([]byte("s"), []byte("b"))} {
		if err := v.Verify(nil, nil, h); err != nil {
			t.Fatalf("AcceptAll rechazo %q: %v", h, err)
		}
	}
}

func TestFakeVerifierRejectAll(t *testing.T) {
	v := NewFakeVerifier().AcceptAll().RejectAll()
	if err := v.Verify(nil, nil, Sign([]byte("s"), []byte("b"))); err == nil {
		t.Fatal("RejectAll acepto")
	}
}

func TestFakeVerifierAcceptOnly(t *testing.T) {
	h := Sign([]byte("s"), []byte("b"))
	v := NewFakeVerifier().AcceptOnly(h)
	if err := v.Verify(nil, nil, h); err != nil {
		t.Fatalf("AcceptOnly rechazo el encabezado programado: %v", err)
	}
	for _, bad := range []string{"", "sha256=", h + "0", strings.TrimPrefix(h, Prefix), "sha256=abc", Sign([]byte("s"), []byte("c"))} {
		if err := v.Verify(nil, nil, bad); err == nil {
			t.Fatalf("AcceptOnly acepto %q", bad)
		}
	}
	// AcceptOnly("") no debe aceptar el encabezado vacio.
	if err := NewFakeVerifier().AcceptOnly("").Verify(nil, nil, ""); err == nil {
		t.Fatal("AcceptOnly vacio acepto encabezado vacio")
	}
}

func TestHMACVerifier(t *testing.T) {
	secret, body := []byte("s3cret"), []byte(`{"a":1}`)
	v := HMACVerifier{}
	if err := v.Verify(secret, body, signVector); err != nil {
		t.Fatalf("vector valido rechazado: %v", err)
	}
	altered := []byte(`{"a":2}`)
	cases := map[string]struct {
		secret, body []byte
		header       string
	}{
		"cuerpo alterado":  {secret, altered, signVector},
		"secreto distinto": {[]byte("otro"), body, signVector},
		"sin prefijo":      {secret, body, signVector[len(Prefix):]},
		"hex impar":        {secret, body, signVector[:len(signVector)-1]},
		"hex invalido":     {secret, body, Prefix + strings.Repeat("zz", 32)},
		"cabecera vacia":   {secret, body, ""},
		"secreto vacio":    {nil, body, signVector},
		"firma corta":      {secret, body, Prefix + "00"},
	}
	for name, c := range cases {
		if err := v.Verify(c.secret, c.body, c.header); !errors.Is(err, ErrInvalidSignature) {
			t.Errorf("%s: err=%v, quiero ErrInvalidSignature", name, err)
		}
	}
}
