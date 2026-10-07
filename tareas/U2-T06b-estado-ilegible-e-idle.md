# U2-T06b — Estado del warm ilegible e idle sin estado optimista (seguimiento de U2-T06)

**Unidad:** U2 — Orquestación & Warm Sandbox Lifecycle
**Historias que implementa:** US-M7.1, US-M7.2 (KPI: reset verificado 100% + higiene de rebuild)
**Depende de:** U2-T06 **fusionada** (PR #42). Cierra los hallazgos abiertos de `revisiones/U2-T06/ronda-3.md` (F-10 y F-11 NARANJA; F-12 (b), (c) y (g) AMARILLO). **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

---

## Alcance

**Dentro** (una línea, concreta):

> Hacer que un `warm-state` ilegible no deje atascado el reset, que `IdleCheck` nunca deje el almacén en `ready` con 0 réplicas aunque falle una escritura, y cerrar tres aristas menores de la política y de las pruebas, todo en `services/go-reset/`.

Detalle:

- **F-10 — `warm-state` ilegible.** `kube.Get` distingue el **error transitorio de la API** (se devuelve como error: no se asume nada) de un **contenido ilegible** (ConfigMap existente con `state` ausente, vacío, que no es JSON o no es un objeto): este último se interpreta como estado **desconocido ⇒ `dirty`, `reset_verified=false`**, de modo que `Reset`, `Rebuild`, `Teardown` y `Housekeeping` pueden sobrescribirlo tras verificar. `IdleCheck` con estado ilegible **no escala** (fail-closed). Se registra un log y una métrica `aqs_warm_state_unreadable_total`.
- **F-11 — `IdleCheck` sin estado optimista.** Hoy escala primero y escribe `idle-escalado` después: si el `Put` falla, el almacén dice `ready`+`reset_verified=true` con 0 réplicas. Nuevo orden: escribir `idle-escalado` **antes** de escalar a 0 y, si el escalado falla, **compensar** (volver a `ready` solo si el warm sigue con ≥ 1 réplica verificada, o dejar `dirty`). En ningún punto de la secuencia el almacén puede decir `ready` con 0 réplicas.
- **F-12 (b).** `WarmPolicy` con `idleScaleDownAfter` ≤ 0 se rechaza (fail-closed: `IdleCheck` no escala y se loguea), como ya hace `config.dur`.
- **F-12 (c).** Prueba de **servicio** (no solo unitaria de `kube`) con transición: pod viejo Ready y estado viejo del Deployment durante N sondeos; `Verify` exige ≥ N sondeos antes de dar `app-ready` (hoy solo la unitaria de `AppReady` lo protege; mutar «`AppReady` ignora el status del Deployment» no pone rojo ninguna prueba de servicio).
- **F-12 (g).** Si el `Put` de cuarentena falla, se alerta igual (`Alerter.Quarantine`) y se registra el error.
- **Barrido en el repo.** Traer al repositorio una versión reducida del barrido del revisor como **prueba de propiedad** (`rapid`): 4 estados del warm × 5 operaciones (Reset, Rebuild, Teardown, Housekeeping, IdleCheck) × bits de fallo (restart, DB sucia, Redis sucio, `Version`, `Replicas`, `Put` fallando en la llamada 0..3, `Publish`), con el oráculo del informe: en el instante de cada `Publish` el mundo real está limpio y el almacén es coherente; y **en ningún instante** el almacén dice `ready` con 0 réplicas.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Contrato de fin de sesión con U2-T02 (`SessionDone`: que `Reset(run)` cierre su sesión), exclusión entre procesos (Lease) de `rebuild`/`housekeeping`, constante con nombre para `ScaleApp(1)` (F-12 a, e, f): candidatas.
- Esquema del evento `warm.quarantined`, contrato `warm-state` compartido con U2-T03, API REST al OpenAPI, cómo se lanza el Job `reset-{run}` y el montaje del script de baseline: **U2-T07** / contratos.
- Modificar `services/go-warm-manager/`, `go-run-controller`, `contracts/**`, `deploy/**`, `policy/**` o workflows.
- Comandos contra un clúster o la nube.

---

## Archivos de contexto

- `revisiones/U2-T06/ronda-3.md` (F-10, F-11, F-12 y sus reproducciones `TestSweepCorruptWarmStateStuck`, `TestSweepFamily`, `TestIdlePolicyNonPositive`; el scratchpad puede no existir: reproduce desde la descripción), `ronda-2.md`, `bitacoras/U2-T06.md`
- `services/go-reset/` (`internal/core/service.go`, `internal/adapters/kube/kube.go`, `internal/core/*_test.go`)
- `tareas/U2-T06-reset-verificado.md`

---

## Criterios de aceptación

Desde la raíz del worktree.

- [ ] **CA-1** — Pruebas en verde con `-race` y lo anterior intacto.
  ```bash
  cd services/go-reset && go test -race -count=1 -v ./... | grep -c -E '^\s*--- PASS'; go test -race -count=1 ./... 2>&1 | grep -c FAIL
  ```
  Esperado: ≥ `120` (los 113 de U2-T06 más ≥ 7) y `0`.

- [ ] **CA-2** — **`warm-state` ilegible no atasca el reset** (con `kube.Client` real sobre `kubernetes/fake`).
  ```bash
  cd services/go-reset && go test -race -count=1 -run 'CorruptWarmState|UnreadableState' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS` en al menos cuatro pruebas (estado vacío, ausente, no JSON y no objeto): Reset, Rebuild, Teardown y Housekeeping terminan en `ready` verificado; `IdleCheck` no escala; un error transitorio de la API **no** se trata como ilegible. **Rojo primero** con `TestSweepCorruptWarmStateStuck` (`err=warm-state ilegible`).

- [ ] **CA-3** — **IdleCheck nunca deja `ready` con 0 réplicas**, ni con `Put` fallando en cualquier llamada ni con un kill.
  ```bash
  cd services/go-reset && go test -race -count=1 -run 'IdleCheck(NeverReadyWithZeroReplicas|PutFailure|Compensates)' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS`; la prueba falla el `Put` en las llamadas 0, 1, 2 y 3 y comprueba, **en cada instante** observable (almacén y réplicas leídos de vuelta), que nunca hay `ready` con 0 réplicas. **Rojo primero** con la reproducción `ready/IdleCheck: (b')` del informe.

- [ ] **CA-4** — La propiedad de barrido vive en el repo y se pone roja con las mutaciones conocidas.
  ```bash
  cd services/go-reset && go test -race -count=1 -run 'SweepProperty' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS`. **Mutaciones obligatorias** registradas en la bitácora con su rojo: `Verify` = «los pasos devolvieron éxito»; ignorar el error del `Put` final; `IdleCheck` con el orden antiguo (escala y luego escribe); `AppReady` ignorando el status del Deployment (debe poner roja una prueba **de servicio**).

- [ ] **CA-5** — Política y alertas.
  ```bash
  cd services/go-reset && go test -race -count=1 -run 'WarmPolicyNonPositive|QuarantinePutFails' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS` en al menos dos pruebas: `idleScaleDownAfter` de `0s` y `-1h` no escala un warm recién verificado; con el `Put` de cuarentena fallando, el `Alerter` igualmente recibe 1 aviso.

- [ ] **CA-6** — Higiene, imagen y alcance.
  ```bash
  (cd services/go-reset && go vet ./... && test -z "$(gofmt -l .)" && echo ok); docker build -q -t aqs-go-reset:ci services/go-reset >/dev/null && docker inspect aqs-go-reset:ci --format '{{.Config.User}}'; git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/go-reset/|bitacoras/U2-T06b.md|revisiones/U2-T06b/)' | wc -l
  ```
  Esperado: `ok`, `65532:65532`, `0` y `0`.

---

## Plan de pruebas

- `kubernetes/fake` para `kube.Client`; fakes de puertos con `FailPutN`; reloj inyectable; un controlador de Deployments simulado **con retardo** (el campo `slow` que el barrido del revisor encontró sin usar) para la prueba de F-12 (c).
- **Barrido de clase obligatorio** (la familia de U2-T06: caminos optimistas o incompletos alrededor de la verificación): tabla camino→prueba en la bitácora para cada operación × estado del warm × fallo, con su mutación.

**Rojo primero:** registrar `TestSweepCorruptWarmStateStuck` y la reproducción de F-11 fallando contra `main`.

---

## Notas

- **Tope de 3 rondas** (decisión del humano). En cada NO-VERDE, barrido de la clase del defecto.
- **La invariante central ya está probada** en U2-T06 (655 360 escenarios sin ningún `reset.verified` falso): esta tarea no debe debilitarla; cualquier cambio que ponga rojo `TestResetPropertyEventIffClean` o `TestResetExhaustiveCombinations` es una regresión.
- **Go y Dockerfile.** `go 1.26.8`; el `Dockerfile` ya existe. Podman: montajes con `:z`.
- **Candidatas a registrar, no hacer:** `SessionDone`, exclusión entre CronJobs, `warm.quarantined`, contrato `warm-state` compartido, `ScaleApp(1)` con nombre.
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
