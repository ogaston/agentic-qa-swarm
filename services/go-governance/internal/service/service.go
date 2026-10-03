// Package service une evaluador, políticas y auditoría con la regla central:
// nunca se devuelve allow=true si la decisión no quedó auditada.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-governance/authz"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/audit"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/policy"
)

// AuditLog es el puerto de auditoría (lo implementa *audit.Log).
type AuditLog interface {
	Append(e audit.Entry) (audit.Entry, error)
	Query(run string, limit int) []audit.Entry
	RunsAllowed(workflow string, day time.Time) int
}

var (
	// ErrAuditFailed: no se pudo registrar la decisión o el cambio.
	ErrAuditFailed = errors.New("auditoría no disponible")
	// ErrUnknownPolicy: nombre de política desconocido (404).
	ErrUnknownPolicy = errors.New("política desconocida")
)

// Service es el núcleo de go-governance.
type Service struct {
	mu       sync.Mutex // serializa decisión+auditoría (cuotas exactas) y PUT de políticas
	policies policy.Store
	audit    AuditLog
	testNS   string
	now      func() time.Time
}

// Config son las dependencias del Service.
type Config struct {
	Policies      policy.Store
	Audit         AuditLog
	TestNamespace string
	Now           func() time.Time
}

// New construye el servicio.
func New(c Config) *Service {
	if c.Now == nil {
		c.Now = time.Now
	}
	return &Service{policies: c.Policies, audit: c.Audit, testNS: c.TestNamespace, now: c.Now}
}

// workflowSource lee la política `workflows` y registra la versión usada.
type workflowSource struct {
	s       *Service
	version int
}

func (w *workflowSource) Workflow(ctx context.Context, name string) (authz.WorkflowRule, bool, error) {
	v, found, err := w.s.policies.Get(ctx, policy.Workflows)
	if err != nil {
		return authz.WorkflowRule{}, false, err
	}
	if !found {
		return authz.WorkflowRule{}, false, nil
	}
	var val policy.WorkflowsValue
	if err := json.Unmarshal(v.Value, &val); err != nil {
		return authz.WorkflowRule{}, false, fmt.Errorf("JSON de la política workflows corrupto: %w", err)
	}
	w.version = v.Version
	for _, it := range val.Workflows {
		if it.Name == name {
			return authz.WorkflowRule{Name: it.Name, MaxRunsPerDay: it.MaxRunsPerDay,
				RequiresApproval: it.RequiresApproval, Version: v.Version}, true, nil
		}
	}
	return authz.WorkflowRule{}, false, nil
}

func (w *workflowSource) RunsAllowed(_ context.Context, wf string, day time.Time) (int, error) {
	return w.s.audit.RunsAllowed(wf, day), nil
}

// Authorize evalúa la transición y la audita. Si devuelve Allow=true, la
// decisión ya está en el log. Ante cualquier error devuelve Allow=false y un error.
func (s *Service) Authorize(ctx context.Context, actor string, in authz.GateInput) (authz.Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	src := &workflowSource{s: s}
	ev := &authz.RuleEvaluator{TestNamespace: s.testNS, Workflows: src, Now: s.now}
	d, evErr := ev.AuthorizeTransition(ctx, in)
	action := "gate.deny"
	switch {
	case evErr != nil:
		d.Allow = false
		action = "gate.error"
		if d.Reason == "" {
			d.Reason = "error interno del evaluador"
		}
		d.Reason += ": " + evErr.Error()
	case d.Allow:
		action = "gate.allow"
	}
	if !d.Allow && d.Reason == "" {
		d.Reason = "denegado"
	}
	detail := map[string]string{
		"from": string(in.From), "to": string(in.To), "target_namespace": in.TargetNamespace,
		"confirmed": in.Confirmed.String(), "reset_verified": in.ResetVerified.String(),
		"ensayo_passed": in.EnsayoPassed.String(), "workflow_allowed": in.WorkflowAllowed.String(),
		"approval_recorded": in.ApprovalRecorded.String(), "reason": d.Reason,
	}
	if in.Workflow != "" {
		detail["workflow"] = in.Workflow
		if src.version > 0 {
			detail["workflows_policy_version"] = strconv.Itoa(src.version)
		}
	}
	e, err := s.audit.Append(audit.Entry{Actor: actor, Action: action, RunID: in.RunID, Detail: detail})
	if err != nil {
		return authz.Decision{Reason: "auditoría no disponible: decisión no registrada, se deniega"},
			fmt.Errorf("%w: %v", ErrAuditFailed, err)
	}
	d.AuditRef = e.Hash
	return d, evErr
}

// GetPolicy devuelve la última versión de la política.
func (s *Service) GetPolicy(ctx context.Context, name string) (policy.Version, bool, error) {
	if !policy.Known(name) {
		return policy.Version{}, false, ErrUnknownPolicy
	}
	return s.policies.Get(ctx, name)
}

// SetPolicy valida, audita y guarda. Se audita ANTES de guardar: sin auditoría no hay cambio.
// Un valor inválido deja `policy.rejected` y devuelve policy.ErrInvalid.
func (s *Service) SetPolicy(ctx context.Context, actor, name string, raw json.RawMessage) (int, error) {
	if !policy.Known(name) {
		return 0, ErrUnknownPolicy
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	norm, verr := policy.Validate(name, raw)
	if verr != nil {
		_, _ = s.audit.Append(audit.Entry{Actor: actor, Action: "policy.rejected",
			Detail: map[string]string{"policy": name, "reason": verr.Error()}})
		return 0, verr
	}
	ver, isNew, err := s.policies.Plan(ctx, name, norm)
	if err != nil {
		return 0, err
	}
	if _, err := s.audit.Append(audit.Entry{Actor: actor, Action: fmt.Sprintf("policy.set:%s@v%d", name, ver),
		Detail: map[string]string{"policy": name, "version": strconv.Itoa(ver), "unchanged": strconv.FormatBool(!isNew)}}); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrAuditFailed, err)
	}
	got, _, err := s.policies.Put(ctx, name, norm, actor)
	if err != nil {
		_, _ = s.audit.Append(audit.Entry{Actor: actor, Action: "policy.error",
			Detail: map[string]string{"policy": name, "version": strconv.Itoa(ver), "reason": "el almacén no guardó la versión"}})
		return 0, err
	}
	if got != ver {
		return 0, fmt.Errorf("versión inesperada %d (plan %d)", got, ver)
	}
	return ver, nil
}

// Rejected audita un PUT rechazado antes de validar el valor (403, 404, 413).
func (s *Service) Rejected(actor, name, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.audit.Append(audit.Entry{Actor: actor, Action: "policy.rejected",
		Detail: map[string]string{"policy": name, "reason": reason}})
	return err
}

// Query consulta la auditoría.
func (s *Service) Query(run string, limit int) []audit.Entry { return s.audit.Query(run, limit) }
