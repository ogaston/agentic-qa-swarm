// Package policy define los esquemas de las políticas administrables y el
// almacén versionado solo-agregar.
package policy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
)

// Nombres de las políticas conocidas.
const (
	Events          = "events"
	ConfirmRequired = "confirm_required"
	WarmQuotas      = "warm_quotas"
	Workflows       = "workflows"
)

// Known indica si el nombre corresponde a una política conocida.
func Known(name string) bool {
	switch name {
	case Events, ConfirmRequired, WarmQuotas, Workflows:
		return true
	}
	return false
}

// ErrInvalid marca un valor que no cumple el esquema (HTTP 422).
var ErrInvalid = errors.New("valor de política inválido")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// EventsValue es la política `events`.
type EventsValue struct {
	EnabledEvents []string `json:"enabled_events"`
}

// ConfirmRequiredValue es la política `confirm_required` (solo true).
type ConfirmRequiredValue struct {
	Required bool `json:"required"`
}

// WarmQuotasValue es la política `warm_quotas`.
type WarmQuotasValue struct {
	MaxRunsPerDay          int `json:"max_runs_per_day"`
	RebuildCadenceHours    int `json:"rebuild_cadence_hours"`
	IdleScaleDownMinutes   int `json:"idle_scale_down_minutes"`
	HousekeepingGraceHours int `json:"housekeeping_grace_hours"`
}

// WorkflowItem es un workflow de la política `workflows`.
type WorkflowItem struct {
	Name             string `json:"name"`
	Complexity       string `json:"complexity"`
	MaxRunsPerDay    int    `json:"max_runs_per_day"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
	RequiresApproval bool   `json:"requires_approval"`
}

// WorkflowsValue es la política `workflows`.
type WorkflowsValue struct {
	Workflows []WorkflowItem `json:"workflows"`
}

var workflowName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// strict decodifica rechazando campos desconocidos, ausentes (vía punteros) y basura posterior.
func strict(raw []byte, into any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return invalid("%v", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return invalid("contenido tras el valor")
	}
	return nil
}

func inRange(name string, v *int, lo, hi int) error {
	if v == nil {
		return invalid("%s es obligatorio", name)
	}
	if *v < lo || *v > hi {
		return invalid("%s fuera de rango %d..%d", name, lo, hi)
	}
	return nil
}

// Validate valida el valor de la política y devuelve su forma normalizada (JSON canónico).
func Validate(name string, raw json.RawMessage) (json.RawMessage, error) {
	switch name {
	case Events:
		var v struct {
			EnabledEvents *[]string `json:"enabled_events"`
		}
		if err := strict(raw, &v); err != nil {
			return nil, err
		}
		if v.EnabledEvents == nil {
			return nil, invalid("enabled_events es obligatorio")
		}
		seen := map[string]bool{}
		for _, e := range *v.EnabledEvents {
			if e != "commit" && e != "pull_request" && e != "tag" {
				return nil, invalid("evento %q no permitido", e)
			}
			if seen[e] {
				return nil, invalid("evento %q repetido", e)
			}
			seen[e] = true
		}
		return json.Marshal(EventsValue{EnabledEvents: *v.EnabledEvents})
	case ConfirmRequired:
		var v struct {
			Required *bool `json:"required"`
		}
		if err := strict(raw, &v); err != nil {
			return nil, err
		}
		if v.Required == nil {
			return nil, invalid("required es obligatorio")
		}
		if !*v.Required {
			return nil, invalid("confirm_required solo acepta true (auto-run apagado)")
		}
		return json.Marshal(ConfirmRequiredValue{Required: true})
	case WarmQuotas:
		var v struct {
			A *int `json:"max_runs_per_day"`
			B *int `json:"rebuild_cadence_hours"`
			C *int `json:"idle_scale_down_minutes"`
			D *int `json:"housekeeping_grace_hours"`
		}
		if err := strict(raw, &v); err != nil {
			return nil, err
		}
		for _, c := range []struct {
			n      string
			p      *int
			lo, hi int
		}{{"max_runs_per_day", v.A, 1, 1000}, {"rebuild_cadence_hours", v.B, 1, 720},
			{"idle_scale_down_minutes", v.C, 1, 1440}, {"housekeeping_grace_hours", v.D, 1, 168}} {
			if err := inRange(c.n, c.p, c.lo, c.hi); err != nil {
				return nil, err
			}
		}
		return json.Marshal(WarmQuotasValue{*v.A, *v.B, *v.C, *v.D})
	case Workflows:
		var v struct {
			Workflows *[]struct {
				Name             *string `json:"name"`
				Complexity       *string `json:"complexity"`
				MaxRunsPerDay    *int    `json:"max_runs_per_day"`
				TimeoutSeconds   *int    `json:"timeout_seconds"`
				RequiresApproval *bool   `json:"requires_approval"`
			} `json:"workflows"`
		}
		if err := strict(raw, &v); err != nil {
			return nil, err
		}
		if v.Workflows == nil {
			return nil, invalid("workflows es obligatorio")
		}
		out := WorkflowsValue{Workflows: []WorkflowItem{}}
		seen := map[string]bool{}
		for i, w := range *v.Workflows {
			if w.Name == nil || !workflowName.MatchString(*w.Name) {
				return nil, invalid("workflows[%d].name inválido (^[a-z][a-z0-9-]{0,62}$)", i)
			}
			if seen[*w.Name] {
				return nil, invalid("workflow %q repetido", *w.Name)
			}
			seen[*w.Name] = true
			if w.Complexity == nil || (*w.Complexity != "low" && *w.Complexity != "medium" && *w.Complexity != "high") {
				return nil, invalid("workflows[%d].complexity debe ser low|medium|high", i)
			}
			if err := inRange("max_runs_per_day", w.MaxRunsPerDay, 1, 1000); err != nil {
				return nil, err
			}
			if err := inRange("timeout_seconds", w.TimeoutSeconds, 1, 3600); err != nil {
				return nil, err
			}
			if w.RequiresApproval == nil {
				return nil, invalid("workflows[%d].requires_approval es obligatorio", i)
			}
			out.Workflows = append(out.Workflows, WorkflowItem{*w.Name, *w.Complexity, *w.MaxRunsPerDay, *w.TimeoutSeconds, *w.RequiresApproval})
		}
		return json.Marshal(out)
	}
	return nil, fmt.Errorf("política desconocida %q", name)
}
