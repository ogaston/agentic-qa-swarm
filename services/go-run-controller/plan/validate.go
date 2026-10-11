package plan

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var methods = map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}

func validMethod(m string) bool { return methods[m] }

func validPath(p string) bool { return strings.HasPrefix(p, "/") }

// ValidateFlowPlan replica contracts/plans/flow-plan.schema.json (la prueba lo contrasta con el
// esquema real). Un plan vacío o roto es un error: el ensayo falla antes de crear ningún Job.
func ValidateFlowPlan(p FlowPlan) error {
	if p.RunID == "" || p.Workflow == "" {
		return errors.New("flow-plan: run_id y workflow son obligatorios")
	}
	if len(p.Flows) == 0 {
		return errors.New("flow-plan: sin flujos")
	}
	for i, f := range p.Flows {
		if f.FlowID == "" || f.Name == "" || f.Invariant == "" || len(f.Steps) == 0 {
			return fmt.Errorf("flow-plan: flujo %d incompleto", i)
		}
		for j, s := range f.Steps {
			if !validMethod(s.Method) || !validPath(s.Path) || s.ExpectStatus < 100 || s.ExpectStatus > 599 {
				return fmt.Errorf("flow-plan: paso %d.%d inválido", i, j)
			}
		}
	}
	return nil
}

// ValidateSurface replica contracts/plans/surface-artifact.schema.json.
func ValidateSurface(s SurfaceArtifact) error {
	if s.RunID == "" {
		return errors.New("surface: run_id vacío")
	}
	if u, err := url.Parse(s.BaseURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("surface: base_url inválida")
	}
	if s.Source != "openapi" && s.Source != "probe" {
		return errors.New("surface: source inválido")
	}
	if s.Endpoints == nil {
		return errors.New("surface: endpoints ausente")
	}
	for _, e := range s.Endpoints {
		if !validMethod(e.Method) || !validPath(e.Path) {
			return errors.New("surface: endpoint inválido")
		}
	}
	return nil
}

// ValidateWarmState replica contracts/plans/warm-state.schema.json.
func ValidateWarmState(w WarmState) error {
	switch w.State {
	case "ready", "dirty", "cuarentena", "idle-escalado":
	default:
		return errors.New("warm-state: state inválido")
	}
	if w.WarmID == "" || w.BaselineVersion == "" {
		return errors.New("warm-state: warm_id y baseline_version son obligatorios")
	}
	return nil
}

// ValidateReport replica contracts/plans/report.schema.json: veredicto de enum, bug exige hallazgos,
// sin-hallazgos e inconcluso no los admiten, y cada hallazgo enlaza evidencia s3:// o https://.
func ValidateReport(r Report) error {
	if r.RunID == "" {
		return errors.New("report: run_id vacío")
	}
	switch r.Verdict {
	case "bug":
		if len(r.Findings) == 0 {
			return errors.New("report: bug sin hallazgos")
		}
	case "sin-hallazgos", "inconcluso":
		if len(r.Findings) != 0 {
			return errors.New("report: el veredicto no admite hallazgos")
		}
	default:
		return errors.New("report: verdict inválido")
	}
	for i, f := range r.Findings {
		if f.FindingID == "" || f.RootCause == "" || f.Invariant == "" || !validMethod(f.Method) || !validPath(f.Path) {
			return fmt.Errorf("report: hallazgo %d incompleto", i)
		}
		if len(f.EvidenceURIs) == 0 {
			return fmt.Errorf("report: hallazgo %d sin evidencia", i)
		}
		for _, u := range f.EvidenceURIs {
			if !strings.HasPrefix(u, "s3://") && !strings.HasPrefix(u, "https://") {
				return fmt.Errorf("report: hallazgo %d con evidencia no s3/https", i)
			}
		}
	}
	return nil
}
