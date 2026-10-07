# Ronda 1 — U2-T02

VEREDICTO: NO-VERDE

Dos pruebas mías, escritas fuera del repo, reproducen dos defectos del controlador: un lanzamiento de reset sin tope y un `rehearsal.passed` que salta el ensayo. Los criterios CA-1 a CA-9 salen verdes tal como están redactados. Los dos defectos viven en caminos que ninguno de esos comandos ejercita.

## Criterios de aceptación, verificados por mí (comandos frescos, `-count=1`)
| # | Criterio | Qué corrí | Resultado |
|---|---|---|---|
| 1 | ≥25 PASS, 0 FAIL, ≥1 SKIP de contrato | los tres comandos de CA-1 | pasa: `43`, `0`, `4` |
| 2 | Gates sin confirmación, reset o ensayo | `go test -run 'Gate(Deny\|Unknown\|Error)\|Without(...)'` | pasa: 7 `--- PASS` en `runctl` |
| 3 | Contra go-governance real | mi `up` y `go test -tags contract` | pasa: 4 `--- PASS`, ningún `FAIL`/`SKIP` |
| 4 | `GET /runs/{id}` extremo a extremo | `up` y curl | pasa: objeto con `id`/`state`/`trace_id`, luego `404 401 503` |
| 4b | Esquema `Run` con ajv real | `yq` (imagen docker) para extraer `components.schemas.Run`, `npx ajv-cli validate` | pasa: `run.json valid`; un estado falso se rechaza |
| 5 | Reanudación, diario, idempotencia | `go test -run 'Resume\|Journal\|Idempotent'` | pasa: 3 pruebas y más |
| 6 | Fail-closed y vallas | el comando literal | pasa: `rc=1` en los 4 casos |
| 7 | Métricas y logs sin secretos | `up`, curl a `/metrics`, grep | pasa: 4 nombres de métrica y `0 0 0` |
| 8 | Imagen y sin k8s | `go list`, `docker build`, `list-services.sh` | pasa: `0`, `65532:65532`, `1` |
| 9 | Higiene y alcance | vet (con y sin `-tags contract`), `-race`, gofmt, status, diff | pasa: `ok`, `0`, `0` |

- **CA-4 y el esquema.** La prueba Go del codificador con `jsonschema/v6` es equivalente a ajv. El esquema `Run` solo tiene `id`, `state` y `trace_id`. Con ajv real el resultado es el mismo.
- **CA-6 no discrimina.** En `prod+fake` y `url no http` la configuración está incompleta, así que el `rc=1` sale por variables faltantes y no por la valla. Lo repetí con configuración completa y `RUN_ENV` en `prod`, `Prod`, `PRODUCTION`, `production` y ` prod `. Todos fallan con "no se permite con RUN_ENV=prod". Con `staging` o vacío arranca. La valla es correcta, como en U1-T07 F-02.
- **Caja negra, gate caído.** Con governance detenido, `r-2` se queda en `confirmed`. Metrics: `gate_calls{error}=7`. No avanza y no hay llamada al gate que dé `allow`.
- **Caja negra, denegación.** Con `RUN_TEST_NAMESPACE=otro-ns`, governance real deniega. El diario muestra `confirmed→warm_ready` denegada y después `confirmed→failed` permitida. `GET /runs/r-1` devuelve `failed`.
- **Reanudación.** Tras truncar y reiniciar el controlador, la corrida se reanuda desde el último estado persistido.
- **Worktree.** Quedó limpio después de todo (`git status` vacío).

## Hallazgos

### F-01 · ROJO · `runctl/controller.go` `Drive` y `phaseFailed`, Alcance «Reintentos (V8)» · el reset se relanza sin tope con el gate caído
Si la fase `reset` agota sus 3 fallos y en ese instante el gate está caído, `phaseFailed` llama a `failRun` y `transition` devuelve `ErrGate`. La corrida queda en `resetting` con `FailReason` ya fijado y `Attempts[reset]=3`.

En el siguiente `Drive`, la rama `r.FailReason != "" && r.State != Resetting` se salta justamente para `Resetting`. Con `Launched[reset]` borrado, vuelve a lanzar el reset. Cada intento vuelve a fallar, hace handoff y llama a `failRun`, hasta que el gate responda. La vía normal (`running`→`resetting` por fin de corrida) cae en el mismo camino.

Prueba mía, fuera del repo, en `scratchpad/probe/p_test.go` (`TestResetExhaustGateDown`):
```
state=resetting reset launches=28 attempts=map[reset:28] handoffs=26
FAIL: lanzamientos de reset > 3: 28
```
Esto viola «nunca hay reintento infinito ni un tercer lanzamiento». Genera un job de reset por tick (500 ms) y 26 handoffs duplicados.

`TestPropertyNeverAdvancesOnDenyNeverLeavesTerminalMaxTwoRetries` no lo detecta por cuatro motivos:
- su tope de lanzamientos es `3*5=15`, no 3;
- los fallos de fase son aleatorios solo para deploy, infer y reset;
- `Attempts>3` no se comprueba contra los lanzamientos;
- la secuencia del gate es aleatoria, pero nunca asegura que el gate esté caído en el instante del agotamiento.

La causa raíz es la misma que la del F-03.

### F-02 · NARANJA · `runctl/controller.go` `Apply` (rehearsal.*) · `ensayo_passed` se acepta fuera de la fase de ensayo
`Apply` fija `EnsayoPassed` en cualquier estado no terminal. Un `rehearsal.passed` que llegue con la corrida en `confirmed` (desorden o evento de otra ronda) hace que, al llegar a `rehearsing`, se lance la fase y se transite de inmediato con `ensayo_passed=true`. El controlador reporta como conocido un hecho que no observó.

