# U4-T01 — Stubs: evaluador fake de `authorizeTransition` + principales/roles de ejemplo

**Unidad:** U4 — Gobernanza & Identidad
**Historias que implementa:** US-M8.3
**Depende de:** U5 completa (contratos en `contracts/`, CI en `.github/workflows/`). Ninguna otra tarea de U4. Puede correr en paralelo con U1-T01.

---

## Alcance

**Dentro** (una línea, concreta):

> Crear los módulos Go `services/go-governance/` y `services/go-identity/` (solo `go.mod`, `go.sum` y paquetes de stubs, **sin `Dockerfile` ni `main`**) con el modelo de dominio de los gates (`Gate`, `Transition`, `Decision`, `Principal`, `Role`), un `FakeEvaluator` de `authorizeTransition` programable que **falla cerrado**, y matrices Allow/Deny de ejemplo en `testdata/`.

Modelo (lo que U2 consumirá; las firmas se documentan en el `doc.go` del paquete):

- **`Principal{ID string, Role Role}`**, `Role` ∈ {`user`, `admin`} (los de las personas Marta y Julián; sin roles inventados). Cualquier otro valor es inválido.
- **Estados** (los del enum `Run.state` del OpenAPI): `confirmed`, `warm_ready`, `deploying`, `inferring`, `rehearsing`, `running`, `resetting`, `reporting`, `done`, `failed`.
- **`GateInput`**: `RunID`, `From`, `To`, y los hechos que el llamante reporta: `Confirmed` (hay un registro de confirmación persistido), `ResetVerified`, `EnsayoPassed`, `TargetNamespace` (string) y `WorkflowAllowed`. **Cada hecho es tri-estado** (`Unknown`/`True`/`False`, no `bool`): un hecho `Unknown` se trata como `False`. Esto es lo que hace que «fail-closed» sea comprobable.
- **`Decision{Allow bool, Reason string, AuditRef string}`**; `Allow=false` siempre lleva `Reason` no vacío.
- **`Evaluator`**: `AuthorizeTransition(ctx, GateInput) (Decision, error)`. Un `error` **nunca** equivale a permitir.
- **`FakeEvaluator`** programable: reglas por `(From, To)` → `Allow` | `Deny(reason)` | `Error(err)`; **por defecto Deny** (sin regla = Deny); registra cada llamada (`Calls()`) para que U2 pueda afirmar sobre ellas; seguro ante concurrencia (`-race`).
- **Matrices de ejemplo** en `services/go-governance/testdata/authorize_matrix.json`: al menos 14 filas `{input, expect: "allow"|"deny", reason_contains}` que cubren: transición válida con todos los hechos; sin `Confirmed`; sin `ResetVerified` al pedir `warm_ready`; sin `EnsayoPassed` al pedir `running`; `TargetNamespace` distinto de `aqs-test`; `WorkflowAllowed=False`; hecho `Unknown`; transición inexistente (`done→running`); estado desconocido; `RunID` vacío. **La matriz es el contrato de comportamiento que U4-T04 debe cumplir**: la misma matriz alimenta su implementación real.
- **Principales y roles de ejemplo** en `services/go-identity/testdata/principals.json`: `u1` (user), `u2` (user), `a1` (admin), más inválidos (rol vacío, rol `root`, ID vacío). Una función `ParseRole` y su prueba.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- La tabla real de gates por transición, las políticas y la auditoría: **U4-T04**.
- Login, hashing, sesiones, tokens, MFA: **U4-T02**. Autorización HTTP y middleware: **U4-T03**.
- `Dockerfile`, `cmd/`, servidor HTTP, `/healthz`, métricas: no son de esta tarea (sin `Dockerfile`, `scripts/ci/list-services.sh` no los lista; es lo esperado).
- Persistencia, red, reloj real o dependencias externas: el fake es en memoria.
- Modificar `contracts/**`, `deploy/**` o workflows.
- Importar `k8s.io/*`: ningún servicio de U4 habla con la API de Kubernetes.

---

## Archivos de contexto

- `unidades-y-tareas.md` (sección U4)
- `aidlc-docs/inception/application-design/unit-task-plans/U4.md`
- `aidlc-docs/inception/application-design/components.md` (C7, C8, C9)
- `aidlc-docs/inception/application-design/component-methods.md` (`authorizeTransition`, `appendAudit`, `login`)
- `aidlc-docs/inception/user-stories/personas.md` y `stories.md` (US-M8.3)
- `contracts/openapi/control-plane.yaml` (enum `Run.state`)

---

## Criterios de aceptación

Desde la raíz del worktree.

- [ ] **CA-1** — Las pruebas de ambos módulos pasan, con detector de carreras.
  ```bash
  for m in go-governance go-identity; do (cd services/$m && go test -race ./... 2>&1 | tail -n 3); done
  ```
  Esperado: ninguna línea `FAIL`; `ok` para los paquetes de `authz`/`principal`. Antes de la tarea: `cd: services/go-governance: No such file or directory` (rojo inicial).

