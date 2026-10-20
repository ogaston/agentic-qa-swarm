# U8-T08a — `run_id` corto: que `ui-api` y `go-warm-manager` hablen el mismo contrato

**Unidad:** U8 — Producto demostrable (ciclo completo en kind)
**Historias que implementa:** US-M2 (la corrida confirmada llega a `deploy`), US-M5.
**Depende de:** U8-T01 a U8-T07 **fusionadas** (ya lo están). **Bloquea a U8-T08** (CA-1 y CA-2). **Tope: 3 rondas**.

---

## Origen (evidencia de U8-T08, ronda 1)

`ui-api` acuña el `run_id` como `"run-" + 32 hex` = **36 caracteres** (`services/ui-api/inbox/store.go`, `newRunID`). `go-warm-manager` lo valida con `^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$` (**máximo 32**, `services/go-warm-manager/types.go:53`). El controlador pasa el `run_id` tal cual a `POST /deploys`, así que toda corrida confirmada termina en `400 invalid_run_id` y pasa a `failed` en la fase `deploy`. Un `run_id` de 36 caracteres da 400; uno corto, 404 (aceptado). Detalle y salida real en `bitacoras/U8-T08.md` (rama `tarea/U8-T08`).

El tope de 32 es el contrato más antiguo y el `run_id` también nombra objetos de Kubernetes (Jobs de `go-reset`, tope 40 en `jobspec/job.go:32`), por eso se acorta el generador y no se relaja el validador.

---

## Alcance

**Dentro** (una línea, concreta):

> `ui-api` genera el `run_id` como `"run-" + 16 hex` (**20 caracteres**, 64 bits aleatorios), y todo lo que valida ese formato exacto (pruebas de `ui-api`, generador PBT, regex del dashboard) pasa a `^run-[0-9a-f]{16}$`; una prueba de contrato une el generador de `ui-api` con la validación real de `go-warm-manager`, para que el desacuerdo no pueda volver a pasar desapercibido.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Relajar `runIDRe` de `go-warm-manager`, `go-reset` o `go-run-controller`. Los validadores no se tocan.
- Migrar `run_id` ya persistidos (36 caracteres) en archivos de confirmación o del controlador: son datos de entornos de prueba; el lector de `ui-api` no los reescribe. Si el codificador ve que un archivo viejo rompe el arranque, lo registra como candidata.
- `scripts/kind/` y todo lo de `U8-T08` (rama `tarea/U8-T08`): se rebasa después de fusionar esta.
- Cambios en `contracts/` salvo que un esquema o ejemplo fije el largo viejo (en ese caso se corrige y se explica en la bitácora).

---

## Archivos de contexto

- `bitacoras/U8-T08.md` (sección «Rojo primero»; rama `tarea/U8-T08`, worktree `../agentic-qa-swarm-wt-U8-T08`)
- `services/ui-api/inbox/store.go`, `store_test.go`, `services/ui-api/inbox/pbt_test.go`, `services/ui-api/internal/gen/gen.go`
- `services/go-warm-manager/types.go`, `services/go-warm-manager/internal/api/`
- `web/dashboard/src/pages/Inbox/InboxPage.tsx`, `web/dashboard/src/pages/Run/RunPage.tsx` y sus pruebas
- `services/go-reset/internal/jobspec/job.go`, `services/go-run-controller/adapters/phases.go` (solo lectura: validadores que no cambian)

---

## Criterios de aceptación

- [ ] **CA-1** — El generador produce el formato nuevo y los validadores existentes de cada servicio lo aceptan.
  ```bash
  cd services/ui-api && go test ./... 2>&1 | tail -5
  ```
  Esperado: `ok` en todos los paquetes; la prueba de `newRunID` exige `len == 20` y `^run-[0-9a-f]{16}$`.

- [ ] **CA-2** — Contrato cruzado: lo que genera `ui-api` pasa la validación de `go-warm-manager`, `go-reset` y `go-run-controller`.
  ```bash
  go test ./services/ui-api/... -run 'RunID.*Contrato' -v 2>&1 | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS` para al menos tres validadores (el de warm-manager con su regex literal copiada y un comentario que apunta a `types.go:53`). La prueba falla si `newRunID` vuelve a 32 hex (demostrado en el plan de pruebas).

- [ ] **CA-3** — El dashboard valida el formato nuevo y rechaza el viejo.
  ```bash
  cd web/dashboard && npm test -- --run 2>&1 | tail -8; grep -rn "{32}" src/pages/Inbox src/pages/Run | grep -c "run-"
  ```
  Esperado: pruebas en verde (incluida una por página con un `run_id` de 36 caracteres que **no** navega/consulta) y `0` coincidencias de `run-…{32}`.

- [ ] **CA-4** — Lectura de vuelta contra el servicio real: un `run_id` generado por `ui-api` es aceptado por `go-warm-manager` (no responde `400`).
  ```bash
  cd services/go-warm-manager && go test ./... 2>&1 | tail -3
  ```
  Esperado: `ok`. Más, en la bitácora, un `curl` a un `go-warm-manager` local (o en kind si el codificador lo levanta con `flock`) con un id de 20 caracteres: respuesta `404 not_found` (aceptado), no `400 invalid_run_id`.

- [ ] **CA-5** — Alcance.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/ui-api/|web/dashboard/src/|bitacoras/U8-T08a\.md|revisiones/U8-T08a/|contracts/)' | wc -l; git diff --name-only $b -- services/go-warm-manager services/go-reset services/go-run-controller | wc -l
  ```
  Esperado: `0`, `0` y `0`.

---

## Plan de pruebas

- **Rojo primero:** antes de cambiar nada, el codificador escribe la prueba de contrato de CA-2 y la corre contra el `newRunID` actual: debe **fallar** con el id de 36 caracteres. Pegar la salida en la bitácora. Luego cambia el generador y vuelve a correrla: pasa.
- Prueba negativa del dashboard: `run-` + 32 hex no pide ni navega (ambas páginas).
- PBT de `ui-api`: el generador `internal/gen/gen.go:173` y `runIDRE` en `inbox/pbt_test.go:208` usan el formato nuevo.
- Unicidad: la prueba existente de ids distintos sigue en verde con 64 bits (generar 10 000 sin colisión).

---

## Notas

- Archivos que se **modifican en su sitio**; nada de duplicados con sufijo.
- Ramas: esta tarea vive en `tarea/U8-T08a`, worktree propio `../agentic-qa-swarm-wt-U8-T08a`, desde `main`.
- Tras fusionarla, el orquestador rebasa `tarea/U8-T08` sobre `main` y despacha la ronda 2 de U8-T08.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
