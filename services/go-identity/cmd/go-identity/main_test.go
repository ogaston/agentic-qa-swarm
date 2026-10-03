package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/passhash"
)

func TestHashPasswordFromStdin(t *testing.T) {
	var out bytes.Buffer
	pw := strings.Repeat("ab", 6)
	if err := hashPassword(strings.NewReader(pw+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	if ok, err := passhash.Verify(pw, strings.TrimSpace(out.String())); err != nil || !ok {
		t.Fatalf("el salto de línea final no forma parte de la contraseña: %v %v", ok, err)
	}
}

func TestHashPasswordLengthLimits(t *testing.T) {
	for name, pw := range map[string]string{"vacía": "", "7 caracteres": strings.Repeat("a", 7), "129 caracteres": strings.Repeat("a", 129)} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			if err := hashPassword(strings.NewReader(pw), &out); err == nil || out.Len() != 0 {
				t.Fatal("debía rechazarse sin imprimir nada")
			}
		})
	}
	var out bytes.Buffer
	if err := hashPassword(strings.NewReader(strings.Repeat("ñ", 128)), &out); err != nil {
		t.Fatalf("128 caracteres multibyte deben aceptarse: %v", err)
	}
}
