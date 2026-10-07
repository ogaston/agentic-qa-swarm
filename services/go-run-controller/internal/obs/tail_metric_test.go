package obs

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestJournalTailDiscardedMetric(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewRunMetrics(reg, func() float64 { return 0 })
	m.JournalTailDiscarded()
	fams, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fams {
		if f.GetName() == "aqs_journal_tail_discarded_total" {
			if v := f.GetMetric()[0].GetCounter().GetValue(); v != 1 {
				t.Fatalf("valor %v", v)
			}
			return
		}
	}
	t.Fatal("la métrica no está registrada")
}
