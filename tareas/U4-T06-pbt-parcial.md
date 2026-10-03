# U4-T06 — PBT parcial: round-trip de políticas/config/workflows, invariantes de gates, generadores y seed (`rapid`)

**Unidad:** U4 — Gobernanza & Identidad
**Historias que implementa:** US-M8.3 (los guardrails se demuestran con propiedades, no solo con ejemplos; extensión Property-Based Testing parcial: PBT-02, PBT-03, PBT-07, PBT-08, PBT-09)
**Depende de:** U4-T02 (`go-identity` real: hashing, TOTP, usuarios) y U4-T04 (`go-governance` real: evaluador de gates, políticas, auditoría).

---

## Alcance

**Dentro** (una línea, concreta):

> Añadir pruebas basadas en propiedades con `pgregory.net/rapid` a `services/go-governance` y `services/go-identity`: generadores de dominio (PBT-07), round-trips de parseo y serialización (PBT-02), **invariantes de los gates** (PBT-03), seed registrado y reproducible (PBT-08), un `PBT.md` por servicio (PBT-09) y tres **mutantes** versionados que las propiedades deben detectar.

Propiedades mínimas (nombres que empiezan por `TestPBT`, para que `-run PBT` las seleccione).

**`go-governance` — invariantes de gates (PBT-03)**, sobre el evaluador **real** de U4-T04, con entradas generadas (estados válidos e inválidos, hechos `true|false|unknown`, namespaces arbitrarios incluidos los casi-iguales a `aqs-test`, `workflow` presente o ausente):

- `TestPBT_Invariant_NoConfirmNeverAllow`: sin `confirmed=true`, el destino ∈ {`warm_ready`, `deploying`, `inferring`, `rehearsing`, `running`} **nunca** da `Allow`.
- `TestPBT_Invariant_NoResetVerifiedNeverAllow`: sin `reset_verified=true`, destino ∈ {`warm_ready`, `deploying`} nunca da `Allow`.
- `TestPBT_Invariant_NoEnsayoNeverRunning`: sin `ensayo_passed=true`, destino `running` nunca da `Allow`.
- `TestPBT_Invariant_NamespaceNotTestNeverAllow`: con `target_namespace != aqs-test` (comparación exacta), todo destino salvo `done` y `failed` da `Deny`.
- `TestPBT_Invariant_UnknownEqualsFalse`: sustituir cualquier hecho `unknown` por `false` no cambia la decisión.
- `TestPBT_Invariant_Monotonic`: pasar un hecho exigido de `true` a `false`/`unknown` **nunca** convierte un `Deny` en `Allow`.
- `TestPBT_Invariant_IllegalTransitionNeverAllow`: un par `(from, to)` fuera del grafo legal siempre da `Deny`, con cualquier combinación de hechos.
- `TestPBT_Invariant_AuditFailureNeverAllow`: con un `AuditLog` que falla, ninguna entrada da `Allow`.
- `TestPBT_Invariant_Deterministic`: la misma entrada dos veces da la misma decisión.

**`go-governance` — round-trips (PBT-02):**

- `TestPBT_RoundTrip_Policies`: para cada política (`events`, `confirm_required`, `warm_quotas`, `workflows`) generada válida, `Parse(Marshal(p)) == p` y `Marshal` es estable (dos veces dan los mismos bytes).
- `TestPBT_RoundTrip_Workflows`: con nombres únicos y complejidades arbitrarias, incluido el límite de longitud del nombre.
- `TestPBT_RoundTrip_AuditChain`: una secuencia generada de entradas se escribe, se relee y verifica íntegra; **cualquier** alteración de un byte de **cualquier** línea (o borrar, duplicar o permutar líneas) hace fallar `verify-audit`.
- `TestPBT_PolicyRejectsInvalid`: un valor generado fuera de rango (cuotas, nombres, enumerados) siempre da error de validación y **no** crea versión.

**`go-identity` (PBT-02 y PBT-03):**

