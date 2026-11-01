package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/auth"
)

// Constantes de gobernanza / repositorios.
const (
	GovernanceUnavailable = "governance_unavailable"
	GovernanceRetryAfter  = 10
)

// repoNameRe valida el patrón OpenAPI '^[a-z0-9-]+/[a-z0-9._-]+$'.
var repoNameRe = regexp.MustCompile(`^[a-z0-9-]+/[a-z0-9._-]+$`)

// readRequestBody lee y valida el Content-Type y tamaño máximo del cuerpo para peticiones con JSON.
func readRequestBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type debe ser application/json")
		return nil, false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxBodyBytes))
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "cuerpo mayor a 64 KiB")
		return nil, false
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "cuerpo invalido")
		return nil, false
	}
	return body, true
}

// proxyRepositories atiende GET y POST sobre /repositories delegando en go-governance.
func (h *Handler) proxyRepositories(w http.ResponseWriter, r *http.Request, _ auth.Principal) {
	if h.govc == nil {
		w.Header().Set("Retry-After", strconv.Itoa(GovernanceRetryAfter))
		writeError(w, http.StatusServiceUnavailable, GovernanceUnavailable, "go-governance no configurado")
		return
	}
	var bodyBytes []byte
	if r.Method == http.MethodPost {
		b, ok := readRequestBody(w, r)
		if !ok {
			return
		}
		bodyBytes = b
	}
	tok, _ := auth.BearerToken(r.Header.Get("Authorization"))
	status, hdrs, body, err := h.govc.do(r.Context(), r.Method, "/repositories", r.URL.Query(), bodyBytes, tok)
	if err != nil {
		w.Header().Set("Retry-After", strconv.Itoa(GovernanceRetryAfter))
		writeError(w, http.StatusServiceUnavailable, GovernanceUnavailable, "go-governance no disponible")
		return
	}
	if r.Method == http.MethodGet && status == http.StatusOK {
		var ok bool
		body, ok = h.overlayRepositoryWarm(r, body, true)
		if !ok {
			w.Header().Set("Retry-After", strconv.Itoa(WarmRetryAfter))
			writeError(w, http.StatusServiceUnavailable, WarmUnavailable, "estado del warm no disponible")
			return
		}
	}
	if ra := hdrs.Get("Retry-After"); ra != "" {
		w.Header().Set("Retry-After", ra)
	}
	if status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	if status == http.StatusNoContent {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	ct := hdrs.Get("Content-Type")
	if ct == "" {
		ct = "application/json"
	}
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// proxyRepositoryItem atiende GET, PATCH y DELETE sobre /repositories/{owner}/{repo} delegando en go-governance.
func (h *Handler) proxyRepositoryItem(w http.ResponseWriter, r *http.Request, _ auth.Principal, owner, repo string) {
	fullName := owner + "/" + repo
	if !repoNameRe.MatchString(fullName) {
		writeError(w, http.StatusBadRequest, "invalid_request", "nombre de repositorio invalido")
		return
	}
	if h.govc == nil {
		w.Header().Set("Retry-After", strconv.Itoa(GovernanceRetryAfter))
		writeError(w, http.StatusServiceUnavailable, GovernanceUnavailable, "go-governance no configurado")
		return
	}
	var bodyBytes []byte
	if r.Method == http.MethodPatch {
		b, ok := readRequestBody(w, r)
		if !ok {
			return
		}
		bodyBytes = b
	}
	targetPath := "/repositories/" + url.PathEscape(owner) + "/" + url.PathEscape(repo)
	tok, _ := auth.BearerToken(r.Header.Get("Authorization"))
	status, hdrs, body, err := h.govc.do(r.Context(), r.Method, targetPath, r.URL.Query(), bodyBytes, tok)
	if err != nil {
		w.Header().Set("Retry-After", strconv.Itoa(GovernanceRetryAfter))
		writeError(w, http.StatusServiceUnavailable, GovernanceUnavailable, "go-governance no disponible")
		return
	}
	if r.Method == http.MethodGet && status == http.StatusOK {
		var ok bool
		body, ok = h.overlayRepositoryWarm(r, body, false)
		if !ok {
			w.Header().Set("Retry-After", strconv.Itoa(WarmRetryAfter))
			writeError(w, http.StatusServiceUnavailable, WarmUnavailable, "estado del warm no disponible")
			return
		}
	}
	if ra := hdrs.Get("Retry-After"); ra != "" {
		w.Header().Set("Retry-After", ra)
	}
	if status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	if status == http.StatusNoContent {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	ct := hdrs.Get("Content-Type")
	if ct == "" {
		ct = "application/json"
	}
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// repositoryActivity atiende GET /repositories/{owner}/{repo}/activity consultando inbox.Store.
func (h *Handler) repositoryActivity(w http.ResponseWriter, r *http.Request, _ auth.Principal, owner, repo string) {
	fullName := owner + "/" + repo
	if !repoNameRe.MatchString(fullName) {
		writeError(w, http.StatusBadRequest, "invalid_request", "nombre de repositorio invalido")
		return
	}
	hasWebhook, lastEvent, lastSHA, lastSeenAt := h.cfg.Store.Activity(fullName)
	type activityResponse struct {
		Repo       string  `json:"repo"`
		HasWebhook bool    `json:"has_webhook"`
		LastEvent  *string `json:"last_event,omitempty"`
		LastSHA    *string `json:"last_sha,omitempty"`
		LastSeenAt *string `json:"last_seen_at,omitempty"`
	}
	resp := activityResponse{
		Repo:       fullName,
		HasWebhook: hasWebhook,
	}
	if hasWebhook {
		resp.LastEvent = &lastEvent
		resp.LastSHA = &lastSHA
		seen := lastSeenAt.UTC().Format(time.RFC3339)
		resp.LastSeenAt = &seen
	}
	writeJSON(w, http.StatusOK, resp)
}

// overlayRepositoryWarm sustituye el estado persistido por la lectura actual del namespace.
func (h *Handler) overlayRepositoryWarm(r *http.Request, body []byte, list bool) ([]byte, bool) {
	var repos []map[string]json.RawMessage
	if list {
		if json.Unmarshal(body, &repos) != nil || repos == nil {
			return nil, false
		}
	} else {
		var repo map[string]json.RawMessage
		if json.Unmarshal(body, &repo) != nil || repo == nil {
			return nil, false
		}
		repos = []map[string]json.RawMessage{repo}
	}
	for _, repo := range repos {
		var warm map[string]json.RawMessage
		if json.Unmarshal(repo["warm"], &warm) != nil || warm == nil {
			return nil, false
		}
		var namespace string
		if json.Unmarshal(warm["namespace"], &namespace) != nil || namespace == "" || h.warmc == nil {
			return nil, false
		}
		request := r.Clone(r.Context())
		request.URL.RawQuery = url.Values{"namespace": {namespace}}.Encode()
		state, ok := h.fetchWarm(request)
		if !ok || (state.Namespace != "" && state.Namespace != namespace) {
			return nil, false
		}
		warm["state"], _ = json.Marshal(state.State)
		repo["warm"], _ = json.Marshal(warm)
	}
	if list {
		body, _ = json.Marshal(repos)
	} else {
		body, _ = json.Marshal(repos[0])
	}
	return body, true
}
