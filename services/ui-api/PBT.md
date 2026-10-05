# Pruebas basadas en propiedades (PBT parcial) — ui-api

Reglas PBT cubiertas: PBT-02 (round-trips), PBT-07 (generadores), PBT-08 (seed) y PBT-09 (framework).

**Por qué `pgregory.net/rapid`** (v1.3.0, fijada en `go.mod`/`go.sum`): genera y reduce (shrinking) sin generar código, usa `testing.T`, imprime el seed al fallar y es compatible con `go 1.26.8`. No hay `testing/quick` ni `go test -fuzz`.

**Cómo correr**
- Todo: `go test -run PBT ./...` (también corre dentro del `go test ./...` normal de CI, unos segundos).
- Más sorteos: `go test -run PBT ./inbox -rapid.checks=1000` (por defecto 100; la cobertura de generadores sube a 500 y el parser contra el esquema, a 500 como mínimo).
- Los flags de rapid van **después** de los paquetes y solo los acepta un paquete con pruebas PBT (`./inbox`, `./internal/gen`); con `./...` fallan los paquetes que no enlazan rapid ("flag provided but not defined").

**Seed (PBT-08).** Cada binario imprime `rapid seed: <n>` (visible con `-v`; lo fija `internal/gen.Main`). Un fallo imprime `To reproduce, specify -run=... -rapid.seed=<m>`: `<m>` puede diferir de `<n>` porque rapid suma el número de iteración al seed base; use el `<m>` del fallo: `go test -run PBT ./inbox -rapid.seed=<m>`. Los failfiles de rapid están apagados. El shrinking sigue activo.

**Generadores (PBT-07):** `internal/gen` (Owner, RepoName, Sha40, Tag válido e inválido, Notification en los tres estados del enum, NotifyCreated, ConfirmationReceipt, Flows). Copia independiente de la de `go-intake` (sin `go.work` ni `replace`); los generadores de webhooks de GitHub solo existen allí.

**Propiedades:** `inbox/pbt_test.go` (notify.created ida y vuelta y contra el esquema; el parser nunca es más laxo que el esquema ante eventos mutados; recibos de confirmación escribir/cerrar/reabrir; orden de `List` e idempotencia de `Apply`) y `internal/gen/coverage_test.go`. Cada una lleva un ejemplo fijo `TestPBT_Fixed_*`. `ParseNotifyCreated` es más estricto que el esquema en `version` (`1.0`), en `t`/`z` minúsculas, en claves duplicadas o con otra capitalización y en el instante cero; las propiedades lo reflejan.

**Límite conocido hallado por las propiedades** (candidata, sin arreglo aquí): el parser acepta `occurred_at` con desplazamiento `+24:00`/`-24:00`/`+23:60` o coma decimal (`10:00:00,5Z`) que el esquema rechaza. Fijado en `TestPBT_Limit_ParserAcceptsDateTimeSchemaRejects`.

**Demo:** la propiedad falsa «todo Sha40 empieza por a» solo existe con `-tags pbt_demo`: `go test -tags pbt_demo -run PBT_Demo ./internal/gen`.

**Mutantes** (parches externos), desde la raíz: `git apply services/ui-api/testdata/mutants/01-sha-sin-validar.patch`, luego `(cd services/ui-api && go test -run PBT ./...)` debe fallar con seed y contraejemplo reducido; `git apply -R <parche>` lo revierte. Parches: `01-sha-sin-validar`, `02-claves-de-data-sin-exactitud`, `03-empate-orden-de-llegada`. Si cambian `inbox/event.go` o `inbox/store.go`, hay que regenerarlos.
