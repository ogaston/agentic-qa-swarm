# Ronda 3 — U1-T04

VEREDICTO: VERDE

HEAD `f78925a`, PR nuevo #34. Contexto: el PR #33 se fusionó en `main` con el código de la ronda 1 (NO-VERDE) antes de que terminaran las rondas 2 y 3; este PR trae el delta (base de comparación `83b3241`). Sin ROJO ni NARANJA. Worktree limpio; sin procesos del revisor vivos. `GOTOOLCHAIN=go1.26.8`.

## Criterios de aceptación, verificados por el revisor
| # | Resultado |
|---|---|
| 1 | 4 paquetes ok, 141 PASS, 0 FAIL con `-race` |
| 2 | `n-1 pending`; el JSON tiene exactamente la forma de `Notification` |
| 3 | `401 401 401 401` y `401` |
| 4 | `201 409 404`, `u1`, `1`, `1` (con reinicio) |
| 5 | `5` y `3` |
| 6 | `https://app.example`, `0`, `0` |
| 7 | 3 repeticiones: 5×200/25×429, 6/24, 6/24; `Retry-After: 1` (dentro del patrón arbitrado 5/25 o 6/24) |
| 8 | `0`, `0`, `1`; `git diff 83b3241..HEAD -- Dockerfile go.mod go.sum` vacío (`docker build` no repetido, arbitrado) |
| 9 | `go vet` y `gofmt` limpios; `0`; fuera de `services/ui-api/` y la bitácora solo `revisiones/U1-T04/*` (arbitrado) |

CI del PR #34 sobre `f78925a`: `ci` (incluidos `vuln (services/ui-api)`, `test`, `build`, `sbom`), `contracts` y `policies` en success: confirmación de govulncheck.

## Verificación del delta
**F-01 de la ronda 2 (claves duplicadas): resuelto.** Diferencial contra `contracts/events/notify.created.schema.json` (jsonschema/v6 con AssertFormat): 53.196 casos × 2 formatos, **0 casos** en que el parser acepta lo que el esquema rechaza (12.754 donde es más estricto: divergencias declaradas y rechazo de cualquier duplicado). Cobertura: todas las claves de raíz, `data` y `artifact` con 6 valores finales y escalares con `null` final o inicial; duplicados con cuatro formas de escape unicode (`repo`, `REPO`, escape del primer o último carácter, ambos) en ambos órdenes (1.328 detectados); capitalización distinta; objetos duplicados; duplicados dentro de arrays y de claves anidadas desconocidas; espacios, tabuladores, `\r\n`, BOM, basura al final, dos documentos; 50.000 mutaciones aleatorias (30.000 con duplicados y escapes). `"repo"` y `"repo"` se detectan como la misma clave (el tokenizador devuelve la clave resuelta). Binario real: cuatro líneas (`"repo":"acme/shop","repo":null`; `"repo":"acme/shop","repo":null`; `"repo":"acme/shop","repo":null`; `"repo":"acme/shop","repo":"acme/shop"`) descartadas con log `clave duplicada "repo"` y ausentes de `GET /notifications`; un webhook firmado real a `go-intake` → `202`, línea en el outbox y una notificación `pending`. Mutantes muertos: sin `rejectDuplicateKeys` y sin descender a objetos anidados (`TestParseRejectsDuplicateKeys`).

**F-06 (bordes de `MaxLineBytes`): resuelto.** Binario real con líneas de 1.048.574, 1.048.575, 1.048.576, 1.048.577 y 2.097.152 bytes de contenido seguidas de una normal: visibles la normal, 1.048.575 y 1.048.574; descartadas 1.048.576, 1.048.577 y 2 MiB con tres logs `supera 1048576 bytes` (el tope cuenta el `\n`). Mutantes muertos: `MaxLineBytes` +1, −1 y 2 MiB, `>=` en lugar de `>` y sin log (`TestFileSubscriberLineLimitBoundary`); `math.Floor` en lugar de `math.Ceil` (`TestRetryAfterRoundsUp`).

**Regresiones de las rondas 1 y 2: ninguna.** 25 confirmaciones concurrentes → 1×`201`, 24×`409`, 1 recibo en disco; doble `Authorization` en ambos órdenes (GET y POST, también dos válidas) → `401`; `state` repetido, `state=bogus` y `state=` → `400`; XFF sin `TRUST_PROXY`: `200 200 429 429`, con `TRUST_PROXY=true`: `200 200 200 200`; las 5 cabeceras de seguridad en 200, 404, 405, 400, 415 y 413; ningún `tok-user` en el log; línea de 50 MB sin salto: HTTP 200 en 5 s, pico 13,9 MB, y al llegar el salto se descarta con log y se lee la siguiente.

## Hallazgos (AMARILLO, no bloquean)
### F-01 · `services/ui-api/inbox/event_test.go` · sin caso de escape unicode en las pruebas
`TestParseRejectsDuplicateKeys` (36 casos) no incluye un duplicado con escape unicode. La protección viene de `json.Decoder.Token()`; el diferencial y el binario la verifican, pero ninguna prueba del repo la fija. Basta añadir 2 casos.

### F-02 · bitácora · evidencia de CA-7
Depende de temporización; la garantía real la dan las pruebas con reloj inyectable.

## Tareas candidatas
- Recordar el offset de la línea parcial pendiente en `Drain` (F-07 de la ronda 2).
- Rechazar claves duplicadas en cualquier otro parser de eventos del repo (`go-intake` y demás consumidores del outbox).

VEREDICTO: VERDE
AMARILLO|services/ui-api/inbox/event_test.go|Sin caso de escape unicode en TestParseRejectsDuplicateKeys (verificado por diferencial y binario, no fijado por prueba)
AMARILLO|CA-7|Depende de temporizacion; la garantia real son las pruebas con reloj inyectable
INFORME: revisiones/U1-T04/ronda-3.md
