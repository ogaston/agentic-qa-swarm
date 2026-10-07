# Ronda 2 — U1-T05

VEREDICTO: NO-VERDE

CA-1 a CA-8 pasan con mis ejecuciones. Queda un NARANJA (F-01): siguen sobreviviendo mutantes plausibles, en las mismas clases de la ronda 1. La batería la ejecuté yo, con seed aleatorio por corrida. El worktree sigue limpio (`git status --short` = 0), producción no cambió y no dejé procesos vivos.

## Criterios de aceptación (comandos literales, `GOTOOLCHAIN=go1.26.8`, `-count=1`)
| CA | Resultado |
|---|---|
| 1 | Pasa. go-intake `20 4 0`, ui-api `17 2 0`. |
| 2 | Pasa. `21` y `18` propiedades, `9` archivos con rapid. |
| 3 | Pasa. Con `./internal/gen` el segundo comando da `5` en ambos servicios. El contraejemplo es `Sha40` = ceros, `failed after 0 tests`. |
| 4 | Pasa. `0` y `0`. |
| 5 | Pasa. La bitácora enumera H-1 y hay 2 coincidencias de regresión. |
| 6 | Pasa. `go.mod:1` en cada servicio, `PBT.md` de 22 y 22 líneas, 4 y 4 coincidencias. |
| 7 | Pasa. `--- PASS: TestPBT_GeneratorCoverage` y `ok` en ambos. |
| 8 | Pasa. vet y gofmt ok en ambos, `0` de `git status`. El diff fuera de alcance son solo los dos informes de `revisiones/U1-T05/`, excluidos por la nota de la tarea. |

Otras verificaciones:
- **Estabilidad:** 12 corridas de la suite PBT por servicio, 0 fallos, ~1 s cada una. `-rapid.checks=1000`: 4,5 s en go-intake y 10 s en ui-api. `-race ./...` ok.
- **Dependencias y alcance:** `go.mod`/`go.sum` solo añaden rapid v1.3.0 en go-intake (ui-api ya lo traía). `cmd/*` no enlaza rapid ni `internal/gen`. No hay `Skip`, `Sleep`, red ni disco fuera de `t.TempDir()`. El único reloj es el seed.
- **Mutantes del codificador:** los 7 `.patch` pasan `git apply --check`.
- **H-1:** coherente. Con un parser corregido en mi copia, fallan `TestPBT_ParserNeverLaxerThanSchema` ("H-1 corregido…", tras 268 sorteos) y `TestPBT_Limit_ParserAcceptsDateTimeSchemaRejects`.
- **H-2:** verificado. Un mutante que rechaza el hex en mayúsculas lo mata solo `TestPBT_Limit_SignatureHexUpperAccepted`.
- **Veracidad de `PBT.md`:** la salvedad es la frase "0 supervivientes no equivalentes" de la bitácora (ver F-01). Cada `TestPBT_*` tiene su `TestPBT_Fixed_*`; esa frase del `PBT.md` es cierta.
- **CI del PR #36** (borrador, head 46eba29): en verde. `test`, `vuln`, `build` y `sbom` de go-intake y ui-api terminaron en `success`; también `discover`, `no-latest`, `validate` y `policies`. `publish` quedó `skipped`, que es lo normal en un PR.

## Mutantes
Mutantes y herramientas están en `/tmp/claude-0/-home-user/4ca8050b-f1f3-5eab-a78d-10b43e5bda2f/scratchpad/rev-u1t05-r2/` (`run.py`, `rununit.py`, `muts*.json`, `out*.txt`).

Corrí unos 275 mutantes nuevos por clase y reproduje 14 de mi ronda 1. Los números de abajo son corridas muertas sobre corridas totales, solo con `-run PBT`. "Suite unitaria" significa `go test -skip PBT`.

