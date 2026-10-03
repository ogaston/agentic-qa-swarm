package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/passhash"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/session"
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

func TestReadyChecksUsersAndSessions(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	good := filepath.Join(dir, "users.json")
	h, _ := passhash.Hash("una-contraseña-larga", nil)
	if err := os.WriteFile(good, []byte(`[{"username":"marta","password_hash":"`+h+`","role":"user"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := usersCheck(good)(ctx); err != nil {
		t.Errorf("archivo válido: %v", err)
	}
	if err := os.WriteFile(good, []byte(`{rota`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := usersCheck(good)(ctx); err == nil {
		t.Error("archivo corrupto debía fallar")
	}
	if err := usersCheck(filepath.Join(dir, "no-existe.json"))(ctx); err == nil {
		t.Error("archivo ausente debía fallar")
	}
	if err := usersCheck("")(ctx); err == nil {
		t.Error("ruta vacía debía fallar")
	}
	if err := sessionsCheck(nil)(ctx); err == nil {
		t.Error("sin almacén de sesiones debía fallar")
	}
	if err := sessionsCheck(session.New(session.Config{}))(ctx); err != nil {
		t.Errorf("almacén operativo: %v", err)
	}
}

func TestUsersCheckRejectsFileWithoutUsers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.json")
	for _, body := range []string{`[]`, `{}`, ``} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := usersCheck(path)(context.Background()); err == nil {
			t.Errorf("un archivo sin usuarios (%q) no está listo", body)
		}
	}
}