- [ ] **CA-2** — El `FakeEvaluator` es fail-closed y programable.
  ```bash
  cd services/go-governance && go test -run 'FakeEvaluator' -v ./... | grep -E '^\s*--- (PASS|FAIL)'
  ```
  Esperado: todas `PASS`, con pruebas cuyo nombre contiene `DefaultDeny` (sin regla → Deny con `Reason` no vacío), `ErrorIsNotAllow` (la regla `Error` devuelve `error` y `Allow=false`), `UnknownFactIsFalse`, `RecordsCalls` y `Concurrent` (bajo `-race`).

- [ ] **CA-3** — La matriz Allow/Deny existe y el fake la reproduce al ser programado con la «verdad» de la matriz.
  ```bash
  jq 'length' services/go-governance/testdata/authorize_matrix.json; jq -r '[.[].expect] | unique | join(",")' services/go-governance/testdata/authorize_matrix.json
  jq -r '[.[] | select(.expect=="deny")] | length' services/go-governance/testdata/authorize_matrix.json
  cd services/go-governance && go test -run 'Matrix' -v ./... | grep -E '^\s*--- (PASS|FAIL)' | sort | uniq -c | head
  ```
  Esperado: un número ≥ `14`; `allow,deny`; un número ≥ `9` (hay más denegaciones que permisos: fail-closed); y subpruebas `PASS` por cada fila, sin `FAIL`.

- [ ] **CA-4** — Cada fila de la matriz cumple la regla de lectura «Unknown es False»: ninguna fila con un hecho `unknown` puede ser `allow`.
  ```bash
  jq -e '[.[] | select(.expect=="allow") | .input | [.confirmed,.reset_verified,.ensayo_passed,.workflow_allowed] | any(. == "unknown")] | any | not' services/go-governance/testdata/authorize_matrix.json
  ```
  Esperado: `true`.

- [ ] **CA-5** — Principales y roles de ejemplo, con los inválidos rechazados.
  ```bash
  cd services/go-identity && go test -run 'Role|Principal' -v ./... | grep -E '^\s*--- (PASS|FAIL)'; jq -r '[.valid[].role] | unique | join(",")' testdata/principals.json; jq '.invalid | length' testdata/principals.json
  ```
  Esperado: todas `PASS`; `admin,user`; y un número ≥ `3`.

- [ ] **CA-6** — Higiene de los módulos; U4 sigue sin ser desplegable y no toca Kubernetes.
  ```bash
  for m in go-governance go-identity; do (cd services/$m && go vet ./... && test -z "$(gofmt -l .)" && test -s go.sum && ! grep -q '^replace' go.mod && echo "$m ok"); done
  test ! -e go.work && echo "sin go.work"
  for m in go-governance go-identity; do go list -C services/$m -deps ./... | grep -c -E 'k8s.io|client-go'; done
  bash scripts/ci/list-services.sh | grep -c -E 'go-governance|go-identity'
  ```
  Esperado: `go-governance ok`, `go-identity ok`, `sin go.work`, `0`, `0` y `0`. (Si `go.sum` queda vacío porque el módulo no tiene dependencias, el codificador lo documenta y `test -s` se sustituye por `test -e`; la regla es que el archivo exista y esté commiteado.)

- [ ] **CA-7** — Árbol limpio tras el commit y nada fuera de alcance tocado.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/(go-governance|go-identity)/|bitacoras/U4-T01.md)' | wc -l
  ```
  Esperado: `0` y `0`.

---

## Plan de pruebas

- `FakeEvaluator`: sin regla, con `Allow`, con `Deny`, con `Error`; regla específica frente a comodín; llamadas registradas en orden; 50 goroutines con `-race`.
- Matriz: una prueba tabular lee el JSON y valida su **forma** (campos obligatorios, `expect` ∈ {allow, deny}, estados del enum) además de reproducirla.
- Negativa: una copia temporal de la matriz con un `allow` y un hecho `unknown` hace fallar la prueba de forma (se ejecuta y se registra, no se commitea).
- `ParseRole`: `""`, `"root"`, `"ADMIN"` (distinguir mayúsculas), `"user "` (espacio) → error.

**Rojo primero:** el codificador pega en su bitácora la salida literal de `ls services/go-governance` (no existe) y del primer comando de CA-1 antes de crear nada.

---

## Notas

- Archivos que se **modifican en su sitio**: ninguno. Todo es nuevo.
- Módulos: `github.com/ogaston/agentic-qa-swarm/services/go-governance` y `.../services/go-identity`, `go 1.24`, cada uno con su `go.mod`/`go.sum` (SEC-10), sin `go.work` ni `replace` (C-05).
- La matriz de `go-governance` y el modelo de `Principal` de `go-identity` se **duplican** si ambos módulos los necesitan (no se comparten módulos): la duplicación se anota y U4-T03 define cuál es la fuente.
- Sin `Dockerfile`, estos módulos no corren en `ci.yml` hasta U4-T02/T04; la verificación hasta entonces es local. Es una limitación conocida, no un defecto.
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
