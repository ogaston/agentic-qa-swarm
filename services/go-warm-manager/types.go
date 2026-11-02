package warmmanager

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// LegacyNamespace es el único namespace de prueba previo a U12.
const LegacyNamespace = "aqs-test"

// Namespace es el namespace legacy conservado para compatibilidad hacia atrás.
const Namespace = LegacyNamespace

// Estados del warm (contracts/plans/warm-state.schema.json).
const (
	StateReady        = "ready"
	StateDirty        = "dirty"
	StateCuarentena   = "cuarentena"
	StateIdleEscalado = "idle-escalado"
	StateProvisioning = "provisioning"
	StateDeleting     = "deleting"
)

// WarmState es el WarmState del contrato.
type WarmState struct {
	WarmID          string `json:"warm_id"`
	State           string `json:"state"`
	ResetVerified   bool   `json:"reset_verified"`
	BaselineVersion string `json:"baseline_version"`
	Namespace       string `json:"namespace,omitempty"`
	Repo            string `json:"repo,omitempty"`
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
	ErrStateConflict      = errors.New("el estado del warm cambio concurrentemente")
	ErrWarmTimeout        = errors.New("el warm no llego a Ready a tiempo")
	ErrInvalidRunID       = errors.New("run_id invalido")
	ErrInvalidArtifact    = errors.New("artefacto invalido")
	ErrRegistryNotAllowed = errors.New("registro no permitido")
	ErrBuildNotSupported  = errors.New("build-from-repo no soportado")
	ErrImageNotAllowed    = errors.New("imagen no permitida por WARM_ALLOWED_IMAGE_PREFIXES")
)

var runIDRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$`)

// ValidateRunID exige un run_id acotado y apto como identificador (DNS-1123, <= 32).
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
	case cur.State == StateProvisioning && to == StateReady:
		next.ResetVerified = resetVerified
	default:
		return cur, fmt.Errorf("%w: %s->%s", ErrIllegalTransition, cur.State, to)
	}
	return next, nil
}

// Valid indica si el WarmState cumple el esquema (campos y enum).
func (w WarmState) Valid() bool {
	switch w.State {
	case StateReady, StateDirty, StateCuarentena, StateIdleEscalado, StateProvisioning, StateDeleting:
	default:
		return false
	}
	return w.WarmID != "" && w.BaselineVersion != ""
}

var (
	repoRefRe = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+@[0-9a-f]{40}$`)
	tagRe     = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,127}$`)
)

// imagePrefixRe exige registro/owner/repo[/sub...] en minúsculas: al menos tres segmentos (así ghcr.io y
// ghcr.io/owner nunca son un permiso), sin comodines, sin tag ni digest, sin barra final ni vacíos.
var imagePrefixRe = regexp.MustCompile(`^[a-z0-9.-]+(/[a-z0-9._-]+){2,}$`)

// hasDotSegment: un prefijo con un segmento "." o ".." no es un repositorio real.
func hasDotSegment(p string) bool {
	for _, s := range strings.Split(p, "/") {
		if s == "." || s == ".." {
			return true
		}
	}
	return false
}

// ParseImagePrefixes lee WARM_ALLOWED_IMAGE_PREFIXES (lista separada por comas). Vacía (o solo espacios) =
// ninguna imagen permitida (fail-closed). Un elemento vacío (`a,,b`, coma final) o un prefijo mal formado es
// error de configuración: el servicio no arranca (nunca se omite en silencio).
func ParseImagePrefixes(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			return nil, errors.New("WARM_ALLOWED_IMAGE_PREFIXES: elemento vacío (sobra una coma)")
		}
		if !imagePrefixRe.MatchString(p) || hasDotSegment(p) {
			return nil, fmt.Errorf("WARM_ALLOWED_IMAGE_PREFIXES: prefijo %q mal formado (exige registro/owner/repo[/sub] en minúsculas)", p)
		}
		out = append(out, p)
	}
	return out, nil
}

// imageRepo devuelve el repositorio de una imagen: todo hasta el primer ':' o '@' (tag o digest).
func imageRepo(ref string) string {
	if i := strings.IndexAny(ref, ":@"); i >= 0 {
		return ref[:i]
	}
	return ref
}

// ValidateArtifact valida kind/ref y que una imagen publicada venga de un registro permitido,
// con tag fijado (nunca latest ni sin tag; un digest sha256 también vale) y de un repositorio
// exactamente igual a uno de allowedPrefixes (ErrImageNotAllowed si no).
func ValidateArtifact(a Artifact, allowedRegistries, allowedPrefixes []string) error {
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
		if r != "" && r == host { // comparacion exacta: ni mayusculas, ni sufijos, ni puerto
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
	} else {
		c := strings.LastIndex(rest, ":")
		if c < 0 || strings.Contains(rest[c:], "/") {
			return fmt.Errorf("%w: la imagen debe llevar tag fijado", ErrInvalidArtifact)
		}
		tag := rest[c+1:]
		if !tagRe.MatchString(tag) || strings.EqualFold(tag, "latest") {
			return fmt.Errorf("%w: tag %q no permitido", ErrInvalidArtifact, tag)
		}
	}
	repo := imageRepo(ref)
	for _, p := range allowedPrefixes {
		if p != "" && p == repo { // exacto hasta ':' o '@': ni prefijos sin límite, ni rutas, ni mayúsculas
			return nil
		}
	}
	return fmt.Errorf("%w: %q", ErrImageNotAllowed, repo)
}

// DefaultAllowedRegistries son los registros permitidos si WARM_ALLOWED_REGISTRIES no se define.
var DefaultAllowedRegistries = []string{"ghcr.io"}
