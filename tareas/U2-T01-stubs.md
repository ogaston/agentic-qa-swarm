# U2-T01 — Stubs: esquemas y fixtures de `FlowPlan`, `SurfaceArtifact`, `EvidenceURIs`, `WarmState` + esqueleto de los tres módulos de U2

**Unidad:** U2 — Orquestación & Warm Sandbox Lifecycle
**Historias que implementa:** US-M5, US-M6 (contrato con U3: flujos de entrada, superficie y evidencia de salida)
**Depende de:** U5 completa (contratos, CI), U1 y U4 fusionadas. Es la primera tarea de U2; **bloquea** U2-T02, T03 y T06.

---

## Alcance

**Dentro** (una línea, concreta):

> Crear los módulos Go vacíos `services/go-run-controller/`, `services/go-warm-manager/` y `services/go-reset/` (solo `go.mod`, `go.sum`, `doc.go` y los paquetes de stubs indicados, **sin `Dockerfile` ni `main`**), añadir **archivos nuevos** `contracts/plans/*.schema.json` con los esquemas de `FlowPlan`, `SurfaceArtifact`, `EvidenceURIs` y `WarmState` más sus ejemplos válidos e inválidos, y unos stubs deterministas (`FakeFlowSource`, `FakeSurface`, `FakeEvidence`) que U2-T02/T04/T05 usarán hasta que U3 exista.

Detalle:

- **Módulos.** `module github.com/ogaston/agentic-qa-swarm/services/go-run-controller`, `.../go-warm-manager`, `.../go-reset`; los tres con `go 1.26.8`, su `go.mod` y su `go.sum`. **Prohibido** `go.work` y `replace` (C-05). Se crean los tres aquí para que T02, T03 y T06 corran en paralelo sin tocar el mismo `go.mod`. `go-warm-manager` y `go-reset` solo llevan `doc.go` con el comentario de paquete; los tipos de cada servicio los crean sus tareas (T03, T06).
- **Esquemas** (JSON Schema 2020-12, `additionalProperties: false`, `$id` estable) en `contracts/plans/`:
  - `flow-plan.schema.json`: `{run_id, workflow, flows: [{flow_id, name, steps: [{method, path, expect_status}], invariant}]}`; `flows` con `minItems: 1`; `method` en `GET|POST|PUT|PATCH|DELETE`; `path` empieza por `/`; `expect_status` entero 100–599.
  - `surface-artifact.schema.json`: `{run_id, base_url, endpoints: [{method, path}], source: "openapi"|"probe"}`; `base_url` URI `http(s)`.
  - `evidence-uris.schema.json`: `{run_id, uris: [uri...]}` con `minItems: 1` y esquema `s3` o `https`.
  - `warm-state.schema.json`: `{warm_id, state: "ready"|"dirty"|"cuarentena"|"idle-escalado", reset_verified: bool, baseline_version}`.
- **Ejemplos** en `contracts/plans/examples/valid/` y `.../invalid/`, nombre `<esquema>.<caso>.json`: ≥ 2 válidos por esquema (el de `FlowPlan` con 1 y con 3 flujos) y ≥ 3 inválidos por esquema (falta campo obligatorio, campo extra, valor fuera de enum o de formato). Datos sintéticos; sin secretos ni repositorios reales.
- **Stubs** (en `services/go-run-controller/stubs`): `FakeFlowSource` (devuelve un `FlowPlan` sintético válido por `run_id`, o un error programado), `FakeSurface` (devuelve un `SurfaceArtifact` sintético), `FakeEvidence` (devuelve `EvidenceURIs` sintéticas). Cada uno **por defecto falla cerrado** (devuelve error hasta que se programe una respuesta). Los tipos Go de estos cuatro objetos viven en `services/go-run-controller/plan`.
- **Pruebas Go.** `TestPlanExamplesAgainstSchemas`: cada válido pasa y cada inválido falla; `TestStubsReturnValidPlans`: lo que devuelven los stubs valida contra su esquema y el valor por defecto es el error.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Modificar **archivos existentes** de `contracts/` (OpenAPI, esquemas de eventos, `validate.sh`) o cualquier workflow. Solo se **añaden** archivos nuevos bajo `contracts/plans/`. Incluirlos en `contracts/validate.sh` es una tarea candidata, no de esta.
- `Dockerfile`, `cmd/`, servidor HTTP, métricas, logging, máquina de estados, cliente de Kubernetes: **U2-T02 a T06**.
- Cualquier dependencia de red o importar `k8s.io/*`.
- Los stubs de LLM o de agentes de U3: aquí solo se simulan sus **salidas**.

---

## Archivos de contexto

- `unidades-y-tareas.md` (sección U2) y `aidlc-docs/inception/application-design/unit-task-plans/U2.md`
- `aidlc-docs/inception/application-design/component-methods.md` (firmas de `FlowPlan`, `SurfaceArtifact`, `EvidenceURIs`, `WarmState`)
- `aidlc-docs/inception/application-design/unit-of-work-dependency.md` (contratos U2↔U3)
- `contracts/events/surface.ready.schema.json`, `run.done.schema.json`, `warm.ready.schema.json` (estilo y nombres de campos)
- `contracts/validate.sh` (invocación de `ajv` que se reutiliza en los criterios)
- `tareas/U1-T01-stubs.md` (patrón de módulos Go sin `Dockerfile`, pruebas de ejemplos)

---

## Criterios de aceptación

Desde la raíz del worktree. Alias:

