// Package plan define los tipos Go de los contratos U2<->U3 (contracts/plans).
package plan

import "encoding/json"

// Step es una llamada HTTP esperada de un flujo. Body es opcional: si está, es un objeto JSON
// (contracts/plans/flow-plan.schema.json, $defs de paso); sin él no se envía cuerpo.
type Step struct {
	Method       string          `json:"method"`
	Path         string          `json:"path"`
	ExpectStatus int             `json:"expect_status"`
	Body         json.RawMessage `json:"body,omitempty"`
}

// Flow es un flujo QA con su invariante de negocio. InvariantProbe (opcional, false por defecto) marca
// un flujo que sondea una invariante: el ensayo solo exige que sea ejecutable, no que cumpla expect_status.
type Flow struct {
	FlowID         string `json:"flow_id"`
	Name           string `json:"name"`
	Steps          []Step `json:"steps"`
	Invariant      string `json:"invariant"`
	InvariantProbe bool   `json:"invariant_probe,omitempty"`
}

// FlowPlan es el plan de flujos de entrada (contracts/plans/flow-plan.schema.json).
type FlowPlan struct {
	RunID    string `json:"run_id"`
	Workflow string `json:"workflow"`
	Flows    []Flow `json:"flows"`
}

// Endpoint es un endpoint externo descubierto.
type Endpoint struct {
	Method      string          `json:"method"`
	Path        string          `json:"path"`
	RequestBody json.RawMessage `json:"request_body,omitempty"`
}

// SurfaceArtifact es la superficie externa inferida.
type SurfaceArtifact struct {
	RunID     string     `json:"run_id"`
	BaseURL   string     `json:"base_url"`
	Endpoints []Endpoint `json:"endpoints"`
	Source    string     `json:"source"`
}

// EvidenceURIs son las URIs de evidencia de salida.
type EvidenceURIs struct {
	RunID string   `json:"run_id"`
	URIs  []string `json:"uris"`
}

// WarmState es el estado del entorno warm.
type WarmState struct {
	WarmID          string `json:"warm_id"`
	State           string `json:"state"`
	ResetVerified   bool   `json:"reset_verified"`
	BaselineVersion string `json:"baseline_version"`
}

// Report es contracts/plans/report.schema.json (veredicto del reporter).
type Report struct {
	RunID   string `json:"run_id"`
	Verdict string `json:"verdict"`
	Summary string `json:"summary"`
	// ReportURI lo añade la respuesta de POST /v1/report (E-1): URI con el que el store guardó el reporte.
	ReportURI string          `json:"report_uri,omitempty"`
	Findings  []ReportFinding `json:"findings"`
}

// ReportFinding es un hallazgo del reporte.
type ReportFinding struct {
	FindingID    string   `json:"finding_id"`
	RootCause    string   `json:"root_cause"`
	Invariant    string   `json:"invariant"`
	Method       string   `json:"method"`
	Path         string   `json:"path"`
	EvidenceURIs []string `json:"evidence_uris"`
}
