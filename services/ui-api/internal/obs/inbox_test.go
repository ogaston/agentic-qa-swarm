package obs

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInboxMetricsStartAtZeroAndTrackCounts(t *testing.T) {
	reg := NewRegistry()
	counts := map[string]int{}
	m := NewInbox(reg, func() map[string]int { return counts })
	h := Wrap(Config{Service: "ui-api", Registry: reg, Metrics: NewHTTPMetrics(reg, "ui-api"),
		Log: NewLogger(&strings.Builder{}, "ui-api", 0)}, nil)
	scrape := func() string {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
		return rec.Body.String()
	}
	out := scrape()
	for _, w := range []string{"aqs_inbox_confirmations_total 0", `aqs_inbox_notifications{state="pending"} 0`,
		`aqs_inbox_notifications{state="confirmed"} 0`, `aqs_inbox_notifications{state="rejected"} 0`} {
		if !strings.Contains(out, w+"\n") {
			t.Errorf("falta la serie inicial %q", w)
		}
	}
	m.Confirmed()
	counts["pending"], counts["confirmed"] = 3, 2
	out = scrape()
	for _, w := range []string{"aqs_inbox_confirmations_total 1", `aqs_inbox_notifications{state="pending"} 3`, `aqs_inbox_notifications{state="confirmed"} 2`} {
		if !strings.Contains(out, w+"\n") {
			t.Errorf("falta %q", w)
		}
	}
	var nilm *Inbox
	nilm.Confirmed()
}
