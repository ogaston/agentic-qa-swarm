# Pruebas basadas en propiedades (PBT parcial) — go-governance

Reglas PBT cubiertas: PBT-02 (round-trips), PBT-03 (invariantes), PBT-07 (generadores), PBT-08 (seed) y PBT-09 (framework).

**Por qué `pgregory.net/rapid`** (v1.3.0, fijada en `go.mod`/`go.sum`): genera y reduce (shrinking) sin generar código, usa `testing.T`, imprime el seed al fallar y es compatible con `go 1.26.8`. No hay `testing/quick` ni `go test -fuzz`.

**Cómo correr**
- Todo: `go test -run PBT ./...` (también corre dentro del `go test ./...` normal de CI, unos segundos).
- Más sorteos: `go test -run PBT ./... -rapid.checks=1000` (las invariantes de gates ya suben a 1000; el valor por defecto de rapid es 100).

**Seed (PBT-08).** Cada binario de pruebas imprime `rapid seed: <n>` (visible con `-v`; lo fija `internal/gen.Main`). Un fallo imprime `To reproduce, specify -run=... -rapid.seed=<n>`. Se repite con el paquete concreto, el flag va **después** de los paquetes: `go test -run PBT ./authz -rapid.seed=<n>`. Los failfiles de rapid están apagados para no ensuciar el árbol.

**Generadores (PBT-07):** `internal/gen` (Role, Principal, RunState, Fact, Namespace casi-iguales y Unicode, WorkflowName, GateInput con sesgo fronterizo, las cuatro políticas válidas e inválidas, AuditEntry). Los de `go-identity` son una copia independiente (sin `go.work` ni `replace`).

**Dónde están las propiedades:** `authz/pbt_test.go` (invariantes de gates), `internal/policy/pbt_test.go`, `internal/audit/pbt_test.go` (cadena), `internal/service/pbt_test.go` (auditoría caída, rechazo de políticas), `internal/gen/coverage_test.go`. La propiedad demostrativa que falla a propósito solo existe con `-tags pbt_demo`: `go test -tags pbt_demo -run PBT_Demo ./authz`.

**Comportamiento aceptado que reflejan las invariantes (U4-T04):** la matriz de U4-T01 manda sobre la tabla; `warm_ready` exige también `workflow_allowed`; el namespace no cuenta para `done`/`failed`; el workflow se contrasta con la política en todo destino salvo `resetting`, `reporting`, `done` y `failed`; cuota diaria y aprobación solo en `running`.

**Límites conocidos hallados por las propiedades** (candidatas, sin arreglo aquí): `audit.Verify` no detecta el borrado de la última línea (la cabeza de la cadena no está anclada) ni ignora bien bytes tras el JSON de una línea. Quedan fijados en `TestPBT_Limit_*`.

**Mutantes** (parches externos; el código de producción no lleva ganchos), desde la raíz del repositorio:
`git apply services/go-governance/testdata/mutants/01-no-confirm.patch`, luego `(cd services/go-governance && go test -run PBT ./...)` debe fallar con un seed y un contraejemplo reducido, y `git apply -R <parche>` lo revierte. Parches: `01-no-confirm`, `02-namespace-prefix`, `03-unknown-as-true`. Si cambia `authz/rules.go` o `authz/types.go`, hay que regenerarlos.
