// Package probe consulta el exterior del Service del warm por HTTP (nunca el código fuente).
package probe

import (
	"context"
	"io"
	"net/http"
	"time"
)

// HTTP implementa warmmanager.SurfaceProber.
type HTTP struct {
	Base   string
	Client *http.Client
}

// BaseURL devuelve la URL del Service.
func (h *HTTP) BaseURL() string { return h.Base }

// Get hace GET base+path (cuerpo limitado a 2 MiB, sin seguir redirecciones).
func (h *HTTP) Get(ctx context.Context, path string) (int, []byte, error) {
	c := h.Client
	if c == nil {
		c = &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.Base+path, nil)
	if err != nil {
		return 0, nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	return resp.StatusCode, b, err
}
