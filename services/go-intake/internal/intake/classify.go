package intake

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

// Errores de clasificacion.
var (
	// ErrUnsupported: evento o accion que no genera notificacion.
	ErrUnsupported = errors.New("intake: evento no soportado")
	// ErrInvalidPayload: faltan repo o sha validos en el payload.
	ErrInvalidPayload = errors.New("intake: payload invalido")
)

const zeroSHA = "0000000000000000000000000000000000000000"

var shaRE = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Classified es el resultado de clasificar un webhook.
type Classified struct {
	GithubEvent string
	Repo        string
	SHA         string
	// Tag es el nombre del tag (github_event == tag).
	Tag string
	// HeadRepo es owner/repo de la rama origen del PR (vacio si no hay).
	HeadRepo string
}

type payload struct {
	Action     string `json:"action"`
	Ref        string `json:"ref"`
	After      string `json:"after"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	PullRequest struct {
		Head struct {
			SHA  string `json:"sha"`
			Repo struct {
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"head"`
	} `json:"pull_request"`
	Release struct {
		TagName         string `json:"tag_name"`
		TargetCommitish string `json:"target_commitish"`
	} `json:"release"`
}

// Classify traduce X-GitHub-Event + payload a github_event/repo/sha.
// Devuelve ErrUnsupported o ErrInvalidPayload; un error de JSON se devuelve tal cual.
func Classify(event string, body []byte) (Classified, error) {
	var p payload
	if err := json.Unmarshal(body, &p); err != nil {
		return Classified{}, err
	}
	var c Classified
	switch event {
	case "push":
		switch {
		case strings.HasPrefix(p.Ref, "refs/heads/"):
			c.GithubEvent = EventCommit
		case strings.HasPrefix(p.Ref, "refs/tags/"):
			c.GithubEvent = EventTag
			c.Tag = strings.TrimPrefix(p.Ref, "refs/tags/")
		default:
			return Classified{}, ErrUnsupported
		}
		if p.After == zeroSHA { // borrado de rama o tag
			return Classified{}, ErrUnsupported
		}
		c.SHA = p.After
	case "pull_request":
		switch p.Action {
		case "opened", "synchronize", "reopened":
		default:
			return Classified{}, ErrUnsupported
		}
		c.GithubEvent = EventPullRequest
		c.SHA = p.PullRequest.Head.SHA
		c.HeadRepo = p.PullRequest.Head.Repo.FullName
	case "release":
		if p.Action != "published" {
			return Classified{}, ErrUnsupported
		}
		c.GithubEvent = EventTag
		c.Tag = p.Release.TagName
		// El payload de release solo trae un SHA si target_commitish lo es;
		// si es un nombre de rama no hay SHA que notificar.
		c.SHA = p.Release.TargetCommitish
	default:
		return Classified{}, ErrUnsupported
	}
	c.Repo = p.Repository.FullName
	if c.Repo == "" || !shaRE.MatchString(c.SHA) {
		return Classified{}, ErrInvalidPayload
	}
	return c, nil
}
