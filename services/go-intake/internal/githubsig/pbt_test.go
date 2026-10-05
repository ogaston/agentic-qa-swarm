package githubsig_test

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/githubsig"
)

var sigRE = regexp.MustCompile(`^sha256=[0-9a-f]{64}$`)

func secrets() *rapid.Generator[[]byte] { return rapid.SliceOfN(rapid.Byte(), 1, 80) }
func bodies() *rapid.Generator[[]byte] {
	return rapid.OneOf(rapid.SliceOfN(rapid.Byte(), 0, 0), rapid.SliceOfN(rapid.Byte(), 1, 300), rapid.SliceOfN(rapid.Byte(), 4096, 4096))
}

// Para todo (secret, body): Verify(secret, body, Sign(secret, body)) es nil, la
// firma tiene el formato sha256=<64 hex>, y alterar un byte del cuerpo o del
// secreto, anexar un byte o quitar el prefijo la hace fallar.
func TestPBT_Signature(t *testing.T) {
	v := githubsig.HMACVerifier{}
	rapid.Check(t, func(t *rapid.T) {
		secret, body := secrets().Draw(t, "secret"), bodies().Draw(t, "body")
		sig := githubsig.Sign(secret, body)
		if !sigRE.MatchString(sig) {
			t.Fatalf("formato de firma %q", sig)
		}
		if err := v.Verify(secret, body, sig); err != nil {
			t.Fatalf("Verify(Sign) = %v", err)
		}
		if len(body) > 0 {
			i, x := rapid.IntRange(0, len(body)-1).Draw(t, "i"), rapid.ByteRange(1, 255).Draw(t, "x")
			bad := append([]byte(nil), body...)
			bad[i] ^= x
			if err := v.Verify(secret, bad, sig); !errors.Is(err, githubsig.ErrInvalidSignature) {
				t.Fatalf("cuerpo alterado en %d (^%#x) aceptado: %v", i, x, err)
			}
		}
		if err := v.Verify(secret, append(append([]byte(nil), body...), rapid.Byte().Draw(t, "extra")), sig); err == nil {
			t.Fatal("cuerpo con un byte anexado aceptado")
		}
		i, x := rapid.IntRange(0, len(secret)-1).Draw(t, "si"), rapid.ByteRange(1, 255).Draw(t, "sx")
		badSecret := append([]byte(nil), secret...)
		badSecret[i] ^= x
		if err := v.Verify(badSecret, body, sig); !errors.Is(err, githubsig.ErrInvalidSignature) {
			t.Fatalf("secreto alterado en %d (^%#x) aceptado: %v", i, x, err)
		}
		if err := v.Verify(secret, body, sig[len(githubsig.Prefix):]); err == nil {
			t.Fatal("firma sin prefijo aceptada")
		}
	})
}

