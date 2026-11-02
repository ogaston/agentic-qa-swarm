package httpapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWebhookSecretGoldenVectors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "contracts", "examples", "webhook-secrets.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Context string `json:"context"`
		Vectors []struct {
			PlatformKey string `json:"platform_key"`
			Repo        string `json:"repo"`
			Version     int    `json:"version"`
			Secret      string `json:"secret"`
		} `json:"vectors"`
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
