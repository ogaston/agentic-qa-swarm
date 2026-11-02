package core_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/core"
	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/fakes"
)

func TestResetHonorsResourceSelection(t *testing.T) {
	for _, tc := range []struct {
		name      string
		resources []string
		checks    []string
	}{
		{name: "stateless", resources: []string{}, checks: []string{core.CheckAppReady}},
		{name: "postgresql", resources: []string{core.ResourcePostgreSQL}, checks: []string{core.CheckAppReady, core.CheckDBBaseline}},
		{name: "redis", resources: []string{core.ResourceRedis}, checks: []string{core.CheckAppReady, core.CheckCacheEmpty}},
		{name: "both", resources: []string{core.ResourcePostgreSQL, core.ResourceRedis}, checks: []string{core.CheckAppReady, core.CheckDBBaseline, core.CheckCacheEmpty}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := fakes.NewBundle()
			b.Svc.Cfg.Resources = tc.resources
			if !core.HasResource(tc.resources, core.ResourcePostgreSQL) {
				b.DB.CleanErr, b.DB.VerErr, b.DB.Rows = fakes.ErrBoom, fakes.ErrBoom, 10
			}
			if !core.HasResource(tc.resources, core.ResourceRedis) {
				b.Cache.FlushErr, b.Cache.Keys = fakes.ErrBoom, 10
			}
			initial := core.WarmState{WarmID: "w", State: core.StateDirty, BaselineVersion: "unverified", InfrastructureReady: true}
			if err := b.State.Put(ctx, initial, time.Now()); err != nil {
				t.Fatal(err)
			}
			res, err := b.Svc.Reset(ctx, "r-resources", "")
			if err != nil || !res.Verified {
				t.Fatalf("reset = %+v, %v", res, err)
			}
			if !reflect.DeepEqual(res.Checks, tc.checks) {
				t.Fatalf("checks = %v, expected %v", res.Checks, tc.checks)
			}
			if got := state(t, b); !got.InfrastructureReady || !got.ResetVerified {
				t.Fatalf("state after reset = %+v", got)
			}
		})
	}
}

func TestLegacyResetStillRequiresBothResources(t *testing.T) {
	b := fakes.NewBundle()
	b.Svc.Cfg.Resources = nil
	b.Cache.FlushErr = fakes.ErrBoom
	res, err := b.Svc.Reset(ctx, "r-legacy", "")
	if err != nil || res.Verified || state(t, b).State != core.StateQuarantine {
		t.Fatalf("legacy reset = %+v, %v", res, err)
	}
}
