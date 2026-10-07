package gen

import (
	"strings"

	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/artifact"
	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/intake"
)

// Este archivo enumera DETERMINISTAMENTE los casos de frontera. Las propiedades
// los recorren todos en cada ejecución (además de los sorteos de rapid), de modo
// que un mutante de límite no dependa del muestreo para morir.

const baseSHA = "0123456789abcdef0123456789abcdef01234567"

// FixedBadSHAs son valores que no son 40 hex en minúsculas: vacío, cada largo de 0
// a 39, 41 y más, basura delante y detrás, mayúsculas puras y con una sola
// mayúscula en cada posición con letra, y un carácter no hex en CADA posición.
func FixedBadSHAs() []string {
	out := []string{""}
	for i := 1; i < 40; i++ {
		out = append(out, baseSHA[:i])
	}
	out = append(out, baseSHA+"0", baseSHA+"a", baseSHA+"g", baseSHA+baseSHA, baseSHA+"\n", baseSHA+" ", baseSHA+"\r\n",
		baseSHA+"@main", " "+baseSHA, "\n"+baseSHA, "x"+baseSHA, "0x"+baseSHA[:38], baseSHA[:39]+"\n",
		strings.ToUpper(baseSHA), strings.Repeat("A", 40), strings.Repeat("F", 40), strings.Repeat("E", 40),
		strings.Repeat("g", 40), strings.Repeat("z", 40), strings.Repeat(" ", 40), strings.Repeat("٣", 40), "٣"+baseSHA[1:], baseSHA[:39]+"٣")
	for i := 0; i < 40; i++ {
		for _, c := range []string{"g", "G", "-", " ", "\n"} {
			out = append(out, baseSHA[:i]+c+baseSHA[i+1:])
		}
		if ch := baseSHA[i]; ch >= 'a' && ch <= 'f' {
			out = append(out, baseSHA[:i]+strings.ToUpper(string(ch))+baseSHA[i+1:])
		}
	}
	return out
}

// FixedValidSHAs son SHAs válidos de frontera (solo dígitos, solo letras, mezcla).
func FixedValidSHAs() []string {
	return []string{baseSHA, strings.Repeat("0", 40), strings.Repeat("9", 40), strings.Repeat("a", 40), strings.Repeat("f", 40), "abcdef0123456789abcdef0123456789abcdef01"}
}

// FixedBadTags son tags que Resolve rechaza: vacío, las 64 capitalizaciones de
// latest, comienzo prohibido, 129 caracteres, barra final y otros caracteres.
func FixedBadTags() []string {
	out := []string{"", "-x", ".x", "-", ".", "..", "v1/", "/", "//", "a/b", "rel/v1", "/v1", "v 1", " v1", "v1 ", "v1\n", "\nv1", "v\t1", "é", "日本", "vé", "v1:x", "v1@x", "v+1", "v#1", "v1?", "v1*", "v1~", "v1^", "v1\\x"}
	for mask := 0; mask < 64; mask++ {
		b := []byte("latest")
		for i := range b {
			if mask&(1<<i) != 0 {
				b[i] -= 'a' - 'A'
			}
		}
		out = append(out, string(b))
	}
	out = append(out, "a"+strings.Repeat("b", 128), "_"+strings.Repeat("b", 128), "1"+strings.Repeat(".", 128), "a"+strings.Repeat("-", 130), strings.Repeat("a", 129), strings.Repeat("a", 200))
	return out
}

// FixedValidTags son tags válidos de frontera: un carácter de cada clase,
// "latest" como prefijo o sufijo, 127 y 128 caracteres.
func FixedValidTags() []string {
	out := []string{"v1", "v1.2.3", "latest-1", "latest.1", "latest_x", "latest1", "mylatest", "_latest", "latests", "V1", "1", "0", "9", "a", "z", "A", "Z", "_", "_a", "a.b", "a-b", "a_b", "a.", "a-", "a_", "v.", "v..", "1.0.0-rc.1", "RELEASE_2026-01-15"}
	out = append(out, "a"+strings.Repeat("b", 127), "_"+strings.Repeat("-", 127), "9"+strings.Repeat(".", 127), strings.Repeat("Z", 128), "a"+strings.Repeat("b", 126))
	return out
}

