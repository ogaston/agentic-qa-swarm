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
	Timeout time.Duration
}

func (c *Cleaner) run(ctx context.Context, arg string) (string, error) {
	t := c.Timeout
	if t == 0 {
		t = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, t)
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
