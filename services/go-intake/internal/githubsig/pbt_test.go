package githubsig_test

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/gen"

	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/githubsig"
)

var sigRE = regexp.MustCompile(`^sha256=[0-9a-f]{64}$`)

func secrets() *rapid.Generator[[]byte] {
	return rapid.OneOf(
		rapid.SliceOfN(rapid.Byte(), 1, 80),
		rapid.Custom(func(t *rapid.T) []byte { // largos de frontera del bloque de HMAC (64 bytes)
			n := rapid.SampledFrom(gen.FixedSecretLengths[1:]).Draw(t, "len")
			return rapid.SliceOfN(rapid.Byte(), n, n).Draw(t, "secret")
		}),
	)
}

func bodies() *rapid.Generator[[]byte] {
	return rapid.OneOf(rapid.SliceOfN(rapid.Byte(), 0, 0), rapid.SliceOfN(rapid.Byte(), 1, 300), rapid.SliceOfN(rapid.Byte(), 4096, 4096))
}

// sameHMACKey indica si HMAC trata ambas claves como la misma (relleno con ceros hasta el bloque).
func sameHMACKey(a, b []byte) bool {
	pad := func(k []byte) []byte {
		if len(k) >= 64 {
			return k
		}
		return append(append([]byte(nil), k...), make([]byte, 64-len(k))...)
	}
	return string(pad(a)) == string(pad(b))
}

type fataler interface {
	Fatalf(string, ...any)
}

// checkSignature aplica a un (secret, body) todo lo que debe cumplirse: firma
// válida con formato sha256=<64 hex>; un byte alterado en el cuerpo (primero,
// segundo, mitad, último y límites de bloque), un byte anexado, el secreto con UN
// BYTE ALTERADO EN CADA POSICIÓN, el secreto (de 64 bytes o más) con un cero anexado, o truncado, y sin
// prefijo: todo se rechaza.
func checkSignature(t fataler, secret, body []byte) {
	v := githubsig.HMACVerifier{}
	sig := githubsig.Sign(secret, body)
	if !sigRE.MatchString(sig) {
		t.Fatalf("formato de firma %q", sig)
	}
	if err := v.Verify(secret, body, sig); err != nil {
		t.Fatalf("Verify(Sign) con secreto de %d bytes y cuerpo de %d: %v", len(secret), len(body), err)
	}
	for _, i := range []int{0, 1, len(body) / 2, 63, 64, 65, len(body) - 1} {
		if i < 0 || i >= len(body) {
			continue
		}
		bad := append([]byte(nil), body...)
		bad[i] ^= 0x01
		if err := v.Verify(secret, bad, sig); !errors.Is(err, githubsig.ErrInvalidSignature) {
			t.Fatalf("cuerpo alterado en %d aceptado (secreto de %d bytes): %v", i, len(secret), err)
		}
	}
	if err := v.Verify(secret, append(append([]byte(nil), body...), 0), sig); err == nil {
		t.Fatalf("cuerpo con un byte anexado aceptado")
	}
	for i := range secret {
		bad := append([]byte(nil), secret...)
		bad[i] ^= 0x80
		if err := v.Verify(bad, body, sig); !errors.Is(err, githubsig.ErrInvalidSignature) {
			t.Fatalf("secreto de %d bytes alterado en %d aceptado: %v", len(secret), i, err)
		}
	}
	// Claves distintas deben firmar distinto, salvo que HMAC las trate como la
	// misma: las menores que el bloque (64 bytes) se rellenan con ceros.
	for what, other := range map[string][]byte{
		"con un cero anexado": append(append([]byte(nil), secret...), 0),
		"sin el último byte":  secret[:len(secret)-1],
		"sin el primer byte":  secret[1:],
		"con un 1 anexado":    append(append([]byte(nil), secret...), 1),
	} {
		if sameHMACKey(secret, other) {
			continue
		}
		if err := v.Verify(other, body, sig); err == nil {
			t.Fatalf("secreto de %d bytes %s aceptado", len(secret), what)
		}
	}
	if err := v.Verify(secret, body, sig[len(githubsig.Prefix):]); err == nil {
		t.Fatalf("firma sin prefijo aceptada")
	}
	// Una sola vez por (secreto, cuerpo) la firma debe depender de AMBOS.
	if other := githubsig.Sign(secret, append(append([]byte(nil), body...), 'x')); other == sig {
		t.Fatalf("la firma no depende del cuerpo")
	}
}