```bash
AJV=(npx --yes -p ajv-cli@5.0.0 -p ajv-formats@3.0.1 ajv)
```

- [ ] **CA-1** — Las pruebas de los tres módulos pasan.
  ```bash
  for m in go-run-controller go-warm-manager go-reset; do (cd services/$m && go test ./... 2>&1 | tail -n 3); done
  ```
  Esperado: ninguna línea `FAIL`; en `go-run-controller` aparecen `ok` los paquetes `plan` y `stubs`. Antes de la tarea: `cd: services/go-run-controller: No such file or directory` (rojo inicial).

- [ ] **CA-2** — Cada ejemplo válido pasa `ajv` y cada inválido es rechazado, con la herramienta real.
  ```bash
  for f in contracts/plans/examples/valid/*.json; do s=$(basename "$f" | cut -d. -f1); "${AJV[@]}" validate --spec=draft2020 -c ajv-formats -s "contracts/plans/$s.schema.json" -d "$f" >/dev/null 2>&1; echo "valid $(basename $f) rc=$?"; done
  for f in contracts/plans/examples/invalid/*.json; do s=$(basename "$f" | cut -d. -f1); "${AJV[@]}" validate --spec=draft2020 -c ajv-formats -s "contracts/plans/$s.schema.json" -d "$f" 2>&1 | grep -q 'invalid$'; echo "invalid $(basename $f) rejected=$?"; done
  ```
  Esperado: todas las líneas `valid ... rc=0` y `invalid ... rejected=0`; al menos 8 `valid` y 12 `invalid`.

- [ ] **CA-3** — Los stubs fallan cerrado y devuelven datos válidos cuando se programan.
  ```bash
  cd services/go-run-controller && go test -run 'Stub|Fake' -v ./stubs/ | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: todas `--- PASS`; hay una prueba cuyo nombre contiene `DefaultFails` por cada stub (3) y una que valida contra su esquema lo que devuelve cada stub programado.

- [ ] **CA-4** — Los esquemas son válidos en sí mismos y cierran los objetos.
  ```bash
  for s in flow-plan surface-artifact evidence-uris warm-state; do jq -e '.additionalProperties == false and ."$schema" == "https://json-schema.org/draft/2020-12/schema"' contracts/plans/$s.schema.json; done
  jq -r '.properties.flows.minItems' contracts/plans/flow-plan.schema.json; jq -r '.properties.state.enum | join(",")' contracts/plans/warm-state.schema.json
  ```
  Esperado: cuatro `true`, `1` y `ready,dirty,cuarentena,idle-escalado`.

- [ ] **CA-5** — Higiene y alcance: ningún archivo existente de `contracts/` modificado, nada desplegable.
  ```bash
  for m in go-run-controller go-warm-manager go-reset; do (cd services/$m && go vet ./... && test -z "$(gofmt -l .)" && test -s go.sum && ! grep -q '^replace' go.mod && echo "$m ok"); done
  test ! -e go.work && echo "sin go.work"
  go list -C services/go-run-controller -deps ./... | grep -c -E 'k8s.io|client-go'
  bash scripts/ci/list-services.sh | grep -c -E 'run-controller|warm-manager|go-reset'
  b=$(git merge-base HEAD origin/main); git diff --name-status $b | grep -E '^[MD]' | grep -E 'contracts/' | wc -l
  ```
  Esperado: tres `ok`, `sin go.work`, `0`, `0` y `0`.

- [ ] **CA-6** — Árbol limpio y nada fuera de alcance.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/(go-run-controller|go-warm-manager|go-reset)/|contracts/plans/|bitacoras/U2-T01.md|revisiones/U2-T01/)' | wc -l
  ```
  Esperado: `0` y `0`.

---

## Plan de pruebas

- Contrato: `TestPlanExamplesAgainstSchemas` (válidos pasan, inválidos fallan, ≥ 8 y ≥ 12) con `github.com/santhosh-tekuri/jsonschema/v6` (formato activo), la misma biblioteca que usa `go-intake`.
- Stubs: valor por defecto (error), respuesta programada, programación de error, concurrencia (`-race`).
- Negativa: un ejemplo `valid` copiado a un directorio temporal con `warm_id` vacío debe fallar la prueba (se ejecuta y se registra, no se commitea).

**Rojo primero:** el codificador pega en su bitácora la salida literal de `ls services/go-run-controller` (no existe) y del primer comando de CA-1.

---

## Notas

- **Go.** `go 1.26.8` y las dependencias más recientes compatibles. Patrón de referencia: `services/go-governance`, `services/go-intake`. `govulncheck` no corre en el entorno del loop (`vuln.go.dev` da 403): lo confirma el job `vuln` de `ci` en el PR (el orquestador lo abre como borrador).
- **Esquemas nuevos y no en el contrato de U5.** `FlowPlan`, `SurfaceArtifact`, `EvidenceURIs` y `WarmState` existían solo como nombres en `component-methods.md`. Esta tarea los fija por primera vez; U3 los consumirá. Los nombres de campo salen de ese documento y de los esquemas de eventos; si el codificador necesita un campo no listado, lo reporta, no lo inventa.
- **Duplicación aceptada.** No hay módulo compartido entre servicios (sin `go.work` ni `replace`): cada servicio de U2 define sus propios tipos y se prueba contra los esquemas de `contracts/plans/`.
- **Informes del loop.** El diff de `revisiones/<tarea>/` no cuenta como desborde.
- **Podman.** La CLI `docker` del entorno local es podman; los montajes de volúmenes llevan `:z`.
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
