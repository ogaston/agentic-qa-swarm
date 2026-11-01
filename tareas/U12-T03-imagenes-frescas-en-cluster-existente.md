# U12-T03 — Imágenes frescas al levantar la demo sobre un clúster existente

**Unidad:** U12 — Control humano de la ejecución
**Historias que implementa:** S5 (soporte operativo: lo fusionado tiene que llegar al clúster de la demo).
**Depende de:** U12-T01 y U12-T02 (fusionadas). **Ola 2**. **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.
**Origen:** candidata C-136. Diagnóstico: el clúster `kind-aqs` corre `go-run-controller` y `ui-api` importadas el 2026-10-26, así que la confirmación de ejecución de U12-T02 (fusionada el 2026-10-31) no aparece. `scripts/demo/up.sh` solo construye y carga imágenes cuando el clúster es nuevo (`fresh=1`), y los pods con tag fijo `:0.0.0` no se reinician aunque la imagen del nodo cambie.

---

## Alcance

**Dentro** (una línea):

> Script nuevo `scripts/kind/refresh-images.sh` que construye, carga, reinicia los deployments de la plataforma y comprueba que todos sus pods arrancaron después del reinicio; `scripts/demo/up.sh` lo llama siempre, con el clúster nuevo o existente.

Detalle:

1. **`scripts/kind/refresh-images.sh`** (nuevo, `set -euo pipefail`, carga `scripts/kind/lib.sh`):
   - Lo primero que hace es `require_kind_context`, antes de cualquier `podman`, `kind` o `kubectl` (sale con 3 si el contexto no es `kind-aqs`).
   - Ejecuta `scripts/kind/build-images.sh` y luego `scripts/kind/load-images.sh`, pasándoles `AQS_IMAGES` si está definida.
   - Anota la hora del reinicio (UTC, RFC 3339) y lista los deployments de **todos** los namespaces cuya imagen de algún contenedor empieza por `$IMAGE_PREFIX/` (con `AQS_IMAGES`, solo los que usen esas imágenes). A cada uno le hace `kubectl --context kind-aqs -n <ns> rollout restart deployment/<nombre>` y después `rollout status --timeout=180s`. No toca deployments con otras imágenes (por ejemplo `aqs-test/warm-app`, que usa `ghcr.io/ogaston/demo-target-app-qa-swarm`).
   - **Lectura de vuelta:** después de los `rollout status`, lista los pods en ejecución de esos deployments y falla con código ≠ 0, nombrando `<ns>/<pod>`, si alguno tiene `.status.startTime` anterior a la hora del reinicio. Si todo va bien, imprime `refresh-images: OK (<n> deployments reiniciados)`.
   - Si no hay ningún deployment de la plataforma (clúster recién creado y aún sin desplegar), no reinicia nada y sale con 0 tras build y load.
2. **`scripts/demo/up.sh`**: se quita el `if [ "$fresh" -eq 1 ]` que envuelve `build-images.sh` y `load-images.sh`. En su lugar:
   - Clúster nuevo: `build-images.sh` y `load-images.sh` como hoy, antes de `demo-preflight.sh --deploy`. Ahí no hace falta reiniciar nada.
   - Clúster existente: `bash "$root/scripts/kind/refresh-images.sh"` después de `demo-preflight.sh --deploy` y antes de `dashboard-up.sh`.
   - Un fallo de `refresh-images.sh` corta `up.sh` (no se registra el webhook con imágenes viejas).
3. **`scripts/demo/README.md`**: una frase que diga que cada `up.sh` sobre un clúster existente reconstruye, carga y reinicia las imágenes de la plataforma.
4. **Prueba nueva `scripts/test/demo-refresh-images-test.py`** (`unittest`, sin clúster): pone en el `PATH` dobles de `kubectl`, `kind` y `podman` que registran sus argumentos en un archivo y devuelven respuestas fijas (contexto, lista de deployments, pods con `startTime`). Usa `jq` real.

**Fuera** (rechazar aunque se vea roto):

- Ejecutar `up.sh`, `refresh-images.sh`, `build-images.sh`, `load-images.sh` o cualquier `kubectl` contra el clúster real `kind-aqs`. Todo se prueba con dobles. La verificación en vivo la hace el humano en el PR.
- Cambiar el tag `0.0.0`, los manifiestos de `deploy/` o la lógica interna de `build-images.sh` y `load-images.sh`.
- Comprobar `awaiting_execution` en el humo (`kind-smoke.sh`). Es la candidata C-137.
- Cambios en servicios Go, agentes o dashboard.

---

## Archivos de contexto

- `scripts/demo/up.sh` (bloque `fresh`, líneas 59-70 aprox.) y `scripts/demo/README.md`
- `scripts/kind/lib.sh` (`require_kind_context`, `IMAGE_PREFIX`, `IMAGE_TAG`, `all_images`, `AQS_IMAGES`)
- `scripts/kind/build-images.sh`, `scripts/kind/load-images.sh`
- `scripts/kind/demo-preflight.sh` (qué hace `--deploy`)
- `scripts/test/demo-preflight-test.py` (estilo de prueba sin clúster que hay que seguir)
- `scripts/kind/kind-e2e.sh:643-663` (cómo se usa ya `rollout restart` + `rollout status`)