// FixedBadRegistries son valores de ARTIFACT_REGISTRY que Resolve rechaza.
func FixedBadRegistries() []string {
	return []string{"ghcr.io/", "ghcr.io//x", "ghcr.io:abc", "ghcr.io:", "a_b.io", "reg istry", "ghcr.io/ac me", "é.io", "ghcr.io:5000:1",
		"ghcr.io/x/", "/ghcr.io", "https://ghcr.io", "ghcr.io\n", "\nghcr.io", ":5000", "ghcr.io@x", "ghcr.io/a:b", "ghcr.io/a b", "g hcr.io",
		"ghcr.io:5000/", "ghcr.io:5000//a", "ghcr.io:+1", "ghcr.io:1a", "ghcr.io:-1", "gh_cr.io:5000", " ghcr.io", "ghcr.io ", "ghcr.io\t", "a/b/", "a//b", "/", "//", ":", "@", "ghcr.io/ñ", "ghcr.io?x", "ghcr.io#x", "ghcr.io*"}
}

// FixedValidRegistries son registros válidos ("" = ghcr.io por defecto).
func FixedValidRegistries() []string {
	return []string{"", "ghcr.io", "Registry.Example.com:5000", "localhost:5000", "a", "a.b-c", "reg:1", "r/s", "reg.io:5000/team/sub", "A.B/C_d.e-f", "x-.-y", "0", "reg.io:65535", "reg.io:0"}
}

// FixedBadRepos son repositorios que Resolve rechaza.
func FixedBadRepos() []string {
	return []string{"", "solo", "a/b/c", "/b", "a/", "/", "//", "a b/c", "a/b c", "ñ/x", "a/ñ", "a/b\n", "\na/b", " a/b", "a/b ", "a@b/c", "a/b:c", "a:b/c",
		"a/b/", "/a/b", "a\\b", "a+b/c", "a/b+c", "a/b!", "a?/b", "a/b#", "a/b%", "a/b~", "a/b$", "a/b*", "a/b(", "a/b,", "a/b;", "a/b=", "a/b'", "a/b\"", "日本/x", "a/日本"}
}

// FixedValidRepos son repositorios válidos de frontera para Resolve (el patrón
// admite . _ - en ambos lados, incluso nombres como "." y "..").
func FixedValidRepos() []string {
	return []string{"acme/shop", "Acme/Shop", "a/b", "a.b/c.d", "a_b/c_d", "a-b/c-d", "0/0", "_/_", "./.", "../..", "-/-", "9/9", "Z/z",
		"A" + strings.Repeat("b", 38) + "/" + strings.Repeat("c", 100)}
}

// FixedResolvable recorre commit, PR del mismo repositorio (con mayúsculas),
// tag válido y registro válido sobre las listas de frontera anteriores.
func FixedResolvable() []ResolveCase {
	var out []ResolveCase
	for _, repo := range FixedValidRepos() {
		for _, sha := range FixedValidSHAs() {
			out = append(out, ResolveCase{Event: artifact.Event{GithubEvent: artifact.EventCommit, Repo: repo, SHA: sha}},
				ResolveCase{Event: artifact.Event{GithubEvent: artifact.EventPullRequest, Repo: repo, SHA: sha, HeadRepo: repo}},
				ResolveCase{Event: artifact.Event{GithubEvent: artifact.EventPullRequest, Repo: repo, SHA: sha, HeadRepo: strings.ToUpper(repo)}},
				ResolveCase{Event: artifact.Event{GithubEvent: artifact.EventPullRequest, Repo: repo, SHA: sha, HeadRepo: strings.ToLower(repo)}})
		}
		for _, tag := range FixedValidTags() {
			for _, reg := range FixedValidRegistries() {
				out = append(out, ResolveCase{Registry: reg, Event: artifact.Event{GithubEvent: artifact.EventTag, Repo: repo, SHA: baseSHA, Tag: tag}})
			}
		}
	}
	return out
}

