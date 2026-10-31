# Ronda 1 — U12-T03

VEREDICTO: VERDE

## Criterios de aceptación, verificados por mí
Corrí cada comando en el worktree `aqs-U12-T03`, en `7f6ed64`. No ejecuté nada contra `kind-aqs`: las pruebas usan dobles.

| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Pruebas nueva y existente | `python3 -I scripts/test/demo-refresh-images-test.py \| tail -1; python3 -I scripts/test/demo-preflight-test.py \| tail -1` | `OK` / `OK` |
| 2 | Solo la plataforma | `... -v \| grep 'test_restarts_only_platform_deployments .* ok'` | pasa |
| 3 | Lectura de vuelta | `... -v \| grep 'test_fails_if_pod_predates_restart .* ok'` | pasa |
| 4 | Guarda de contexto | `... -v \| grep 'test_wrong_context_touches_nothing .* ok'` | pasa |
| 5 | Filtro `AQS_IMAGES` | `... -v \| grep 'test_aqs_images_subset .* ok'` | pasa |
| 6 | `up.sh` refresca | `... -v \| grep 'test_up_refreshes_existing_cluster .* ok'` | pasa |
| 7 | Sintaxis | `bash -n ...` | `sintaxis-ok` |
| 8 | Alcance | `git diff --name-only $b \| grep -v ... \| wc -l` | `0` |

El worktree quedó limpio después de la revisión. No pude correr `shellcheck`: no está instalado.

## Puntos que pediste mirar
- **Guarda de contexto:** `require_kind_context` es lo primero que ejecuta `scripts/kind/refresh-images.sh` (línea 16), antes de `podman`, `kind` y `kubectl`. El único `kubectl` previo es el de la guarda.
  - Quité la guarda en una copia en `/tmp` y `test_wrong_context_touches_nothing` falló. La prueba sí detecta esa regresión.
- **Lectura de vuelta por `startTime`:**
  - La hora del reinicio se toma con `date -u` después de build y load, y antes de los `rollout restart` (líneas 31-32).
  - El script convierte esa hora y cada `startTime` a epoch y falla con `aqs-system/ui-api-old` en stderr si el pod es anterior (líneas 92-93).
  - La prueba usa un pod con `startTime` de 2000 que debe fallar y otro de 2099 que debe pasar. No se puede pasar sin comparar de verdad.
  - Excluye pods con `deletionTimestamp` y los que no están `Running`.
  - Intenté una mutación del comparador en una copia en `/tmp`. Falló demasiadas pruebas porque la copia carecía de `services/`, así que esa mutación no es concluyente. La lógica es correcta por lectura y por el caso 2000/2099.
- **Deployments ajenos:** el filtro `jq` exige que la imagen empiece por `$IMAGE_PREFIX/`. `warm-app` queda fuera y la prueba comprueba que ninguna llamada de `kubectl` lo nombra.
- **Sin deployments:** imprime `OK (0 deployments reiniciados)`, sale con 0 y no llama a `rollout`. Hay una prueba extra, `test_no_platform_deployments_is_a_noop`.
- **Orden en `up.sh`:**
  - Con `fresh=1`, build y load van antes de `demo-preflight.sh --deploy` y no se llama a `refresh-images.sh`.
  - Con `fresh=0`, `refresh-images.sh` va después de `demo-preflight.sh --deploy` y antes de `dashboard-up.sh`.
  - Un fallo de `refresh-images.sh` corta `up.sh` por `set -euo pipefail`.
  - La prueba ejecuta el tramo real de `up.sh` con `bash` sustituido por una función que registra los argumentos, así que no es tautológica.
- **Dobles:** las aserciones leen los registros de llamadas que escriben `kubectl`, `kind` y `podman`, no solo el código de salida. `jq` es el real. Según la bitácora, el rojo previo fue real (CA-6 falló antes del cambio).

## Hallazgos
### F-01 · AMARILLO · `scripts/test/demo-refresh-images-test.py:26-45` · El doble de `kubectl` ignora el selector `-l`
El doble devuelve el mismo `pods-<ns>.json` sin importar el selector. La construcción del selector en `refresh-images.sh` no se prueba. Un selector mal formado pasaría las pruebas. No bloquea.

### F-02 · AMARILLO · `scripts/kind/refresh-images.sh:32` · Resolución de un segundo en `restart_time`
`restart_time` se trunca al segundo. Un pod previo cuyo `startTime` cayera en el mismo segundo pasaría como "posterior". El riesgo es despreciable porque entre build/load y el reinicio hay tiempo. Solo lo anoto.

### F-03 · AMARILLO · `scripts/test/demo-refresh-images-test.py` · Sin caso de pods ajenos antiguos
No hay caso con pods ajenos y antiguos, por ejemplo `warm-app` con `startTime` viejo, que demuestre que no se leen ni fallan. Hoy `warm-app-1` tiene `FUTURE`, así que no se ejercita. No bloquea.

## Tareas candidatas
- Ninguna.

## Rutas de transcripciones largas
- Ninguna.

VEREDICTO: VERDE
AMARILLO|scripts/test/demo-refresh-images-test.py:26-45|El doble de kubectl ignora el selector -l
INFORME: revisiones/U12-T03/ronda-1.md
