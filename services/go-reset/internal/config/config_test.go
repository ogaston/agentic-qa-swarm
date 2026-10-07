package config_test

import (
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/config"
)

func set(t *testing.T, kv map[string]string) {
	for _, k := range []string{"RESET_NAMESPACE", "RESET_SERVICE_TOKEN", "RESET_OUTBOX_FILE", "RESET_DATA_DIR", "RESET_BASELINE_SCRIPT", "HOUSEKEEPING_GRACE", "RESET_BACKEND", "RESET_ALLOW_FAKE_BACKEND", "KUBERNETES_SERVICE_HOST", "RESET_ENV", "RESET_REDIS_ADDR", "RESET_WARM_ID", "RESET_READY_TIMEOUT", "RESET_REDIS_TIMEOUT"} {
		t.Setenv(k, "")
	}
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

var ok = map[string]string{"RESET_NAMESPACE": "aqs-test", "RESET_SERVICE_TOKEN": "x", "RESET_OUTBOX_FILE": "o", "RESET_DATA_DIR": "d", "RESET_BASELINE_SCRIPT": "s", "RESET_REDIS_ADDR": "r:6379", "RESET_WARM_ID": "w"}

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
		"sin warm id":    with(map[string]string{"RESET_WARM_ID": ""}),
		"sin redis":      with(map[string]string{"RESET_REDIS_ADDR": ""}),
		"timeout malo":   with(map[string]string{"RESET_READY_TIMEOUT": "0s"}),
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

func TestFakeBackendFences(t *testing.T) {
	base := map[string]string{"RESET_BACKEND": "fake", "RESET_ALLOW_FAKE_BACKEND": "true"}
	set(t, with(base))
	if c, err := config.Load(true); err != nil || c.WarmID == "" || c.BaselineVersion == "" {
		t.Fatalf("fake permitido debe arrancar: %+v %v", c, err)
	}
	cases := map[string]map[string]string{
		"sin permiso":        {"RESET_ALLOW_FAKE_BACKEND": ""},
		"permiso distinto":   {"RESET_ALLOW_FAKE_BACKEND": "yes"},
		"dentro del clúster": {"KUBERNETES_SERVICE_HOST": "10.0.0.1"},
		"env prod":           {"RESET_ENV": "prod"},
		"env Production":     {"RESET_ENV": " Production "},
		"env PROD":           {"RESET_ENV": "PROD"},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			env := with(base)
			for k, v := range extra {
				env[k] = v
			}
			set(t, env)
			if _, err := config.Load(true); err == nil {
				t.Fatal("el backend fake llegó a arrancar")
			}
		})
	}
}

func TestFakeBackendAllowedInStagingEnv(t *testing.T) {
	set(t, with(map[string]string{"RESET_BACKEND": "fake", "RESET_ALLOW_FAKE_BACKEND": "true", "RESET_ENV": "dev"}))
	if _, err := config.Load(true); err != nil {
		t.Fatal(err)
	}
}

func TestConfigTimeoutsAndImageFromEnv(t *testing.T) {
	set(t, with(map[string]string{"RESET_READY_TIMEOUT": "7s", "RESET_REDIS_TIMEOUT": "1s", "RESET_BASELINE_VERSION": "v9", "RESET_JOB_IMAGE": "x/y:1"}))
	c, err := config.Load(true)
	if err != nil || c.ReadyTimeout != 7*time.Second || c.RedisTimeout != time.Second || c.BaselineVersion != "v9" || c.JobImage != "x/y:1" {
		t.Fatalf("%+v %v", c, err)
	}
}