// FixedUnresolvable recorre todos los límites que Resolve debe rechazar.
func FixedUnresolvable() []ResolveCase {
	var out []ResolveCase
	repo := "acme/shop"
	for _, sha := range FixedBadSHAs() {
		out = append(out,
			ResolveCase{Event: artifact.Event{GithubEvent: artifact.EventCommit, Repo: repo, SHA: sha}},
			ResolveCase{Event: artifact.Event{GithubEvent: artifact.EventPullRequest, Repo: repo, SHA: sha, HeadRepo: repo}})
	}
	for _, tag := range FixedBadTags() {
		out = append(out, ResolveCase{Event: artifact.Event{GithubEvent: artifact.EventTag, Repo: repo, SHA: baseSHA, Tag: tag}},
			ResolveCase{Registry: "ghcr.io", Event: artifact.Event{GithubEvent: artifact.EventTag, Repo: repo, SHA: baseSHA, Tag: tag}})
	}
	for _, reg := range FixedBadRegistries() {
		out = append(out, ResolveCase{Registry: reg, Event: artifact.Event{GithubEvent: artifact.EventTag, Repo: repo, SHA: baseSHA, Tag: "v1"}})
	}
	for _, r := range FixedBadRepos() {
		for _, ev := range []string{artifact.EventCommit, artifact.EventTag} {
			out = append(out, ResolveCase{Event: artifact.Event{GithubEvent: ev, Repo: r, SHA: baseSHA, Tag: "v1", HeadRepo: r}})
		}
		out = append(out, ResolveCase{Event: artifact.Event{GithubEvent: artifact.EventPullRequest, Repo: r, SHA: baseSHA, HeadRepo: r}})
	}
	for _, head := range []string{"", "other/shop", "acme/shop2", "acme/shopx", "acme2/shop", "acme/shop/", "acme/shop\n", " acme/shop", "acme/sho", "cme/shop", "a/shop"} {
		out = append(out, ResolveCase{Event: artifact.Event{GithubEvent: artifact.EventPullRequest, Repo: repo, SHA: baseSHA, HeadRepo: head}})
	}
	for _, ev := range []string{"", "branch", "Commit", "COMMIT", "release", "Tag", "TAG", "tag ", " tag", "push", "pull_request ", "Pull_Request", "PULL_REQUEST", "pr", "commit\n"} {
		out = append(out, ResolveCase{Event: artifact.Event{GithubEvent: ev, Repo: repo, SHA: baseSHA, Tag: "v1", HeadRepo: repo}})
	}
	return out
}

// FixedSecretLengths son los largos de secreto de frontera de HMAC-SHA256
// (bloque de 64 bytes): 0 no es un secreto válido para Verify.
var FixedSecretLengths = []int{0, 1, 2, 15, 16, 17, 31, 32, 33, 47, 48, 63, 64, 65, 96, 127, 128, 129, 200, 1000}

// FixedSecret es un secreto determinista de n bytes (sin repetir el patrón entre
// posiciones vecinas, para que truncar o rellenar cambie el HMAC).
func FixedSecret(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*37 + n*11 + 1)
	}
	return b
}

// FixedBodies son cuerpos de frontera (vacío, 1, límites de bloque, grande).
func FixedBodies() [][]byte {
	var out [][]byte
	for _, n := range []int{0, 1, 55, 56, 63, 64, 65, 300, 4096} {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(i*13 + n)
		}
		out = append(out, b)
	}
	return out
}

