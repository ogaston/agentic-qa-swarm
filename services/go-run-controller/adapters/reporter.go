package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/plan"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

// Parámetros del borde U8 (reporter). El plazo es mayor que el de U3: el LLM del reporter tarda.
const (
	ReporterTimeout        = 60 * time.Second
	ReporterCircuitThresh  = 5
	ReporterCircuitOpenFor = 10 * time.Second
)

// ReporterClient implementa la fase report contra POST {REPORTER_URL}/v1/report {run_id, evidence_uris}.
// Falla cerrado: cualquier error, estado distinto de 200, redirección, cuerpo inválido según
// report.schema.json o de otra corrida devuelve error y nunca un veredicto. Solo http(s), sin
// redirecciones, plazo de 60 s y circuito (5 fallos seguidos abren 10 s; una sonda a la vez).
type ReporterClient struct {
	base   string
	client *http.Client
	// Timeout y Now son inyectables en pruebas.
	Timeout time.Duration
	Now     func() time.Time

	mu        sync.Mutex
	fails     int
	openUntil time.Time
	probing   bool
}

// NewReporterClient valida la URL (solo http/https con host).
func NewReporterClient(base string) (*ReporterClient, error) {
	u, err := ValidateHTTPURL(base)
	if err != nil {
		return nil, fmt.Errorf("REPORTER_URL %w", err)
	}
	return &ReporterClient{base: strings.TrimRight(u.String(), "/"), Timeout: ReporterTimeout, Now: time.Now,
		client: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// Report pide el reporte de la corrida con sus URIs de evidencia y devuelve el veredicto validado.
func (c *ReporterClient) Report(ctx context.Context, runID string, evidence []string) (plan.Report, error) {
	if !runIDRe.MatchString(runID) {
		return plan.Report{}, errors.New("run_id inválido")
	}
	if len(evidence) == 0 {
		return plan.Report{}, errors.New("reporter: sin evidencia que reportar")
	}
	probe, admitted := c.admit()
	if !admitted {
		return plan.Report{}, errors.New("reporter: circuito abierto")
	}
	rep, healthy, err := c.fetch(ctx, runID, evidence)
	c.settle(probe, healthy)
	return rep, err
}

func (c *ReporterClient) fetch(ctx context.Context, runID string, evidence []string) (plan.Report, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	in, err := json.Marshal(map[string]any{"run_id": runID, "evidence_uris": evidence})
	if err != nil {
		return plan.Report{}, true, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/v1/report", bytes.NewReader(in))
	if err != nil {
		return plan.Report{}, true, err
	}
	req.Header.Set("Content-Type", "application/json")
	if tr := runctl.TraceFrom(ctx); tr != "" {
		req.Header.Set("traceparent", "00-"+tr+"-0000000000000001-01")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return plan.Report{}, false, errors.New("reporter /v1/report inalcanzable")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		return plan.Report{}, false, errors.New("reporter /v1/report: cuerpo ilegible o demasiado grande")
	}
	if resp.StatusCode != http.StatusOK {
		return plan.Report{}, false, fmt.Errorf("reporter /v1/report respondió %d", resp.StatusCode)
	}
	var rep plan.Report
	if err := strict(body, &rep); err != nil {
		return plan.Report{}, false, errors.New("reporter: reporte ilegible")
	}
	if err := plan.ValidateReport(rep); err != nil {
		return plan.Report{}, false, err
	}
	if rep.ReportURI == "" { // E-1: sin URI del reporte no hay reporte guardado que referenciar
		return plan.Report{}, false, errors.New("reporter: respuesta sin report_uri")
	}
	if rep.RunID != runID {
		return plan.Report{}, false, errors.New("reporter: reporte de otra corrida")
	}
	return rep, true, nil
}

func (c *ReporterClient) admit() (probe, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.openUntil.IsZero() {
		return false, true
	}
	if c.Now().Before(c.openUntil) || c.probing {
		return false, false
	}
	c.probing = true
	return true, true
}

func (c *ReporterClient) settle(probe, healthy bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if probe {
		c.probing = false
	}
	if healthy {
		c.fails, c.openUntil = 0, time.Time{}
		return
	}
	c.fails++
	if probe || c.fails >= ReporterCircuitThresh {
		c.openUntil = c.Now().Add(ReporterCircuitOpenFor)
	}
}
