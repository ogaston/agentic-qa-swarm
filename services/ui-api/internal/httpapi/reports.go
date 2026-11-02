package httpapi

// GET /reports/{run}: propiedad por recibo (GET /confirmations), report_uri desde go-run-controller
// (GET /runs/{id}, con el token de la PERSONA: el controlador lo valida igual que cualquier cliente) y
// el reporte leído a través de agent-reporter (GET /v1/reports?uri=, sin credenciales: no hay token de
// servicio para el reporter hoy; ver candidatas de U9-T01). Sin LLM y sin cliente S3 aquí.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/auth"
)

// Códigos de error estables de GET /reports/{run}.
const (
	ReportsUnavailable       = "reports_unavailable"
	RunControllerUnavailable = "run_controller_unavailable"
	ReporterUnavailable      = "reporter_unavailable"
	ReportNotReady           = "report_not_ready"
	ReportMismatch           = "report_mismatch"
	RunControllerInvalid     = "run_controller_invalid"
	ReportsRetryAfter        = 10
	reportMaxBody            = 4 << 20 // tope del cuerpo que aceptamos de un upstream
)

// runIDRe es la misma forma que acepta agent-reporter para run_id; lo que no cumple no sale de ui-api.
var runIDRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// errUpstreamDown agrupa circuito abierto, fallo de transporte, plazo vencido y cuerpo excesivo.
var errUpstreamDown = errors.New("upstream no disponible")

// upstreamClient es una llamada GET a otro servicio con plazo y cortacircuitos.
type upstreamClient struct {
	base    string
	timeout time.Duration
	hc      *http.Client
	br      *breaker
}

func newUpstreamClient(raw string, timeout time.Duration, now func() time.Time, name string) (*upstreamClient, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil, errors.New(name + " debe ser una URL http(s) con host")
	}
	if timeout <= 0 {
		timeout = WarmDefaultTimeout
	}
	return &upstreamClient{
		base:    strings.TrimRight(u.String(), "/"),
		timeout: timeout,
		hc: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
		br: newBreaker(WarmCircuitThresh, WarmCircuitOpenFor, now),
	}, nil
}

// do ejecuta una petición HTTP al upstream con timeout y cortacircuitos.
func (u *upstreamClient) do(parent context.Context, method, path string, q url.Values, body []byte, bearer string) (int, http.Header, []byte, error) {
	probe, ok := u.br.admit()
	if !ok {
		return 0, nil, nil, errUpstreamDown
	}
	ctx, cancel := context.WithTimeout(parent, u.timeout)
	defer cancel()
	target := u.base + path
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bodyReader)
	if err != nil {
		u.br.abort(probe)
		return 0, nil, nil, errUpstreamDown
	}
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := u.hc.Do(req)
	if err != nil {
		if parent.Err() != nil { // lo abortó el cliente de ui-api: no es fallo del upstream
			u.br.abort(probe)
			return 0, nil, nil, errUpstreamDown
		}
		u.br.settle(probe, false)
		return 0, nil, nil, errUpstreamDown
	}
	defer resp.Body.Close()
	u.br.settle(probe, true)
	b, err := io.ReadAll(io.LimitReader(resp.Body, reportMaxBody+1))
	if err != nil || len(b) > reportMaxBody {
		return 0, nil, nil, errUpstreamDown
	}
	return resp.StatusCode, resp.Header.Clone(), b, nil
}

// get devuelve el status y el cuerpo, o errUpstreamDown si no hubo respuesta utilizable.
func (u *upstreamClient) get(parent context.Context, path string, q url.Values, bearer string) (int, []byte, error) {
	status, _, body, err := u.do(parent, http.MethodGet, path, q, nil, bearer)
	return status, body, err
}

// report atiende GET /reports/{run} ya autenticado (pr = persona, token = su bearer).
func (h *Handler) report(w http.ResponseWriter, r *http.Request, pr auth.Principal, runID string) {
	if !runIDRe.MatchString(runID) {
		writeError(w, http.StatusBadRequest, "invalid_request", "run mal formado")
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
		w.Header().Set("Retry-After", strconv.Itoa(ReportsRetryAfter))
		writeError(w, http.StatusServiceUnavailable, ReportsUnavailable, "reportes no configurados")
		return
	}
	tok, _ := auth.BearerToken(r.Header.Get("Authorization"))
	status, body, err := h.runc.get(r.Context(), "/runs/"+url.PathEscape(runID), nil, tok)
	switch {
	case err != nil || (status != http.StatusOK && status != http.StatusUnauthorized && status != http.StatusNotFound):
		w.Header().Set("Retry-After", strconv.Itoa(ReportsRetryAfter))
		writeError(w, http.StatusServiceUnavailable, RunControllerUnavailable, "go-run-controller no disponible")
		return
	case status == http.StatusUnauthorized:
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "unauthorized", "credenciales ausentes o invalidas")
		return
	case status == http.StatusNotFound:
		writeError(w, http.StatusNotFound, "not_found", "corrida inexistente")
		return
	}
	var run struct {
		ID        string `json:"id"`
		ReportURI string `json:"report_uri"`
	}
	if json.Unmarshal(body, &run) != nil || run.ID != runID {
		writeError(w, http.StatusBadGateway, RunControllerInvalid, "respuesta del controlador no válida")
		return
	}
	if run.ReportURI == "" {
		writeError(w, http.StatusNotFound, ReportNotReady, "el reporte aún no está listo")
		return
	}
	status, body, err = h.repc.get(r.Context(), "/v1/reports", url.Values{"uri": {run.ReportURI}}, "")
	if err != nil || status != http.StatusOK {
		writeError(w, http.StatusBadGateway, ReporterUnavailable, "agent-reporter no disponible")
		return
	}
	var rep struct {
		RunID string `json:"run_id"`
	}
	if json.Unmarshal(body, &rep) != nil || rep.RunID != runID {
		writeError(w, http.StatusBadGateway, ReportMismatch, "el reporte no corresponde a la corrida")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