// FixedRejected recorre, para cada evento, todas las ramas que Classify debe
// rechazar, cada una sobre un cuerpo que de otro modo se aceptaría.
func FixedRejected() []RejectedCase {
	repo := map[string]any{"full_name": "acme/shop"}
	sha := baseSHA
	push := func(ref, after string) []byte {
		return mustJSON(map[string]any{"ref": ref, "after": after, "repository": repo})
	}
	pr := func(action, headSHA string, r map[string]any) []byte {
		return mustJSON(map[string]any{"action": action, "pull_request": map[string]any{"head": map[string]any{"sha": headSHA}}, "repository": r})
	}
	rel := func(action, commitish string, r map[string]any) []byte {
		return mustJSON(map[string]any{"action": action, "release": map[string]any{"tag_name": "v1", "target_commitish": commitish}, "repository": r})
	}
	var out []RejectedCase
	add := func(class, ev string, body []byte, want error) {
		out = append(out, RejectedCase{Class: class, Event: ev, Body: body, Want: want})
	}
	for _, ref := range []string{"", "refs/notes/x", "refs/pull/1/head", "heads/main", "tags/v1", "refs/HEADS/x", "refs/Heads/x", "REFS/HEADS/x", "refs/Tags/v1", "REFS/TAGS/v1",
		"refs/headsX", "refs/headsX/main", "refs/heads", "refs/tags", "refs/tagsX/v1", "refs/tagsv1", "refs/headsmain", "/refs/heads/x", " refs/heads/x", " refs/tags/v1",
		"refs/head/x", "refs/tag/v1", "x/refs/tags/v1", "x/refs/heads/main", "xrefs/heads/main", "xrefs/tags/v1", "origin/refs/heads/main", "refs/remotes/origin/main",
		"refs//heads/x", "refs//tags/v1", "refs/heads\n", "refs/tags\n", "\nrefs/heads/x", "refs\\heads\\x", "ref/heads/x", "refs/head", "refs/tags.", "refs/heads."} {
		add("push-ref", "push", push(ref, sha), intake.ErrUnsupported)
	}
	for _, ref := range []string{"refs/heads/x", "refs/heads/", "refs/tags/v1", "refs/tags/"} {
		add("push-delete", "push", push(ref, ZeroSHA), intake.ErrUnsupported)
	}
	for _, act := range []string{"closed", "edited", "labeled", "unlabeled", "assigned", "unassigned", "review_requested", "review_request_removed", "locked", "unlocked",
		"ready_for_review", "converted_to_draft", "auto_merge_enabled", "synchronized", "open", "reopen", "", "OPENED", "Opened", "SYNCHRONIZE", "Reopened", "opened ", " opened", "opened\n", "synchronize ", "closed "} {
		add("pr-action", "pull_request", pr(act, sha, repo), intake.ErrUnsupported)
	}
	for _, act := range []string{"created", "edited", "prereleased", "released", "deleted", "unpublished", "", "Published", "PUBLISHED", "published ", " published", "published\n", "publish", "publishe"} {
		add("release-action", "release", rel(act, sha, repo), intake.ErrUnsupported)
	}
	for _, ev := range []string{"ping", "issues", "star", "watch", "create", "delete", "tag", "commit", "branch", "pr", "releases", "pull_requests", "", " ", "Push", "PUSH", "push ", " push", "push\n"} {
		add("event-name", ev, push("refs/heads/x", sha), intake.ErrUnsupported)
	}
	for _, ev := range []string{"Pull_Request", "PULL_REQUEST", "pull_request ", "pull-request", "pullrequest", "pull_requests"} {
		add("event-name-pr", ev, pr("opened", sha, repo), intake.ErrUnsupported)
	}
	for _, ev := range []string{"Release", "RELEASE", "release ", "releases", "released"} {
		add("event-name-release", ev, rel("published", sha, repo), intake.ErrUnsupported)
	}
	for _, bad := range FixedBadSHAs() {
		add("push-bad-sha", "push", push("refs/heads/x", bad), intake.ErrInvalidPayload)
		add("push-tag-bad-sha", "push", push("refs/tags/v1", bad), intake.ErrInvalidPayload)
		add("pr-bad-sha", "pull_request", pr("opened", bad, repo), intake.ErrInvalidPayload)
		add("release-bad-sha", "release", rel("published", bad, repo), intake.ErrInvalidPayload)
	}
	for _, c := range []string{"main", "master", "HEAD", "v1", "refs/heads/main"} {
		add("release-branch-commitish", "release", rel("published", c, repo), intake.ErrInvalidPayload)
	}
	for _, r := range []map[string]any{{"full_name": ""}, {}, {"full_name": nil}, {"fullname": "acme/shop"}} {
		add("push-no-repo", "push", mustJSON(map[string]any{"ref": "refs/heads/x", "after": sha, "repository": r}), intake.ErrInvalidPayload)
		add("pr-no-repo", "pull_request", pr("opened", sha, r), intake.ErrInvalidPayload)
		add("release-no-repo", "release", rel("published", sha, r), intake.ErrInvalidPayload)
	}
	add("push-sin-repository", "push", mustJSON(map[string]any{"ref": "refs/heads/x", "after": sha}), intake.ErrInvalidPayload)
	add("push-sin-after", "push", mustJSON(map[string]any{"ref": "refs/heads/x", "repository": repo}), intake.ErrInvalidPayload)
	add("pr-sin-head", "pull_request", mustJSON(map[string]any{"action": "opened", "repository": repo}), intake.ErrInvalidPayload)
	return out
}

