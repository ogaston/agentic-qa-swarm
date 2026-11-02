package adapters

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

// Regla de imágenes permitidas para el deploy (U11-T03). Es copia de services/go-warm-manager/types.go
// (ValidateArtifact y ParseImagePrefixes): los módulos no se importan entre sí. La paridad se prueba con
// paridad_imagen_test.go, que comparte el mismo conjunto de casos en ambos módulos.

// WarmAllowedRegistry es el registro fijo que acepta el controlador. Debe coincidir con WARM_ALLOWED_REGISTRIES
// de go-warm-manager (su valor por defecto); si se cambia allí, la paridad deja de cubrirlo.
const WarmAllowedRegistry = "ghcr.io"

// ErrImageNotAllowed: la imagen publicada no está en WARM_ALLOWED_IMAGE_PREFIXES.
var ErrImageNotAllowed = errors.New("imagen no permitida por WARM_ALLOWED_IMAGE_PREFIXES")

// ErrInvalidImage: el artefacto del deploy no cumple el formato (kind, registro, tag fijado).
var ErrInvalidImage = errors.New("artefacto invalido")

// Mismo criterio que go-warm-manager (types.go): al menos tres segmentos, sin vacíos ni . / ..
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

var (
	imageTagRe    = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,127}$`)
	imageDigestRe = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// ValidateImage decide si el artefacto puede pedirse a go-warm-manager. Solo published-image es
// desplegable desde el controlador; build-from-repo se rechaza aquí (go-warm-manager tampoco lo despliega).
func ValidateImage(kind, ref string, prefixes []string) error {
	if kind != "published-image" {
		return fmt.Errorf("%w: kind %q no desplegable", ErrInvalidImage, kind)
	}
	slash := strings.Index(ref, "/")
	if slash <= 0 || strings.ContainsAny(ref, " \t\r\n\"'\\") {
		return fmt.Errorf("%w: la imagen debe llevar registro explicito", ErrInvalidImage)
	}
	if host := ref[:slash]; host != WarmAllowedRegistry {
		return fmt.Errorf("%w: registro no permitido %q", ErrInvalidImage, host)
	}
	rest := ref[slash+1:]
	if i := strings.Index(rest, "@sha256:"); i >= 0 {
		if !imageDigestRe.MatchString(rest[i+len("@sha256:"):]) {
			return fmt.Errorf("%w: digest invalido", ErrInvalidImage)
		}
	} else {
		c := strings.LastIndex(rest, ":")
		if c < 0 || strings.Contains(rest[c:], "/") {
			return fmt.Errorf("%w: la imagen debe llevar tag fijado", ErrInvalidImage)
		}
		tag := rest[c+1:]
		if !imageTagRe.MatchString(tag) || strings.EqualFold(tag, "latest") {
			return fmt.Errorf("%w: tag %q no permitido", ErrInvalidImage, tag)
		}
	}
	repo := ref
	if i := strings.IndexAny(ref, ":@"); i >= 0 {
		repo = ref[:i]
	}
	for _, p := range prefixes {
		if p != "" && p == repo { // exacto hasta ':' o '@'
			return nil
		}
	}
	return fmt.Errorf("%w: %q", ErrImageNotAllowed, repo)
}

// ExtractImagePrefix extrae la parte de repositorio de una referencia de imagen (sin tag ni digest).
func ExtractImagePrefix(ref string) string {
	if i := strings.IndexAny(ref, ":@"); i >= 0 {
		return ref[:i]
	}
	return ref
}

// Slug genera un slug seguro para nombres de Kubernetes a partir de un nombre de repositorio.
func Slug(repo string) string {
	r := strings.ToLower(strings.TrimSpace(repo))
	var sb strings.Builder
	lastHyphen := false
	for _, ch := range r {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			sb.WriteRune(ch)
			lastHyphen = false
		} else {
			if !lastHyphen && sb.Len() > 0 {
				sb.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	s := strings.Trim(sb.String(), "-")
	if len(s) > 40 {
		s = strings.TrimRight(s[:40], "-")
	}
	return s
}

// AllowedPrefixesForRun combina los prefijos semilla de WARM_ALLOWED_IMAGE_PREFIXES con el
// prefijo de imagen específico del repositorio si está presente en el artefacto o metadatos de la corrida.
func AllowedPrefixesForRun(run runctl.Run, seedPrefixes []string, ref string) []string {
	prefixes := append([]string(nil), seedPrefixes...)
	if run.Artifact != nil && run.Artifact.ImagePrefix != "" {
		prefixes = append(prefixes, run.Artifact.ImagePrefix)
	}
	if run.Metadata != nil {
		if p := run.Metadata["image_prefix"]; p != "" {
			prefixes = append(prefixes, p)
		}
		if p := run.Metadata["image_repo"]; p != "" {
			prefixes = append(prefixes, p)
		}
	}
	if run.Warm != nil && strings.HasPrefix(run.Warm.Namespace, "aqs-w-") {
		prefix := ExtractImagePrefix(ref)
		if strings.HasPrefix(prefix, WarmAllowedRegistry+"/") {
			repoPart := strings.TrimPrefix(prefix, WarmAllowedRegistry+"/")
			targetSlug := strings.TrimPrefix(run.Warm.Namespace, "aqs-w-")
			if Slug(repoPart) == targetSlug {
				prefixes = append(prefixes, prefix)
			}
		}
	}
	return prefixes
}
