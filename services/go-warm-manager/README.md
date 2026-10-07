# go-warm-manager

Gestiona el entorno warm (`aqs-test`): estado, deploy en proceso por corrida y superficie externa. No hace reset (U2-T06) ni consulta gates (U2-T02).

## API (servicio a servicio, `Authorization: Bearer $WARM_SERVICE_TOKEN`; sin token valido: 401)

| Metodo y ruta | Respuesta |
|---|---|
| `GET /warm` | `200` WarmState |
| `POST /warm/ensure` (sin cuerpo) | `200` WarmState si `ready` + `reset_verified` + sondas verdes (publica `warm.ready`); `409` WarmState si no (`dirty`, `cuarentena`, sin reset, sonda caida, timeout al salir de `idle-escalado`) |
| `POST /deploys` `{run_id, artifact:{kind,ref}}` | `202` `{run_id,state,attempts}`; `409` WarmState si el warm no esta listo (no se crea estado `pending`); `400` ante campos extra, `run_id` invalido o artefacto rechazado (registro no permitido, `latest`); `422` `{error:unsupported_artifact, reason:"build-from-repo no soportado"}` para `build-from-repo` (sin tocar el warm ni el Deployment) |
| `GET /deploys/{run_id}` | `{run_id, state: pending\|done\|failed, attempts, reason?}` (`reason` visible si `failed`) |
| `POST /surface` `{run_id}` | `200` SurfaceArtifact (validado contra el esquema en tiempo de ejecucion, guardado en el ObjectStore y `surface.ready`); `409` si el deploy de la corrida no termino bien o el warm no esta tomado (`dirty`); `502` si la app no responde a ningun sondeo o no hay endpoints (nunca `200` con 0 endpoints) |

Abiertas sin token: `/healthz`, `/readyz`, `/metrics` (`aqs_warm_state{state}`, `aqs_deploy_attempts_total{result}`, `aqs_handoff_total{phase}`).

> El esquema `Authorization: Bearer` no distingue mayusculas; la autenticacion se evalua antes del enrutado.

## Deploy en proceso y fail-closed
El deploy NO crea Jobs: el servicio (con su propia ServiceAccount) hace un `patch` de la imagen del contenedor `warm-app` del Deployment `warm-app` a `artifact.ref` y espera el rollout COMPLETO hasta `WARM_DEPLOY_TIMEOUT` (criterio copiado de `AppReady` de go-reset: `observedGeneration`, `updatedReplicas`=`replicas`=`availableReplicas`, sin pods extra, todos Running/Ready, ninguno terminando; ademas la plantilla y todos los pods llevan la imagen pedida). Un timeout del rollout NUNCA es exito. Un parche rechazado, un rollout que no completa o lecturas fallidas seguidas (5) cuentan como intento fallido; maximo 3 intentos (inicial + 2 reintentos). Agotados: `deploy.failed` con `reason` y `Alerter.Handoff` (uno). El deploy exige warm `ready` con `reset_verified` y lo deja `dirty` de forma atomica ANTES de parchear (mutex + compare-and-swap con `resourceVersion` en el ConfigMap): dos deploys concurrentes nunca parchean dos veces.

Solo `published-image` (registros permitidos y tag fijado, nunca `latest`). `build-from-repo` se rechaza con `422` (fail-closed). El RBAC del servicio para parchear `deployments` y el cableado quedan para U2-T07.

## Estado de deploy: solo memoria
La tabla de deploys vive en memoria (crece sin limite mientras el proceso viva; un `run_id` terminado es terminal en esa instancia; un `StartDeploy` repetido devuelve el estado existente sin tocar el warm). No hay otra persistencia: tras un reinicio `GET /deploys/{run_id}` responde `404` y el warm persistido esta `dirty` (CAS en el ConfigMap `warm-state`), asi que NO se despliega nada hasta que go-reset lo devuelva a `ready` con reset verificado. Limitaciones conocidas: un deploy en vuelo al reiniciar queda sin cierre (ni `deploy.done` ni `deploy.failed`); `warm.ready` lleva un `event_id` distinto por llamada de `ensure`; `probe.HTTP` trunca cuerpos a 2 MiB.

## Variables
`WARM_NAMESPACE` (solo `aqs-test`), `WARM_SERVICE_TOKEN`, `WARM_OUTBOX_FILE` (obligatorias), `LISTEN_ADDR` (`:8080`), `WARM_OBJECT_STORE` (OBLIGATORIA y explicita: `file` solo con `WARM_ALLOW_FILE_STORE=true`, con `WARM_OBJECT_DIR`, y rechazado con `KUBERNETES_SERVICE_HOST` o `WARM_ENV` prod; o `s3` con `WARM_S3_ENDPOINT`, `WARM_S3_ACCESS_KEY`, `WARM_S3_SECRET_KEY`, `WARM_S3_BUCKET`=`evidence`, `WARM_S3_SECURE`, seguro por defecto: declarar `false` para MinIO sin TLS), `WARM_ALLOWED_REGISTRIES` (`ghcr.io`), `WARM_APP_URL` (http/https obligatorio, validada al arrancar), `WARM_READY_TIMEOUT` (120s), `WARM_DEPLOY_TIMEOUT` (10m, espera del rollout de cada intento), `WARM_POLL_INTERVAL` (2s) (duraciones positivas; `WARM_READY_TIMEOUT` <= 10m, `WARM_POLL_INTERVAL` >= 100ms, `WARM_DEPLOY_TIMEOUT` >= `WARM_POLL_INTERVAL` y <= 1h; si no, no arranca; el `WriteTimeout` HTTP es `WARM_READY_TIMEOUT` + 30s), `WARM_KUBE` (`incluster` por defecto; `fake` exige `WARM_ALLOW_FAKE_KUBE=true`, se rechaza con `KUBERNETES_SERVICE_HOST` o `WARM_ENV` prod/production, y admite `WARM_FAKE_STATE`). Si falta el ConfigMap `warm-state`, el warm arranca `dirty` con `reset_verified=false`: solo go-reset lo declara listo.

## Pruebas
`go test -race ./...`; con MinIO real: `go test -tags minio -run ObjectStoreS3 ./...` (requiere `docker`).
