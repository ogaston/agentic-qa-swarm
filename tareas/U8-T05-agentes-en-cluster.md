# U8-T05 — Agentes en el clúster: Deployments, evidencia en MinIO y usuario propio

**Unidad:** U8 — Producto demostrable (ciclo completo en kind)
**Historias que implementa:** US-M3, US-M4 (planner en el clúster), US-M9 (reporter lee la evidencia real); decisiones C-83 y C-107; backlog P2 (parte) y P3.
**Depende de:** U8-T04 **fusionada** (fixtures del LLM falso sobre la superficie de la app de referencia). **Ola 2**. **Tope: 3 rondas**.

---

## Alcance

**Dentro** (una línea, concreta):

> Desplegar `agent-planner` y `agent-reporter` como Deployments en `aqs-system` (Service `:8080`, sondas, `securityContext` endurecido, NetworkPolicy: solo `go-run-controller` les habla; egress solo a MinIO y, para el futuro LiteLLM, a `litellm.aqs-system.svc:4000`), dar al reporter un `EvidenceReader` y un `ReportStore` sobre S3/MinIO (`s3://<bucket>/runs/<run>/…`), y que `minio-init` cree el usuario `aqs-evidence` con una política limitada al bucket `evidence` (lectura y escritura de objetos, sin admin) cuyas credenciales son las del Secret `aqs-evidence-s3`.

Detalle:

- **Usuario MinIO (C-107, decidido)**: `minio-init` usa `minio/mc` (imagen fijada) para `mc admin user add` + `mc admin policy create/attach` de forma idempotente. Política: `s3:GetObject`, `s3:PutObject`, `s3:ListBucket` sobre `arn:aws:s3:::evidence` y `evidence/*`. La raíz nunca sale de `minio-root`.
- **LLM mientras no hay clave**: en kind ambos agentes corren con `LLM_PROVIDER=fake` (`*_ALLOW_FAKE`, nunca con `*_ENV=prod`) y fixtures generadas para la superficie de la app de referencia de U8-T04, montadas desde un ConfigMap del overlay `kind`. En `base`, `dev` y `prod` el proveedor es `http` hacia LiteLLM (`LLM_BASE_URL=http://litellm.aqs-system.svc:4000/v1`, `LLM_MODEL=deepseek/deepseek-chat`, `LLM_API_KEY_FILE` de un Secret) aunque LiteLLM se despliega en U8-T09.
- **Reporter**: nuevas implementaciones `S3EvidenceReader` y `S3ReportStore` (`boto3` o cliente equivalente con versión fijada), seleccionables por entorno (`REPORTER_EVIDENCE=s3`); `DirEvidenceReader` sigue para las pruebas locales. URIs con el bucket configurado (`EVIDENCE_BUCKET`), no con `aqs-evidence` fijo. TLS con la CA de MinIO montada.
- Imágenes de los agentes: las de U7-T02, sin cambios de Dockerfile salvo dependencias nuevas.

**Fuera**:

- Desplegar LiteLLM o usar una clave real (U8-T09).
- Llamar al reporter desde el controlador (U8-T06).
- Publicar `report.ready` por NATS (el controlador lo registra; candidata).

---

## Archivos de contexto

- `agents/agent-planner/src/agent_planner/{config,server}.py`, `agents/agent-reporter/src/agent_reporter/{server,ports,reporter,report_model}.py`, `agents/*/README.md`, `agents/*/Dockerfile`
- `tareas/candidatas.md` (C-82, C-83, C-107), `revisiones/U2-T07/ronda-2.md` (F-A1)
- `deploy/flux/base/minio/`, `deploy/flux/base/control-plane.yaml`, `deploy/flux/base/security/`, `deploy/flux/kind/kustomization.yaml`
- `services/go-run-controller/runner/s3.go` (formato de los URIs de evidencia)
- `tareas/U8-T04-app-de-referencia.md` (superficie de la app de referencia)

---

## Criterios de aceptación

Con la plataforma en kind para CA-2 a CA-4.

- [ ] **CA-1** — Pruebas de los agentes.
  ```bash
  for a in agent-planner agent-reporter; do (cd agents/$a && .venv/bin/python -m pytest -q >/dev/null 2>&1; echo "$a rc=$?"); done
  ```
  Esperado: `rc=0` en los dos (incluye pruebas nuevas de los adaptadores S3 contra MinIO local con la marca `minio`).

- [ ] **CA-2** — Usuario MinIO con permisos mínimos, leído de vuelta.
  ```bash
  bash scripts/test/u8-minio-user-kind.sh; echo "rc=$?"
  ```
  Esperado: `OK put-evidence` y `OK get-evidence` con las credenciales de `aqs-evidence-s3`, `OK sin-admin` (`mc admin info` con ese usuario falla), `OK otro-bucket-denegado`, `rc=0`.

- [ ] **CA-3** — Agentes disponibles y aislados.
  ```bash
  kubectl -n aqs-system get deploy agent-planner agent-reporter -o jsonpath='{range .items[*]}{.metadata.name}={.status.availableReplicas}{"\n"}{end}'
  bash scripts/test/u8-agents-kind.sh; echo "rc=$?"
  ```
  Esperado: `=1` en los dos; `OK planner-plan-valido` (`POST /v1/plan` con la superficie de la app de referencia desde un pod con la etiqueta de `go-run-controller` → `FlowPlan` válido contra el esquema), `OK reporter-lee-minio` (evidencia sembrada en MinIO → reporte válido contra `report.schema.json` y guardado en MinIO, leído de vuelta), `OK otro-pod-bloqueado`, `rc=0`.

- [ ] **CA-4** — Políticas y alcance.
  ```bash
  bash scripts/ci/policies.sh 2>&1 | grep -c '^FALLA'
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(agents/agent-(planner|reporter)/|deploy/flux/(base|dev|prod|kind)/|scripts/kind/|scripts/test/u8-(minio-user|agents)-kind\.sh|docs/operaciones/kind-local\.md|deploy/kind/README\.md|bitacoras/U8-T05\.md|revisiones/U8-T05/)' | wc -l
  ```
  Esperado: `0`, `0` y `0`.

---

## Plan de pruebas

- **Rojo primero:** con las credenciales de `aqs-evidence-s3` actuales, `PutObject` falla (`InvalidAccessKeyId`); pegar la salida.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
