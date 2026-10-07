# U2-T02b — Diario y entrada de eventos ante fallos de disco (seguimiento de U2-T02)

**Unidad:** U2 — Orquestación & Warm Sandbox Lifecycle
**Historias que implementa:** US-M2, US-M5 (la máquina de estados de U2-T02 ya es sólida; esta tarea endurece sus adaptadores de disco)
**Depende de:** U2-T02 **fusionada** (PR #40). **Bloquea** cualquier despliegue de `go-run-controller` en un entorno compartido. Es el cierre de los hallazgos abiertos de U2-T02 (`revisiones/U2-T02/ronda-3.md`, F-01 a F-03 y F-04/F-05 de documentación). **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

---

## Alcance

**Dentro** (una línea, concreta):

> Hacer que el diario de corridas sobreviva a una escritura parcial y a un `kill -9`, que un evento aplicado con el disco caído no se pierda, y que el cableado de `/readyz` con la salud de persistencia tenga prueba, todo en `services/go-run-controller/`.

Detalle:

- **Diario (`adapters/journal.go`, `Save`).** Ante cualquier error de `Write` o `Sync`, antes de aceptar otro `Save` el diario debe volver a un estado consistente: truncar el archivo al tamaño previo al intento (o reabrirlo y revalidar la cadena) y no avanzar `seq`/`last`; si no puede garantizarlo, se marca **roto** y los `Save` siguientes devuelven `ErrPersist` (el controlador ya no avanza ni lanza nada; `/readyz` no listo). Un `Sync` fallido cuya línea quedó en disco no puede dejar `seq` desalineado con el archivo.
- **Arranque (`OpenJournal`).** Tolera y **descarta una cola sin `\n` final** (escritura interrumpida que nunca tuvo un `Save` exitoso detrás), registrando una línea de log con los bytes descartados y `aqs_journal_tail_discarded_total`. Una línea **con** `\n` final que no encadena sigue siendo error de arranque (corrupción real).
- **Entrada de eventos (`adapters/events.go` `Poll`, `cmd/go-run-controller/main.go` `loop`).** El offset del archivo de eventos se confirma (ack) **solo cuando `Apply` sale bien**. Si `Apply` devuelve `ErrPersist`, el evento se vuelve a entregar en el siguiente tick (la idempotencia por `Seen` y por `run_id` evita duplicados). `/readyz` y `aqs_persist_errors_total` ya existen y se conservan.
- **Cableado de `/readyz`.** Refactor mínimo de `main.go` para que el chequeo `persist` sea comprobable sin arrancar el proceso, y prueba que lo mata la mutación «el chequeo devuelve siempre `nil`».
- **Documentación** (comentario del puerto y README): (a) `PhaseLauncher.Launch` es **idempotente por (corrida, fase, `Started[fase]`)** y la corrida llega con `Started` para nombrar el Job de forma determinista (lo implementan U2-T04/T05); (b) el handoff es «a lo sumo una vez»: un crash entre guardar `HandedOff` y avisar lo pierde (y `halted` no es visible en `GET /runs`).

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- HMAC y ancla del último `seq` del diario, validación de legalidad al reproducirlo y rotación/compactación del diario (candidatas aceptadas desde la ronda 1).
- Hacer visible `halted` en `GET /runs/{id}` (cambia el esquema `Run`), tiempo máximo de espera de `rehearsal.passed`, circuito del cliente de identidad.
- Cambiar la máquina de estados, los gates o el mecanismo `Started`/`HandedOff` de U2-T02, salvo lo imprescindible para el ack de eventos.
- Cualquier cosa fuera de `services/go-run-controller/`, `bitacoras/U2-T02b.md` y `revisiones/U2-T02b/`.
- Comandos contra un clúster o la nube.

---

## Archivos de contexto

- `revisiones/U2-T02/ronda-3.md` (reproducciones de F-01 a F-06 y rutas del scratchpad; el scratchpad de la sesión puede no existir: reproduce desde la descripción), `ronda-2.md`, `bitacoras/U2-T02.md`
- `services/go-run-controller/adapters/journal.go`, `adapters/events.go`, `cmd/go-run-controller/main.go`, `runctl/controller.go`, `runctl/ports.go`
- `tareas/U2-T02-run-controller.md` (CA-6 corregido)

---

## Criterios de aceptación

Desde la raíz del worktree.

- [ ] **CA-1** — Pruebas en verde con `-race` (≥ 8 casos nuevos) y todo lo anterior intacto.
  ```bash
  cd services/go-run-controller && go test -race -count=1 -v ./... | grep -c -E '^\s*--- PASS'; go test -race -count=1 ./... 2>&1 | grep -c FAIL
  ```
  Esperado: ≥ `67` (los 59 de U2-T02 más ≥ 8) y `0`.

- [ ] **CA-2** — **Escritura parcial: el diario sigue siendo recuperable.** Se prueba con el diario real y un escritor que falla a media línea (inyectado) y con `Sync` que falla.
  ```bash
  cd services/go-run-controller && go test -race -count=1 -run 'Journal(PartialWrite|SyncFailure|TornTail)' -v ./adapters/ | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS` en al menos cuatro pruebas: `Write` parcial → siguiente `Save` ok → `OpenJournal` reabre sin error; `Sync` fallido; cola sin `\n` descartada al abrir (con log y métrica); línea con `\n` que no encadena sigue siendo error de arranque. **Rojo primero** con la reproducción del revisor (`TestJournalPartialWriteThenRecoveryCorrupts`: «línea N ilegible»).

- [ ] **CA-3** — **Caja negra con disco lleno**: el servicio real no pierde la corrida ni deja de arrancar.
  ```bash
  # el codificador documenta el guion en la bitácora: tmpfs de 8 KiB con `unshare -rm` (o un archivo de bucle), servicio real con RUN_DATA_DIR ahí, corrida en curso
  ```
  Esperado: con el disco lleno, la corrida no avanza ni falla ni hace handoff, `aqs_persist_errors_total` sube y `/readyz` da `{"checks":{"persist":"fail"}...}`; al liberar espacio la corrida continúa; y **tras `kill -9` y reinicio el servicio arranca** y la corrida sigue desde su último estado. Si `unshare -rm` no está disponible, el codificador lo declara y usa el escritor inyectado de CA-2 (no rellena a ciegas).

- [ ] **CA-4** — **Evento no perdido con el disco caído.**
  ```bash
  cd services/go-run-controller && go test -race -count=1 -run 'Event(NotLostWhenSaveFails|AckAfterApply|RedeliveredOnPersistError)' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS` en al menos tres pruebas: con `Save` fallando, `run.confirmed` no se pierde y la corrida se crea al sanar (30 ticks); `rehearsal.passed` perdido ya no deja la corrida esperando; un evento ya aplicado no se aplica dos veces. **Rojo primero** con `TestEventLostWhenSaveFailsDuringApply` del informe.

- [ ] **CA-5** — **El cableado de `/readyz` está bajo prueba** (mutación que antes sobrevivía).
  ```bash
  cd services/go-run-controller && go test -race -count=1 -run 'Readyz(Persist|Wiring)' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS`. **Mutación obligatoria** registrada en la bitácora: con el chequeo `persist` de `/readyz` devolviendo siempre `nil`, la prueba falla (rojo) y vuelve a pasar al revertir.

- [ ] **CA-6** — Documentación del puerto y del handoff presentes.
  ```bash
  grep -c -i 'idempotente' services/go-run-controller/runctl/ports.go; grep -c -i -E 'a lo sumo una vez|at-most-once' services/go-run-controller/README.md
  ```
  Esperado: ≥ `1` y ≥ `1`.

- [ ] **CA-7** — Higiene, imagen y alcance.
  ```bash
  (cd services/go-run-controller && go vet ./... && go vet -tags contract ./... && test -z "$(gofmt -l .)" && echo ok); docker build -q -t aqs-go-run-controller:ci services/go-run-controller >/dev/null && docker inspect aqs-go-run-controller:ci --format '{{.Config.User}}'; git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/go-run-controller/|bitacoras/U2-T02b.md|revisiones/U2-T02b/)' | wc -l
  ```
  Esperado: `ok`, `65532:65532`, `0` y `0`.

---

## Plan de pruebas

- Escritor de disco inyectable en el diario (falla en el byte N, falla en `Sync`), reloj inyectable, reinicio simulado reabriendo el diario.
- Propiedad (`rapid`): para toda secuencia de `Save` con fallos aleatorios en `Write`/`Sync`, reabrir el diario **nunca** falla salvo por una línea completa que no encadena, y el estado reabierto es igual al del último `Save` exitoso.
- **Barrido de clase obligatorio** (la familia de los tres informes de U2-T02: efecto externo + fallo del almacén): tabla camino→prueba en la bitácora para diario, entrada de eventos, salida de eventos (`Publish`), handoff y lanzamiento de fases; cada fila con su prueba y, cuando aplica, su mutación.

**Rojo primero:** registrar las dos reproducciones del revisor (diario irrecuperable, evento perdido) fallando contra `main` antes de tocar nada.

---

## Notas

- **Tope de 3 rondas** (decisión del humano; `.claude/agents/orquestador.md`). En cada NO-VERDE el codificador hace el barrido de la clase del defecto, no solo el arreglo puntual.
- **Go y Dockerfile.** `go 1.26.8`; el `Dockerfile` ya existe. Podman: montajes con `:z`. `ajv`/`yq` no están en el PATH local: `npx` o la imagen `mikefarah/yq`.
- **Candidatas a registrar, no hacer:** HMAC + ancla del `seq`; rotación del diario; `halted` visible; timeout de `rehearsal.passed`.
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
