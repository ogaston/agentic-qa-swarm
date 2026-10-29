package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/auth"
)

var evidenceRunID = regexp.MustCompile(`^run-[0-9a-f]{16}$`)
var evidenceFlowID = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,33}[a-z0-9])?$`)

// evidence checks ownership and the stored plan before requesting a fixed runner artifact.
func (h *Handler) evidence(w http.ResponseWriter, r *http.Request, pr auth.Principal, runID, flowID, name string) {
	if !evidenceRunID.MatchString(runID) || !evidenceFlowID.MatchString(flowID) || (name != "result.json" && name != "logs.txt") {
		writeError(w, http.StatusBadRequest, "invalid_request", "evidencia no válida")
		return
	}
	rc, ok := h.cfg.Store.ReceiptByRun(runID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "corrida inexistente")
		return
	}
	if pr.Role != auth.RoleAdmin && rc.ConfirmedBy != pr.ID {
		writeError(w, http.StatusForbidden, "forbidden", "la corrida pertenece a otro usuario")
		return
	}
	if h.runc == nil || h.repc == nil {
		writeError(w, http.StatusServiceUnavailable, ReportsUnavailable, "evidencias no configuradas")
		return
	}
	tok, _ := auth.BearerToken(r.Header.Get("Authorization"))
	status, body, err := h.runc.get(r.Context(), "/runs/"+url.PathEscape(runID)+"/plan", nil, tok)
	if err != nil || status != http.StatusOK {
		writeError(w, http.StatusBadGateway, RunControllerUnavailable, "plan no disponible")
		return
	}
	var plan struct {
		RunID string `json:"run_id"`
		Flows []struct {
			ID string `json:"flow_id"`
		} `json:"flows"`
	}
	if json.Unmarshal(body, &plan) != nil || plan.RunID != runID {
		writeError(w, http.StatusBadGateway, RunControllerInvalid, "plan no válido")
		return
	}
	found := false
	for _, flow := range plan.Flows {
		if flow.ID == flowID {
			found = true
			break
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "flujo inexistente")
		return
	}
	status, body, err = h.repc.get(r.Context(), "/v1/evidence", url.Values{"run": {runID}, "flow": {flowID}, "name": {name}}, "")
	if err != nil || status != http.StatusOK {
		writeError(w, http.StatusBadGateway, ReporterUnavailable, "evidencia no disponible")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// reportEvidence only serves references present in the owner's persisted report.
func (h *Handler) reportEvidence(w http.ResponseWriter, r *http.Request, pr auth.Principal, runID, findingIndex, evidenceIndex string) {
	fi, err1 := strconv.Atoi(findingIndex)
	ei, err2 := strconv.Atoi(evidenceIndex)
	if !evidenceRunID.MatchString(runID) || err1 != nil || err2 != nil || fi < 0 || ei < 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "evidencia no válida")
		return
	}
	report := httptest.NewRecorder()
	h.report(report, r, pr, runID)
	if report.Code != http.StatusOK {
		for key, values := range report.Header() {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(report.Code)
		_, _ = w.Write(report.Body.Bytes())
		return
	}
	var data struct {
		Findings []struct {
			URIs []string `json:"evidence_uris"`
		} `json:"findings"`
	}
	if json.Unmarshal(report.Body.Bytes(), &data) != nil || fi >= len(data.Findings) || ei >= len(data.Findings[fi].URIs) {
		writeError(w, http.StatusNotFound, "not_found", "evidencia inexistente")
		return
	}
	uri := data.Findings[fi].URIs[ei]
	if !strings.HasPrefix(uri, "s3://") {
		writeError(w, http.StatusBadRequest, "invalid_request", "evidencia no válida")
		return
	}
	status, body, err := h.repc.get(r.Context(), "/v1/report-evidence", url.Values{"run": {runID}, "uri": {uri}}, "")
	if err != nil || status != http.StatusOK {
		writeError(w, http.StatusBadGateway, ReporterUnavailable, "evidencia no disponible")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="evidencia"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
