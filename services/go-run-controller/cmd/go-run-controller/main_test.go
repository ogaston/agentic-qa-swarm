package main

import (
	"log/slog"
	"testing"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func good() map[string]string {
	return map[string]string{"RUN_DATA_DIR": "d", "RUN_EVENTS_FILE": "e", "RUN_OUTBOX_FILE": "o", "GOVERNANCE_URL": "http://g",
		"GOVERNANCE_SERVICE_TOKEN": "t", "IDENTITY_URL": "http://i", "LISTEN_ADDR": ":1", "RUN_PHASES": "fake", "RUN_ALLOW_FAKE_PHASES": "true"}
}

func TestLoadConfigOK(t *testing.T) {
	c, err := loadConfig(envOf(good()))
	if err != nil || c.namespace != "aqs-test" {
		t.Fatal(c, err)
	}
}

func TestLoadConfigFailsClosed(t *testing.T) {
	mut := map[string]func(map[string]string){
		"prod+fake":     func(m map[string]string) { m["RUN_ENV"] = "prod" },
		"fake sin perm": func(m map[string]string) { delete(m, "RUN_ALLOW_FAKE_PHASES") },
		"sin phases":    func(m map[string]string) { delete(m, "RUN_PHASES") },
		"url ftp":       func(m map[string]string) { m["GOVERNANCE_URL"] = "ftp://x" },
		"identity ftp":  func(m map[string]string) { m["IDENTITY_URL"] = "file:///x" },
		"ns prod":       func(m map[string]string) { m["RUN_TEST_NAMESPACE"] = "prod" },
	}
	for _, k := range []string{"RUN_DATA_DIR", "RUN_EVENTS_FILE", "RUN_OUTBOX_FILE", "GOVERNANCE_URL", "GOVERNANCE_SERVICE_TOKEN", "IDENTITY_URL", "LISTEN_ADDR"} {
		mut["sin "+k] = func(m map[string]string) { delete(m, k) }
	}
	for name, f := range mut {
		m := good()
		f(m)
		if _, err := loadConfig(envOf(m)); err == nil {
			t.Errorf("%s: debía fallar", name)
		}
	}
}

func nopLog() *slog.Logger { return slog.New(slog.DiscardHandler) }
