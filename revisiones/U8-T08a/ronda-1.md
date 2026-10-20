# Ronda 1 — U8-T08a

VEREDICTO: VERDE

## Criterios de aceptación, verificados por el revisor (todo con -count=1, worktree sha f9bea92)

| # | Criterio | Comando que corrió | Resultado |
|---|---|---|---|
| 1 | Generador nuevo, ui-api en verde | `cd services/ui-api && go test ./... -count=1` | pasa (6 paquetes ok; `inbox` 6.9 s, no es suite vacía). `store_test` exige len==20; la prueba de formato exige `^run-[0-9a-f]{16}$`. |
| 2 | Contrato cruzado | `cd services/ui-api && go test ./... -run 'RunID.*Contrato' -count=1 -v` | pasa: 6 PASS (Formato, WarmManager, GoReset, RunController, IdLargoRechazado, Unicidad 10 000). |
| 2b | Sensibilidad | Cambió `[8]byte` a `[16]byte` en `store.go` (32 hex), corrió y revirtió con `git checkout` | FALLAN `TestRunIDContratoFormato` y `TestRunIDContratoWarmManager`. También falla la suite `inbox` entera. Worktree limpio después. |
| 3 | Dashboard | `cd web/dashboard && npm test -- --run`; `grep -rn "{32}" src/pages/Inbox src/pages/Run \| grep -c "run-"` | pasa: 11 archivos, 104 tests, grep = 0. Sensibilidad: con `RUN_ID` de Inbox en `{32}`, la prueba nueva de 36 caracteres falla y se revirtió. Rojo de Run lo muestra la bitácora. |
| 4 | go-warm-manager | `cd services/go-warm-manager && go test ./... -count=1` | pasa (6 paquetes ok) |
| 5 | Alcance | `git status --short \| wc -l`; los dos `git diff --name-only` de CA-5 contra `ceb618e` | `0`, `0`, `0`. Contra `main` salen 2 archivos, que son los dos de docs de `ceb618e`. |

Rojo primero: la bitácora trae la salida del fallo contra el generador de 36 caracteres. La prueba de sensibilidad (2b) reproduce ese rojo sobre el código actual.

Validadores intactos: `git diff ceb618e -- services/go-warm-manager services/go-reset services/go-run-controller` no devuelve nada. `contracts/` y `specs/` no fijan el largo viejo (grep vacío). Las tres regex copiadas en `runid_contrato_test.go` coinciden carácter a carácter con las fuentes `types.go:53`, `job.go:32` y `phases.go:25`, cada una con su comentario de referencia. Worktree limpio al terminar.

## Hallazgos

Sin ROJO ni NARANJA.

### F-01 · AMARILLO · tarea CA-2 · el comando literal de CA-2 no corre desde la raíz
`go test ./services/ui-api/...` desde la raíz da `[setup failed]`, porque no hay `go.mod` ni `go.work` en la raíz. Es un defecto de la especificación, no del código. El codificador lo notó y corrió el comando dentro del módulo. Conviene corregir el comando en la tarea o en la plantilla.

### F-02 · AMARILLO · bitácora CA-4 · lectura de vuelta contra el servicio real, solo por transcripción
El `curl` pedido no se pudo hacer porque el binario exige clúster. El codificador usó un test temporal con `srv()` de `internal/api` y lo borró. Resultado: 404 para el id de 20, 202 en el POST y 400 para el de 36. No se pudo reproducir porque ya no existe, pero el comportamiento se deduce de la regex literal y de la suite de warm-manager en verde. Aceptable: el criterio admitía alternativa y U8-T08 lo ejercita de punta a punta en kind.

### F-03 · AMARILLO · `services/ui-api/inbox/runid_contrato_test.go` · regex copiadas, no importadas
Si un validador cambia, la copia no se entera. Es lo que pide la tarea, porque los paquetes son internos o de otro módulo. Se mitiga con las referencias archivo:línea en los comentarios.

## Tareas candidatas

- `web/dashboard/src/pages/Runs/RunsPage.test.tsx:10,16` conserva fixtures `'run-' + 'a'.repeat(32)` (formato viejo). No rompe nada y la página no valida el formato.
- Migración de `run_id` ya persistidos de 36 caracteres. Fuera de alcance, como indicó la tarea.

VEREDICTO: VERDE
