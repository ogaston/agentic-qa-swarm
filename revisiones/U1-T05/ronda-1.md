# Ronda 1 — U1-T05

VEREDICTO: NO-VERDE

CA-1 a CA-8 pasan con mi ejecución propia. Queda un NARANJA (F-01) que bloquea: mutantes plausibles de la firma, el resolvedor y `Classify` sobreviven a las propiedades. El worktree quedó limpio (`git status --short` = 0) y no dejé procesos vivos. Mis sondas y mutantes están en el scratchpad (`mut.sh`, `mut2.sh`, `r2-*`, `rate-*`).

## Criterios de aceptación, verificados por mí
Usé `GOTOOLCHAIN=go1.26.8` y `-count=1`.

| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Propiedades pasan, seed visible | comando literal de la tarea | pasa: go-intake `14 4 0`, ui-api `10 2 0`; 1,2 s cada uno |
| 2 | Número de propiedades | greps literales | pasa: `15`, `11`, `9` |
| 3 | Demo falla con seed y reproduce | comando literal, en los dos servicios | pasa con la salvedad de abajo |
| 4 | `go test ./...` normal sin demo | comando literal | pasa: `0` y `0`, todos los paquetes `ok` |
| 5 | Regresión del defecto hallado | comando literal | pasa: la bitácora enumera H-1 y hay 1 coincidencia |
| 6 | Framework fijado y documentado | comando literal | pasa: `go.mod:1` dos veces, `PBT.md` de 20 y 22 líneas, 4 y 4 coincidencias |
| 7 | Cobertura de generadores | `-run PBT_GeneratorCoverage -v` | pasa: `--- PASS` y `ok` en ambos |
| 8 | Higiene y alcance | comando literal | pasa: vet y gofmt ok en ambos, `0` y `0` |

