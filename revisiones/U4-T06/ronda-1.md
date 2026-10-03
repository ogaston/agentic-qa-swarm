# Ronda 1 — U4-T06

VEREDICTO: VERDE

CA-1..CA-9 corridos por el revisor con `GOTOOLCHAIN=go1.26.8` y `-count=1`. Sin ROJO ni NARANJA. CA-8 pasa por el arbitraje (H-1 y H-2 son defectos/límites de producción registrados y fijados con pruebas `Limit`, sin arreglo por alcance). Worktree limpio al empezar y al terminar.

## Criterios de aceptación, verificados por el revisor
| # | Resultado |
|---|---|
| 1 | governance `21 / 5 / 0`, identity `12 / 6 / 0` (esperado ≥13/≥1/0 y ≥6/≥1/0) |
| 2 | sin líneas `FALTA`, tres marcas `fin-…` |
| 3 | con el `rc` capturado antes: los 4 mutantes aplican limpio, dan `rc=1` y la semilla se ve; tras `git apply -R`, `git status`=0 |
| 4 | con `./authz "-rapid.seed=N"`: `rapid.seed=`, `failed after`, `FAIL`; al repetir con el seed, el mismo contraejemplo `"AQS-TEST"` |
| 5 | `0` y `0`; governance 2 s, identity 4 s |
| 6 | `--- PASS: TestPBT_GeneratorCoverage` y `ok` en ambos |
| 7 | `go.mod:1` ×2; `PBT.md` de 22 y 18 líneas; coincidencias 4 y 4 |
| 8 | el grep da 1; H-1 y H-2 fijados en `TestPBT_Limit_*` |
| 9 | `ok`, `ok`, `0`, `0`, `0` |

Defectos de especificación confirmados: en CA-3 `$?` se expande tras `$(basename …)` y siempre imprime `rc=0`; en CA-4 el segundo comando corre en un directorio sin paquete (`no Go files … [setup failed]`). `go 1.26.8` intacto en ambos módulos con `pgregory.net/rapid` v1.3.0. CI del PR #24 sobre `f68aeff`: `ci` (con `vuln` de ambos módulos, `test`, `build`, `sbom`), `contracts` y `policies` en success.

## Foco adversarial
- Mutantes de la tarea: con 31 seeds cada uno, las propiedades los matan 31 de 31 (01, 02, 03; el de ventana TOTP también).
- Mutantes propios del revisor, todos detectados: quitar `reset_verified` solo en `deploying`, quitar `ensayo_passed` en `running`, `failed` desde un estado terminal, quitar el namespace en `warm_ready` o en `running`, `EqualFold` en el namespace, ignorar el fallo de auditoría en `Authorize`, cuota 1001 y 0, complejidad sin validar, workflow duplicado, `confirm_required=false`, `audit.Verify` sin `prev_hash` o sin hash, reutilización de TOTP con `<` y ventana 0. Equivalentes (no cuentan): `len(code) < Digits` y `&& false`. Superviviente parcial: nombre de workflow de 64 caracteres aceptado (`{0,63}`): lo mata el ejemplo fijo, la propiedad sola en 20 de 60 seeds (F-01).
- Oráculo independiente: el grafo legal está escrito a mano en `gen.LegalEdges` y se compara con `authz.LegalTransition`; las invariantes de gates son negativas y se evalúan sobre el `RuleEvaluator` real; `TestPBT_Invariant_AuditFailureNeverAllow` falla si ninguna entrada se permite con auditoría sana.
- Cobertura de generadores: `AQS-TEST`, `aqs-test `, vacío, prefijo, Unicode confundible, estado inválido, `unknown`/`true`/`false`, workflow vacío, de la política y desconocido.
- Seed y demo: el seed reproduce con el mismo `failed after N` y contraejemplo; el shrinking sigue activo; sin la etiqueta `-run PBT_Demo` da `no tests to run`.
- Sin `Sleep` ni `time.Now`; suites de 2–4 s; sin red; disco solo en `t.TempDir()`.
- H-1 y H-2 confirmados con las pruebas `Limit` sobre el código sin tocar: `Verify` acepta la línea fusionada con `0x00` y devuelve 1 entrada, y acepta el prefijo tras truncar la cola. Las pruebas documentan el límite y fallan si alguien lo corrige. El no-arreglo está justificado (producción fuera de alcance; CA-9 solo deja tocar `go.mod` y `go.sum`).

## Hallazgos (AMARILLO, no bloquean)
### F-01 · `services/go-governance/internal/gen/gen.go:402` · el límite de 64 caracteres del nombre de workflow queda poco muestreado
Rama dedicada o más peso a longitud 64.

### F-02 · `services/go-governance/internal/audit/pbt_test.go` · `TestPBT_Limit_AuditTailTruncationUndetected` mezcla dos límites (H-2 y H-3, este último no es defecto)
Separarlos aclararía cuál afirmación falla si se arregla H-2.

### F-03 · `services/go-identity/internal/session/pbt_test.go` · `TestPBT_Token` es débil por construcción
Los bloques aleatorios inyectados son distintos por construcción; solo detectaría un truncado de entropía.

### F-04 · `services/go-governance/PBT.md` · `rapid seed` y el seed de reproducción pueden diferir
`rapid seed: N` es el seed base y rapid imprime `-rapid.seed=N+k`; ambos reproducen. El documento no lo aclara.

## Tareas candidatas (fuera de alcance)
- `audit.Verify` (H-1): rechazar bytes tras el valor JSON de cada línea (`dec.Token() == io.EOF`, como `policy.strict`) y actualizar `TestPBT_Limit_AuditTrailingBytes`.
- `audit.Verify` (H-2): anclar la cabeza de la cadena fuera del archivo; hasta entonces ajustar la invariante «cualquier borrado» (límite ya conocido, C-64).
- Corregir los comandos de CA-3 y CA-4 de la especificación.

VEREDICTO: VERDE
AMARILLO|services/go-governance/internal/gen/gen.go:402|límite de 64 caracteres del nombre de workflow poco muestreado (solo el ejemplo fijo lo mata siempre)
INFORME: revisiones/U4-T06/ronda-1.md