Prueba mía `TestEarlyRehearsalPassed`: `state=done`, y el gate recibió `to=running` con `ensayo_passed=true`. La corrida cierra sin ensayo real. Aunque go-governance decide, aquí el controlador le miente con un hecho; la tarea exige que sea `unknown` cuando no lo conoce. Corrección esperada: aceptar `rehearsal.*` solo en `rehearsing` (con la fase lanzada), o descartar y registrar en otro caso, más una prueba.

### F-03 · NARANJA · `runctl/controller_test.go:280` y CA-1 · pruebas que no sostienen lo que prometen
- La propiedad solo comprueba que el último gate llamado tiene `To == after.State`. No comprueba que ese gate dijera `allow`, y la tarea pide justamente «nunca avanza si el gate denegó».
- No hay prueba del gate caído en `resetting` ni en el agotamiento, que es la clase de F-01.
- CA-2 incluye `TestGateErrorDownStopsRun`, pero solo desde `confirmed`.

Barrido de clase: no hay prueba de fase agotada cuando el gate no responde, y no hay prueba de evento fuera de estado (F-02).

### F-04 · NARANJA · `internal/obs/obs.go:78` más `controller.go` (todos los `*Context` con `"trace_id"`) · clave `trace_id` duplicada en los logs
El manejador del logger añade `trace_id` desde el contexto, y vale `""` fuera de una petición. El controlador además pasa su propio `trace_id`. La línea sale con dos claves:
`"trace_id":"4bf92f..."` … `"request_id":"","trace_id":""`.

Con `jq` o cualquier parser que tome la última clave, el `trace_id` de la corrida se pierde. El criterio pide logs con `trace_id`, y ninguna prueba lo comprueba. Corrección sugerida: poner el `trace_id` de la corrida en el contexto, o no duplicar la clave.

### F-05 · AMARILLO · `adapters/journal.go` · el hash encadenado no es anclado
- Truncar la cola del diario es indetectable. Lo probé: borrar las líneas 3 a 8 devuelve `r-1` a `deploying`, y la corrida repite fases.
- Un atacante con escritura sobre `RUN_DATA_DIR` puede recalcular la cadena, porque no hay HMAC.
- La reproducción no valida la legalidad de las transiciones entre líneas sucesivas.

Esto es aceptable respecto al texto de la tarea («hash de la anterior»). Anótalo como defecto para una tarea candidata (HMAC y ancla del último `seq`).

### F-06 · AMARILLO · bitácora, función `up` · el token de servicio puede salir de menos de 32 caracteres
`head -c 24 /dev/urandom | base64 | tr -d '=+/'` a veces produce menos de 32 caracteres. Entonces go-governance no arranca ("debe tener al menos 32 caracteres"). Me falló en una de mis ejecuciones. Usar `head -c 48`.

### F-07 · AMARILLO · `failRun`, rama de denegación · el error de persistencia se confunde con una denegación
Si `Store.Save` falla en `transition`, `failRun` lo trata como un `deny`. Pone `Halted=true`, hace handoff «sin salida segura» y trata de guardar de nuevo, cuando el estado real es solo un error de disco. Además, `halted` no es visible en `GET /runs/{id}`.

## Puntos que miraste con lupa
- **(1) Fail-closed y gates:** correcto en gate denegado, caído, con timeout y con respuesta inválida. `unknown` nunca viaja como `true`. Salvo F-02, las decisiones sobre `confirmed`, `workflow_allowed`, `reset_verified` y `ensayo_passed` son coherentes con go-governance.
- **(2) Salida segura y límite V8:** a partir de `deploying`, `inferring`, `rehearsing` o `running` la salida segura se reintenta una vez por tick y no hay bucle apretado. El límite de 2 reintentos se rompe en `resetting` (F-01).
- **(3) Idempotencia:** `run.confirmed` repetido no crea otra corrida. `Seen` deduplica los eventos de ensayo. La reanudación funciona.
- **(4) Valla de fases falsas y `RUN_ENV`:** correcta.
- **(5) Secretos:** el token reenviado a identidad y `GOVERNANCE_SERVICE_TOKEN` no aparecen en logs ni en `/metrics` (`0 0 0`).
- **(6) Alcance «Fuera»:** sin `k8s.io`, y sin tocar `contracts`, `deploy` ni workflows (diff fuera de alcance = 0).

## Tareas candidatas
- HMAC y ancla del último `seq` en el diario (F-05).
- Tiempo máximo de espera de `rehearsal.passed`; el codificador ya lo anotó.
- Visibilidad del estado `halted` en `GET /runs/{id}` y reanudación por operador.
- El CA-6 literal de la tarea es poco discriminante para `prod+fake`. Convendría redactarlo con la configuración completa.

VEREDICTO: NO-VERDE
ROJO|services/go-run-controller/runctl/controller.go (Drive/phaseFailed, V8)|El reset se relanza sin tope (28 lanzamientos, 26 handoffs) si el gate cae al agotar sus 3 fallos
NARANJA|services/go-run-controller/runctl/controller.go (Apply rehearsal.*)|ensayo_passed se acepta fuera de rehearsing: la corrida llega a done sin ensayo real
NARANJA|services/go-run-controller/runctl/controller_test.go:280|La propiedad no verifica allow ni tope de lanzamientos; sin prueba de gate caído en resetting ni de evento fuera de estado
NARANJA|services/go-run-controller/internal/obs/obs.go:78|trace_id duplicado en los logs: la última clave vacía borra el de la corrida
INFORME: revisiones/U2-T02/ronda-1.md
