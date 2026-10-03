// Package passhash implementa hashing de contraseñas con argon2id en formato PHC.
package passhash

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Parámetros mínimos aceptados (y usados al generar): m=19 MiB, t=2, p=1.
const (
	MinMemoryKiB = 19 * 1024
	MinTime      = 2
	MinThreads   = 1

	maxMemoryKiB = 1 << 20 // 1 GiB: evita que un archivo de usuarios agote la memoria.
	maxTime      = 16
	maxThreads   = 32
	saltLen      = 16
	keyLen       = 32
	minKeyLen    = 16
	maxKeyLen    = 64
)

// ErrFormat indica un hash que no es argon2id PHC válido o con parámetros insuficientes.
var ErrFormat = errors.New("hash argon2id PHC inválido")

type parsed struct {
	mem, time uint32
	threads   uint8
	salt, key []byte
}

// Hash genera el hash PHC de la contraseña. rnd es la fuente de la sal (nil usa crypto/rand).
func Hash(password string, rnd io.Reader) (string, error) {
	if rnd == nil {
		rnd = rand.Reader
	}
	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rnd, salt); err != nil {
		return "", fmt.Errorf("sal: %w", err)
	}
	return encode(MinMemoryKiB, MinTime, MinThreads, salt, password), nil
}

func encode(mem, t uint32, p uint8, salt []byte, password string) string {
	key := argon2.IDKey([]byte(password), salt, t, mem, p, keyLen)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, mem, t, p, b64.EncodeToString(salt), b64.EncodeToString(key))
}

// Validate comprueba formato y parámetros mínimos sin hashear nada.
func Validate(phc string) error {
	_, err := parse(phc)
	return err
}

// Verify compara la contraseña con el hash PHC en tiempo constante.
func Verify(password, phc string) (bool, error) {
	h, err := parse(phc)
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(password), h.salt, h.time, h.mem, h.threads, uint32(len(h.key)))
	return subtle.ConstantTimeCompare(got, h.key) == 1, nil
}

func parse(phc string) (parsed, error) {
	var h parsed
	parts := strings.Split(phc, "$")
	// "", "argon2id", "v=19", "m=..,t=..,p=..", salt, key
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return h, ErrFormat
	}
	var v int
	if n, err := fmt.Sscanf(parts[2], "v=%d", &v); n != 1 || err != nil || v != argon2.Version || parts[2] != fmt.Sprintf("v=%d", v) {
		return h, ErrFormat
	}
	var m, t uint32
	var p uint8
	if n, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); n != 3 || err != nil {
		return h, ErrFormat
	}
	if parts[3] != fmt.Sprintf("m=%d,t=%d,p=%d", m, t, p) {
		return h, ErrFormat
	}
	if m < MinMemoryKiB || t < MinTime || p < MinThreads || m > maxMemoryKiB || t > maxTime || p > maxThreads {
		return h, fmt.Errorf("%w: parámetros fuera de rango", ErrFormat)
	}
	b64 := base64.RawStdEncoding.Strict()
	salt, err := b64.DecodeString(parts[4])
	if err != nil || len(salt) < 8 {
		return h, ErrFormat
	}
	key, err := b64.DecodeString(parts[5])
	if err != nil || len(key) < minKeyLen || len(key) > maxKeyLen {
		return h, ErrFormat
	}
	return parsed{mem: m, time: t, threads: p, salt: salt, key: key}, nil
}

// HashWithParams es para pruebas: genera un hash con parámetros arbitrarios (posiblemente bajos).
func HashWithParams(password string, mem, t uint32, p uint8, salt []byte) string {
	return encode(mem, t, p, salt, password)
}
