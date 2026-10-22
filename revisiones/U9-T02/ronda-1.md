# Ronda 1 — U9-T02

VEREDICTO: NO-VERDE

Todos los criterios de aceptación pasan con mi propia ejecución. Queda un NARANJA en pie: el diario nuevo rompe el rollback a un binario anterior (F-01). Los hallazgos de abajo vienen de mi lectura del diff y de mis ejecuciones.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Contratos y tipos | `bash contracts/validate.sh >/dev/null 2>&1; echo rc=$?; (cd web/dashboard && npm run -s check:api ...; echo rc=$?)` | `rc=0` y `rc=0`, pasa |
| 2 | Controlador | `go vet ./... && go test -count=1 ./... \| grep -c FAIL` y `go test -run 'Plan(Stored\|Endpoint)' -v ./... \| grep '^--- PASS'` | vet limpio, `0` FAIL, 11 PASS (el mínimo era 6), pasa. Cubre los 7 casos del criterio: guardado en la primera obtención, `Flows` una sola vez entre ensayo y runners, reinicio con store reabierto, 200, `run_not_found`, `plan_not_ready` y plan corrupto que falla cerrado. |
| 3 | kind, con `--bug oversell` (arbitraje 1) | Con flock: `AQS_IMAGES=go-run-controller build-images.sh`, `load-images.sh`, `rollout restart` de go-run-controller (imagen de HEAD), y luego `kind-e2e.sh --bug oversell`. Log: `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/7c206815-66cc-47fb-9c25-52913a74a0bf/scratchpad/rev-e2e.log` | `OK plan-inspectable`. Hay 21 OK y 2 FALLA: `veredicto-bug` (`verdict=sin-hallazgos`) y `hallazgo-cita-invariante-y-evidencia`. Esos dos son los excluidos por el arbitraje 2, así que el conteo que cuenta es 0 y el criterio pasa con esa excepción. |
| 4 | Alcance | `git status --short \| wc -l`; `git diff --name-only $(git merge-base HEAD origin/main) \| grep -v -E '<patrón>' \| wc -l` | `0` y `0`; la base `baca6ec` coincide con origin/main, pasa |

Sobre la conclusión del codificador (arbitraje 2), con la corrida `run-3a425c5a9dc07bd3` de mi e2e:
- El planner recibió una sola petición para esa corrida (una línea en los logs de agent-planner).
- El controlador resolvió 3 flujos: `happy-products`, `happy-health` y `happy-orders`.
- Hay 3 Jobs `runner-run-3a425c5a9dc07bd3-*` y 1 `rehearsal-…-1`.
- `plan-inspectable` comprobó plan válido por esquema, `run_id` correcto y n_flujos = n_Jobs.
- Ese plan no trae el probe de stock, así que no se pierde ningún flujo. Plan guardado = plan ejecutado, y la conclusión se sostiene.

## Hallazgos

### F-01 · NARANJA · `runctl/ports.go:74` (`Run.Plan`) + `adapters/journal.go:81` · El diario escrito por esta rama no lo puede abrir un binario anterior: el rollback AR4 queda roto
Este diff añade `Run.Plan` al registro persistido. `JournalStore` abre el diario con `DisallowUnknownFields` (preexistente), así que un `go-run-controller` de main aborta al arrancar si encuentra una sola línea con `plan`. Lo reproduje sin tocar el clúster:
- Escribí un diario con el código de HEAD, usando `StoredPlans` sobre `JournalStore`, en `$S/jdir/runs.jsonl`. La línea 2 trae `"plan":{"run_id":"r-1",…}`.
- Lo abrí con `OpenJournal` del código de `origin/main`. Resultado:
```
--- FAIL: TestZZOpen
    zz_open_test.go:3: diario: línea 2 ilegible: json: unknown field "plan"
```
Es exactamente el CrashLoopBackOff que vio el orquestador. Es un defecto de este diff por tres razones:
- El diario es append-only y no se compacta, así que toda corrida que llegó a `rehearse` deja `plan` para siempre. Tras el primer despliegue de esta rama no existe rollback a ningún binario pinneado anterior, que es lo que promete AR4.
- Recuperar el estado exige borrar el diario (se pierde el estado de las corridas) o editarlo a mano, y editarlo rompe la cadena de hash.
- La tarea pedía "documentar el formato". Solo hay un comentario Go en `planstore.go`; ni `services/go-run-controller/README.md` ni ningún runbook mencionan que el diario pasa a ser irreversible.

Atenuantes, para que decidas la severidad final:
- La tarea mandó "mismo store, misma atomicidad", y esa elección implica la incompatibilidad. Con la especificación tal cual no hay solución de código que evite el fallo; un archivo aparte sí la evitaría, pero contradice la especificación.
- Hay precedente: `HandedOff` y `Launched` se añadieron igual en U2.

Por eso lo dejo en NARANJA y no en ROJO. Para cerrarlo, una de dos:
- (a) El codificador documenta en el README del controlador el formato de `Run.Plan` y la regla de despliegue: "no se hace rollback a una versión sin `Run.Plan` con un diario que ya tenga planes; hay que vaciar el volumen o ir hacia delante".
- (b) El orquestador cambia la especificación (almacén de planes separado del diario).

Antes de cerrar, hay que decidir si el rollback AR4 se acepta como roto para este campo.

### F-02 · AMARILLO · `adapters/journal.go` `Save` · El plan se repite en cada línea posterior del diario
`Run` entero se serializa en cada `Save`, y el plan puede pesar varios KB con el LLM real. Cada guardado posterior al ensayo (unos 10 por corrida) lo duplica en disco, y el arranque lo relee y revalida todo. No bloquea. Un esquema con referencia al plan (hash) o un archivo aparte lo evitaría.

### F-03 · AMARILLO · `scripts/kind/kind-e2e.sh` `plan_inspectable` · Solo compara conteos
`n == j` pasa también si el planner devolviera flujos con nombres distintos a los de los Jobs, porque no compara `flow_id` con el sufijo `runner-<run>-<flow_id>`. Lo pedía la tarea (cantidad), así que no bloquea.

## Tareas candidatas (fuera de alcance)
- Versionado y lectura tolerante del diario (campo de versión, o ignorar campos desconocidos), para que un binario nuevo no deje irrecuperable el rollback. Es la clase que cubre `HandedOff`, `Launched` y `Plan`.
- Planner con LLM real que no genera el probe de stock (ya abierta por decisión del humano, arbitraje 2).
- TTL o limpieza de Jobs en `aqs-test` (la cuota de 100 Jobs ya bloqueó una corrida; candidata ya anotada por el codificador).

## Estado del entorno
- El worktree `agentic-qa-swarm-wt-U9-T02` quedó limpio (`git status --short | wc -l` = 0).
- El clúster quedó con la imagen go-run-controller `0.0.0` de HEAD cargada y desplegada. Solo reconstruí y cargué esa imagen. No toqué Secrets, credenciales ni PVCs.
- Los diarios y copias de las pruebas de F-01 están en el scratchpad (`…/scratchpad/{branchsrc,mainsrc,jdir}`), fuera del repo.

## Rutas de transcripciones largas
- `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/7c206815-66cc-47fb-9c25-52913a74a0bf/scratchpad/rev-e2e.log`

VEREDICTO: NO-VERDE
NARANJA|services/go-run-controller/runctl/ports.go:74 + adapters/journal.go:81|Un binario anterior no puede leer el diario con `plan`: rollback AR4 roto y formato sin documentar
INFORME: revisiones/U9-T02/ronda-1.md