// FixedValidClassified son webhooks válidos de frontera con su clasificación.
func FixedValidClassified() []GitHubCase {
	repo := map[string]any{"full_name": "acme/shop"}
	sha := baseSHA
	var out []GitHubCase
	for _, ref := range []string{"refs/heads/main", "refs/heads/", "refs/heads/a/b", "refs/heads/refs/tags/x"} {
		out = append(out, GitHubCase{Class: "push-branch", Event: "push", Body: mustJSON(map[string]any{"ref": ref, "after": sha, "repository": repo}),
			Want: intake.Classified{GithubEvent: intake.EventCommit, Repo: "acme/shop", SHA: sha}})
	}
	for _, tag := range []string{"v1", "", "v1/", "/", "rel/v1", "heads/x", "refs/tags/x", "latest", "LATEST", " v1", "v1 ", " v1 ", "v1\n", "\tv1", "v 1", "\nv1\n"} {
		out = append(out, GitHubCase{Class: "push-tag", Event: "push", Body: mustJSON(map[string]any{"ref": "refs/tags/" + tag, "after": sha, "repository": repo}),
			Want: intake.Classified{GithubEvent: intake.EventTag, Repo: "acme/shop", SHA: sha, Tag: tag}})
		out = append(out, GitHubCase{Class: "release", Event: "release", Body: mustJSON(map[string]any{"action": "published", "release": map[string]any{"tag_name": tag, "target_commitish": sha}, "repository": repo}),
			Want: intake.Classified{GithubEvent: intake.EventTag, Repo: "acme/shop", SHA: sha, Tag: tag}})
	}
	for _, act := range []string{"opened", "synchronize", "reopened"} {
		for _, hr := range []any{nil, map[string]any{"full_name": "acme/shop"}, map[string]any{"full_name": "ACME/SHOP"}, map[string]any{"full_name": "fork/shop"}} {
			want := ""
			if m, ok := hr.(map[string]any); ok {
				want = m["full_name"].(string)
			}
			out = append(out, GitHubCase{Class: "pr", Event: "pull_request", Body: mustJSON(map[string]any{"action": act, "pull_request": map[string]any{"head": map[string]any{"sha": sha, "repo": hr}}, "repository": repo}),
				Want: intake.Classified{GithubEvent: intake.EventPullRequest, Repo: "acme/shop", SHA: sha, HeadRepo: want}})
		}
	}
	for _, s := range FixedValidSHAs() { // el SHA cero solo es "borrado" en un push: en PR y release es válido
		if s != ZeroSHA {
			out = append(out, GitHubCase{Class: "push-branch", Event: "push", Body: mustJSON(map[string]any{"ref": "refs/heads/x", "after": s, "repository": repo}),
				Want: intake.Classified{GithubEvent: intake.EventCommit, Repo: "acme/shop", SHA: s}})
		}
		out = append(out, GitHubCase{Class: "pr", Event: "pull_request", Body: mustJSON(map[string]any{"action": "opened", "pull_request": map[string]any{"head": map[string]any{"sha": s}}, "repository": repo}),
			Want: intake.Classified{GithubEvent: intake.EventPullRequest, Repo: "acme/shop", SHA: s}},
			GitHubCase{Class: "release", Event: "release", Body: mustJSON(map[string]any{"action": "published", "release": map[string]any{"tag_name": "v1", "target_commitish": s}, "repository": repo}),
				Want: intake.Classified{GithubEvent: intake.EventTag, Repo: "acme/shop", SHA: s, Tag: "v1"}})
	}
	return out
}
