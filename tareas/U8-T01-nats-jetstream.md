# U8-T01 — NATS JetStream en la plataforma (manifiestos, políticas y kind)

**Unidad:** U8 — Producto demostrable (ciclo completo en kind)
**Historias que implementa:** US-M1, US-M2 (transporte de `notify.created` y `run.confirmed` entre pods); decisión C-45.
**Depende de:** U7 cerrada (U7-T04 y U7-T05 fusionadas). **Ola 1**. **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

---

## Alcance

**Dentro** (una línea, concreta):

> Añadir a `deploy/flux/base/` un servidor NATS con JetStream (StatefulSet de 1 réplica, imagen fijada por tag y digest, PVC, Service `nats.aqs-system.svc:4222`, sondas a `:8222/healthz`), sus NetworkPolicy (solo `go-intake`, `ui-api` y `go-run-controller` le hablan) y un Job idempotente `nats-init` que crea el stream `AQS_EVENTS` (sujetos `aqs.events.>`, retención por límites, `max_age` 7 días, deduplicación de 2 min por `Nats-Msg-Id`); que pase `policies.sh` en dev, prod y kind y que `deploy.sh` de kind lo espere disponible.

Detalle:

- Sujetos: `aqs.events.<tipo>` con el tipo del evento tal cual (`aqs.events.notify.created`, `aqs.events.run.confirmed`, …). Se documentan en `deploy/flux/base/nats/README.md`. Los consumidores durables los crea cada servicio en U8-T02 (no aquí).
- Sin autenticación de NATS dentro del clúster en esta tarea: el aislamiento es la NetworkPolicy (candidata: credenciales NKey). Puerto de monitorización `8222` solo dentro del pod (no en el Service).
- `securityContext` endurecido como el resto de `base` (no root, `readOnlyRootFilesystem`, `drop: [ALL]`, seccomp). Recursos con requests y limits.
- `scripts/kind/deploy.sh` añade NATS y `nats-init` a la espera de disponibilidad (`OK|FALLA`).

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Cambiar código de servicios para publicar o consumir (U8-T02).
- Clúster NATS de varias réplicas, TLS, NKeys, leaf nodes (candidatas).
- Cambiar `policy/*.rego` para hacer pasar NATS: si una regla lo bloquea, se ajusta el manifiesto, no la regla.

---

## Archivos de contexto

- `tareas/candidatas.md` (C-45)
- `deploy/flux/base/kustomization.yaml`, `deploy/flux/base/minio/` (patrón de StatefulSet + Job de init), `deploy/flux/base/security/`
- `deploy/flux/dev/`, `deploy/flux/prod/`, `deploy/flux/kind/kustomization.yaml`
- `scripts/ci/policies.sh`, `policy/*.rego`, `scripts/kind/deploy.sh`, `deploy/kind/README.md`
- `docs/operaciones/kind-local.md`

---

## Criterios de aceptación

Desde la raíz del worktree. CA-3 y CA-4 con la plataforma en kind (`kind-up`, `build-images`, `load-images`, `deploy`).

- [ ] **CA-1** — Políticas en verde, sin relajar reglas.
  ```bash
  bash scripts/ci/policies.sh 2>&1 | grep -c '^FALLA'; git diff --name-only $(git merge-base HEAD origin/main) -- policy | wc -l
  ```
  Esperado: `0` y `0`.

- [ ] **CA-2** — NATS en los tres overlays, con imagen fijada.
  ```bash
  for o in dev prod kind; do kubectl kustomize deploy/flux/$o | grep -c -E '^\s+image: (docker\.io/)?nats:[0-9.]+(-alpine)?@sha256:[0-9a-f]{64}$'; done
  ```
  Esperado: al menos `1` en cada overlay.

- [ ] **CA-3** — Desplegado y con el stream creado, leído de vuelta.
  ```bash
  bash scripts/kind/deploy.sh 2>&1 | grep -E '^(OK|FALLA) .*nats'
  kubectl -n aqs-system run nats-probe --rm -i --restart=Never --image=docker.io/natsio/nats-box:0.14.5 -- nats --server nats://nats.aqs-system.svc:4222 stream info AQS_EVENTS --json | jq -r '.config.subjects[0], .config.duplicate_window'
  ```
  Esperado: líneas `OK` sin `FALLA`; `aqs.events.>` y `120000000000`. (Si la NetworkPolicy impide el pod de prueba, el codificador documenta el pod etiquetado como uno de los clientes permitidos y lo usa.)

- [ ] **CA-4** — Aislamiento: un pod sin la etiqueta de cliente no alcanza NATS; uno con ella, sí.
  ```bash
  bash scripts/kind/nats-netpol-check.sh; echo "rc=$?"
  ```
  Esperado: `OK nats-cliente-permitido`, `OK nats-otro-pod-bloqueado`, `rc=0`. El script (nuevo, en `scripts/kind/`) usa `require_kind_context` y borra sus pods siempre.

- [ ] **CA-5** — Alcance.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(deploy/flux/(base|dev|prod|kind)/|scripts/kind/|deploy/kind/README\.md|docs/operaciones/kind-local\.md|bitacoras/U8-T01\.md|revisiones/U8-T01/)' | wc -l
  ```
  Esperado: `0` y `0`.

---

## Plan de pruebas

- **Rojo primero:** pegar `kubectl kustomize deploy/flux/kind | grep -c nats` (0) y la salida de `nats stream info` fallando.
- CA-4 prueba la política en ambos sentidos (positivo y negativo).
- Al terminar: `bash scripts/kind/kind-down.sh` y el contexto como estaba.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