- `TestPBT_RoundTrip_UsersFile`: la carga de `IDENTITY_USERS_FILE` de `Serialize(users)` devuelve los mismos usuarios (excepto el orden normalizado), y entradas inválidas generadas (hash no argon2id, duplicado sin distinguir mayúsculas, rol fuera de {`user`,`admin`}, `admin` sin `mfa_secret`) siempre se rechazan.
- `TestPBT_RoundTrip_Base32Secret` y `TestPBT_TOTP`: un código generado para `(secreto, t)` se acepta en `t` y en `t±30 s`, se rechaza en `t±90 s`, y **no se acepta dos veces**.
- `TestPBT_Hash`: `Verify(Hash(p), p)` es verdadero y `Verify(Hash(p), p')` falso para `p' != p` (acotado a 20 comprobaciones por el coste de argon2id; se documenta).
- `TestPBT_Invariant_AuthZ`: para todo `(principal, acción, recurso)` generado, `Authorize` deniega si el rol, la acción o el tipo de recurso son desconocidos, si un `user` no es propietario, o si el recurso no tiene propietario; y **nunca** permite a un `user` lo que la tabla reserva a `admin`.
- `TestPBT_Token`: los tokens generados cumplen el patrón de 43 caracteres base64url y dos sesiones distintas nunca comparten token ni `session_id`.

**Generadores (PBT-07)** en `internal/gen` de cada servicio (duplicados, no compartidos): `Principal`, `Role` (válido e inválido), `RunState` (válido e inválido), `Fact` (tri-estado), `GateInput` (con sesgo hacia casos fronterizos), `Namespace` (incluye `aqs-test`, `AQS-TEST`, `aqs-test `, vacío, `staging`, `prod`, Unicode confundible), `WorkflowName`, cada política, `AuditEntry`, `User`, `Secret`. Una prueba `TestPBT_GeneratorCoverage` demuestra que, en 500 sorteos, cada estado, cada destino y cada combinación (`Allow`, `Deny`) aparecen al menos una vez.

**Seed (PBT-08).** Cada ejecución de `TestPBT*` imprime `rapid seed: <n>` (rapid solo lo muestra al fallar); `-rapid.seed=<n>` repite la ejecución idéntica. El **shrinking** queda activo.

**Mutantes.** Tres parches en `services/go-governance/testdata/mutants/*.patch` (`01-no-confirm.patch`, `02-namespace-prefix.patch`, `03-unknown-as-true.patch`) que, aplicados con `git apply` al evaluador real, introducen un defecto plausible (omitir la comprobación de `confirmed`; comparar el namespace con `strings.HasPrefix`; tratar `unknown` como verdadero). Cada uno **debe** hacer fallar al menos una propiedad con seed y contraejemplo reducido. Y un parche en `go-identity` (`testdata/mutants/01-totp-window.patch`: ventana ±10 en vez de ±1).

**Framework (PBT-09).** `rapid` fijado en `go.mod`/`go.sum` de ambos módulos. `services/go-governance/PBT.md` y `services/go-identity/PBT.md` (10 a 40 líneas): por qué `rapid`, cómo correr (`go test -run PBT ./...`), cómo reproducir (`-rapid.seed`), el número de comprobaciones (`-rapid.checks`), dónde están los generadores y cómo aplicar los mutantes.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Cambiar la lógica de producción para que una propiedad pase **sin** registrar el defecto: si una propiedad encuentra un defecto real, se registra en la bitácora con el contraejemplo reducido, se arregla y el arreglo lleva su prueba de ejemplo fija (regresión).
- Otras reglas PBT, fuzzing nativo (`go test -fuzz`), `testing/quick`, otras librerías.
- Pruebas de integración HTTP, de carga o contra `go-identity`/`go-governance` levantados.
- Modificar `ci.yml` (las propiedades corren dentro de `go test ./...`).
- Compartir generadores entre módulos con `replace` o `go.work` (prohibidos).
- Poner ganchos de mutación en el código de producción (el mutante es un parche externo, no una bandera de compilación).
- Cambiar `contracts/**`, `deploy/**` o workflows.

---

## Archivos de contexto

- `unidades-y-tareas.md` (sección U4)
- `aidlc-docs/inception/application-design/unit-task-plans/U4.md`
- `aidlc-docs/inception/requirements/requirements.md` (NF-PBT-01 a NF-PBT-05)
- `tareas/U4-T01-stubs.md`, `tareas/U4-T02-go-identity-autenticacion.md`, `tareas/U4-T03-autorizacion.md`, `tareas/U4-T04-go-governance.md` (reglas y tabla de gates que las invariantes deben reflejar)
- `services/go-governance/`, `services/go-identity/`
- `tareas/U1-T05-pbt-parcial.md` (mismo patrón de `-tags pbt_demo` y de `PBT.md`)

