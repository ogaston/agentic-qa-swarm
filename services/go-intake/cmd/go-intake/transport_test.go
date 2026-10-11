package main

import (
	"context"
	"testing"
)

// Con nats, sin servidor, el servicio arranca y /readyz da nats: fail; sin el chequeo de outbox.
func TestPublisherNATSSinServidorArrancaYNoEstaListo(t *testing.T) {
	t.Setenv("EVENTS_TRANSPORT", "nats")
	t.Setenv("NATS_URL", "nats://127.0.0.1:1")
	_, ready, err := publisherFromEnv(t.TempDir(), []byte("s"))
	if err != nil {
		t.Fatalf("nats debe arrancar aunque NATS no responda: %v", err)
	}
	var natsCheck func(context.Context) error
	for _, c := range ready {
		if c.Name == "outbox" {
			t.Fatal("con nats no debe haber chequeo outbox")
		}
		if c.Name == "nats" {
			natsCheck = c.Fn
		}
	}
	if natsCheck == nil || natsCheck(context.Background()) == nil {
		t.Fatal("falta el chequeo nats o no falla sin conexión")
	}
}

func TestPublisherNATSExigeNATSURL(t *testing.T) {
	t.Setenv("EVENTS_TRANSPORT", "nats")
	t.Setenv("NATS_URL", "")
	if _, _, err := publisherFromEnv(t.TempDir(), []byte("s")); err == nil {
		t.Fatal("nats sin NATS_URL debe fallar")
	}
}

func TestPublisherFileExigeEventsFile(t *testing.T) {
	t.Setenv("EVENTS_TRANSPORT", "file")
	t.Setenv("INTAKE_EVENTS_FILE", "")
	if _, _, err := publisherFromEnv(t.TempDir(), []byte("s")); err == nil {
		t.Fatal("file sin INTAKE_EVENTS_FILE debe fallar")
	}
}

func TestPublisherTransportDesconocidoFalla(t *testing.T) {
	t.Setenv("EVENTS_TRANSPORT", "kafka")
	if _, _, err := publisherFromEnv(t.TempDir(), []byte("s")); err == nil {
		t.Fatal("EVENTS_TRANSPORT desconocido debe fallar")
	}
}
