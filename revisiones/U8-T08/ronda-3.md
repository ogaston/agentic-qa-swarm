# Ronda 3 — U8-T08

VEREDICTO: VERDE

Sha revisado: 1ec8000. El revisor corrió CA-1 a CA-5 sobre el clúster `kind-aqs` ya desplegado.

| # | Criterio | Comando | Resultado |
|---|---|---|---|
| 1 | Camino feliz | `bash scripts/kind/kind-e2e.sh --bug off` | 21 OK, 0 FALLA, rc=0 |
| 2 | Bug sembrado detectado | `bash scripts/kind/kind-e2e.sh --bug oversell` | `OK veredicto-bug`, `OK hallazgo-cita-invariante-y-evidencia`, 23 OK, rc=0 |
| 3 | Puerta humana | `bash scripts/kind/kind-e2e.sh --sin-confirmar` | `OK sin-confirmar-no-crea-corrida`, rc=0 |
| 4 | Humo y limpieza | `kind-smoke.sh`, `ss -ltn`, `kubectl get pods -A` | `0`, `0`, `0`; «smoke: OK» |
| 5 | Alcance | `git status --short \| wc -l`; diff filtrado | `0` y `0` (4 archivos: bitácora, kind-local.md, kind-e2e.sh, kind-smoke.sh) |

## Puntos de escrutinio
1. **Defecto antes del ensayo:** cumple el espíritu y no enmascara. El controlador lanza los runners al terminar el ensayo, así que un parche posterior llega tarde. En la corrida oversell el ensayo registra `status:201, ok:true, probe:true` y el runner estricto de `flow-1` registra `expect_status 409, status 201, ok:false` (Job `Failed`). El reporter rechaza `bug` sin flujo fallido (`reporter.py:174-176`), por lo que CA-1/CA-2 forman un par sensible. El reset verificado pasó.
2. **`wait_state` acepta `resetting`:** no altera ningún CA; no hay falso verde global (`ensayo-job`, `reset-verificado`, veredicto y reporte son independientes).

## Hallazgos
- **F-01 · AMARILLO · `scripts/kind/kind-e2e.sh:987`:** el check `runners` acepta `resetting` (estado de salida de una corrida fallida). Mejora: mirar `fail_reason` vacío o Jobs `runner-*` completos.
- **F-02 · AMARILLO · `kind-e2e.sh` (`regen_planner_fixture`, ~603):** comentario dice «aplicado tras el ensayo»; ahora es antes.
- **F-03 · AMARILLO · aviso del lock en `kind-e2e.sh`:** `aqs-kind.lock` envenenado por residuos del bringup; se continúa sin lock (candidata 4, declarada).

## Candidatas (fuera de alcance)
- Pausa explícita entre ensayo y runners en `go-run-controller`.
- El log del controlador debería incluir el `detail` del 422 del reporter.
- Lock `aqs-kind.lock` envenenado por fds heredados de los contenedores de kind.
- `kind-smoke.sh` crea una corrida real y no la limpia (herencia declarada).

VEREDICTO: VERDE