---

## Criterios de aceptación

Desde la raíz del worktree.

- [ ] **CA-1** — Las propiedades corren y pasan en ambos servicios, con seed visible y sin fallos.
  ```bash
  for m in go-governance go-identity; do (cd services/$m && l=$(mktemp) && go test -run PBT -v ./... > "$l" 2>&1; grep -c -E '^\s*--- PASS: TestPBT' "$l"; grep -c -E 'rapid seed: [0-9-]+' "$l"; grep -c FAIL "$l"); done
  ```
  Esperado: para `go-governance` un número ≥ `13`, ≥ `1` y `0`; para `go-identity` ≥ `6`, ≥ `1` y `0`. Antes de la tarea: `0 0 0` (rojo inicial).

- [ ] **CA-2** — Cada invariante de la lista existe con su nombre.
  ```bash
  for n in NoConfirmNeverAllow NoResetVerifiedNeverAllow NoEnsayoNeverRunning NamespaceNotTestNeverAllow UnknownEqualsFalse Monotonic IllegalTransitionNeverAllow AuditFailureNeverAllow Deterministic; do grep -r -q "func TestPBT_Invariant_$n" services/go-governance --include='*_test.go' || echo "FALTA $n"; done; echo fin-governance
  for n in RoundTrip_Policies RoundTrip_Workflows RoundTrip_AuditChain PolicyRejectsInvalid GeneratorCoverage; do grep -r -q "func TestPBT_$n" services/go-governance --include='*_test.go' || echo "FALTA $n"; done; echo fin-gov-rt
  for n in RoundTrip_UsersFile RoundTrip_Base32Secret TOTP Hash Invariant_AuthZ Token GeneratorCoverage; do grep -r -q "func TestPBT_$n" services/go-identity --include='*_test.go' || echo "FALTA $n"; done; echo fin-identity
  ```
  Esperado: ninguna línea `FALTA` y las tres marcas `fin-…`.

- [ ] **CA-3** — Cada mutante rompe al menos una propiedad (las propiedades **detectan** defectos de verdad).
  ```bash
  o=$(mktemp)
  for p in services/go-governance/testdata/mutants/*.patch; do git apply "$p" || { echo "NO APLICA $p"; continue; }; (cd services/go-governance && go test -run PBT ./... > "$o" 2>&1; echo "$(basename $p) rc=$? $(grep -c -E 'rapid\.seed=|Falsifying|Failed after' "$o")"); git apply -R "$p"; done
  p=services/go-identity/testdata/mutants/01-totp-window.patch; git apply "$p" && (cd services/go-identity && go test -run PBT ./... > "$o" 2>&1; echo "$(basename $p) rc=$? $(grep -c -E 'rapid\.seed=|Falsifying|Failed after' "$o")"); git apply -R "$p"
  git status --short | wc -l
  ```
  Esperado: cuatro líneas con `rc=1` y un número ≥ `1` (seed o contraejemplo reducido visibles), ninguna `NO APLICA`, y `0` (todo restaurado). Sin los mutantes, CA-1 pasa.

- [ ] **CA-4** — Un fallo muestra el seed y se reproduce con él.
  ```bash
  cd services/go-governance && l=$(mktemp) && go test -tags pbt_demo -run 'PBT_Demo' -v ./... > "$l" 2>&1; grep -E 'rapid\.seed=|Falsifying|FAIL' "$l" | head -n 5
  s=$(grep -o -E 'rapid\.seed=[0-9-]+' "$l" | head -n1); go test -tags pbt_demo -run 'PBT_Demo' "-$s" 2>&1 | grep -c -E 'Falsifying|FAIL'
  ```
  Esperado: líneas con `rapid.seed=`, `Falsifying example` (o `Failed after`) y `FAIL`; y un número ≥ `1` al repetir con el mismo seed. `TestPBT_Demo*` existe **solo** detrás de la etiqueta `pbt_demo`.

