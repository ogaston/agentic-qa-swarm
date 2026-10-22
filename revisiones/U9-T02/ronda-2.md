# Ronda 2 — U9-T02

VEREDICTO: VERDE

Corrí yo mismo CA-1, CA-2 y CA-4, y todos pasan. El diff de esta ronda solo toca documentación y la bitácora. No repetí el e2e de kind porque el código no cambió desde 3742719. No quedan ROJO ni NARANJA.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Contratos y tipos | `bash contracts/validate.sh >/dev/null 2>&1; echo rc=$?` y `(cd web/dashboard && npm run -s check:api …; echo rc=$?)` | `rc=0` y `rc=0`, pasa |
| 2 | Controlador | `go vet ./...`; `go test -count=1 ./... \| grep -c FAIL`; `go test -count=1 -run 'Plan(Stored\|Endpoint)' -v ./... \| grep -c '^--- PASS'` | vet limpio, `0` FAIL, 11 PASS (el mínimo era 6), pasa |
| 3 | kind, con `--bug oversell` | No repetido, a propósito. `git diff 3742719 HEAD --stat` lista solo 3 archivos: `bitacoras/U9-T02.md` (+18), `docs/operaciones/gestion-de-cambios.md` (+5) y `services/go-run-controller/README.md` (+16). Los 39 insertos son documentación, y no hay cambios en `*.go`, `scripts/` ni `contracts/`. | El resultado de la ronda 1 sigue vigente: `OK plan-inspectable`, con solo los dos FALLA excluidos por el arbitraje 2 (`veredicto-bug`, `hallazgo-cita-invariante-y-evidencia`). No toqué el clúster en esta ronda. |
| 4 | Alcance | `git status --short \| wc -l`; `git diff --name-only $(git merge-base HEAD origin/main) \| grep -v -E '<patrón>' \| wc -l` | `0` y `1`. El único archivo fuera del patrón es `docs/operaciones/gestion-de-cambios.md`, que el arbitraje 4 declara dentro del alcance. Con esa excepción el conteo es 0 y el criterio pasa. |

## F-01 (NARANJA en la ronda 1): cerrado
Comprobé cada dato de la documentación contra el código y el manifiesto.

- **Campo `plan` (`Run.Plan`, `omitempty`):** exacto. En `runctl/ports.go:74` está `Plan json.RawMessage \`json:"plan,omitempty"\``.
- **`DisallowUnknownFields` en la apertura del diario:** exacto. Está en `adapters/journal.go:81`.
- **Texto del error:** exacto. `adapters/journal.go:85` produce `diario: línea %d ilegible: %w`. El `%w` envuelve el error estándar de Go `json: unknown field "plan"`, que ya reproduje en la ronda 1. La documentación escribe `línea N`, que es lo correcto.
- **Archivo del diario:** exacto. `RUN_DATA_DIR/runs.jsonl` coincide con `JournalFile = "runs.jsonl"` (`journal.go:20`) y con `RUN_DATA_DIR` en `main.go`.
- **Volumen `go-run-controller-data`:** exacto. Es el `claimName` en `deploy/flux/base/control-plane.yaml:289` y el nombre del PVC en la línea 307.
- **«No se edita el diario a mano: rompe la cadena de hash»:** exacto. Cada línea lleva `seq` y `prev` (`journal.go:24`), y la apertura devuelve `la cadena se rompe en la línea N` si no coinciden.
- **«Mismo almacén y misma atomicidad que `HandedOff` y `Launched`»:** coherente con la especificación y con el precedente de U2 que señalé en la ronda 1.
- **Regla de despliegue:** el README la dice con las dos salidas, ir hacia delante o vaciar el volumen y perder el estado de las corridas. Cierra la opción (a) que propuse. Añade como candidata el versionado y la lectura tolerante, que no se implementa.
- **Aviso en el lugar de rollback:** `docs/operaciones/gestion-de-cambios.md` es el único documento de `docs/` con una sección de rollback («Nota de rollback»). `docs/operaciones/README.md` solo enlaza a ese archivo, así que el aviso está donde corresponde.

## Hallazgos nuevos
### F-04 · AMARILLO · `docs/operaciones/gestion-de-cambios.md:20` · La condición del aviso es más amplia que la real
El texto dice «tras ese primer despliegue, el rollback no es posible». El README, que es más preciso, dice que falla solo si el diario ya contiene al menos una línea con `plan`. Un despliegue en el que ninguna corrida llegó a `rehearse` sí admitiría rollback. Es una sobrecautela segura y no bloquea.

## Tareas candidatas (fuera de alcance)
- Versionado y lectura tolerante del diario (campo de versión, o ignorar campos desconocidos). Cubre `HandedOff`, `Launched` y `Plan`, y hace reversible el rollback.
- F-02 de la ronda 1: el plan se repite en cada línea posterior del diario. Un hash de referencia o un archivo aparte lo evitaría.
- F-03 de la ronda 1: `plan_inspectable` solo compara conteos. Habría que comparar `flow_id` con el sufijo `runner-<run>-<flow_id>`.
- Planner con LLM real que no genera el sondeo de stock (ya abierta por decisión del humano, arbitraje 2).
- TTL o limpieza de Jobs en `aqs-test`.

## Estado del entorno
Worktree limpio (`git status --short | wc -l` = 0). No ejecuté nada contra el clúster. Informe de la ronda 1 en `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U9-T02/revisiones/U9-T02/ronda-1.md`.

VEREDICTO: VERDE
AMARILLO|docs/operaciones/gestion-de-cambios.md:20|La condición del aviso de rollback es más amplia que la real (el README es el preciso)
INFORME: revisiones/U9-T02/ronda-2.md
