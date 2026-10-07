# go-warm-manager

Gestiona el entorno warm (`aqs-test`): estado, deploy por corrida (`deploy-{run}-{n}`) y superficie externa. No hace reset (U2-T06) ni consulta gates (U2-T02).

## API (servicio a servicio, `Authorization: Bearer $WARM_SERVICE_TOKEN`; sin token valido: 401)

| Metodo y ruta | Respuesta |
|---|---|
| `GET /warm` | `200` WarmState |
| `POST /warm/ensure` (sin cuerpo) | `200` WarmState si `ready` + `reset_verified` + sondas verdes (publica `warm.ready`); `409` WarmState si no (`dirty`, `cuarentena`, sin reset, sonda caida, timeout al salir de `idle-escalado`) |
| `POST /deploys` `{run_id, artifact:{kind,ref}}` | `202` `{run_id,state,attempts}`; `409` WarmState si el warm no esta listo (no se crea estado `pending`); `400` ante campos extra, `run_id` invalido o artefacto rechazado (registro no permitido, `latest`) |
| `GET /deploys/{run_id}` | `{run_id, state: pending\|done\|failed, attempts, reason?}` (`reason` visible si `failed`) |
| `POST /surface` `{run_id}` | `200` SurfaceArtifact (validado contra el esquema en tiempo de ejecucion, guardado en el ObjectStore y `surface.ready`); `409` si el deploy de la corrida no termino bien o el warm no esta tomado (`dirty`); `502` si la app no responde a ningun sondeo o no hay endpoints (nunca `200` con 0 endpoints) |

Abiertas sin token: `/healthz`, `/readyz`, `/metrics` (`aqs_warm_state{state}`, `aqs_deploy_attempts_total{result}`, `aqs_handoff_total{phase}`).

> El esquema `Authorization: Bearer` no distingue mayusculas; la autenticacion se evalua antes del enrutado.

## Deploy y fail-closed
Job `deploy-{run}-0`; si falla, hasta 2 reintentos (`-1`, `-2`): nunca un cuarto Job. Agotados: `deploy.failed` con `reason` y `Alerter.Handoff`. El deploy exige warm `ready` con `reset_verified` y lo deja `dirty` de forma atomica (mutex + compare-and-swap con `resourceVersion` en el ConfigMap): dos deploys concurrentes nunca crean dos Jobs. Un error al crear o leer el Job (intento indeterminado) NO consume un reintento ni crea otro Job: falla cerrado (`deploy.failed` + handoff).

**PLACEHOLDER:** la imagen del deployer `ghcr.io/ogaston/aqs-warm-deployer:0.1.0` aun no existe; su construccion, la SA `warm-deployer` y el RBAC quedan para U2-T07.

## Estado de deploy: memoria + Jobs
La tabla de deploys vive en memoria (y crece sin limite mientras el proceso viva; un `run_id` que termino `failed` es terminal en esa instancia). No hay otra persistencia: tras un reinicio `GET /deploys/{run_id}` deriva el estado de los Jobs por la etiqueta `aqs.io/run-id`, y al arrancar `ResolveOrphans` cierra los deploys sin observador de forma fail-closed: un Job aun en vuelo se da por indeterminado (`deploy.failed` + handoff; el warm sigue `dirty`, nada se despliega encima); un Job terminado reemite su evento terminal (mismo `event_id`, el outbox deduplica). Limitaciones conocidas: `warm.ready` lleva un `event_id` distinto por llamada de `ensure`; `probe.HTTP` trunca cuerpos a 2 MiB.

## Subcomando
`go-warm-manager render-deploy-job --run <id> --artifact-kind <k> --artifact-ref <ref>` imprime el Job en YAML (sin tocar clusters).

## Variables
`WARM_NAMESPACE` (solo `aqs-test`), `WARM_SERVICE_TOKEN`, `WARM_OUTBOX_FILE` (obligatorias), `LISTEN_ADDR` (`:8080`), `WARM_OBJECT_STORE` (OBLIGATORIA y explicita: `file` solo con `WARM_ALLOW_FILE_STORE=true`, con `WARM_OBJECT_DIR`, y rechazado con `KUBERNETES_SERVICE_HOST` o `WARM_ENV` prod; o `s3` con `WARM_S3_ENDPOINT`, `WARM_S3_ACCESS_KEY`, `WARM_S3_SECRET_KEY`, `WARM_S3_BUCKET`=`evidence`, `WARM_S3_SECURE`, seguro por defecto: declarar `false` para MinIO sin TLS), `WARM_ALLOWED_REGISTRIES` (`ghcr.io`), `WARM_DEPLOYER_IMAGE` (obligatoria fuera del modo fake), `WARM_APP_URL` (http/https obligatorio, validada al arrancar), `WARM_READY_TIMEOUT` (120s), `WARM_JOB_TIMEOUT` (10m), `WARM_POLL_INTERVAL` (2s) (duraciones positivas; `WARM_READY_TIMEOUT` <= 10m, `WARM_POLL_INTERVAL` >= 100ms, `WARM_JOB_TIMEOUT` > 10m (el `activeDeadlineSeconds` del Job) y <= 1h; si no, no arranca; el `WriteTimeout` HTTP es `WARM_READY_TIMEOUT` + 30s), `WARM_KUBE` (`incluster` por defecto; `fake` exige `WARM_ALLOW_FAKE_KUBE=true`, se rechaza con `KUBERNETES_SERVICE_HOST` o `WARM_ENV` prod/production, y admite `WARM_FAKE_STATE`). Si falta el ConfigMap `warm-state`, el warm arranca `dirty` con `reset_verified=false`: solo go-reset lo declara listo.

## Pruebas
`go test -race ./...`; con MinIO real: `go test -tags minio -run ObjectStoreS3 ./...` (requiere `docker`).
