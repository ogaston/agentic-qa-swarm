// aqs-runner ejecuta los pasos HTTP de un flujo contra el warm. Es la imagen que usan los Jobs de
// go-run-controller: `rehearse` (Job de ensayo, lee REHEARSAL_STEPS y REHEARSAL_INVARIANT) y
// `http-steps` (Job runner, lee RUNNER_STEPS y RUNNER_INVARIANT). Sin LLM, sin credenciales y sin
// egress más allá de --target. Sale 0 si todos los pasos cumplen su expect_status, 1 si algún paso
// no, y 2 si la configuración es inválida (en ese caso no se hace ninguna petición).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	exitOK      = 0
	exitFailed  = 1
	exitConfig  = 2
	stepTimeout = 5 * time.Second
	maxBodyRead = 64 << 10
)

// step es el contrato de plan.Step (services/go-run-controller/plan/plan.go).
type step struct {
	Method       string `json:"method"`
	Path         string `json:"path"`
	ExpectStatus int    `json:"expect_status"`
}

// result es el resumen JSON por paso que sale en stdout (una línea por paso).
type result struct {
	FlowID       string `json:"flow_id"`
	Invariant    string `json:"invariant"`
	Step         int    `json:"step"`
	Method       string `json:"method"`
	Path         string `json:"path"`
	ExpectStatus int    `json:"expect_status"`
	Status       int    `json:"status"` // 0 si no hubo respuesta
	OK           bool   `json:"ok"`
	Error        string `json:"error,omitempty"`
}

// doer es lo único que el binario usa para hablar HTTP; los tests lo sustituyen.
type doer interface {
	Do(*http.Request) (*http.Response, error)
}

// newClient devuelve el cliente de producción: timeout por paso y sin seguir redirecciones, para que
// el estado observado sea el del destino y no el de otro host.
func newClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `uso:
  aqs-runner rehearse   --run <id> --flow <id> --target <url>   (lee REHEARSAL_STEPS, REHEARSAL_INVARIANT)
  aqs-runner http-steps --flow <id> --target <url>              (lee RUNNER_STEPS, RUNNER_INVARIANT)
--target debe ser http(s)://host[:puerto]. Cada paso es un JSON array de {method, path, expect_status}.
Salida: un JSON por paso en stdout. Códigos: 0 todos cumplen, 1 algún paso falla, 2 configuración inválida.
`)
}

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr, newClient(stepTimeout)))
}

func run(args []string, getenv func(string) string, stdout, stderr io.Writer, do doer) int {
	if len(args) == 0 {
		usage(stderr)
		return exitConfig
	}
	switch args[0] {
	case "-h", "--help", "help":
		usage(stdout)
		return exitOK
	case "rehearse":
		return execute("rehearse", args[1:], getenv, "REHEARSAL_STEPS", "REHEARSAL_INVARIANT", stdout, stderr, do)
	case "http-steps":
		return execute("http-steps", args[1:], getenv, "RUNNER_STEPS", "RUNNER_INVARIANT", stdout, stderr, do)
	default:
		fmt.Fprintf(stderr, "subcomando desconocido %q\n", args[0])
		usage(stderr)
		return exitConfig
	}
}