// Para todo (secret, body): Verify(secret, body, Sign(secret, body)) es nil y
// todo lo de checkSignature. Antes del muestreo se recorren, siempre, los largos
// de secreto de frontera de gen.FixedSecretLengths (1, 31, 32, 33, 63, 64, 65, 128,
// 1000...: claves menores, iguales y mayores que el bloque de HMAC) por los
// cuerpos de gen.FixedBodies.
func TestPBT_Signature(t *testing.T) {
	for _, n := range gen.FixedSecretLengths[1:] {
		for _, body := range gen.FixedBodies() {
			checkSignature(t, gen.FixedSecret(n), body)
		}
	}
	if err := (githubsig.HMACVerifier{}).Verify(nil, []byte("x"), githubsig.Sign(nil, []byte("x"))); err == nil {
		t.Fatalf("secreto vacío aceptado")
	}
	rapid.Check(t, func(t *rapid.T) { checkSignature(t, secrets().Draw(t, "secret"), bodies().Draw(t, "body")) })
}

// Firmas alteradas: para todo (secret, body), toda firma que difiera de la
// válida se rechaza: un carácter cambiado en CADA posición (prefijo e hex,
// incluida la última), truncada a cualquier prefijo propio (incl. 16 y 31 bytes de
// HMAC), extendida, con el prefijo o todo el texto en mayúsculas, con espacio o
// salto de línea, con otro algoritmo o con la firma de otro cuerpo.
func TestPBT_SignatureTampered(t *testing.T) {
	v := githubsig.HMACVerifier{}
	check := func(t fataler, secret, body []byte) {
		sig := githubsig.Sign(secret, body)
		reject := func(what, h string) {
			if h == sig {
				return
			}
			if err := v.Verify(secret, body, h); !errors.Is(err, githubsig.ErrInvalidSignature) {
				t.Fatalf("%s aceptada (%q): %v", what, h, err)
			}
		}
		for i := 0; i < len(sig); i++ {
			for _, c := range []byte("0f97x=-_ \n") { // un carácter distinto en cada posición
				if c != sig[i] {
					reject(fmt.Sprintf("firma con el carácter %d cambiado a %q", i, c), sig[:i]+string(c)+sig[i+1:])
				}
			}
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
	}
	for _, n := range []int{1, 31, 32, 33, 64, 65} {
		check(t, gen.FixedSecret(n), []byte("Hello, World!"))
	}
	rapid.Check(t, func(t *rapid.T) { check(t, secrets().Draw(t, "secret"), bodies().Draw(t, "body")) })
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
		t.Fatalf("cuerpo alterado aceptado")
	}
	if err := (githubsig.HMACVerifier{}).Verify(nil, body, githubsig.Sign(nil, body)); err == nil {
		t.Fatalf("secreto vacío aceptado")
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

// FakeVerifier: por defecto y con RejectAll rechaza todo; AcceptAll acepta todo
// (incluso un encabezado vacío); AcceptOnly(h) acepta solo h y nunca un encabezado
// vacío, para cualquier secreto y cuerpo.
func TestPBT_FakeVerifier(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		secret, body := secrets().Draw(t, "secret"), bodies().Draw(t, "body")
		h := rapid.OneOf(rapid.Just(""), rapid.String(), rapid.Just(githubsig.Sign(secret, body))).Draw(t, "header")
		other := h + "x"
		if err := githubsig.NewFakeVerifier().Verify(secret, body, h); err == nil {
			t.Fatal("el valor cero acepta")
		}
		if err := githubsig.NewFakeVerifier().RejectAll().Verify(secret, body, h); err == nil {
			t.Fatal("RejectAll acepta")
		}
		if err := githubsig.NewFakeVerifier().AcceptAll().Verify(secret, body, h); err != nil {
			t.Fatalf("AcceptAll rechaza: %v", err)
		}
		f := githubsig.NewFakeVerifier().AcceptOnly(h)
		if err := f.Verify(secret, body, h); (h == "") != (err != nil) {
			t.Fatalf("AcceptOnly(%q) con el mismo encabezado: %v", h, err)
		}
		if err := f.Verify(secret, body, other); err == nil {
			t.Fatalf("AcceptOnly(%q) acepta %q", h, other)
		}
		if err := f.Verify(secret, body, ""); err == nil {
			t.Fatal("AcceptOnly acepta un encabezado vacío")
		}
	})
}

func TestPBT_Fixed_FakeVerifier(t *testing.T) {
	f := githubsig.NewFakeVerifier().AcceptOnly("sha256=ab")
	if f.Verify(nil, nil, "sha256=ab") != nil || f.Verify(nil, nil, "sha256=ac") == nil || f.Verify(nil, nil, "") == nil {
		t.Fatal("AcceptOnly")
	}
	if githubsig.NewFakeVerifier().AcceptOnly("").Verify(nil, nil, "") == nil {
		t.Fatal("AcceptOnly vacío acepta un encabezado vacío")
	}
}
