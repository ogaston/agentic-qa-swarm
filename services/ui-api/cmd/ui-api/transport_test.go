package main

import (
	"context"
	"log/slog"
	"testing"
)

// Con nats, sin servidor, el servicio arranca (reintenta en segundo plano) y /readyz da nats: fail.
func TestTransportNATSSinServidorArrancaYNoEstaListo(t *testing.T) {
	t.Setenv("EVENTS_TRANSPORT", "nats")
	t.Setenv("NATS_URL", "nats://127.0.0.1:1")
	tr, err := transportFromEnv(slog.New(slog.DiscardHandler), t.TempDir(), func() bool { return true })
	if err != nil {
		t.Fatalf("nats debe arrancar aunque NATS no responda: %v", err)
	}
	if tr.nats == nil || tr.file != nil || tr.pub == nil {
		t.Fatal("transporte nats mal armado")
	}
	var natsCheck func(context.Context) error
	for _, c := range tr.ready {
		if c.Name == "events_file" {
			t.Fatal("con nats no debe haber chequeo events_file")
		}
		if c.Name == "nats" {
			natsCheck = c.Fn
		}
	}
	if natsCheck == nil || natsCheck(context.Background()) == nil {
		t.Fatal("falta el chequeo nats o no falla sin conexión")
	}
}

func TestTransportNATSExigeURLYDataDir(t *testing.T) {
	t.Setenv("EVENTS_TRANSPORT", "nats")
	t.Setenv("NATS_URL", "")
	if _, err := transportFromEnv(slog.New(slog.DiscardHandler), t.TempDir(), nil); err == nil {
		t.Fatal("nats sin NATS_URL debe fallar")
	}
}

func TestTransportDesconocidoFalla(t *testing.T) {
	t.Setenv("EVENTS_TRANSPORT", "kafka")
	if _, err := transportFromEnv(slog.New(slog.DiscardHandler), t.TempDir(), nil); err == nil {
		t.Fatal("EVENTS_TRANSPORT desconocido debe fallar")
	}
}
