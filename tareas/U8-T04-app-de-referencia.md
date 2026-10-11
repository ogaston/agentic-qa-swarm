# U8-T04 — App de referencia y `baseline.sh` real (reemplazable por la app del cliente)

**Unidad:** U8 — Producto demostrable (ciclo completo en kind)
**Historias que implementa:** US-M2 (deploy sobre el warm), US-M3 (superficie inferible), US-M7.1 (reset verificado); candidatas C-100 (`target-app`) y C-104 (baseline real).
**Depende de:** U7 cerrada. **Ola 1**. **Tope: 3 rondas**.

---

## Alcance

**Dentro** (una línea, concreta):

> Crear `demo/target-app/` (Go, con Dockerfile, imagen `ghcr.io/ogaston/agentic-qa-swarm/target-app:0.0.0`): una API transaccional mínima de pedidos sobre la Postgres del warm (`warm-db`) que publica `/openapi.json`, `/health`, `GET/POST /products`, `GET/POST /orders`, con la invariante «el stock nunca es negativo» y un **defecto sembrado** activable con `TARGET_APP_BUG=oversell` (permite vender más del stock); y un `baseline.sh` real para `go-reset` con el contrato `clean|verify|version` que habla con la app por endpoints de administración autenticados con un token del Secret `target-app-baseline-token`.

Detalle:

- **Endpoints de baseline** (solo dentro del clúster, puerto aparte `:9090`, token obligatorio): `POST /__aqs/baseline/clean` (trunca y vuelve a sembrar los datos fijos), `GET /__aqs/baseline/verify` (imprime el número de filas que difieren del baseline; `0` = limpio), `GET /__aqs/baseline/version` (imprime `ref-v1`).
- **`baseline.sh`** (`demo/target-app/baseline/baseline.sh`): usa solo `sh` y `wget` de busybox (la imagen de `go-reset` es alpine), lee el token de un archivo montado, imprime solo lo que exige `services/go-reset/internal/adapters/script/script.go` (`verify` → un entero). Sustituye al relleno `kind-stub` de U7-T03: `scripts/kind/secrets.sh` crea el ConfigMap `go-reset-baseline` desde este archivo (sin pisar uno existente que no sea `kind-stub`) y el Secret del token.
- **Imágenes**: `scripts/kind/build-images.sh` y `load-images.sh` construyen y cargan también las imágenes de `demo/*/` (lista derivada de los Dockerfile, no escrita a mano).
- **Reemplazable**: `docs/operaciones/app-objetivo.md` explica qué debe cumplir la app del cliente para sustituir a esta (OpenAPI en una ruta conocida, contrato de baseline, imagen con tag fijado) y cómo apuntar `RUN_ARTIFACT_REF` a ella. La app definitiva la aporta el humano en U8-T09.
- La NetworkPolicy que permite `go-reset` → `warm-app:9090` (si hace falta) es acotada a ese par y puerto; `policies.sh` sin `FALLA`.

**Fuera**:

- Cambiar `go-reset`, `go-warm-manager` o `go-run-controller`.
- Autenticación de usuarios finales en la app, UI, migraciones complejas.
- La app definitiva del cliente (U8-T09).

---

## Archivos de contexto

- `services/go-reset/internal/adapters/script/script.go`, `services/go-reset/README.md`
- `services/go-warm-manager/service.go` (`openAPIPaths`, `probePaths`, `Deploy`/`SetImage`)
- `deploy/flux/base/warm.yaml` (`warm-app`, `warm-db`, Secret `warm-db-credentials`), `deploy/flux/base/control-plane.yaml` (`RUN_ARTIFACT_REF`, `go-reset`)
- `tareas/U7-T03-overlay-kind-y-despliegue.md` (errata E-2: relleno `kind-stub`), `scripts/kind/secrets.sh`, `scripts/kind/build-images.sh`, `scripts/kind/load-images.sh`
- `contracts/plans/` (`SurfaceArtifact`)

---

## Criterios de aceptación

- [ ] **CA-1** — Pruebas de la app (incluida la invariante y el defecto).
  ```bash
  bash scripts/test/u8-target-app-local.sh; echo "rc=$?"
  ```
  Esperado: con Postgres local por podman: `OK openapi-valido`, `OK invariante-stock-sin-defecto` (vender de más → `409`), `OK defecto-oversell-sembrado` (con `TARGET_APP_BUG=oversell` el stock queda negativo), `OK baseline-clean-verify-0`, `OK baseline-sucio-verify-mayor-que-0`, `OK baseline-sin-token-401`, `rc=0`.

- [ ] **CA-2** — `baseline.sh` cumple el contrato que parsea `go-reset`.
  ```bash
  bash scripts/test/u8-target-app-local.sh --baseline-script; echo "rc=$?"
  ```
  Esperado: ejecutando el script **dentro de la imagen de `go-reset`** (`podman run … go-reset:0.0.0 sh /baseline.sh <sub>`): `clean` rc 0, `verify` imprime un entero, `version` imprime `ref-v1`; `rc=0`.

- [ ] **CA-3** — En kind: deploy sobre el warm y superficie inferida, leídos de vuelta.
  ```bash
  bash scripts/kind/deploy.sh >/dev/null 2>&1; echo "deploy rc=$?"
  kubectl -n aqs-system get cm go-reset-baseline -o jsonpath='{.metadata.labels.aqs\.io/kind-stub}'; echo "<stub"
  bash scripts/test/u8-target-app-kind.sh; echo "rc=$?"
  ```
  Esperado: `deploy rc=0`; etiqueta vacía (`<stub`, ya no es relleno); el script (nuevo, con `require_kind_context`) hace `POST /warm/ensure` + `POST /deploys` con `target-app:0.0.0` en `go-warm-manager`, espera `done`, y luego `POST /surface`: `OK deploy-target-app`, `OK superficie-con-orders` (la superficie leída de vuelta contiene `POST /orders`), `OK reset-verificado` (`POST /resets` en `go-reset` → `reset_verified=true` con este baseline), `rc=0`.

- [ ] **CA-4** — Políticas y alcance.
  ```bash
  bash scripts/ci/policies.sh 2>&1 | grep -c '^FALLA'
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(demo/target-app/|deploy/flux/(base|kind)/|scripts/kind/|scripts/test/u8-target-app-(local|kind)\.sh|docs/operaciones/(app-objetivo|kind-local)\.md|deploy/kind/README\.md|bitacoras/U8-T04\.md|revisiones/U8-T04/)' | wc -l
  ```
  Esperado: `0`, `0` y `0`.

---

## Plan de pruebas

- **Rojo primero:** `POST /resets` con el relleno da `reset_verified=true` sin verificar nada; con el baseline real y datos sucios, `verify` da `>0`. Pegar ambos.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
