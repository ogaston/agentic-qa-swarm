package warmmanager

import (
	"context"
	"time"
)

// RealClock es el reloj de pared.
type RealClock struct{}

// Now devuelve la hora actual.
func (RealClock) Now() time.Time { return time.Now() }

// Sleep espera d o hasta que ctx termine.
func (RealClock) Sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