Muertos de forma fiable (6/6 o más):
- los 6 mutantes de prefijo del HMAC (los ocho de comparación: 16, 31, 1, sin el último, sin el primero…),
- anclas y largos de SHA, registro, tag y repo en `Resolve`,
- `refs/heads` sin barra y tag con `/` final,
- el parser contra el esquema (anclas de uuid y sha, validaciones de campo),
- `Apply`, `List` y `Confirm` (idempotencia, orden, empate, estado, persistencia),
- los 7 mutantes de la ronda 1 que reproduje excepto uno (el SHA en mayúsculas de `Resolve`, F-01),
- 5 de 5 mutantes de generadores (cobertura de clases).

Los 5 "equivalentes" del codificador lo son de verdad:
- **M09:** `hmac.Equal` ya compara longitudes.
- **M39:** `repoRE` exige repo no vacío.
- **U17:** `exactKeys` rechaza `artifact: null` antes.
- **U20:** `json.Unmarshal` rechaza datos tras el objeto.
- **U28:** el almacén no expone los recibos, así que no es observable por la API pública.

## Hallazgos
### F-01 · NARANJA · `gen.BadSHA`, `gen.GitHubRejected`, `githubsig` (`secrets()`), `ui-api` `variants` y `mutate` · Mutantes plausibles de firma, resolvedor, `Classify` y parser sobreviven o mueren por azar
La bitácora dice "Clase firma, resolvedor y `Classify`: 0 supervivientes no equivalentes". No es cierto.

Sobreviven siempre:
- **`Resolve` acepta SHA en mayúsculas** (`^[0-9a-fA-F]{40}$`): 0/14 (6+8). Es mi mutante de ronda 1 `r2-R14-sha-mayus`, y ya no muere. Causa: `BadSHA` caso 4 termina con `+ "A"`, así que casi nunca genera 40 caracteres en mayúsculas puras. Solo lo mata la prueba unitaria `TestArtifactRejections`. El codificador dice cubrir "mayúsculas" y no tiene este mutante en su tabla.
- **`Classify` con `[0-9a-f]{39,40}`:** 0/26, y sobrevive también la suite unitaria. `badSHA` usa `{0,39}` y casi nunca saca exactamente 39. El límite superior (41 a 45) sí se fuerza; el inferior no.
- **`Classify` con `strings.Contains` en lugar de `HasPrefix` para `refs/tags/`:** 0/26, y sobrevive la suite unitaria. Con `refs/heads/` mata 14/20. La lista de refs rechazados no trae el caso equivalente de tags.
- **ui-api, SHA con `{39,40}`:** 0/26. `variants["sha"]` no tiene longitud 39. Sobrevive la suite unitaria.
- **ui-api, uuid con `{8,9}` en el primer grupo:** 0/26. Sobrevive la suite unitaria.
- **ui-api, uuid con un guion opcional (grupo 2):** 0/6. Sobrevive la suite unitaria.
- **ui-api, uuid con la clase `[0-9a-gA-F]` en el primer grupo:** 0/6. Sobrevive la suite unitaria.
- **Longitud del secreto en `Verify` y `Sign`.** `secrets()` casi nunca pasa de 16 bytes: en 500 sorteos, 475 menores de 16 y 25 entre 16 y 31, ninguno de 32 o más.
  - `Verify` que trunca el secreto a 32 bytes: mata solo 5/20.
  - `Sign` que trunca el secreto a 32 bytes: 8/20.
  - Añadir un cero al secreto en `Sign`: 0/20 y sobrevive la suite unitaria, porque con menos de 64 bytes es equivalente. Nunca se prueba una clave mayor que el bloque de HMAC (64 bytes).

Mueren de forma no fiable. La propiedad muestrea pocos valores discretos entre 100 o 500 sorteos:
| Mutante | Muertes |
|---|---|
| PR `closed` aceptado | 15/20 |
| release `created` | 18/20 |
| release `released` | 16/20 |
| `refs/heads` con `Contains` | 14/20 |
| `latest*` rechazado como prefijo | 15/20 |
| SHA en mayúsculas en `Classify` | 18/20 |
| `Classify` sin distinguir mayúsculas del evento | 18/20 |
| registro `ghcr.io:` con puerto vacío | 17/20 |
| `version` sin validar | 18/20 |
| `version < 1` aceptado | 6/20 |
| `version == 2` aceptado | 6/20 |

