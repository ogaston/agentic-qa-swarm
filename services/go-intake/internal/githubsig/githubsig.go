// Package githubsig implementa el formato de firma X-Hub-Signature-256 de
// GitHub y un verificador falso programable para pruebas. La verificacion real
// (HMAC + hmac.Equal) llega en U1-T02.
package githubsig

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
)

// Prefix es el prefijo del encabezado X-Hub-Signature-256.
const Prefix = "sha256="

// ErrInvalidSignature indica que la firma no se acepta.
var ErrInvalidSignature = errors.New("githubsig: firma invalida")

// Sign devuelve "sha256=<hex>" del HMAC-SHA256 de body con secret.
func Sign(secret []byte, body []byte) string {
	m := hmac.New(sha256.New, secret)
	m.Write(body)
	return Prefix + hex.EncodeToString(m.Sum(nil))
}

// Verifier valida el encabezado de firma de un cuerpo de webhook.
type Verifier interface {
	Verify(secret, body []byte, header string) error
}

// Mode es el comportamiento programado de un FakeVerifier.
type Mode int

const (
	// RejectAll rechaza todo (valor por defecto, fail-closed).
	RejectAll Mode = iota
	// AcceptAll acepta todo.
	AcceptAll
	// AcceptOnly acepta solo el encabezado programado.
	AcceptOnly
)

// FakeVerifier es un Verifier programable. Su valor cero rechaza todo.
type FakeVerifier struct {
	mu     sync.Mutex
	mode   Mode
	header string
}

var _ Verifier = (*FakeVerifier)(nil)

// NewFakeVerifier devuelve un verificador que rechaza todo.
func NewFakeVerifier() *FakeVerifier { return &FakeVerifier{} }

// AcceptAll programa aceptar cualquier firma.
func (f *FakeVerifier) AcceptAll() *FakeVerifier { f.set(AcceptAll, ""); return f }

// RejectAll programa rechazar cualquier firma.
func (f *FakeVerifier) RejectAll() *FakeVerifier { f.set(RejectAll, ""); return f }

// AcceptOnly programa aceptar unicamente el encabezado dado (no vacio).
func (f *FakeVerifier) AcceptOnly(header string) *FakeVerifier {
	f.set(AcceptOnly, header)
	return f
}

func (f *FakeVerifier) set(m Mode, h string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mode, f.header = m, h
}

// Verify aplica el modo programado. Un encabezado vacio nunca se acepta,
// salvo en AcceptAll.
func (f *FakeVerifier) Verify(_, _ []byte, header string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch f.mode {
	case AcceptAll:
		return nil
	case AcceptOnly:
		if header != "" && header == f.header {
			return nil
		}
	}
	return ErrInvalidSignature
}
