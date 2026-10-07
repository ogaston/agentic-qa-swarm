// Package config lee y valida el entorno (fail-closed).
package config

import (
	"errors"
	"fmt"
	"os"
	"time"
)

type Config struct {
	Namespace, Token, OutboxFile, DataDir, BaselineScript, Listen, RedisAddr, Backend string
	Grace                                                                             time.Duration
}

// Load valida. needToken exige RESET_SERVICE_TOKEN (modo servidor).
func Load(needToken bool) (Config, error) {
	c := Config{
		Namespace: os.Getenv("RESET_NAMESPACE"), Token: os.Getenv("RESET_SERVICE_TOKEN"),
		OutboxFile: os.Getenv("RESET_OUTBOX_FILE"), DataDir: os.Getenv("RESET_DATA_DIR"),
		BaselineScript: os.Getenv("RESET_BASELINE_SCRIPT"), Listen: os.Getenv("LISTEN_ADDR"),
		RedisAddr: os.Getenv("RESET_REDIS_ADDR"), Backend: os.Getenv("RESET_BACKEND"),
		Grace: 24 * time.Hour,
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
	if g := os.Getenv("HOUSEKEEPING_GRACE"); g != "" {
		d, err := time.ParseDuration(g)
		if err != nil || d <= 0 {
			return c, errors.New("HOUSEKEEPING_GRACE inválido")
		}
		c.Grace = d
	}
	switch c.Backend {
	case "", "kube":
		c.Backend = "kube"
		if c.BaselineScript == "" {
			return c, errors.New("RESET_BASELINE_SCRIPT obligatoria")
		}
		if c.RedisAddr == "" {
			c.RedisAddr = "warm-redis.aqs-test.svc.cluster.local:6379"
		}
	case "fake":
	default:
		return c, errors.New("RESET_BACKEND inválido (kube|fake)")
	}
	if c.Listen == "" {
		c.Listen = ":8080"
	}
	return c, nil
}
