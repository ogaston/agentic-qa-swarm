package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/plan"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

// PhaseTimeout es el plazo de cada llamada HTTP a go-warm-manager y go-reset.
const PhaseTimeout = 5 * time.Second

const maxBody = 1 << 20

var runIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// svc es un cliente de servicio: bearer, timeout, sin redirecciones, solo http(s). Todo lo que no
// sea el código esperado con cuerpo válido es un error (nunca éxito).
type svc struct {
	name, base, token string
	client            *http.Client
}

func newSvc(name, base, token string) (*svc, error) {
	u, err := ValidateHTTPURL(base)
	if err != nil {
		return nil, fmt.Errorf("%s: URL %w", name, err)
	}
	if token == "" {
		return nil, fmt.Errorf("%s: el token de servicio es obligatorio", name)
	}
	return &svc{name: name, base: strings.TrimRight(u.String(), "/"), token: token,
		client: &http.Client{Timeout: PhaseTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// call hace la petición y devuelve el cuerpo si el estado es uno de ok; si no, error con el estado.
func (s *svc) call(ctx context.Context, method, path string, in any, ok ...int) ([]byte, error) {
	b, _, err := s.callStatus(ctx, method, path, in, ok...)
	return b, err
}

func (s *svc) callStatus(ctx context.Context, method, path string, in any, ok ...int) ([]byte, int, error) {
	var rd io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return nil, 0, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.base+path, rd)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if tr := runctl.TraceFrom(ctx); tr != "" {
		req.Header.Set("traceparent", "00-"+tr+"-0000000000000001-01")
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("%s %s inalcanzable", s.name, path)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		return nil, 0, fmt.Errorf("%s %s: cuerpo ilegible o demasiado grande", s.name, path)
	}
	for _, c := range ok {
		if resp.StatusCode == c {
			return body, resp.StatusCode, nil
		}
	}
	return nil, resp.StatusCode, fmt.Errorf("%s %s respondió %d", s.name, path, resp.StatusCode)
}

// strict decodifica un único objeto JSON sin campos desconocidos.
func strict(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return errors.New("cuerpo inválido")
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("cuerpo inválido")
	}
	return nil
}

// WarmClient habla con la API REST de go-warm-manager (sin Jobs: el deploy es en proceso allá).
type WarmClient struct {
	s *svc
	// PollEvery y MaxWait gobiernan la espera de GET /deploys/{run_id}; Sleep es inyectable.
	PollEvery, MaxWait time.Duration
	Sleep              func(context.Context, time.Duration) error
}

// NewWarmClient valida URL http(s) y token.
func NewWarmClient(base, token string) (*WarmClient, error) {
	s, err := newSvc("go-warm-manager", base, token)
	if err != nil {
		return nil, err
	}
	return &WarmClient{s: s, PollEvery: 2 * time.Second, MaxWait: 12 * time.Minute, Sleep: sleepCtx}, nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// State lee GET /warm (validado contra warm-state.schema.json).
func (w *WarmClient) State(ctx context.Context) (plan.WarmState, error) {
	b, err := w.s.call(ctx, http.MethodGet, "/warm", nil, 200)
	if err != nil {
		return plan.WarmState{}, err
	}
	return decodeWarm(b)
}

func decodeWarm(b []byte) (plan.WarmState, error) {
	var ws plan.WarmState
	var present map[string]json.RawMessage
	if err := strict(b, &ws); err != nil {
		return ws, err
	}
	if json.Unmarshal(b, &present) != nil || present["reset_verified"] == nil {
		return ws, errors.New("warm-state: falta reset_verified")
	}
	return ws, plan.ValidateWarmState(ws)
}

// ResetVerified implementa runctl.WarmStateReader: solo true/false cuando el warm respondió bien.
func (w *WarmClient) ResetVerified(ctx context.Context) (runctl.Fact, error) {
	ws, err := w.State(ctx)
	if err != nil {
		return runctl.Unknown, err
	}
	return runctl.FactOf(ws.ResetVerified), nil
}

// Ensure exige 200 (409, 5xx, cuerpo inválido o warm no ready+verificado son error).
func (w *WarmClient) Ensure(ctx context.Context) error {
	b, err := w.s.call(ctx, http.MethodPost, "/warm/ensure", nil, 200)
	if err != nil {
		return err
	}
	ws, err := decodeWarm(b)
	if err != nil {
		return err
	}
	if ws.State != "ready" || !ws.ResetVerified {
		return errors.New("go-warm-manager: el warm no está listo con reset verificado")
	}
	return nil
}

type deployState struct {
	RunID    string `json:"run_id"`
	State    string `json:"state"`
	Attempts int    `json:"attempts"`
	Reason   string `json:"reason,omitempty"`
}

func (w *WarmClient) deployState(ctx context.Context, runID string, codes ...int) (deployState, bool, error) {
	b, err := w.s.call(ctx, http.MethodGet, "/deploys/"+url.PathEscape(runID), nil, append(codes, 404)...)
	if err != nil {
		return deployState{}, false, err
	}
	var d deployState
	if err := strict(b, &d); err != nil {
		return d, false, err
	}
	if d.RunID != runID || (d.State != "pending" && d.State != "done" && d.State != "failed") {
		return d, false, errors.New("go-warm-manager: estado de deploy inválido")
	}
	return d, true, nil
}

// Deploy es idempotente por corrida: si el deploy ya existe lo adopta (no vuelve a pedir ensure ni
// a crearlo); si no, ensure + POST /deploys; luego espera done. failed, timeout o cualquier error: fallo.
func (w *WarmClient) Deploy(ctx context.Context, runID, kind, ref string) error {
	if !runIDRe.MatchString(runID) {
		return errors.New("run_id inválido")
	}
	d, found, err := w.deployStateOrMissing(ctx, runID)
	if err != nil {
		return err
	}
	if !found {
		if err := w.Ensure(ctx); err != nil {
			return err
		}
		b, err := w.s.call(ctx, http.MethodPost, "/deploys", map[string]any{"run_id": runID,
			"artifact": map[string]string{"kind": kind, "ref": ref}}, 202)
		if err != nil {
			return err
		}
		if err := strict(b, &d); err != nil || d.RunID != runID {
			return errors.New("go-warm-manager: respuesta de /deploys inválida")
		}
	}
	deadline := time.Now().Add(w.MaxWait)
	for {
		switch d.State {
		case "done":
			return nil
		case "failed":
			return fmt.Errorf("deploy falló: %s", d.Reason)
		}
		if time.Now().After(deadline) {
			return errors.New("deploy: tiempo de espera agotado")
		}
		if err := w.Sleep(ctx, w.PollEvery); err != nil {
			return err
		}
		var ok bool
		if d, ok, err = w.deployState(ctx, runID, 200); err != nil || !ok {
			return errors.Join(errors.New("deploy: estado no disponible"), err)
		}
	}
}

func (w *WarmClient) deployStateOrMissing(ctx context.Context, runID string) (deployState, bool, error) {
	b, code, err := w.s.callStatus(ctx, http.MethodGet, "/deploys/"+url.PathEscape(runID), nil, 200, 404)
	if err != nil {
		return deployState{}, false, err
	}
	if code == 404 {
		return deployState{}, false, nil
	}
	var d deployState
	if err := strict(b, &d); err != nil || d.RunID != runID || (d.State != "pending" && d.State != "done" && d.State != "failed") {
		return d, false, errors.New("go-warm-manager: estado de deploy inválido")
	}
	return d, true, nil
}

// Surface pide POST /surface; solo 200 con un SurfaceArtifact válido (de la misma corrida) es éxito.
func (w *WarmClient) Surface(ctx context.Context, runID string) (plan.SurfaceArtifact, error) {
	if !runIDRe.MatchString(runID) {
		return plan.SurfaceArtifact{}, errors.New("run_id inválido")
	}
	b, err := w.s.call(ctx, http.MethodPost, "/surface", map[string]string{"run_id": runID}, 200)
	if err != nil {
		return plan.SurfaceArtifact{}, err
	}
	var sa plan.SurfaceArtifact
	if err := strict(b, &sa); err != nil {
		return sa, err
	}
	if err := plan.ValidateSurface(sa); err != nil {
		return sa, err
	}
	if sa.RunID != runID || len(sa.Endpoints) == 0 {
		return sa, errors.New("surface: corrida distinta o sin endpoints")
	}
	return sa, nil
}

// ResetClient habla con go-reset.
type ResetClient struct{ s *svc }

// NewResetClient valida URL http(s) y token.
func NewResetClient(base, token string) (*ResetClient, error) {
	s, err := newSvc("go-reset", base, token)
	if err != nil {
		return nil, err
	}
	return &ResetClient{s: s}, nil
}

// Reset pide POST /resets: solo 200 con reset_verified=true es éxito (409, 5xx, cuerpo inválido: error).
func (r *ResetClient) Reset(ctx context.Context, runID string) error {
	if !runIDRe.MatchString(runID) {
		return errors.New("run_id inválido")
	}
	in := map[string]string{"run_id": runID}
	if tr := runctl.TraceFrom(ctx); tr != "" && runIDRe.MatchString(tr) {
		in["trace_id"] = tr
	}
	b, err := r.s.call(ctx, http.MethodPost, "/resets", in, 200)
	if err != nil {
		return err
	}
	var out struct {
		State         string          `json:"state"`
		ResetVerified *bool           `json:"reset_verified"`
		Checks        json.RawMessage `json:"checks"`
		Attempts      int             `json:"attempts"`
	}
	if err := strict(b, &out); err != nil || out.ResetVerified == nil || !*out.ResetVerified || out.State != "ready" {
		return errors.New("go-reset: el reset no quedó verificado")
	}
	return nil
}

// ArtifactSource entrega el artefacto a desplegar por corrida.
type ArtifactSource func(runID string) (kind, ref string, err error)

// RealPhases es el PhaseLauncher de deploy, superficie y reset sobre HTTP, y delega el ensayo en
// Rehearse (Job). run y report no existen hasta U2-T05: fallan cerrado.
type RealPhases struct {
	Warm     *WarmClient
	Reset    *ResetClient
	Artifact ArtifactSource
	Rehearse runctl.PhaseLauncher
}

var _ runctl.PhaseLauncher = (*RealPhases)(nil)

// ErrNotImplemented marca fases que llegan con U2-T05.
var ErrNotImplemented = errors.New("fase no implementada (U2-T05): falla cerrado")

// Launch implementa runctl.PhaseLauncher.
func (p *RealPhases) Launch(ctx context.Context, phase string, run runctl.Run) ([]string, error) {
	switch phase {
	case runctl.PhaseDeploy:
		if p.Artifact == nil {
			return nil, errors.New("sin fuente de artefacto")
		}
		kind, ref, err := p.Artifact(run.ID)
		if err != nil {
			return nil, err
		}
		return nil, p.Warm.Deploy(ctx, run.ID, kind, ref)
	case runctl.PhaseInfer:
		_, err := p.Warm.Surface(ctx, run.ID)
		return nil, err
	case runctl.PhaseRehearse:
		if p.Rehearse == nil {
			return nil, errors.New("sin lanzador de ensayo")
		}
		return p.Rehearse.Launch(ctx, phase, run)
	case runctl.PhaseReset:
		return nil, p.Reset.Reset(ctx, run.ID)
	}
	return nil, fmt.Errorf("%w: %s", ErrNotImplemented, phase)
}
