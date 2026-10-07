package config_test

import (
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/config"
)

func set(t *testing.T, kv map[string]string) {
	for _, k := range []string{"RESET_NAMESPACE", "RESET_SERVICE_TOKEN", "RESET_OUTBOX_FILE", "RESET_DATA_DIR", "RESET_BASELINE_SCRIPT", "HOUSEKEEPING_GRACE", "RESET_BACKEND"} {
		t.Setenv(k, "")
	}
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

var ok = map[string]string{"RESET_NAMESPACE": "aqs-test", "RESET_SERVICE_TOKEN": "x", "RESET_OUTBOX_FILE": "o", "RESET_DATA_DIR": "d", "RESET_BASELINE_SCRIPT": "s"}

func with(extra map[string]string) map[string]string {
	m := map[string]string{}
	for k, v := range ok {
		m[k] = v
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func TestConfigValid(t *testing.T) {
	set(t, ok)
	c, err := config.Load(true)
	if err != nil || c.Grace != 24*time.Hour {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestConfigFailClosed(t *testing.T) {
	cases := map[string]map[string]string{
		"ns ajeno":       with(map[string]string{"RESET_NAMESPACE": "aqs-prod"}),
		"sin token":      with(map[string]string{"RESET_SERVICE_TOKEN": ""}),
		"grace inválido": with(map[string]string{"HOUSEKEEPING_GRACE": "abc"}),
		"grace negativo": with(map[string]string{"HOUSEKEEPING_GRACE": "-1h"}),
		"sin outbox":     with(map[string]string{"RESET_OUTBOX_FILE": ""}),
		"sin script":     with(map[string]string{"RESET_BASELINE_SCRIPT": ""}),
		"backend raro":   with(map[string]string{"RESET_BACKEND": "otro"}),
		"vacío":          {},
	}
	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			set(t, env)
			if _, err := config.Load(true); err == nil {
				t.Fatal("aceptado")
			}
		})
	}
}

func TestConfigGraceConfigurable(t *testing.T) {
	set(t, with(map[string]string{"HOUSEKEEPING_GRACE": "1h"}))
	if c, err := config.Load(true); err != nil || c.Grace != time.Hour {
		t.Fatalf("%+v %v", c, err)
	}
}
