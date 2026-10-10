#!/usr/bin/env bash
# Recorrido local de la demostración de U2 (U2-T08): sin clúster ni nube. Cada comprobación corre una
# prueba Go con fakes de Kubernetes, gate de gobernanza simulado y un stub HTTP de U3 en loopback.
# Imprime "OK|FALLA <comprobacion>" y sale distinto de 0 si alguna falla.
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root/services/go-run-controller" || exit 1
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"

fail=0
check() { # <comprobacion> <prueba>
  if out=$(go test -count=1 -race -run "^$2\$" ./cmd/go-run-controller 2>&1); then
    echo "OK $1"
  else
    echo "FALLA $1"
    echo "$out" | sed 's/^/    /' >&2
    fail=1
  fi
}

check camino-feliz TestDemoLocal_CaminoFeliz
check reset-entre-corridas TestDemoLocal_ResetEntreCorridas
check fail-closed-gobernanza TestDemoLocal_FailClosedGobernanza
check cuarentena-por-db-sucia TestDemoLocal_CuarentenaPorDBSucia
check bloqueo-fuera-de-aqs-test TestDemoLocal_BloqueoFueraDeAqsTest
exit "$fail"
