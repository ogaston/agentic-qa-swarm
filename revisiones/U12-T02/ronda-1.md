# Ronda 1 — U12-T02

VEREDICTO: VERDE

## Criterios de aceptación, verificados por mí
Los corrí en el worktree, con `GOTOOLCHAIN=auto` para los comandos Go.

| # | Criterio | Resultado |
|---|---|---|
| 1 | `go vet ./... && go test -race -count=1 ./...` y contar `FAIL` | pasa: `0`, todos los paquetes `ok` |
| 2 | `TestEsperaAprobacionAntesDeRunning` | pasa |
| 3 | `TestApproveExecution` | pasa: 7 subcasos (no existe, otro usuario, admin ajeno, antes del ensayo, en running, ya aprobada por otra persona, dueño dos veces) |
| 4 | `TestAprobacionCaduca` | pasa |
| 5 | `-run Execute ./internal/httpapi` | pasa: `TestExecuteRun401SinToken`, `404`, `403`, `409`, `202YReadback` |
| 6 | `contracts/validate.sh` y `npm run check:api` | pasa: `contrato-ok` y `api-ok` |
| 7 | lint, typecheck y `vitest run` | pasa: lint y typecheck sin salida, conteo de fallos `0`, 274 pruebas pasan; `-t 'Ejecutar pruebas'` lista 2 en verde |
| 8 | Cerca de alcance | pasa: `0` archivos fuera de alcance |

El worktree quedó limpio (`git status --short` sin salida).

No pude comprobar el hash de la tarea. `sha256sum` da otro valor que el `882a…` indicado, pero ese hash es de `git hash-object`, que no corrí.

## Puntos que pediste vigilar
- **Gate de U4:** se sigue consultando. En `Drive`, `case True` sin aprobación devuelve `nil` antes del gate. Con aprobación pasa al flujo normal y `transition` llama a `Gate.Authorize`. El gate duro de `transition` ahora exige también `ExecutionApprovedBy != ""`, y si falta devuelve `ErrIllegal`.
- **Serialización e idempotencia:** `ApproveExecution` toma `k.mu` (`controller.go:98`), igual que `Drive` (`:401`) y `transition` (`:216`). El mismo principal puede repetir la aprobación y recibe `nil`. Otra persona recibe `ErrNotAwaiting`.
- **Caducidad:** usa `k.now()`, que sale del `Config.Now` inyectado, y llama a `failRun(..., "ejecución no aprobada a tiempo")`, que lleva la corrida a `resetting`. Lo comprueba `TestAprobacionCaduca`.
- **Propiedad en el POST:** el handler toma `principal.ID` y `principal.Admin` de `Verify`, que lee `principal_id` y `role` de `GET /auth/session`. No lee nada del cliente: no hay cuerpo ni cabecera de identidad.
- **Dashboard:** el sondeo sigue (no es estado terminal). El botón se deshabilita mientras la petición está en vuelo. El 409 refresca en silencio y 403 u otros errores muestran `RunErrorNotice`.

## Hallazgos
No hay ROJO ni NARANJA.

### F-01 · AMARILLO · `web/dashboard/src/pages/Run/RunPage.test.tsx`
Falta una prueba del 409 como refresco silencioso y otra del botón `disabled` durante la petición. Ambos comportamientos están implementados pero sin cubrir. La prueba del 403 se llama «un 403 al ejecutar muestra el aviso de acceso», no contiene «Ejecutar pruebas», así que el filtro `-t 'Ejecutar pruebas'` de CA-7 no la lista. Sí pasa en la suite completa. No bloquea.

### F-02 · AMARILLO · `runctl/controller.go:94-120`
`ApproveExecution` acepta la aprobación aunque `AwaitingSince` ya haya pasado el TTL, mientras ningún `Drive` haya hecho caducar la corrida. Es una carrera benigna: gana la aprobación o gana la caducidad, y ambas quedan registradas de forma coherente. Conviene dejarlo documentado.

## Tareas candidatas
- Ninguna.

## Rutas de transcripciones largas
- `/tmp/vt.txt` (salida de `vitest run`)

VEREDICTO: VERDE
AMARILLO|web/dashboard/src/pages/Run/RunPage.test.tsx|Sin pruebas del 409 silencioso ni del botón deshabilitado en vuelo
AMARILLO|runctl/controller.go:94|Aprobación aceptada tras el TTL si Drive aún no caducó la corrida
INFORME: revisiones/U12-T02/ronda-1.md
