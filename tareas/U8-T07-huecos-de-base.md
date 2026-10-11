# U8-T07 — Cerrar en `base` los huecos de configuración que hoy solo cubre kind (C-103)

**Unidad:** U8 — Producto demostrable (ciclo completo en kind)
**Historias que implementa:** US-M1, US-M8.3 (los servicios arrancan en dev y prod, no solo en kind).
**Depende de:** U8-T02 y U8-T06 **fusionadas** (tocan el mismo `control-plane.yaml`). **Ola 3**. **Tope: 3 rondas**.

---

## Alcance

**Dentro** (una línea, concreta):

> Llevar a `deploy/flux/base/control-plane.yaml` lo que hoy parchea el overlay `kind` (errata E-2 de U7-T03 y E-3 de U7-T04): `GITHUB_WEBHOOK_SECRET` de `go-intake` desde el Secret `go-intake-webhook`, `UIAPI_AUTH=identity` + `IDENTITY_URL` y `WARM_URL` + `UIAPI_WARM_TOKEN_FILE` (montado de `go-warm-manager-service-token`) en `ui-api`; quitar esos parches del overlay `kind`; y añadir los Secrets nuevos (`go-intake-webhook`, `target-app-baseline-token`, la clave del LLM) a `scripts/secrets/generate.sh` y `docs/operaciones/secrets.md` para dev y prod.

Detalle:

- Tras el cambio, `kubectl kustomize deploy/flux/kind` debe producir **el mismo** entorno efectivo para esos tres Deployments que antes (mismas variables y montajes), salvo el origen.
- El ConfigMap `go-reset-baseline` sigue aportándolo el humano por app en dev/prod (documentado); en kind lo crea `secrets.sh` (U8-T04).
- Los Secrets de dev/prod siguen cifrados con SOPS; aquí solo se generan las plantillas y la documentación, no se cifran valores reales.

**Fuera**:

- Valores reales de dev/prod, aplicar nada fuera de `kind-aqs`, cambiar código de servicios.

---

## Archivos de contexto

- `tareas/U7-T03-overlay-kind-y-despliegue.md` y `tareas/U7-T04-humo-y-aislamiento.md` (sección «Errata»)
- `deploy/flux/base/control-plane.yaml`, `deploy/flux/kind/kustomization.yaml`, `deploy/flux/dev/`, `deploy/flux/prod/`
- `scripts/secrets/generate.sh`, `docs/operaciones/secrets.md`, `scripts/ci/check-secrets.sh`, `scripts/kind/secrets.sh`

---

## Criterios de aceptación

- [ ] **CA-1** — Variables en `base` y fuera del overlay.
  ```bash
  kubectl kustomize deploy/flux/dev | grep -c -E 'name: (GITHUB_WEBHOOK_SECRET|UIAPI_AUTH|UIAPI_WARM_TOKEN_FILE)$'
  grep -c -E 'GITHUB_WEBHOOK_SECRET|UIAPI_AUTH|UIAPI_WARM_TOKEN_FILE' deploy/flux/kind/kustomization.yaml
  ```
  Esperado: `3` y `0`.

- [ ] **CA-2** — Mismo entorno efectivo en kind que antes del cambio.
  ```bash
  b=$(git merge-base HEAD origin/main); w="${TMPDIR:-/tmp}/u8t07-base"; git worktree add -q "$w" "$b"
  envde() { kubectl kustomize "$1" | podman run --rm -i docker.io/mikefarah/yq:4.44.3 "select(.kind==\"Deployment\" and .metadata.name==\"$2\") | .spec.template.spec.containers[0].env | sort_by(.name)"; }
  for d in go-intake ui-api; do diff <(envde "$w/deploy/flux/kind" $d) <(envde deploy/flux/kind $d) >/dev/null && echo "$d igual"; done; git worktree remove --force "$w"
  ```
  Esperado: `go-intake igual` y `ui-api igual`.

- [ ] **CA-3** — Plataforma en kind sana (sin regresión del humo).
  ```bash
  bash scripts/kind/deploy.sh >/dev/null 2>&1; echo "deploy rc=$?"; bash scripts/kind/kind-smoke.sh 2>&1 | grep -c '^FALLA'
  ```
  Esperado: `deploy rc=0` y `0`.

- [ ] **CA-4** — Políticas, secretos y alcance.
  ```bash
  bash scripts/ci/policies.sh 2>&1 | grep -c '^FALLA'
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(deploy/flux/(base|kind|dev|prod)/|scripts/secrets/|scripts/kind/|docs/operaciones/(secrets|kind-local)\.md|bitacoras/U8-T07\.md|revisiones/U8-T07/)' | wc -l
  ```
  Esperado: `0`, `0` y `0`.

---

## Plan de pruebas

- **Rojo primero:** `kubectl kustomize deploy/flux/dev | grep -c GITHUB_WEBHOOK_SECRET` da `0` hoy.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
