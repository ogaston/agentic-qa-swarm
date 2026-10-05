package obs

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIntakeMetricsStartAtZeroAndCount(t *testing.T) {
	reg := NewRegistry()
	m := NewIntake(reg)
	h := Wrap(Config{Service: "go-intake", Registry: reg, Metrics: NewHTTPMetrics(reg, "go-intake"),
		Log: NewLogger(&strings.Builder{}, "go-intake", 0)}, nil)
	scrape := func() string {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
		return rec.Body.String()
	}
	out := scrape()
	want := []string{"aqs_intake_notifications_created_total 0", "aqs_intake_publish_failures_total 0"}
	for _, r := range RejectReasons {
		want = append(want, `aqs_intake_webhook_rejected_total{reason="`+r+`"} 0`)
	}
	for _, w := range want {
		if !strings.Contains(out, w+"\n") {
			t.Errorf("falta la serie inicial %q", w)
		}
	}
	m.Created()
	m.PublishFailed()
	m.Rejected("too_large")
	m.Rejected("motivo-inventado") // se agrupa: la cardinalidad es cerrada
	out = scrape()
	for _, w := range []string{"aqs_intake_notifications_created_total 1", "aqs_intake_publish_failures_total 1",
		`aqs_intake_webhook_rejected_total{reason="too_large"} 1`, `aqs_intake_webhook_rejected_total{reason="bad_request"} 1`} {
		if !strings.Contains(out, w+"\n") {
			t.Errorf("falta %q", w)
		}
	}
	if strings.Contains(out, "motivo-inventado") {
		t.Error("un motivo desconocido no debe crear una serie")
	}
	var nilm *Intake
	nilm.Created()
	nilm.Rejected("x")
	nilm.PublishFailed()
}