// Firmas alteradas: para todo (secret, body), toda firma que difiera de la
// válida se rechaza: un carácter cambiado en CADA posición (prefijo e hex,
// incluida la última), truncada a cualquier prefijo propio (incl. 16 y 31 bytes de
// HMAC), extendida, con el prefijo o todo el texto en mayúsculas, con espacio o
// salto de línea, con otro algoritmo o con la firma de otro cuerpo.
func TestPBT_SignatureTampered(t *testing.T) {
	v := githubsig.HMACVerifier{}
	rapid.Check(t, func(t *rapid.T) {
		secret, body := secrets().Draw(t, "secret"), bodies().Draw(t, "body")
		sig := githubsig.Sign(secret, body)
		reject := func(what, h string) {
			t.Helper()
			if h == sig {
				return
			}
			if err := v.Verify(secret, body, h); !errors.Is(err, githubsig.ErrInvalidSignature) {
				t.Fatalf("%s aceptada (%q): %v", what, h, err)
			}
		}
		for i := 0; i < len(sig); i++ { // un carácter distinto en cada posición
			c := rapid.SampledFrom([]byte("0123456789abcdefxyz=-_ ")).Filter(func(b byte) bool { return b != sig[i] }).Draw(t, "c")
			reject("firma con carácter cambiado en "+string(rune('0'+i/10))+string(rune('0'+i%10)), sig[:i]+string(c)+sig[i+1:])
		}
		for n := 0; n < len(sig); n++ { // todo prefijo propio
			reject("firma truncada", sig[:n])
		}
		reject("firma truncada a 16 bytes de HMAC", sig[:len(githubsig.Prefix)+32])
		reject("firma truncada a 31 bytes de HMAC", sig[:len(githubsig.Prefix)+62])
		reject("firma truncada a 1 byte de HMAC", sig[:len(githubsig.Prefix)+2])
		for _, extra := range []string{"0", "00", "a", " ", "\n", "\x00", "=", sig[len(githubsig.Prefix):]} {
			reject("firma extendida", sig+extra)
		}
		reject("todo en mayúsculas", strings.ToUpper(sig))
		reject("prefijo en mayúsculas", "SHA256="+sig[len(githubsig.Prefix):])
		reject("prefijo mixto", "Sha256="+sig[len(githubsig.Prefix):])
		reject("otro algoritmo", "sha1="+sig[len(githubsig.Prefix):])
		reject("sin prefijo", sig[len(githubsig.Prefix):])
		reject("espacio delante", " "+sig)
		reject("prefijo duplicado", githubsig.Prefix+sig)
		other := githubsig.Sign(secret, append(append([]byte(nil), body...), 'x'))
		reject("firma de otro cuerpo", other)
	})
}

// Límite conocido: el hex de la firma se decodifica sin distinguir mayúsculas, así
// que sha256=<HEX en mayúsculas> se acepta (es el mismo HMAC; GitHub siempre envía
// minúsculas). Inocuo, pero la especificación de la firma lo rechazaría; se fija el
// comportamiento actual y es la prueba de regresión de la candidata: si se endurece
// Verify, hay que invertir esta prueba.
func TestPBT_Limit_SignatureHexUpperAccepted(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		secret, body := secrets().Draw(t, "secret"), bodies().Draw(t, "body")
		sig := githubsig.Sign(secret, body)
		up := githubsig.Prefix + strings.ToUpper(sig[len(githubsig.Prefix):])
		if err := (githubsig.HMACVerifier{}).Verify(secret, body, up); err != nil {
			t.Fatalf("Verify ya rechaza el hex en mayúsculas (%v): invertir esta prueba", err)
		}
	})
}

// Ejemplo fijo (canónico) de la propiedad de firma: vector de la documentación de GitHub.
func TestPBT_Fixed_Signature(t *testing.T) {
	secret, body := []byte("It's a Secret to Everybody"), []byte("Hello, World!")
	const want = "sha256=757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17"
	if got := githubsig.Sign(secret, body); got != want {
		t.Fatalf("Sign = %s, quiero %s", got, want)
	}
	if err := (githubsig.HMACVerifier{}).Verify(secret, body, want); err != nil {
		t.Fatal(err)
	}
	if err := (githubsig.HMACVerifier{}).Verify(secret, []byte("Hello, World?"), want); err == nil {
		t.Fatal("cuerpo alterado aceptado")
	}
	if err := (githubsig.HMACVerifier{}).Verify(nil, body, githubsig.Sign(nil, body)); err == nil {
		t.Fatal("secreto vacío aceptado")
	}
}

// Ejemplo fijo de TestPBT_SignatureTampered: el vector de GitHub con el último
// carácter cambiado, truncado a 16 bytes y en mayúsculas.
func TestPBT_Fixed_SignatureTampered(t *testing.T) {
	secret, body := []byte("It's a Secret to Everybody"), []byte("Hello, World!")
	const sig = "sha256=757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17"
	v := githubsig.HMACVerifier{}
	for _, h := range []string{sig[:len(sig)-1] + "6", sig[:len(sig)-1], sig[:7+32], sig[:7+62], strings.ToUpper(sig), sig + "0", sig[7:]} {
		if err := v.Verify(secret, body, h); err == nil {
			t.Fatalf("aceptada: %q", h)
		}
	}
}
