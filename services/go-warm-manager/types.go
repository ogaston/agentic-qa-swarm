package warmmanager

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Namespace es el único namespace de prueba que el servicio acepta.
const Namespace = "aqs-test"

// Estados del warm (contracts/plans/warm-state.schema.json).
const (
	StateReady        = "ready"
	StateDirty        = "dirty"
	StateCuarentena   = "cuarentena"
	StateIdleEscalado = "idle-escalado"
)

// WarmState es el WarmState del contrato.
type WarmState struct {
	WarmID          string `json:"warm_id"`
	State           string `json:"state"`
	ResetVerified   bool   `json:"reset_verified"`
	BaselineVersion string `json:"baseline_version"`
}

// Artefactos soportados.
const (
	KindBuildFromRepo  = "build-from-repo"
	KindPublishedImage = "published-image"
)

// Artifact es el artefacto a desplegar sobre el warm.
type Artifact struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
}

// Errores del dominio.
var (
	ErrIllegalTransition  = errors.New("transicion de estado del warm no permitida")
	ErrNotReady           = errors.New("el warm no esta listo")
	ErrWarmTimeout        = errors.New("el warm no llego a Ready a tiempo")
	ErrInvalidRunID       = errors.New("run_id invalido")
	ErrInvalidArtifact    = errors.New("artefacto invalido")
	ErrRegistryNotAllowed = errors.New("registro no permitido")
)

var runIDRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$`)

// ValidateRunID exige un run_id apto como parte del nombre de un Job (DNS-1123, <= 32).
func ValidateRunID(id string) error {
	if !runIDRe.MatchString(id) {
		return fmt.Errorf("%w: %q", ErrInvalidRunID, id)
	}
	return nil
}

// Transition aplica una transición legal del estado del warm.
// resetVerified solo importa en dirty->ready y cuarentena->ready (el reset verificado de go-reset);
// en cualquier otra transición no puede volver verdadero a reset_verified.
func Transition(cur WarmState, to string, resetVerified bool) (WarmState, error) {
	next := cur
	next.State = to
	switch {
	case cur.State == StateReady && to == StateDirty:
		next.ResetVerified = false
	case cur.State == StateDirty && to == StateReady, cur.State == StateCuarentena && to == StateReady:
		if !resetVerified {
			return cur, fmt.Errorf("%w: %s->ready exige reset_verified=true", ErrIllegalTransition, cur.State)
		}
		next.ResetVerified = true
	case cur.State == StateDirty && to == StateCuarentena:
		next.ResetVerified = false
	case cur.State == StateReady && to == StateIdleEscalado, cur.State == StateIdleEscalado && to == StateReady:
		// conserva reset_verified
	default:
		return cur, fmt.Errorf("%w: %s->%s", ErrIllegalTransition, cur.State, to)
	}
	return next, nil
}

// Valid indica si el WarmState cumple el esquema (campos y enum).
func (w WarmState) Valid() bool {
	switch w.State {
	case StateReady, StateDirty, StateCuarentena, StateIdleEscalado:
	default:
		return false
	}
	return w.WarmID != "" && w.BaselineVersion != ""
}

var (
	repoRefRe = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+@[0-9a-f]{40}$`)
	tagRe     = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,127}$`)
)

// ValidateArtifact valida kind/ref y que una imagen publicada venga de un registro permitido
// y con tag fijado (nunca latest ni sin tag; un digest sha256 también vale).
func ValidateArtifact(a Artifact, allowedRegistries []string) error {
	switch a.Kind {
	case KindBuildFromRepo:
		if !repoRefRe.MatchString(a.Ref) {
			return fmt.Errorf("%w: build-from-repo exige owner/repo@<sha de 40 hex>", ErrInvalidArtifact)
		}
		return nil
	case KindPublishedImage:
	default:
		return fmt.Errorf("%w: kind %q", ErrInvalidArtifact, a.Kind)
	}
	ref := a.Ref
	slash := strings.Index(ref, "/")
	if slash <= 0 || strings.ContainsAny(ref, " \t\r\n\"'\\") {
		return fmt.Errorf("%w: la imagen debe llevar registro explicito", ErrInvalidArtifact)
	}
	host := ref[:slash]
	ok := false
	for _, r := range allowedRegistries {
		if r != "" && strings.EqualFold(r, host) {
			ok = true
		}
	}
	if !ok {
		return fmt.Errorf("%w: %q", ErrRegistryNotAllowed, host)
	}
	rest := ref[slash+1:]
	if i := strings.Index(rest, "@sha256:"); i >= 0 {
		if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(rest[i+len("@sha256:"):]) {
			return fmt.Errorf("%w: digest invalido", ErrInvalidArtifact)
		}
		return nil
	}
	c := strings.LastIndex(rest, ":")
	if c < 0 || strings.Contains(rest[c:], "/") {
		return fmt.Errorf("%w: la imagen debe llevar tag fijado", ErrInvalidArtifact)
	}
	tag := rest[c+1:]
	if !tagRe.MatchString(tag) || strings.EqualFold(tag, "latest") {
		return fmt.Errorf("%w: tag %q no permitido", ErrInvalidArtifact, tag)
	}
	return nil
}