- [ ] **CA-5** — El `go test ./...` normal (el de CI) incluye las propiedades y sigue en verde, sin el demo.
  ```bash
  for m in go-governance go-identity; do (cd services/$m && go test -race ./... 2>&1 | grep -c -E 'FAIL|Demo'); done
  for m in go-governance go-identity; do (cd services/$m && s=$(date +%s) && go test -run PBT ./... >/dev/null 2>&1; echo "$m $(( $(date +%s) - s ))s"); done
  ```
  Esperado: `0` y `0`; y cada servicio con un tiempo ≤ `60s` (el codificador documenta el tiempo medido y el de argon2id).

- [ ] **CA-6** — Los generadores cubren las clases de dominio.
  ```bash
  for m in go-governance go-identity; do (cd services/$m && go test -run 'PBT_GeneratorCoverage' -v ./... | grep -E '^(--- |ok|FAIL)'); done
  ```
  Esperado: `--- PASS: TestPBT_GeneratorCoverage` y `ok` en ambos.

- [ ] **CA-7** — PBT-09: framework fijado y documentado.
  ```bash
  grep -c 'pgregory.net/rapid' services/go-governance/go.mod services/go-identity/go.mod; wc -l < services/go-governance/PBT.md; wc -l < services/go-identity/PBT.md
  grep -c -E 'rapid\.seed|go test -run PBT|git apply' services/go-governance/PBT.md services/go-identity/PBT.md
  ```
  Esperado: `…go.mod:1` dos veces; dos números entre `10` y `40`; y al menos `1` coincidencia en cada `PBT.md`.

- [ ] **CA-8** — Si una propiedad encontró un defecto de producción, hay regresión fija. (Si no, la bitácora lo declara y este criterio se cumple vacío.)
  ```bash
  grep -n -i -E 'defecto|hallazgo|contraejemplo' bitacoras/U4-T06.md | head -n 5; grep -r -c -E 'Regression|regresion|regresión' services/go-governance services/go-identity --include='*_test.go' | awk -F: '{s+=$2} END {print s}'
  ```
  Esperado: si la bitácora enumera un defecto, el segundo número es ≥ `1`; si declara «sin defectos hallados», no hay requisito.

- [ ] **CA-9** — Higiene y alcance.
  ```bash
  for m in go-governance go-identity; do (cd services/$m && go vet ./... && test -z "$(gofmt -l .)" && echo "$m ok"); done; git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/(go-governance|go-identity)/|bitacoras/U4-T06.md)' | wc -l
  git diff --name-only $b -- services | grep -v -E '_test\.go$|/testdata/|/internal/gen/|PBT\.md$|go\.(mod|sum)$' | wc -l
  ```
  Esperado: `go-governance ok`, `go-identity ok`, `0`, `0` y `0` (los únicos archivos de producción tocables son `go.mod`/`go.sum`; si una propiedad exige un arreglo, el codificador lo reporta y el humano autoriza ampliar el alcance en la ronda).

---

## Plan de pruebas

- Las propiedades **son** las pruebas; además, un caso fijo (ejemplo canónico) por invariante, para que un fallo de generadores no deje la propiedad sin cobertura.
- Los mutantes son la prueba de que las propiedades no son decorativas: la bitácora pega, por mutante, el seed y el contraejemplo reducido que mostró `rapid`.
- Las propiedades no usan red, reloj real ni disco fuera de `t.TempDir()`; los relojes y los generadores aleatorios del servicio se inyectan.

**Rojo primero:** el codificador registra en su bitácora que `go test -run PBT ./...` no encuentra pruebas (`testing: warning: no tests to run`) en ambos servicios antes de escribir la primera propiedad.

---

## Notas

- Archivos que se **modifican en su sitio**: `go.mod`/`go.sum` de ambos servicios. Nada de duplicados con sufijo.
- Los mutantes son parches contra la versión de `main` en el momento de la tarea; el codificador los genera con `git diff` tras aplicar cada cambio a mano, y los comprueba con `git apply --check`. Si una tarea posterior toca el evaluador, el mutante se regenera en esa tarea.
- La invariante `NamespaceNotTestNeverAllow` excluye `done` y `failed` porque la tabla de U4-T04 no exige namespace para ellos; si el humano decide exigirlo, se cambia en U4-T04, no aquí.
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
