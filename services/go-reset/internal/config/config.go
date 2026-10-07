// Package config lee y valida el entorno (fail-closed).
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Namespace, Token, OutboxFile, DataDir, BaselineScript, Listen, RedisAddr, Backend string
	WarmID, BaselineVersion, JobImage                                                 string
	Grace, ReadyTimeout, ReadyInterval, ScriptTimeout, RedisTimeout                   time.Duration
}

// Valores por defecto de las duraciones (todas se sobreescriben por variable).
const (
	defGrace         = 24 * time.Hour    // US-M7.2: grace de higiene
	defReadyTimeout  = 120 * time.Second // espera a Ready tras restart (igual que go-warm-manager)
	defReadyInterval = 2 * time.Second   // sondeo de Ready
	defScriptTimeout = 60 * time.Second  // script de baseline
	defRedisTimeout  = 5 * time.Second   // una orden RESP
	defListen        = ":8080"           // puerto del Dockerfile
)

func dur(name string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(name)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s inválido", name)
	}
	return d, nil
}

// fakeAllowed aplica las vallas del backend fake: permiso explícito, fuera de clúster y fuera de prod.
func fakeAllowed() error {
	if os.Getenv("RESET_ALLOW_FAKE_BACKEND") != "true" {
		return errors.New("RESET_BACKEND=fake exige RESET_ALLOW_FAKE_BACKEND=true")
	}
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return errors.New("RESET_BACKEND=fake prohibido dentro de un clúster (KUBERNETES_SERVICE_HOST)")
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("RESET_ENV"))) {
	case "prod", "production":
		return errors.New("RESET_BACKEND=fake prohibido con RESET_ENV=prod")
	}
	return nil
}

// Load valida. needToken exige RESET_SERVICE_TOKEN (modo servidor).
func Load(needToken bool) (Config, error) {
	c := Config{
		Namespace: os.Getenv("RESET_NAMESPACE"), Token: os.Getenv("RESET_SERVICE_TOKEN"),
		OutboxFile: os.Getenv("RESET_OUTBOX_FILE"), DataDir: os.Getenv("RESET_DATA_DIR"),
		BaselineScript: os.Getenv("RESET_BASELINE_SCRIPT"), Listen: os.Getenv("LISTEN_ADDR"),
		RedisAddr: os.Getenv("RESET_REDIS_ADDR"), Backend: os.Getenv("RESET_BACKEND"),
		WarmID: os.Getenv("RESET_WARM_ID"), BaselineVersion: os.Getenv("RESET_BASELINE_VERSION"),
		JobImage: os.Getenv("RESET_JOB_IMAGE"),
	}
	if c.Namespace != "aqs-test" {
		return c, fmt.Errorf("RESET_NAMESPACE debe ser aqs-test")
	}
	if needToken && c.Token == "" {
		return c, errors.New("RESET_SERVICE_TOKEN vacío")
	}
	if c.OutboxFile == "" || c.DataDir == "" {
		return c, errors.New("RESET_OUTBOX_FILE y RESET_DATA_DIR son obligatorias")
	}
	var err error
	for _, d := range []struct {
		dst  *time.Duration
		name string
		def  time.Duration
	}{
		{&c.Grace, "HOUSEKEEPING_GRACE", defGrace}, {&c.ReadyTimeout, "RESET_READY_TIMEOUT", defReadyTimeout},
		{&c.ReadyInterval, "RESET_READY_INTERVAL", defReadyInterval}, {&c.ScriptTimeout, "RESET_SCRIPT_TIMEOUT", defScriptTimeout},
		{&c.RedisTimeout, "RESET_REDIS_TIMEOUT", defRedisTimeout},
	} {
		if *d.dst, err = dur(d.name, d.def); err != nil {
			return c, err
		}
	}
	switch c.Backend {
	case "", "kube":
		c.Backend = "kube"
		if c.BaselineScript == "" || c.RedisAddr == "" || c.WarmID == "" {
			return c, errors.New("RESET_BASELINE_SCRIPT, RESET_REDIS_ADDR y RESET_WARM_ID son obligatorias")
		}
	case "fake":
		if err := fakeAllowed(); err != nil {
			return c, err
		}
		if c.WarmID == "" {
			c.WarmID = "warm-fake"
		}
		if c.BaselineVersion == "" {
			c.BaselineVersion = "fake-1"
		}
	default:
		return c, errors.New("RESET_BACKEND inválido (kube|fake)")
	}
	if c.Listen == "" {
		c.Listen = defListen
	}
	return c, nil
}
