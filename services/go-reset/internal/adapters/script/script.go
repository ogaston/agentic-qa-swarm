// Package script es el DatabaseCleaner que ejecuta RESET_BASELINE_SCRIPT.
// Contrato del script: `script clean` deja la DB en el baseline; `script verify` imprime en stdout
// el número de filas que difieren del baseline (0 = limpia).
package script

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Cleaner struct {
	Path    string
	Timeout time.Duration // obligatorio (viene de RESET_SCRIPT_TIMEOUT)
	// StaticVersion (RESET_BASELINE_VERSION) tiene prioridad; si está vacía, `script version` la informa.
	StaticVersion string
}

// Version devuelve la versión del baseline o error si no se conoce.
func (c *Cleaner) Version(ctx context.Context) (string, error) {
	if c.StaticVersion != "" {
		return c.StaticVersion, nil
	}
	o, err := c.run(ctx, "version")
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(o)
	if v == "" {
		return "", fmt.Errorf("`version` no imprimió nada")
	}
	return v, nil
}

func (c *Cleaner) run(ctx context.Context, arg string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	var out, errb bytes.Buffer
	cmd := exec.CommandContext(ctx, c.Path, arg) //nolint:gosec // ruta fijada por configuración
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s %s: %w", c.Path, arg, err)
	}
	return out.String(), nil
}

func (c *Cleaner) Clean(ctx context.Context) error {
	_, err := c.run(ctx, "clean")
	return err
}

func (c *Cleaner) Diff(ctx context.Context) (int, error) {
	o, err := c.run(ctx, "verify")
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(o))
	if err != nil || n < 0 {
		return 0, fmt.Errorf("salida de verify inválida: %q", strings.TrimSpace(o))
	}
	return n, nil
}