`version` solo se prueba con valores sorteados dentro de `replacements`; no hay una variante determinista. Como patrón, el tag de 129 caracteres sí se fuerza y las acciones o versiones no.

Pedido:
- Hacer que `BadSHA` genere 40 hex en mayúsculas puras y 39 exactos. Añadir `{39}` a `GitHubRejected` y a `variants["sha"]`.
- Recorrer de forma determinista (ejemplo fijo o bucle) todas las acciones y eventos rechazados, y los valores numéricos de `version` (0, 2, -1).
- Añadir a `variants` variantes por grupo de uuid: guion ausente, 9 caracteres en el primero, un `g` en cada grupo.
- Ampliar `secrets()` con los límites 31, 32, 33, 63, 64, 65 y un secreto largo.
- Añadir un tag con `Contains` (por ejemplo `x/refs/tags/v1`) a los refs rechazados.
- Corregir la afirmación de la bitácora.

### F-02 · AMARILLO · `gen` · La cobertura no mide variedad
`TestPBT_GeneratorCoverage` solo comprueba presencia de clases. Sí atrapa los mutantes que quitan una clase: sin `pr-fork`, fecha siempre `Z`, sin tag de 128, sin 129, estado sin `confirmed`, `Flows` vacío (5/5 muertos). Pero un `Sha40` constante (`aaaa…`) pasa en go-intake y ui-api (0/4). Basta una comprobación de entropía mínima, por ejemplo que haya más de N SHA distintos en 500 sorteos.

### F-03 · AMARILLO · `TestPBT_NotificationJSONRoundTrip`, `TestPBT_ReceiptJSONRoundTrip`, go-intake `TestPBT_NotifyCreatedRoundTrip` · Round-trips casi tautológicos
Son `json.Marshal`/`Unmarshal` de los mismos structs. El único valor propio es el esquema, y ahí valida al generador, no a producción. Como `go-intake` no tiene `MarshalEvent`, esto está declarado en `PBT.md`; para `Notification` y `Receipt`, no.
- Renombrar `json:"run_id"` en `inbox/model.go` sobrevive las propiedades PBT (la suite unitaria lo atrapa).
- Cambiar `json:"sha"` por `commit` sobrevive PBT **y** la suite unitaria completa. Falta contrastar `Notification` y `Receipt` con el OpenAPI.
- No exijo arreglo en esta tarea, pero conviene dejarlo como candidata.

### F-04 · AMARILLO · propiedad `ReceiptStoreRoundTrip` · Huecos menores en `Store`
- `Confirm` sin `ErrNotFound` (confirmar un id inexistente) sobrevive PBT 0/6; lo atrapa la suite unitaria.
- El orden con empates de sub-segundo (`Unix()` en lugar de `Equal`) sobrevive PBT 0/6 y la suite unitaria.
- `ConfirmedAt` sin `.UTC()` sobrevive porque el entorno es UTC y se compara con `Equal`.
- Sin validación de `run_id`, `confirmed_by` o `notification_id` al cargar: sobrevive PBT y la suite unitaria. No es una de las cinco propiedades mínimas.
- Los mutantes de carga con línea truncada (`needsEOL`, línea final) los atrapa la suite unitaria.

## Supervivientes no equivalentes
Todos los de F-01 con 0 muertes (resolvedor con mayúsculas, `Classify` con 39 y con `Contains` de tags, parser ui-api con 39, uuid grupo 1 y guiones, clave HMAC mayor que 64) y los de F-04. Mi mutante `Verify` con `\n?$` en el nombre del repo (13/20) lo considero poco plausible en Go.

## Para la ronda 3
1. Corregir los generadores y las variantes de F-01 y la afirmación de la bitácora.
2. Volver a ejecutar mi batería para confirmar que mueren siempre. Los mutantes de F-01 están en `muts1.json`, `muts2.json` y `mutsR1.json`; las tasas están en `out20.txt` y `outR1.txt`.

VEREDICTO: NO-VERDE