- **CA-3, primer comando:** falla en `Sha40` con `-rapid.seed=1791231203124368348` (go-intake) y `...5689987188` (ui-api), y da `FAIL`.
- **CA-3, segundo comando:** con `./...` cuenta 6 (go-intake) y 8 (ui-api), porque los paquetes sin rapid dan `flag provided but not defined`. Con `./internal/gen` reproduce el mismo `failed after 0 tests` y el mismo contraejemplo. Coincido con el arbitraje: es defecto de especificación, no del codificador.
- **Otros comandos de `PBT.md`:** `-rapid.checks=1000` con `./internal/...` o `./inbox` pasa (8,9 s y 4,8 s). Con `./...` falla en los paquetes sin rapid, como el documento dice. El `go test -race ./...` normal pasa en ambos servicios.
- **Estabilidad:** 15 corridas de la suite PBT por servicio, 0 fallos.
- **Dependencias:** `go 1.26.8` intacto, `pgregory.net/rapid v1.3.0`, `go mod tidy` sin cambios. Los binarios `cmd/*` no enlazan `internal/gen` ni rapid. No hay `Skip`, `Sleep`, red ni disco fuera de `t.TempDir()`. No hay `go.work` ni `replace`.
- **CI (PR #36, borrador):** no está confirmada.
  - El run `ci` de 16746c6 terminó en `failure`, pero no por un fallo de pruebas. Los jobs `discover` y `no-latest` no tuvieron runner durante 15 min y quedaron `cancelled`; `test`, `vuln`, `build`, `publish` y `sbom` quedaron `skipped`. El job `vuln` no corrió.
  - `contracts` y `policies` terminaron en `success`; un primer `contracts` quedó `cancelled`.
  - Hay que relanzar `ci`. No es culpa del codificador.

## Hallazgos
### F-01 · NARANJA · propiedades `Signature`, `ArtifactRejectsInvalid`, `ClassifyRejects` · Mutantes plausibles sobreviven por límites que los generadores no tocan
El criterio fue: todo mutante plausible que sobrevive es hallazgo. Medí la tasa de muerte con el código de salida de los binarios de prueba (`mut2.sh`, 15 corridas con seed aleatorio). Los parches del codificador y mis 30 mutantes propios están más abajo.

Mutantes que sobreviven:
- **Firma (`githubsig.go`).** `Verify` compara solo los 16 bytes iniciales del HMAC (`hmac.Equal(got[:16], sum[:16])`): muerto 0/15. Con 31 bytes también sobrevive. Tampoco lo caza ninguna prueba unitaria (`go test -skip PBT` da `ok`). Causa: `TestPBT_Signature` altera cuerpo y secreto, pero nunca la firma. Un `Verify` que acepta firmas truncadas o falsificadas pasa en verde.
- **Resolvedor (`artifact.go`).** `shaRE` sin ancla `^…$` (0/15) y `[0-9a-f]{40,41}` (0/15) sobreviven, y los tests de ejemplo tampoco los cazan. `UnresolvableEvent` solo genera SHAs más cortos, nunca más largos ni con basura.
- **Resolvedor, registro.** Quitar `if !regRE.MatchString(reg)` sobrevive (0/15). Solo lo caza el test unitario de ejemplo: `UnresolvableEvent` no genera registros inválidos, y la propiedad dice cubrir el fail-closed.
- **`Classify`.** `HasPrefix(p.Ref, "refs/heads")` sin la barra sobrevive (0/15; no se genera `refs/headsX`). Recortar un `/` final del tag se detecta solo 2/15 (13 %).
- **Detección parcial, sin sobrevivir siempre.**
  - Tag válido máximo 128→129: 7/20.
  - uuid con 11 a 13 hex finales en `ui-api`: 6/15.
  - sha en mayúsculas en `ui-api`: 11/15.
  - Acción `closed` aceptada en PR: 7/8.

Pedido, en la misma ronda:
- Alterar un byte de la firma, o acortarla o alargarla un byte, en `TestPBT_Signature`.
- Añadir a `UnresolvableEvent` SHAs de 41 o más caracteres, SHAs con basura delante o detrás, y registros inválidos.
- Forzar el largo exacto 129 y el tag con `/` final en `InvalidTag`.

### F-02 · AMARILLO · `ui-api/inbox/pbt_test.go` `variants` · `ParserNeverLaxerThanSchema` es ciega al defecto H-1
La propiedad está verde con 20000 sorteos (6 s), porque `variants["occurred_at"]` no contiene desplazamientos fuera de rango. La bitácora dice que "la halló"; eso fue antes de sacar esos valores. Hoy H-1 lo cubren solo las 4 muestras del `Limit`. Cuando se corrija el parser, conviene añadir `+24:00`, `-23:60` y `,5Z` a `variants`.

### F-03 · AMARILLO · `go-intake/internal/intake/pbt_test.go:~170` · El mensaje de reproducción no es idéntico
`check` itera un `map`, y con el mismo seed reporta `d-b` en una corrida y `d-a` en otra. El fallo y la entrada son los mismos; cambia solo la clave citada. Basta iterar en orden.

### F-04 · AMARILLO · `ui-api/internal/gen` · Dos generadores no los usa ninguna propiedad
`Notification` (con sus 3 estados) y `ConfirmationReceipt` solo se sortean en `TestPBT_GeneratorCoverage`. La cobertura de estados es cobertura de un generador decorativo. Además, `Free` excluye la cadena vacía, aunque la tarea pide vacíos entre las cadenas arbitrarias. Aparecen solo en tags y ramas.

### F-05 · AMARILLO · `PBT.md` de ambos servicios · "Cada una lleva un ejemplo fijo" es demasiado
`ParserNeverLaxerThanSchema`, `ClassifyRejects`, `StoreLastWinsPerID` y `ArtifactRejectsInvalid` no tienen su `TestPBT_Fixed_*` propio. Los cubren parcialmente los ejemplos de la propiedad vecina.

## Valoración de los puntos del arbitraje
**H-1: verificado, y el no-arreglo está justificado.**
- Corrí el esquema real (jsonschema/v6, `AssertFormat`) contra `ParseNotifyCreated`. Esquema y parser divergen en `+24:00`, `-24:00`, `+23:60` y `10:00:00,5Z`, y también en `+24:01`, `-23:60` y `+00:60`, que la bitácora no lista. Es una sola causa: `time.Parse` es más permisivo que `date-time`.
- La prueba `Limit` documenta bien. Afirma que el esquema rechaza y el parser acepta. Con un parser corregido en mi copia, falló con "defecto corregido; invertir esta prueba".
- La tarea solo permite tocar `go.mod` y `go.sum`, y la nota de U4-T06 prevé el `Limit` sin arreglo.
- La exposición es baja: `go-intake` emite siempre UTC con `Format(time.RFC3339)` (`handler.go:172`). Solo se alcanza con un outbox editado a mano.

**Otras divergencias de la misma clase (parser contra esquema).** Probé 123 valores sobre `event_id`, `sha`, `trace_id`, `notification_id`, `repo`, `ref`, `version` y `occurred_at`, y 20 casos estructurales.
- **Parser más laxo que el esquema:** solo `occurred_at` con los desplazamientos y la coma de H-1. No hay divergencia en `uuid`, `sha` (incluidas mayúsculas, `\n`, 41 caracteres y dígitos Unicode), `minLength` (espacios, `\u0000`, sustitutos sueltos), bytes inválidos, BOM, basura final, comentarios ni NaN.
- **Parser más estricto que el esquema:**
  - Documentadas: segundo intercalar `23:59:60Z`, `t` y `z` en minúscula, y `version` como `1.0`, `1e0`, `1E0`, `1.00`, `0.1e1`, `10e-1`.
  - Hallazgo mío, no documentado: una `\u005a` escapada en JSON dentro de `occurred_at` (`…10:00:00\u005a`). El esquema la acepta y `time.Time.UnmarshalJSON` la rechaza.

**Mutantes del codificador.**
- Los 7 `.patch` pasan `git apply --check` sobre `origin/main` y son mutaciones plausibles.
- Cada uno rompe una propiedad con seed y contraejemplo reducido: go-intake 01 a 04 y ui-api 01 a 03 (3/3 corridas; el 03, 10/10).
- Reproduje con `-rapid.seed` el fallo del 04 de go-intake.
- El 01 de ui-api llegó a tardar 24,8 s en reducir el fallo.

**Mis 30 mutantes.** Firma: prefijo ignorado (muerto 3/3), HMAC con clave y mensaje invertidos (muerto 3/3), secreto vacío aceptado (muerto), compara 16 y 31 bytes (sobrevive, F-01).
- **Almacén JSONL:**
  - pierde la última línea al cargar: muerto
  - no indexa la entrega: muerto
  - no escribe los pendientes sin artefacto: muerto
  - primera línea gana: muerto
- **Resolvedor:**
  - acepta `latest` y tag vacío→`latest`: muertos
  - tag en minúsculas: muerto
  - sin chequeo de fork: muerto
  - comparación de fork sensible a mayúsculas: muerto
  - repo sin minúsculas: muerto
  - tag con inicio `-`: muerto
  - sin chequeo de SHA en commit: muerto
  - evento en minúsculas: muerto
  - repo sin ancla: muerto
  - sobreviven: SHA sin ancla y `{40,41}`, registro sin validar (F-01)
- **`Classify`:**
  - 8 mutantes muertos: SHA cero solo en ramas, SHA en mayúsculas y sin ancla, PR `closed`, release `created`, `HeadRepo` de la base, tag con `TrimSpace`, repo en minúsculas, evento sin distinguir mayúsculas
  - sobreviven: `refs/heads` sin barra y tag con `/` final recortado (F-01)
- **`ui-api`:**
  - muertos o casi siempre muertos: type, kind, ref vacío, repo vacío, notification_id vacío y trace_id vacío ignorados, y los campos que siguen al objeto
  - sobreviven: uuid sin `^` y claves duplicadas aceptadas (el esquema las acepta; el parser es más estricto a propósito)

Una primera tanda mía con `grep` sobre la salida dio falsos "SURVIVE" porque la salida trae bytes NUL. Rehice todos los dudosos por código de salida; los números de arriba son de esa segunda pasada.

**`TestMain` en los paquetes sin PBT.** No hace falta. La CI corre `go test ./...` sin flags, y eso pasa. El flujo de reproducción de `PBT.md` ya dice que los flags van sobre un paquete con rapid. Además `contract` es un paquete solo de pruebas y `cmd/*` está fuera de alcance.

**Oráculos y generadores.**
- Son independientes en el parser contra el esquema real, el almacén JSONL (lee el archivo), el resolvedor (regex más `repo@sha`) y `Classify` (el generador trae el valor esperado).
- `Signature` es autoconsistente (`Sign` contra `Verify`); solo el vector fijo de GitHub es independiente.
- Los generadores cubren las clases de GitHub, forks, tags válidos e inválidos incluido `latest` en 4 capitalizaciones, Unicode y largos límite. El shrinking sigue activo (no hay flags que lo toquen).
- El seed se imprime siempre (`rapid seed: N`, visible con `-v`) y `-rapid.seed` reproduce.

## Tareas candidatas (defectos reales fuera de alcance)
- **H-1:** endurecer `occurred_at` en `inbox.ParseNotifyCreated` (regex RFC 3339 antes de `time.Parse`) e invertir `TestPBT_Limit_ParserAcceptsDateTimeSchemaRejects`.
- **Escapes JSON en `occurred_at`:** aceptar el escape `\u005a` (o decidir y documentar que se rechaza).
- **Segundo intercalar:** decidir y documentar `23:59:60Z`.
- **Resolvedor:** alinear el criterio de SHA entre `intake.Classify` y `artifact.Resolve` y probarlo con casos de largo mayor y basura, además del registro inválido.
- **CI:** relanzar `ci` en el PR #36 para confirmar `test` y `vuln`.

## Rutas de transcripciones largas
- `/tmp/claude-0/-home-user/4ca8050b-f1f3-5eab-a78d-10b43e5bda2f/scratchpad/mut.sh` y `mut2.sh` (harness de mutantes)
- `/tmp/claude-0/-home-user/4ca8050b-f1f3-5eab-a78d-10b43e5bda2f/scratchpad/m/services/ui-api/inbox/probe_test.go` y `probe2_test.go` (sondas parser contra esquema)

VEREDICTO: NO-VERDE
NARANJA|services/go-intake/internal/githubsig + artifact + intake (pbt_test.go, internal/gen)|Mutantes plausibles sobreviven: Verify compara solo un prefijo del HMAC (0/15, nada lo detecta), Resolve con SHA sin ancla o {40,41} y sin validar registro, Classify con refs/heads sin barra; la firma nunca se altera y los generadores no tocan límites superiores
INFORME: revisiones/U1-T05/ronda-1.md