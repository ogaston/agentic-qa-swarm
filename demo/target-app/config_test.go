package main

import (
	"os"
	"path/filepath"
	"testing"
)

// U8-T09a F-03: el modo memoria es solo para pruebas; no admite DATABASE_URL.
func TestModoMemoriaRechazaDatabaseURL(t *testing.T) {
	tok := filepath.Join(t.TempDir(), "tok")
	if err := os.WriteFile(tok, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASELINE_TOKEN_FILE", tok)
	t.Setenv("TARGET_APP_STORE", "memoria")
	t.Setenv("DATABASE_URL", "postgres://u:p@db:5432/x")
	if _, err := loadConfig(); err == nil {
		t.Fatal("memoria con DATABASE_URL debe rechazarse al arrancar")
	}
}

func TestModoMemoriaSinDatabaseURLArranca(t *testing.T) {
	tok := filepath.Join(t.TempDir(), "tok")
	if err := os.WriteFile(tok, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASELINE_TOKEN_FILE", tok)
	t.Setenv("TARGET_APP_STORE", "memoria")
	t.Setenv("DATABASE_URL", "")
	c, err := loadConfig()
	if err != nil || !c.StoreMem {
		t.Fatalf("cfg=%+v err=%v", c, err)
	}
}
