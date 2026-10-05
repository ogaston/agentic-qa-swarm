# Pruebas basadas en propiedades (PBT parcial) — go-intake

Reglas PBT cubiertas: PBT-02 (round-trips), PBT-07 (generadores), PBT-08 (seed) y PBT-09 (framework).

**Por qué `pgregory.net/rapid`** (v1.3.0, fijada en `go.mod`/`go.sum`): genera y reduce (shrinking) sin generar código, usa `testing.T`, imprime el seed al fallar y es compatible con `go 1.26.8`. No hay `testing/quick` ni `go test -fuzz`.

**Cómo correr**
- Todo: `go test -run PBT ./...` (también corre dentro del `go test ./...` normal de CI, unos segundos).
- Más sorteos: `go test -run PBT ./internal/... -rapid.checks=1000` (por defecto 100; la cobertura de generadores sube a 500).
- Los flags de rapid van **después** de los paquetes y solo los acepta un paquete con pruebas PBT (`internal/...`); con `./...` falla `contract`, que no enlaza rapid ("flag provided but not defined").

**Seed (PBT-08).** Cada binario imprime `rapid seed: <n>` (visible con `-v`; lo fija `internal/gen.Main`). Un fallo imprime `To reproduce, specify -run=... -rapid.seed=<m>`: `<m>` puede diferir de `<n>` porque rapid suma el número de iteración al seed base; use el `<m>` del fallo: `go test -run PBT ./internal/intake -rapid.seed=<m>`. Los failfiles de rapid están apagados (no se ensucia el árbol). El shrinking sigue activo.

**Generadores (PBT-07):** `internal/gen` (Owner, RepoName, Sha40, Tag válido e inválido, GitHubPushBranch, GitHubPushTag, GitHubPullRequest con y sin fork, GitHubRelease, GitHubRejected, Notification, Record, Event, ResolvableEvent). Los de `ui-api` son una copia independiente (sin `go.work` ni `replace`); `ConfirmationReceipt` solo existe allí.

**Propiedades:** `internal/intake/pbt_test.go` (payload de GitHub: Classify(Marshal(Classify(p))) == Classify(p); rechazos; notify.created ida y vuelta y contra el esquema; almacén JSONL y de-duplicación), `internal/githubsig/pbt_test.go` (firma válida y firmas alteradas: un carácter en cada posición, truncadas, extendidas, mayúsculas), `internal/artifact/pbt_test.go` (ref nunca `:latest`; fail-closed con SHA largo o con basura, tag de 129 o con `/`, registro y repo inválidos), `internal/gen/coverage_test.go`. Cada propiedad lleva un ejemplo fijo `TestPBT_Fixed_*`. go-intake no tiene `Parse`/`MarshalEvent`: el "Marshal" de la propiedad 1 reconstruye un webhook canónico desde `intake.Classified`, y el evento se serializa con `json.Marshal`.

**Límite conocido** (candidata, sin arreglo aquí): `HMACVerifier.Verify` acepta el hex de la firma en mayúsculas (`TestPBT_Limit_SignatureHexUpperAccepted`).

**Demo:** la propiedad falsa «todo Sha40 empieza por a» solo existe con `-tags pbt_demo`: `go test -tags pbt_demo -run PBT_Demo ./internal/gen`.

**Mutantes** (parches externos; producción sin ganchos), desde la raíz del repositorio: `git apply services/go-intake/testdata/mutants/01-sign-sin-prefijo.patch`, luego `(cd services/go-intake && go test -run PBT ./...)` debe fallar con seed y contraejemplo reducido; `git apply -R <parche>` lo revierte. Parches: `01-sign-sin-prefijo`, `02-latest-solo-minusculas`, `03-pr-sin-repo-origen`, `04-almacen-primera-gana`. Si cambian `githubsig.go`, `artifact.go`, `classify.go` o `store.go`, hay que regenerarlos.
