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

// Parámetros del borde U2→U3 (planner de flujos).
const (
	FlowSourceTimeout    = 30 * time.Second
	FlowCircuitThreshold = 5
	FlowCircuitOpen      = 10 * time.Second
	flowCacheMax         = 256
)

// SurfaceFunc entrega el SurfaceArtifact de la corrida (normalmente WarmClient.Surface).
type SurfaceFunc func(ctx context.Context, runID string) (plan.SurfaceArtifact, error)

// WorkflowFunc entrega el workflow confirmado de la corrida.
type WorkflowFunc func(runID string) (string, bool)

// FlowSourceU3 implementa FlowSource contra POST {U3_URL}/v1/plan {surface, workflow} → FlowPlan.
// Falla cerrado: cualquier error, estado distinto de 200, redirección, plan inválido, vacío, de otra
// corrida o con pasos fuera de la raíz del warm devuelve error y nunca un plan. Solo http(s), sin
// redirecciones, plazo de 30 s y circuito (5 fallos seguidos abren 10 s; una sonda a la vez).
// El plan validado se guarda por corrida: ensayo y runners usan el mismo plan.
type FlowSourceU3 struct {
	base     string
	client   *http.Client
	surface  SurfaceFunc
	workflow WorkflowFunc
	// Timeout y Now son inyectables en pruebas.
	Timeout time.Duration
	Now     func() time.Time

	mu        sync.Mutex
	cache     map[string]plan.FlowPlan
	fails     int
	openUntil time.Time
	probing   bool
}

// NewFlowSourceU3 valida la URL (solo http/https con host) y las dependencias.
func NewFlowSourceU3(base string, surface SurfaceFunc, workflow WorkflowFunc) (*FlowSourceU3, error) {
	u, err := ValidateHTTPURL(base)
	if err != nil {
		return nil, fmt.Errorf("U3_URL %w", err)
	}
	if surface == nil || workflow == nil {
		return nil, errors.New("U3: superficie y workflow son obligatorios")
	}
	return &FlowSourceU3{base: strings.TrimRight(u.String(), "/"), surface: surface, workflow: workflow,
		Timeout: FlowSourceTimeout, Now: time.Now, cache: map[string]plan.FlowPlan{},
		client: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// Flows devuelve el FlowPlan validado de la corrida o un error (fail-closed).
func (f *FlowSourceU3) Flows(runID string) (plan.FlowPlan, error) {
	if !runIDRe.MatchString(runID) {
		return plan.FlowPlan{}, errors.New("run_id inválido")
	}
	f.mu.Lock()
	if fp, ok := f.cache[runID]; ok {
		f.mu.Unlock()
		return fp, nil
	}
	f.mu.Unlock()
	wf, ok := f.workflow(runID)
	if !ok || wf == "" {
		return plan.FlowPlan{}, errors.New("U3: la corrida no tiene workflow")
	}
	probe, admitted := f.admit()
	if !admitted {
		return plan.FlowPlan{}, errors.New("U3: circuito abierto")
	}
	fp, healthy, err := f.fetch(runID, wf)
	f.settle(probe, healthy)
	if err != nil {
		return plan.FlowPlan{}, err
	}
	f.mu.Lock()
	if len(f.cache) >= flowCacheMax {
		f.cache = map[string]plan.FlowPlan{}
	}
	f.cache[runID] = fp
	f.mu.Unlock()
	return fp, nil
}

// fetch pide y valida el plan. healthy=false cuenta para el circuito (U3 no respondió de forma usable).
func (f *FlowSourceU3) fetch(runID, wf string) (plan.FlowPlan, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), f.Timeout)
	defer cancel()
	sa, err := f.surface(ctx, runID)
	if err != nil {
		return plan.FlowPlan{}, true, fmt.Errorf("U3: superficie: %w", err) // no es culpa de U3
	}
	if err := plan.ValidateSurface(sa); err != nil || sa.RunID != runID {
		return plan.FlowPlan{}, true, errors.New("U3: superficie inválida o de otra corrida")
	}
	in, err := json.Marshal(map[string]any{"surface": sa, "workflow": wf})
	if err != nil {
		return plan.FlowPlan{}, true, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.base+"/v1/plan", bytes.NewReader(in))
	if err != nil {
		return plan.FlowPlan{}, true, err
	}
	req.Header.Set("Content-Type", "application/json")
	if tr := runctl.TraceFrom(ctx); tr != "" {
		req.Header.Set("traceparent", "00-"+tr+"-0000000000000001-01")
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return plan.FlowPlan{}, false, errors.New("U3 /v1/plan inalcanzable")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		return plan.FlowPlan{}, false, errors.New("U3 /v1/plan: cuerpo ilegible o demasiado grande")
	}
	if resp.StatusCode != http.StatusOK {
		return plan.FlowPlan{}, false, fmt.Errorf("U3 /v1/plan respondió %d", resp.StatusCode)
	}
	var fp plan.FlowPlan
	if err := strict(body, &fp); err != nil {
		return plan.FlowPlan{}, false, errors.New("U3: flow-plan ilegible")
	}
	if err := validateU3Plan(fp, runID, wf); err != nil {
		return plan.FlowPlan{}, false, err
	}
	return fp, true, nil
}

// validateU3Plan aplica el esquema y exige que cada paso sea una ruta relativa al warm.
func validateU3Plan(fp plan.FlowPlan, runID, wf string) error {
	if err := plan.ValidateFlowPlan(fp); err != nil {
		return err
	}
	if fp.RunID != runID || fp.Workflow != wf {
		return errors.New("U3: flow-plan de otra corrida o workflow")
	}
	for _, fl := range fp.Flows {
		for _, s := range fl.Steps {
			if strings.HasPrefix(s.Path, "//") || strings.ContainsAny(s.Path, "\\\r\n") || strings.Contains(strings.SplitN(s.Path, "?", 2)[0], "://") {
				return errors.New("U3: paso fuera de la raíz del warm")
			}
		}
	}
	return nil
}

func (f *FlowSourceU3) admit() (probe, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.openUntil.IsZero() {
		return false, true
	}
	if f.Now().Before(f.openUntil) || f.probing {
		return false, false
	}
	f.probing = true
	return true, true
}

func (f *FlowSourceU3) settle(probe, healthy bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if probe {
		f.probing = false
	}
	if healthy {
		f.fails, f.openUntil = 0, time.Time{}
		return
	}
	f.fails++
	if probe || f.fails >= FlowCircuitThreshold {
		f.openUntil = f.Now().Add(FlowCircuitOpen)
	}
}
