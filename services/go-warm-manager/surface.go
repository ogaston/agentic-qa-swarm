package warmmanager

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	baseURLRe = regexp.MustCompile(`^https?://`)
	pathRe    = regexp.MustCompile(`^/`)
)

// ValidateSurface valida el SurfaceArtifact en tiempo de ejecucion con las mismas reglas que
// contracts/plans/surface-artifact.schema.json (run_id no vacio, base_url uri con ^https?://,
// endpoints con method del enum y path ^/, source openapi|probe). Una prueba de paridad la
// contrasta con el esquema real.
func ValidateSurface(sa SurfaceArtifact) error {
	if sa.RunID == "" {
		return fmt.Errorf("surface: run_id vacio")
	}
	u, err := url.Parse(sa.BaseURL)
	if err != nil || !baseURLRe.MatchString(sa.BaseURL) || u.Host == "" {
		return fmt.Errorf("surface: base_url %q no es http(s)://host", sa.BaseURL)
	}
	if sa.Endpoints == nil {
		return fmt.Errorf("surface: endpoints ausente")
	}
	for _, e := range sa.Endpoints {
		if _, ok := methods[lower(e.Method)]; !ok || e.Method != upper(e.Method) {
			return fmt.Errorf("surface: method %q fuera del enum", e.Method)
		}
		if !pathRe.MatchString(e.Path) {
			return fmt.Errorf("surface: path %q no empieza con /", e.Path)
		}
	}
	if sa.Source != "openapi" && sa.Source != "probe" {
		return fmt.Errorf("surface: source %q fuera del enum", sa.Source)
	}
	return nil
}

func lower(s string) string { return strings.ToLower(s) }
func upper(s string) string { return strings.ToUpper(s) }
