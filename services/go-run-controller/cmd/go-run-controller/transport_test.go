package main

import "testing"

// Con EVENTS_TRANSPORT=nats no se exigen los archivos de eventos; sí NATS_URL.
func TestLoadConfigTransportNATS(t *testing.T) {
	m := good()
	m["EVENTS_TRANSPORT"] = "nats"
	m["NATS_URL"] = "nats://127.0.0.1:4222"
	delete(m, "RUN_EVENTS_FILE")
	delete(m, "RUN_OUTBOX_FILE")
	c, err := loadConfig(envOf(m))
	if err != nil {
		t.Fatalf("nats sin archivos debe arrancar: %v", err)
	}
	if c.transport != "nats" || c.natsURL != "nats://127.0.0.1:4222" {
		t.Fatalf("transport=%q natsURL=%q", c.transport, c.natsURL)
	}
}

func TestLoadConfigTransportNATSExigeNATSURL(t *testing.T) {
	m := good()
	m["EVENTS_TRANSPORT"] = "nats"
	delete(m, "RUN_EVENTS_FILE")
	delete(m, "RUN_OUTBOX_FILE")
	if _, err := loadConfig(envOf(m)); err == nil {
		t.Fatal("nats sin NATS_URL debe fallar")
	}
}

func TestLoadConfigFileSigueExigiendoArchivos(t *testing.T) {
	m := good()
	m["EVENTS_TRANSPORT"] = "file"
	delete(m, "RUN_EVENTS_FILE")
	if _, err := loadConfig(envOf(m)); err == nil {
		t.Fatal("file sin RUN_EVENTS_FILE debe fallar")
	}
}

func TestLoadConfigTransportDesconocido(t *testing.T) {
	m := good()
	m["EVENTS_TRANSPORT"] = "kafka"
	if _, err := loadConfig(envOf(m)); err == nil {
		t.Fatal("EVENTS_TRANSPORT desconocido debe fallar")
	}
}
