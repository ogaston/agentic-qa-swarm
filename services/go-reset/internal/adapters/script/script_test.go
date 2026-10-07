package script_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/adapters/script"
)

func write(t *testing.T, body string) string {
	p := filepath.Join(t.TempDir(), "baseline.sh")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestScriptCleanerCleanAndDiffReadBack(t *testing.T) {
	st := filepath.Join(t.TempDir(), "rows")
	_ = os.WriteFile(st, []byte("4"), 0o600)
	p := write(t, `case "$1" in clean) printf 0 > `+st+`;; verify) cat `+st+`;; esac`)
	c := &script.Cleaner{Path: p, Timeout: 5 * time.Second}
	ctx := context.Background()
	if n, err := c.Diff(ctx); err != nil || n != 4 {
		t.Fatalf("%d %v", n, err)
	}
	if err := c.Clean(ctx); err != nil {
		t.Fatal(err)
	}
	if n, err := c.Diff(ctx); err != nil || n != 0 {
		t.Fatalf("%d %v", n, err)
	}
}

func TestScriptCleanerFailuresAndGarbage(t *testing.T) {
	if err := (&script.Cleaner{Path: write(t, "exit 3"), Timeout: 5 * time.Second}).Clean(context.Background()); err == nil {
		t.Fatal("exit 3 aceptado")
	}
	if _, err := (&script.Cleaner{Path: write(t, "echo no-es-numero"), Timeout: 5 * time.Second}).Diff(context.Background()); err == nil {
		t.Fatal("salida inválida aceptada")
	}
}

func TestScriptCleanerVersion(t *testing.T) {
	ctx := context.Background()
	c := &script.Cleaner{Path: write(t, `[ "$1" = version ] && echo " v42 "`), Timeout: 5 * time.Second}
	if v, err := c.Version(ctx); err != nil || v != "v42" {
		t.Fatalf("%q %v", v, err)
	}
	if v, _ := (&script.Cleaner{Path: "/nonexistent", Timeout: time.Second, StaticVersion: "env-1"}).Version(ctx); v != "env-1" {
		t.Fatal("la variable debe tener prioridad")
	}
	if _, err := (&script.Cleaner{Path: write(t, "true"), Timeout: 5 * time.Second}).Version(ctx); err == nil {
		t.Fatal("versión vacía aceptada")
	}
	if _, err := (&script.Cleaner{Path: write(t, "exit 1"), Timeout: 5 * time.Second}).Version(ctx); err == nil {
		t.Fatal("fallo de version aceptado")
	}
}