---

## Criterios de aceptación

- [ ] **CA-1** — La prueba nueva y la existente pasan.
  ```bash
  python3 -I scripts/test/demo-refresh-images-test.py 2>&1 | tail -1; python3 -I scripts/test/demo-preflight-test.py 2>&1 | tail -1
  ```
  Esperado: `OK` dos veces.

- [ ] **CA-2** — Reinicia solo lo de la plataforma. `test_restarts_only_platform_deployments`: con dobles que devuelven `aqs-system/go-run-controller`, `aqs-system/ui-api` (imagen `ghcr.io/ogaston/agentic-qa-swarm/...:0.0.0`) y `aqs-test/warm-app` (otra imagen), el registro de llamadas de `kubectl` tiene `rollout restart deployment/go-run-controller` y `deployment/ui-api` con su `-n`, cada uno seguido de su `rollout status`, y **ninguna** llamada que nombre `warm-app`. La salida tiene `refresh-images: OK (2 deployments reiniciados)`.
  ```bash
  python3 -I scripts/test/demo-refresh-images-test.py -v 2>&1 | grep -E 'test_restarts_only_platform_deployments .* ok'
  ```
  Esperado: una línea.

- [ ] **CA-3** — Lectura de vuelta. `test_fails_if_pod_predates_restart`: si el doble de `kubectl get pods` devuelve un pod de `ui-api` con `startTime` anterior a la hora del reinicio, el script sale con código ≠ 0 y stderr nombra `aqs-system/<pod>`. Con todos los `startTime` posteriores, sale con 0.
  ```bash
  python3 -I scripts/test/demo-refresh-images-test.py -v 2>&1 | grep -E 'test_fails_if_pod_predates_restart .* ok'
  ```
  Esperado: una línea.

- [ ] **CA-4** — Guarda de contexto. `test_wrong_context_touches_nothing`: con el doble de `kubectl config current-context` devolviendo `kind-cks2`, el script sale con 3 y el registro de llamadas no tiene ninguna invocación de `podman` ni `kind`, y la única de `kubectl` es `config current-context`.
  ```bash
  python3 -I scripts/test/demo-refresh-images-test.py -v 2>&1 | grep -E 'test_wrong_context_touches_nothing .* ok'
  ```
  Esperado: una línea.

- [ ] **CA-5** — Filtro `AQS_IMAGES`. `test_aqs_images_subset`: con `AQS_IMAGES=ui-api`, solo se construye, carga y reinicia `ui-api` (registro de `podman build`, `kind load` y `rollout restart`).
  ```bash
  python3 -I scripts/test/demo-refresh-images-test.py -v 2>&1 | grep -E 'test_aqs_images_subset .* ok'
  ```
  Esperado: una línea.

- [ ] **CA-6** — `up.sh` refresca en un clúster existente. `test_up_refreshes_existing_cluster`: extrayendo de `scripts/demo/up.sh` el tramo entre `bash "$root/scripts/kind/kind-up.sh"` y `bash "$root/scripts/kind/dashboard-up.sh"` (como hace `test_setup_uses_preflight`) y ejecutándolo con `bash` reemplazado por una función que registra sus argumentos, con `fresh=0` queda registrado `scripts/kind/refresh-images.sh` después de `demo-preflight.sh --deploy`; con `fresh=1` quedan `build-images.sh` y `load-images.sh` antes de `demo-preflight.sh --deploy`.
  ```bash
  python3 -I scripts/test/demo-refresh-images-test.py -v 2>&1 | grep -E 'test_up_refreshes_existing_cluster .* ok'
  ```
  Esperado: una línea.

- [ ] **CA-7** — Sintaxis.
  ```bash
  bash -n scripts/kind/refresh-images.sh && bash -n scripts/demo/up.sh && echo sintaxis-ok
  ```
  Esperado: `sintaxis-ok`.

- [ ] **CA-8** — Alcance.
  ```bash
  b=$(git merge-base HEAD main); git diff --name-only $b | grep -v -E '^(scripts/kind/refresh-images\.sh|scripts/demo/(up\.sh|README\.md)|scripts/test/demo-refresh-images-test\.py|bitacoras/U12-T03(\.md|/)|revisiones/U12-T03/)' | wc -l
  ```
  Esperado: `0`.

---

## Plan de pruebas

- **Rojo primero:** CA-6 debe fallar hoy (con `fresh=0`, `up.sh` no construye ni carga nada). Pegar el fallo en la bitácora.
- Las aserciones leen el archivo de registro que escriben los dobles. No vale comprobar solo el código de salida.

## Verificación en vivo (la hace el humano en el PR, no el codificador)

```bash
bash scripts/demo/up.sh
kubectl --context kind-aqs -n aqs-system get pods -l app=go-run-controller -o jsonpath='{.items[*].status.startTime}'
```
El `startTime` debe ser de hoy. Una corrida nueva debe quedarse en «Esperando tu confirmación» en `/runs/:id`.
