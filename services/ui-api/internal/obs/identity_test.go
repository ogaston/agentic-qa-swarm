package obs

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestIdentityMetricsInitializedAndCount(t *testing.T) {
	reg := NewRegistry()
	open := false
	m := NewIdentity(reg, func() bool { return open })
	pending := 2
	RegisterPublishPending(reg, func() int { return pending })
	m.IdentityCall("ok")
	m.IdentityCall("circuit_open")
	open = true
	exp := `# HELP aqs_identity_calls_total Validaciones de token contra go-identity por resultado.
# TYPE aqs_identity_calls_total counter
aqs_identity_calls_total{result="circuit_open"} 1
aqs_identity_calls_total{result="error"} 0
aqs_identity_calls_total{result="ok"} 1
aqs_identity_calls_total{result="unauthorized"} 0
# HELP aqs_identity_circuit_open 1 si el circuito hacia go-identity está abierto.
# TYPE aqs_identity_circuit_open gauge
aqs_identity_circuit_open 1
# HELP aqs_inbox_publish_pending Confirmaciones cuyo run.confirmed aún no se publicó.
# TYPE aqs_inbox_publish_pending gauge
aqs_inbox_publish_pending 2
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(exp), "aqs_identity_calls_total", "aqs_identity_circuit_open", "aqs_inbox_publish_pending"); err != nil {
		t.Fatal(err)
	}
	(*Identity)(nil).IdentityCall("ok") // nil no hace nada
}