func execute(name string, args []string, getenv func(string) string, stepsVar, invVar string,
	stdout, stderr io.Writer, do doer) int {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	var runID, flowID, target string
	if name == "rehearse" {
		fs.StringVar(&runID, "run", "", "id de la corrida")
	}
	fs.StringVar(&flowID, "flow", "", "id del flujo")
	fs.StringVar(&target, "target", "", "destino http(s)://host[:puerto]")
	if err := fs.Parse(args); err != nil {
		return exitConfig
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "%s: argumento sobrante %q\n", name, fs.Arg(0))
		return exitConfig
	}
	if name == "rehearse" && runID == "" {
		fmt.Fprintln(stderr, "rehearse: --run es obligatorio")
		return exitConfig
	}
	if flowID == "" {
		fmt.Fprintf(stderr, "%s: --flow es obligatorio\n", name)
		return exitConfig
	}
	base, err := parseTarget(target)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return exitConfig
	}
	invariant := getenv(invVar)
	if strings.TrimSpace(invariant) == "" {
		fmt.Fprintf(stderr, "%s: falta %s\n", name, invVar)
		return exitConfig
	}
	steps, err := parseSteps(getenv(stepsVar))
	if err != nil {
		fmt.Fprintf(stderr, "%s: %s: %v\n", name, stepsVar, err)
		return exitConfig
	}
	// Todas las URL se validan ANTES de la primera petición: ninguna puede salir del destino.
	urls := make([]string, len(steps))
	for i, s := range steps {
		u, err := stepURL(base, s.Path)
		if err != nil {
			fmt.Fprintf(stderr, "%s: paso %d: %v\n", name, i+1, err)
			return exitConfig
		}
		urls[i] = u
	}

	enc := json.NewEncoder(stdout)
	failed := false
	for i, s := range steps {
		r := runStep(do, s, urls[i])
		r.FlowID, r.Invariant, r.Step = flowID, invariant, i+1
		if !r.OK {
			failed = true
		}
		if err := enc.Encode(r); err != nil {
			fmt.Fprintf(stderr, "%s: escribiendo resultado: %v\n", name, err)
			return exitFailed
		}
	}
	if failed {
		return exitFailed
	}
	return exitOK
}

// parseTarget exige http(s) con host y sin query, fragmento ni credenciales; devuelve la base sin barra final.
func parseTarget(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("--target no es una URL válida")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("--target debe empezar por http:// o https://")
	}
	if u.Host == "" || u.Hostname() == "" {
		return "", errors.New("--target no tiene host")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("--target no admite credenciales, query ni fragmento")
	}
	return strings.TrimRight(raw, "/"), nil
}

var allowedMethods = map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}

func parseSteps(raw string) ([]step, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("vacío")
	}
	var steps []step
	if err := json.Unmarshal([]byte(raw), &steps); err != nil {
		return nil, fmt.Errorf("JSON inválido: %w", err)
	}
	if len(steps) == 0 {
		return nil, errors.New("sin pasos")
	}
	for i, s := range steps {
		switch {
		case !allowedMethods[s.Method]:
			return nil, fmt.Errorf("paso %d: método %q no permitido", i+1, s.Method)
		case !strings.HasPrefix(s.Path, "/"):
			return nil, fmt.Errorf("paso %d: la ruta %q debe empezar por /", i+1, s.Path)
		case s.ExpectStatus < 100 || s.ExpectStatus > 599:
			return nil, fmt.Errorf("paso %d: expect_status %d fuera de 100..599", i+1, s.ExpectStatus)
		}
	}
	return steps, nil
}

// stepURL une la base y la ruta, y exige que el host resultante sea el del destino.
func stepURL(base, path string) (string, error) {
	full := base + path
	bu, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	fu, err := url.Parse(full)
	if err != nil {
		return "", fmt.Errorf("ruta %q inválida", path)
	}
	if fu.Host != bu.Host {
		return "", fmt.Errorf("la ruta %q cambia de host", path)
	}
	return full, nil
}

func runStep(do doer, s step, target string) result {
	r := result{Method: s.Method, Path: s.Path, ExpectStatus: s.ExpectStatus}
	ctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, s.Method, target, bytes.NewReader(nil))
	if err != nil {
		r.Error = err.Error()
		return r
	}
	req.Header.Set("User-Agent", "aqs-runner/0.0.0")
	resp, err := do.Do(req)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBodyRead))
	r.Status = resp.StatusCode
	r.OK = resp.StatusCode == s.ExpectStatus
	if !r.OK {
		r.Error = fmt.Sprintf("esperado %d, recibido %d", s.ExpectStatus, resp.StatusCode)
	}
	return r
}
