# Ronda 1 — U6-T02

VEREDICTO: NO-VERDE

Worktree `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U6-T02-web` (rama `tarea/U6-T02`, sha 29c0351). Lo dejé limpio (`git status --short | wc -l` = 0). Mi reproducción del defecto F-01 la hice en una copia en el scratchpad, no en el worktree.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Lo anterior sigue en verde | `cd services/ui-api && go vet ./... && go test -count=1 -race ./... \| grep -c -E '^(FAIL\|--- FAIL)'` | `0`. Los 6 paquetes dan `ok`. |
| 2 | `GET /warm` cumple su contrato | `go test -count=1 -race -run 'Warm' -v ./internal/httpapi/... \| grep -c -- '--- PASS'` | `9` (mínimo 8), pasa |
| 3 | `GET /confirmations` respeta la propiedad | igual con `'Confirmations'` | `7` (mínimo 6), pasa |
| 4 | Contrato válido y aditivo | `validate.sh`, lint de Redocly 1.25.0, `git diff $b -- control-plane.yaml \| grep -c '^-[^-]'`, `grep -c '^  /(warm\|confirmations):'` | `validate rc=0`, `lint rc=0`, `0` líneas borradas, `2` rutas. Pasa. |
| 5 | Higiene y alcance | comando literal de la tarea; y `git diff --name-only docs/u6-tareas HEAD \| grep -v ... \| wc -l` | Literal: `0` y `9`. Con la base real: `0` y `0`. Los 9 archivos son los del commit base `docs/u6-tareas` y no son de esta tarea. Pasa según tu aclaración. |

Los cinco comandos cumplen. Aun así el veredicto es NO-VERDE por F-01: el criterio CA-3 no cubre el reinicio, pero la tarea sí lo exige.

## Hallazgos

### F-01 · ROJO · `services/ui-api/inbox/store.go:62-96` (`OpenStore`) · `GET /confirmations` devuelve `[]` para todos los recibos persistidos tras un reinicio
La tarea dice que `/confirmations` «sale del almacén existente (`confirmations.jsonl`)». `Store.Receipts` itera `s.rorder`, pero `rorder` solo se rellena en `ConfirmTraced` (store.go:219). `OpenStore` carga los recibos en `s.receipt` y nunca toca `rorder`.

La bitácora dice «orden de llegada mantenido en `rorder` (al cargar y al confirmar)». Eso es falso: el diff no tiene ningún `append(s.rorder…)` en la carga. Es evidencia que no pudo salir del diff actual.

Efecto: tras cualquier reinicio de `ui-api`, el dashboard ve cero confirmaciones, aunque `confirmations.jsonl` las tenga. Con la entrega en `emptyDir` o PVC pasa en cada rollout.

Lo demostré con una copia en `.../scratchpad/ui/inbox/zz_restart_test.go`:
```
Confirm("a","u1") -> Receipts(20, todos) = 1 antes del reinicio
s2 := OpenStore(mismo dir)  -> Receipts(20, todos) = 0
zz_restart_test.go:17: tras reinicio: 0 recibos (esperado 1)
--- FAIL: TestZZReceiptsTrasReinicio
```
Hay que poblar `rorder` al cargar, con el orden del archivo (solo-agregar, sin duplicados de `notification_id`). Hay que añadir una prueba de reinicio en `inbox` y otra a nivel HTTP. Las 7 pruebas `Confirmations` usan un store en memoria creado en la misma sesión, así que esta clase de defecto no la puede detectar.

### F-02 · NARANJA · `warm_test.go` y `cmd/ui-api` · Faltan pruebas de dos comportamientos que el código ya tiene
Los comandos que busqué con grep no encontraron cobertura de:
- **Arranque con `WARM_URL` sin `UIAPI_WARM_TOKEN_FILE`.** El código de `newWarmClient` devuelve error, pero `grep -rn "TokenFile\|WARM"` no encuentra ninguna prueba que lo ejerza. Tampoco hay prueba de un `WARM_URL` inválido, es decir, sin esquema http(s) o con credenciales.
- **El 5xx no abre el circuito.** Es la interpretación (1) que el codificador pide juzgar. `TestWarmCircuito…` solo cubre el cierre de conexión.

Debería haber una prueba que haga 6 llamadas con un 5xx y compruebe que las 6 llegan al warm.

## Juicio de las tres interpretaciones
1. **El circuito cuenta solo transporte y plazo; un 5xx no lo abre.** Aceptada. La tarea dice literalmente «5 fallos de transporte seguidos». Un 5xx devuelve 503 con `Retry-After`, igual que antes. La interpretación necesita la prueba de F-02.
2. **Con `WARM_URL` y sin `UIAPI_WARM_TOKEN_FILE` no arranca.** Aceptada. Es fail-fast ante una mala configuración, coherente con «el token es un archivo, nunca una variable». Sin `WARM_URL` arranca y `/notifications` sigue en 200, que está probado. Falta la prueba (F-02).
3. **Un rol desconocido se trata como `user`.** Aceptada. Es el fallo seguro (`pr.Role != RoleAdmin` limita al propio ID). En la práctica el verificador ya rechaza roles distintos de `user` y `admin` (`auth.go:63`, `identity.go:196`), así que es una defensa en profundidad inalcanzable hoy.

## Otros puntos que revisé y están bien
- Los dos tokens quedan separados. El de servicio se lee del archivo en cada llamada y el de la persona no sale. Una redirección no se sigue (`ErrUseLastResponse`), así que el token tampoco se filtra por ahí.
- Se valida el `WarmState` (campos requeridos, sin extras, enum, minLength 1), y es equivalente al esquema de `contracts/plans/`.
- `limit` solo acepta dígitos, 1–100, y rechaza vacío o duplicado.
- El rol sale del `Principal` y no de `X-Role`.
- Sin literales visibles incrustados. Plazo, umbrales y `Retry-After` son constantes con nombre.
- El OpenAPI es aditivo y el `WarmState` copiado coincide con el esquema.

## Hallazgos AMARILLO
- F-03 · Falta una prueba de `readToken` con archivo ilegible o vacío (hoy da 503 y registra el error). Tampoco hay prueba del medio abierto del circuito, es decir, que se recupere tras 10 s.
- F-04 · Duplicación del circuito entre `internal/auth/identity.go` y `warm.go`. El codificador ya la anotó como candidata, y es fuera de alcance.

## Tareas candidatas
- Unificar el circuito de identidad y el del warm en un paquete común (la candidata del codificador, F-04).

## Rutas de transcripciones largas
- `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/4b6347b3-cdf1-46a9-87ea-67b85dc7ad0b/scratchpad/ca1.txt` (salida de CA-1)
- `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/4b6347b3-cdf1-46a9-87ea-67b85dc7ad0b/scratchpad/ui/inbox/zz_restart_test.go` (reproducción de F-01, fuera del repo)

VEREDICTO: NO-VERDE
ROJO|services/ui-api/inbox/store.go:62-96 (OpenStore no puebla rorder)|GET /confirmations devuelve vacío tras reinicio; la bitácora afirma lo contrario
NARANJA|warm_test.go / cmd/ui-api|Sin pruebas de arranque con WARM_URL sin token ni de que un 5xx no abre el circuito
INFORME: revisiones/U6-T02/ronda-1.md
