# Pruebas basadas en propiedades (PBT parcial) — go-identity

Reglas PBT cubiertas: PBT-02 (round-trips), PBT-03 (invariantes), PBT-07 (generadores), PBT-08 (seed) y PBT-09 (framework).

**Por qué `pgregory.net/rapid`** (v1.3.0, fijada en `go.mod`/`go.sum`): genera y reduce (shrinking) sin generar código, usa `testing.T`, imprime el seed al fallar y es compatible con `go 1.26.8`. No hay `testing/quick` ni `go test -fuzz`.

**Cómo correr**
- Todo: `go test -run PBT ./...` (también corre dentro del `go test ./...` normal de CI).
- Más sorteos: `go test -run PBT ./... -rapid.checks=1000`. Por defecto rapid hace 100; `TestPBT_Hash` se acota a 20 (argon2id con 19 MiB cuesta decenas de ms por hash; unos 2 s en total).

**Seed (PBT-08).** Cada binario de pruebas imprime `rapid seed: <n>` (visible con `-v`; lo fija `internal/gen.Main`). Un fallo imprime `To reproduce, specify -run=... -rapid.seed=<n>`. Se repite con el paquete concreto, el flag va **después** de los paquetes: `go test -run PBT ./internal/totp -rapid.seed=<n>`. Los failfiles de rapid están apagados para no ensuciar el árbol.

**Generadores (PBT-07):** `internal/gen` (Role, Principal, Action, Resource, Secret y variantes base32, Instant, hashes PHC válidos e inválidos sin calcular argon2, User y archivos de usuarios válidos e inválidos). Son una copia independiente de los de `go-governance` (sin `go.work` ni `replace`).

**Dónde están las propiedades:** `authz/pbt_test.go` (AuthZ contra una tabla escrita aparte), `internal/users/pbt_test.go` (archivo de usuarios), `internal/totp/pbt_test.go` (base32 y TOTP con ventana ±1 y sin reutilización), `internal/passhash/pbt_test.go`, `internal/session/pbt_test.go` (tokens de 43 caracteres y session_id únicos), `internal/gen/coverage_test.go`.

**Mutante** (parche externo; el código de producción no lleva ganchos), desde la raíz del repositorio:
`git apply services/go-identity/testdata/mutants/01-totp-window.patch`, luego `(cd services/go-identity && go test -run PBT ./...)` debe fallar con un seed y un contraejemplo reducido (`TestPBT_TOTP`), y `git apply -R <parche>` lo revierte. El parche cambia la ventana ±1 por ±10 en `internal/totp/totp.go`; si ese archivo cambia, hay que regenerarlo.
