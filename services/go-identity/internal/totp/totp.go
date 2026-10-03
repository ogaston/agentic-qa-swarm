// Package totp implementa TOTP (RFC 6238: SHA-1, 6 dígitos, 30 s) con ventana y anti-reutilización.
package totp

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // RFC 6238 exige HMAC-SHA1 por compatibilidad con los autenticadores.
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// Period es el paso de tiempo en segundos.
	Period = 30
	// Digits es la longitud del código.
	Digits = 6
	// MinSecretBytes son 160 bits.
	MinSecretBytes = 20
)

// ParseSecret decodifica un secreto base32 (con o sin relleno, sin distinguir mayúsculas) de al menos 160 bits.
func ParseSecret(s string) ([]byte, error) {
	s = strings.ToUpper(strings.TrimRight(strings.TrimSpace(s), "="))
	b, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s)
	if err != nil {
		return nil, errors.New("secreto TOTP no es base32 válido")
	}
	if len(b) < MinSecretBytes {
		return nil, fmt.Errorf("secreto TOTP de %d bits: se exigen al menos %d", len(b)*8, MinSecretBytes*8)
	}
	return b, nil
}

// HOTP calcula el código de `digits` dígitos para un contador (RFC 4226).
func HOTP(key []byte, counter uint64, digits int) string {
	mac := hmac.New(sha1.New, key)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	bin := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, bin%mod)
}

// Counter devuelve el contador TOTP del instante t.
func Counter(t time.Time) uint64 { return uint64(t.Unix() / Period) }

// Code devuelve el código TOTP de 6 dígitos del instante t.
func Code(key []byte, t time.Time) string { return HOTP(key, Counter(t), Digits) }

// Verify acepta el código si coincide con alguno de los contadores en ventana ±1 que sea
// estrictamente mayor que lastUsed (rechazo de reutilización). Devuelve el contador aceptado.
func Verify(key []byte, code string, now time.Time, lastUsed uint64) (uint64, bool) {
	if len(code) != Digits {
		return 0, false
	}
	cur := Counter(now)
	var accepted uint64
	ok := false
	for _, c := range []uint64{cur - 1, cur, cur + 1} {
		if c <= lastUsed {
			continue
		}
		// Se recorren los tres sin cortocircuitar para no filtrar cuál coincidió.
		if subtle.ConstantTimeCompare([]byte(HOTP(key, c, Digits)), []byte(code)) == 1 && !ok {
			accepted, ok = c, true
		}
	}
	return accepted, ok
}
