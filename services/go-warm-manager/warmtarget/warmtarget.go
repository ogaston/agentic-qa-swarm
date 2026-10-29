package warmtarget

import (
	"fmt"
	"regexp"
	"strings"
)

// LegacyNamespace es el namespace único previo a U12.
const LegacyNamespace = "aqs-test"

// LegacyWarmID es el warm_id único previo a U12.
const LegacyWarmID = "warm-1"

var warmNamespaceRegex = regexp.MustCompile(`^aqs-w-[a-z0-9]([a-z0-9-]{0,44}[a-z0-9])?$`)

// WarmTarget representa el destino de despliegue y pruebas de un repositorio.
type WarmTarget struct {
	Repo      string `json:"repo"`
	Namespace string `json:"namespace"`
	WarmID    string `json:"warm_id"`
	AppURL    string `json:"app_url"`
}

// TargetForRepo calcula el WarmTarget a partir de un repo.
// Si repo está vacío o es el de demo pre-U12, puede devolver el target legacy si se solicita.
func TargetForRepo(repo string) WarmTarget {
	ns := NamespaceForRepo(repo)
	wid := WarmIDForRepo(repo)
	return WarmTarget{
		Repo:      strings.ToLower(strings.TrimSpace(repo)),
		Namespace: ns,
		WarmID:    wid,
		AppURL:    fmt.Sprintf("http://warm-app.%s.svc:8080", ns),
	}
}

// LegacyTarget devuelve el target para el entorno legacy aqs-test.
func LegacyTarget() WarmTarget {
	return WarmTarget{
		Repo:      "ogaston/demo-target-app-qa-swarm",
		Namespace: LegacyNamespace,
		WarmID:    LegacyWarmID,
		AppURL:    "http://warm-app.aqs-test.svc:8080",
	}
}

// Slug genera un slug seguro para nombres de Kubernetes a partir de un nombre de repositorio.
// Reglas:
// 1. Minúsculas y trim.
// 2. Reemplazo de '/' y '.' por '-'.
// 3. Cualquier caracter no alfanumérico se sustituye por '-'.
// 4. Se colapsan guiones repetidos y se eliminan guiones al inicio y fin.
// 5. Se recorta a un máximo de 40 caracteres (sin guión final).
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

// NamespaceForRepo genera el nombre del namespace para un repo: aqs-w-<slug>.
func NamespaceForRepo(repo string) string {
	return "aqs-w-" + Slug(repo)
}

// WarmIDForRepo genera el warm_id para un repo: w-<slug>.
func WarmIDForRepo(repo string) string {
	return "w-" + Slug(repo)
}

// ValidWarmNamespace verifica si un namespace es válido para un entorno warm (aqs-test o aqs-w-*).
func ValidWarmNamespace(ns string) bool {
	if ns == LegacyNamespace {
		return true
	}
	return warmNamespaceRegex.MatchString(ns)
}
