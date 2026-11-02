package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
)

const webhookSecretContext = "aqs/webhook-secret/v1"

var webhookRepoRE = regexp.MustCompile(`^[a-z0-9-]+/[a-z0-9._-]+$`)

func deriveWebhookSecret(platformKey []byte, repo string, version int) (string, error) {
	repo = strings.ToLower(strings.TrimSpace(repo))
	if len(platformKey) == 0 || version < 1 || !webhookRepoRE.MatchString(repo) {
		return "", errors.New("parametros de credencial webhook invalidos")
	}
	mac := hmac.New(sha256.New, platformKey)
	_, _ = io.WriteString(mac, webhookSecretContext)
	_, _ = mac.Write([]byte{0})
	_, _ = io.WriteString(mac, repo)
	_, _ = mac.Write([]byte{0})
	_, _ = io.WriteString(mac, strconv.Itoa(version))
	return "whsec_" + hex.EncodeToString(mac.Sum(nil)), nil
}
