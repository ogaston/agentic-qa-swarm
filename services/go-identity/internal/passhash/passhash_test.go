package passhash

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
)

func randPW(t *testing.T) string {
	t.Helper()
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

func TestHashVerify(t *testing.T) {
	pw := randPW(t)
	h, err := Hash(pw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("formato inesperado: %s", h)
	}
	if ok, err := Verify(pw, h); err != nil || !ok {
		t.Fatalf("la contraseña correcta debe verificar: %v %v", ok, err)
	}
	if ok, _ := Verify(pw+"x", h); ok {
		t.Fatal("una contraseña distinta no debe verificar")
	}
}

func TestHashSaltIsRandomAndInjectable(t *testing.T) {
	pw := randPW(t)
	a, _ := Hash(pw, nil)
	b, _ := Hash(pw, nil)
	if a == b {
		t.Fatal("dos hashes de la misma contraseña deben diferir (sal aleatoria)")
	}
	fixed := bytes.Repeat([]byte{7}, 16)
	c, _ := Hash(pw, bytes.NewReader(fixed))
	d, _ := Hash(pw, bytes.NewReader(fixed))
	if c != d {
		t.Fatal("con la misma sal el hash debe ser determinista")
	}
}

func TestHigherParamsAccepted(t *testing.T) {
	pw := randPW(t)
	h := HashWithParams(pw, MinMemoryKiB*2, MinTime+1, 2, bytes.Repeat([]byte{1}, 16))
	if err := Validate(h); err != nil {
		t.Fatal(err)
	}
	if ok, err := Verify(pw, h); err != nil || !ok {
		t.Fatalf("hash con parámetros altos debe verificar: %v %v", ok, err)
	}
}

func TestLowParamsRejected(t *testing.T) {
	salt := bytes.Repeat([]byte{1}, 16)
	cases := map[string]string{
		"memoria baja": HashWithParams("x", 8*1024, 2, 1, salt),
		"tiempo bajo":  HashWithParams("x", MinMemoryKiB, 1, 1, salt),
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			if err := Validate(h); err == nil {
				t.Fatal("debía rechazarse")
			}
		})
	}
}

func TestInvalidPHC(t *testing.T) {
	good, _ := Hash(randPW(t), nil)
	for name, h := range map[string]string{
		"vacío":         "",
		"texto plano":   "plano",
		"argon2i":       strings.Replace(good, "argon2id", "argon2i", 1),
		"sin clave":     good[:strings.LastIndex(good, "$")],
		"base64 roto":   good[:len(good)-3] + "!!!",
		"versión mala":  strings.Replace(good, "v=19", "v=16", 1),
		"campo extra":   good + "$x",
		"params basura": strings.Replace(good, "m=19456", "m=abc", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if err := Validate(h); err == nil {
				t.Fatal("debía rechazarse")
			}
			if ok, err := Verify("x", h); ok || err == nil {
				t.Fatal("Verify debía fallar con error")
			}
		})
	}
}

func TestDecoyCopiesHighestParams(t *testing.T) {
	salt := bytes.Repeat([]byte{1}, 16)
	a := HashWithParams("x", MinMemoryKiB*2, MinTime, 1, salt)
	b := HashWithParams("x", MinMemoryKiB, MinTime+2, 3, salt)
	d, err := Decoy([]string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	m, tt, p, err := Params(d)
	if err != nil || m != MinMemoryKiB*2 || tt != MinTime+2 || p != 3 {
		t.Fatalf("parámetros del señuelo: m=%d t=%d p=%d %v", m, tt, p, err)
	}
	min, _ := Decoy(nil)
	if m, tt, p, _ := Params(min); m != MinMemoryKiB || tt != MinTime || p != MinThreads {
		t.Fatalf("sin usuarios, mínimos: %d %d %d", m, tt, p)
	}
	if _, err := Decoy([]string{"plano"}); err == nil {
		t.Fatal("hash inválido debía fallar")
	}
}
