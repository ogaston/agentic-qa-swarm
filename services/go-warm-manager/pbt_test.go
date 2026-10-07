package warmmanager_test

import (
	"context"
	"testing"

	wm "github.com/ogaston/agentic-qa-swarm/services/go-warm-manager"
	"pgregory.net/rapid"
)

// Para toda secuencia de transiciones: reset_verified solo pasa a true por la transición
// del reset verificado (->ready con reset_verified=true desde dirty/cuarentena).
func TestPBTResetVerifiedOnlyByReset(t *testing.T) {
	states := []string{"ready", "dirty", "cuarentena", "idle-escalado"}
	rapid.Check(t, func(t *rapid.T) {
		cur := wm.WarmState{WarmID: "w", State: "ready", ResetVerified: false, BaselineVersion: "b"}
		for i := 0; i < rapid.IntRange(1, 40).Draw(t, "n"); i++ {
			to := rapid.SampledFrom(states).Draw(t, "to")
			rv := rapid.Bool().Draw(t, "rv")
			next, err := wm.Transition(cur, to, rv)
			if err != nil {
				if next != cur {
					t.Fatal("una transicion ilegal cambio el estado")
				}
				continue
			}
			resetTransition := to == "ready" && rv && (cur.State == "dirty" || cur.State == "cuarentena")
			if next.ResetVerified && !cur.ResetVerified && !resetTransition {
				t.Fatalf("reset_verified paso a true por %s->%s", cur.State, to)
			}
			if (to == "dirty" || to == "cuarentena") && next.ResetVerified {
				t.Fatal("dirty/cuarentena con reset_verified")
			}
			cur = next
		}
	})
}

// Para toda secuencia de resultados de rollout: nunca mas de 3 parches por corrida.
func TestPBTAtMostThreePatches(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		outcomes := rapid.SliceOfN(rapid.Bool(), 0, 10).Draw(t, "outcomes")
		r := newRig(ws("ready", true))
		r.dep.Rollout = func(int) (bool, error) {
			n := r.dep.Patches() - 1
			return n < len(outcomes) && outcomes[n], nil
		}
		_ = r.svc.Deploy(context.Background(), "r-1", okArt, "t")
		if r.dep.Patches() > 3 || r.dep.Patches() < 1 {
			t.Fatalf("parches=%d", r.dep.Patches())
		}
	})
}
