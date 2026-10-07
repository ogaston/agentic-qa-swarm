# go-warm-manager

Gestiona el entorno warm (`aqs-test`): estado, deploy por corrida (`deploy-{run}-{n}`) y superficie externa. No hace reset (U2-T06) ni consulta gates (U2-T02).

## API (servicio a servicio, `Authorization: Bearer $WARM_SERVICE_TOKEN`; sin token valido: 401)

| Metodo y ruta | Respuesta |
|---|---|
| `GET /warm` | `200` WarmState |
| `POST /warm/ensure` (sin cuerpo) | `200` WarmState si `ready` + `reset_verified` + sondas verdes (publica `warm.ready`); `409` WarmState si no (`dirty`, `cuarentena`, sin reset, sonda caida, timeout al salir de `idle-escalado`) |
| `POST /deploys` `{run_id, artifact:{kind,ref}}` | `202` `{run_id,state,attempts}`; `409` WarmState si el warm no esta listo (no se crea estado `pending`); `400` ante campos extra, `run_id` invalido o artefacto rechazado (registro no permitido, `latest`) |
| `GET /deploys/{run_id}` | `{run_id, state: pending\|done\|failed, attempts, reason?}` (`reason` visible si `failed`) |
| `POST /surface` `{run_id}` | `200` SurfaceArtifact (se guarda en el ObjectStore y publica `surface.ready`) |

Abiertas sin token: `/healthz`, `/readyz`, `/metrics` (`aqs_warm_state{state}`, `aqs_deploy_attempts_total{result}`, `aqs_handoff_total{phase}`).

> El esquema `Authorization: Bearer` no distingue mayusculas; la autenticacion se evalua antes del enrutado.

## Deploy y fail-closed
Job `deploy-{run}-0`; si falla, hasta 2 reintentos (`-1`, `-2`): nunca un cuarto Job. Agotados: `deploy.failed` con `reason` y `Alerter.Handoff`. El deploy exige warm `ready` con `reset_verified` y lo deja `dirty` de forma atomica (mutex + compare-and-swap con `resourceVersion` en el ConfigMap): dos deploys concurrentes nunca crean dos Jobs. Un error al crear o leer el Job (intento indeterminado) NO consume un reintento ni crea otro Job: falla cerrado (`deploy.failed` + handoff).

**PLACEHOLDER:** la imagen del deployer `ghcr.io/ogaston/aqs-warm-deployer:0.1.0` aun no existe; su construccion, la SA `warm-deployer` y el RBAC quedan para U2-T07.

## Subcomando
`go-warm-manager render-deploy-job --run <id> --artifact-kind <k> --artifact-ref <ref>` imprime el Job en YAML (sin tocar clusters).

## Variables
`WARM_NAMESPACE` (solo `aqs-test`), `WARM_SERVICE_TOKEN`, `WARM_OUTBOX_FILE` (obligatorias), `LISTEN_ADDR` (`:8080`), `WARM_OBJECT_STORE` (`file` con `WARM_OBJECT_DIR`, o `s3` con `WARM_S3_ENDPOINT`, `WARM_S3_ACCESS_KEY`, `WARM_S3_SECRET_KEY`, `WARM_S3_BUCKET`=`evidence`, `WARM_S3_SECURE`), `WARM_ALLOWED_REGISTRIES` (`ghcr.io`), `WARM_DEPLOYER_IMAGE`, `WARM_APP_URL`, `WARM_READY_TIMEOUT` (120s), `WARM_JOB_TIMEOUT` (10m), `WARM_POLL_INTERVAL` (2s) (duraciones positivas; si no, no arranca; el `WriteTimeout` HTTP es `WARM_READY_TIMEOUT` + 30s), `WARM_KUBE` (`incluster` por defecto; `fake` exige `WARM_ALLOW_FAKE_KUBE=true`, se rechaza con `KUBERNETES_SERVICE_HOST` o `WARM_ENV` prod/production, y admite `WARM_FAKE_STATE`). Si falta el ConfigMap `warm-state`, el warm arranca `dirty` con `reset_verified=false`: solo go-reset lo declara listo.

## Pruebas
`go test -race ./...`; con MinIO real: `go test -tags minio -run ObjectStoreS3 ./...` (requiere `docker`).
