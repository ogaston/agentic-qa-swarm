package intake

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type webhookSecretVector struct {
	PlatformKey string `json:"platform_key"`
	Repo        string `json:"repo"`
	Version     int    `json:"version"`
	Secret      string `json:"secret"`
}

func TestWebhookSecretGoldenVectors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "contracts", "examples", "webhook-secrets.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Context string               `json:"context"`
		Vectors []webhookSecretVector `json:"vectors"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Context != webhookSecretContext || len(fixture.Vectors) == 0 {
		t.Fatal("fixture de credenciales incompleto")
	}
	for _, vector := range fixture.Vectors {
		got, err := deriveWebhookSecret([]byte(vector.PlatformKey), vector.Repo, vector.Version)
		if err != nil || got != vector.Secret {
			t.Fatalf("repo=%q version=%d: got=%q err=%v", vector.Repo, vector.Version, got, err)
		}
	}
}

func TestWebhookSecretRejectsLegacyAndInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		key     string
		repo    string
		version int
	}{{"", "acme/shop", 1}, {"key", "acme/shop", 0}, {"key", "invalid", 1}} {
		if got, err := deriveWebhookSecret([]byte(tc.key), tc.repo, tc.version); err == nil || got != "" {
			t.Fatalf("key=%q repo=%q version=%d: got=%q err=%v", tc.key, tc.repo, tc.version, got, err)
		}
	}
}
