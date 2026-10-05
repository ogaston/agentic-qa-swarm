package githubsig_test

import (
	"errors"
	"regexp"
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
