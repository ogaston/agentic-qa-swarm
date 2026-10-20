# Runbook: plataforma local en kind (U7-T04)

Clúster efímero `aqs` sobre podman, contexto de kubectl `kind-aqs`. Prerrequisitos y
detalles de instalación: `deploy/kind/README.md`. Todos los comandos se corren desde la
raíz del repositorio y no tocan otros clústeres (`aqs-poc`, `ckad`).

## Secuencia

```bash
bash scripts/kind/kind-up.sh        # crea el clúster aqs (o reutiliza) y deja kind-aqs
bash scripts/kind/build-images.sh   # construye las 10 imágenes: las 9 de la plataforma y `demo/target-app` (U8-T04)
bash scripts/kind/load-images.sh    # carga las imágenes en el nodo con podman save + kind load
bash scripts/kind/deploy.sh         # secrets.sh + kubectl apply -k deploy/flux/kind + espera de disponibilidad
bash scripts/kind/kind-smoke.sh     # prueba de humo: port-forward en 127.0.0.1:18600-18606 y comprobaciones OK|FALLA
bash scripts/kind/kind-down.sh      # destruye el clúster aqs
```

Tras `kind-down` el contexto de kubectl queda vacío; vuelve a dejarlo donde estabas
(`kubectl config use-context <ctx>`).

## Qué comprueba `kind-smoke.sh`

Cada línea sale como `OK <comprobación>` o `FALLA <comprobación>`; el script sale con 0 solo
si ninguna falla. Las comprobaciones son:

- `healthz-<svc>` y `readyz-<svc>` (200) para los 7 servicios del control plane.
- `login-demo`: `POST /auth/login` en go-identity con el usuario `demo` (contraseña en
  `${XDG_RUNTIME_DIR:-/tmp}/aqs-kind/demo.txt`) y `GET /auth/session` con rol `user`.
- `inbox-con-token` (200 y JSON) e `inbox-sin-token-401` en ui-api.
- `warm-estado`: `GET /warm` en ui-api (200 con un `state` del enum de WarmState).
- `rbac-test-ns-only`: `kubectl auth can-i` para go-warm-manager y go-reset frente a
  `docs/seguridad/rbac-matriz.csv`.
- `netpol-control-positivo` y `netpol-sin-egress-test`: un pod efímero `aqs-smoke-probe` en
  `aqs-test` (imagen `busybox:1.37.0`, borrado al terminar) alcanza warm-app y no alcanza
  el testigo interno (ClusterIP de `kubernetes.default`, puerto 443, abierto sin políticas y sin
  depender de Internet) ni el testigo externo `1.1.1.1:443`. go-identity en `aqs-system` no sirve
  como testigo: lo bloquea el ingress de `aqs-system` aunque no haya políticas en `aqs-test`.
- `secrets-no-en-logs`: los logs de los 7 Deployments no contienen `password_hash`,
  `$argon2id$`, tokens de servicio ni la contraseña demo.
- `ciclo-webhook-confirm-corrida` (U8-T08): la cadena de control de punta a punta ya funciona en
  kind: webhook de GitHub **firmado** (HMAC) a go-intake → `202` → la notificación `pending` en el
  inbox de ui-api (transporte de eventos entre pods, U8-T01/U8-T06) → confirmación por ui-api → la
  corrida existe en go-run-controller. El ciclo completo (deploy sobre el warm, superficie, plan,
  ensayo, runners, reset y reporte) lo cubre `scripts/kind/kind-e2e.sh` (ver «Demo de punta a punta»).

La sensibilidad de la comprobación de red se verifica borrando las NetworkPolicy de `aqs-test`:
`netpol-sin-egress-test` debe pasar a `FALLA` y la línea `FALLA` nombra el testigo alcanzable
(`kubernetes.default`, con el ClusterIP leído de `kubectl get svc kubernetes -n default`). Ese
testigo no depende de Internet; `1.1.1.1:443` queda como segundo testigo externo.

## Qué funciona y qué queda pendiente en kind

| Parte del sistema | Estado en kind | Referencia |
|---|---|---|
| Arranque y salud de los 7 servicios del control plane | Funciona | `healthz-*`, `readyz-*` |
| Login con usuario demo e inbox de ui-api | Funciona | `login-demo`, `inbox-*` |
| RBAC de go-warm-manager y go-reset según la matriz | Funciona | `rbac-test-ns-only` |
| NetworkPolicy de aqs-test aplicada (kindnet) | Funciona | `netpol-*` |
| Logs sin secretos | Funciona | `secrets-no-en-logs` |
| Estado del warm desde ui-api (`GET /warm`) | Funciona. Solo en kind: el overlay `deploy/flux/kind/` define `WARM_URL` y `UIAPI_WARM_TOKEN_FILE` (errata E-3 de U7-T04). En `base`, `dev` y `prod` el hueco sigue abierto (C-103) | `warm-estado` |
| Ciclo de control: webhook → notificación → confirm → corrida creada en el controlador | Funciona (U8-T01/U8-T06) | `ciclo-webhook-confirm-corrida` de `kind-smoke.sh` |
| Ciclo completo hasta el reporte, camino feliz (`--bug off`: deploy, superficie, plan, ensayo, runners, evidencia, reset, reporte) | Funciona (U8-T08, ronda 2), con las precondiciones de la sección «Demo de punta a punta» | `scripts/kind/kind-e2e.sh --bug off` |
| Bug sembrado detectado en la corrida (`--bug oversell`) | Funciona (U8-T08 ronda 3, con U8-T08b): el paso `POST /orders` lleva `body` (`qty` 11 sobre stock 10), el flujo es `invariant_probe: true` (en el ensayo basta con que sea ejecutable) y el defecto se activa antes de los runners | `scripts/kind/kind-e2e.sh --bug oversell` |
| Planner y reporter en el clúster | Funciona con LLM real vía LiteLLM (U8-T09): `LLM_PROVIDER=http` en kind; el LLM falso queda solo en `scripts/test/` | `llm-real-configurado`, `litellm-egress-acotado` en `kind-smoke.sh`; `kind-e2e.sh` |
| Reset verificado del warm | Verificado (U8-T04) | `go-reset-baseline` es el baseline real de `demo/target-app`; `scripts/test/u8-target-app-kind.sh` ensucia la base, resetea y lee `GET /orders` vacío y `verify` 0 |
| Evidencia en MinIO desde los agentes | Funciona (U8-T05). El usuario `aqs-evidence` lo crea `minio-init` con política solo sobre `evidence` | `scripts/test/u8-minio-user-kind.sh` (`put-evidence`, `get-evidence`, `sin-admin`, `otro-bucket-denegado`) |

El smoke imprime estos pendientes como `PENDIENTE <paso> (<motivo>)`; no cuentan como fallo.

## Demo de punta a punta (U8-T08)

`scripts/kind/kind-e2e.sh` recorre el ciclo completo sobre la plataforma ya desplegada en `kind-aqs`
y lee el estado del **sistema** en cada fase (nunca su propio log): `GET /runs/{id}` y `GET /warm` en
ui-api, `GET /deploys/{id}` en go-warm-manager, los Jobs etiquetados `aqs.io/run-id=<run>` y los
objetos de `s3://evidence/runs/<run>/` listados con el usuario `aqs-evidence`.

```bash
bash scripts/kind/kind-e2e.sh --bug off         # camino feliz: veredicto `sin-hallazgos`
bash scripts/kind/kind-e2e.sh --bug oversell    # bug sembrado en la app de referencia: veredicto `bug`
bash scripts/kind/kind-e2e.sh --sin-confirmar   # puerta humana: sin confirmar no nace corrida
```

Guion de presentación (qué mostrar en cada paso; para verlo en el dashboard, arrancarlo con
`dashboard-up.sh` —vite en `127.0.0.1:18610`— antes de lanzar el e2e):

| Paso | En el script | Qué mostrar |
|---|---|---|
| 1. Webhook firmado | `POST /webhooks/github` a go-intake con `X-Hub-Signature-256` (HMAC del secreto `go-intake-webhook`) | El `202` con la `notification.id` (una firma inválida da `401`) |
| 2. Inbox | `GET /notifications` en ui-api | La notificación en el inbox, en estado `pending` |
| 3. Confirmación humana | `POST /notifications/{id}/confirm` | El recibo con el `run_id`; en el dashboard, la corrida que arranca |
| 4. Deploy sobre el warm | `GET /deploys/{run_id}` en go-warm-manager | La app de referencia en `aqs-test` (`kubectl -n aqs-test get pods`) |
| 5. Superficie externa | `POST /surface` en go-warm-manager | Los endpoints inferidos |
| 6. Plan y ensayo | `GET /runs/{run_id}`; Jobs con `aqs.io/phase=rehearse` | La fase `rehearsing` y el ensayo |
| 7. Runners con evidencia | Jobs con `aqs.io/phase=run`; `flow-*/logs.txt` y `flow-*/result.json` en MinIO | Los flujos ejecutados y su evidencia |
| 8. Reset verificado | `GET /warm` → `state=ready` y `reset_verified=true` | El warm limpio tras la corrida |
| 9. Reporte | `report-*.json` en MinIO y `verdict` en `GET /runs/{id}` | El veredicto y los hallazgos (con `evidence_uris` reales) |

Salida: una línea `OK <comprobación>` o `FALLA <comprobación>` por paso; el código de salida es 0 solo
si no hay ninguna `FALLA` (en el camino feliz y en `--sin-confirmar`, el primer `FALLA` corta). El
camino feliz imprime al menos 12 `OK`; `--bug oversell` debe imprimir `OK veredicto-bug` y
`OK hallazgo-cita-invariante-y-evidencia`, cada `evidence_uri` del hallazgo existente en MinIO;
`--sin-confirmar` debe imprimir `OK sin-confirmar-no-crea-corrida`.

Higiene: el e2e usa `127.0.0.1:18615-18619` (el humo usa `18600-18606` y el dashboard
`18600`/`18601`/`18603` + vite en `18610`: sin solape, para poder correr la demo con el dashboard
arriba), cierra sus port-forwards
siempre, borra sus pods efímeros, revierte el `TARGET_APP_BUG` del warm y restaura las fixtures de
los agentes. La evidencia y las corridas registradas **no** se borran: el controlador no expone
borrado y la evidencia es el resultado. El lock `${XDG_RUNTIME_DIR:-/tmp}/aqs-kind.lock` serializa lo
que toca el clúster (ver la nota del final de este documento).

**Precondiciones que el e2e aplica en un clúster recién desplegado** (medidas en vivo; ninguna la
hace el ciclo por sí solo):

- **Warm verificado** (`warm_prepare`): un warm nuevo nace con `reset_verified=false` y el gate de
  `confirmed → warm_ready` lo rechaza. Si `GET /warm` no está `ready`+verificado, el e2e pide un
  reset inicial a go-reset (`POST /resets`, token `go-reset-service-token`) y lo comprueba.
- **Política `workflows`** (`gov_policy_seed`): `go-governance` arranca con `policies.jsonl` vacío y
  deniega todo workflow. El e2e da de alta el `U3_DEFAULT_WORKFLOW` del controlador (`happy-path`)
  como lo haría el admin: login de `admin` con OTP (secreto local de `scripts/kind/secrets.sh`) y
  `PUT /policies/workflows`, sin borrar lo que ya hubiera. Es idempotente.
- **Ventana del planner** (`planner_pause_wait`): el controlador llama a `/v1/plan` en cuanto el warm
  tiene superficie, y la fixture del planner depende del `run_id`. El e2e pausa el controlador tras
  el `POST /deploys`, obtiene deploy, superficie y fixture sin él, y lo reanuda (reanudar en
  `deploying` es seguro: `WarmClient.Deploy` no reenvía el deploy).

**Cómo se detecta el defecto de `oversell` (U8-T08 ronda 3, con U8-T08b):**

- El plan del escenario trae un paso `POST /orders` con `body` `{"product_id":1,"qty":11}` (stock
  inicial 10, `demo/target-app/store.go`) y `expect_status: 409`, en un flujo con
  `invariant_probe: true`. Sin defecto la app responde 409 (`insufficient_stock`); con
  `TARGET_APP_BUG=oversell` responde 201 porque se omite la comprobación de stock.
- El defecto se aplica al deployment `warm-app` **después del deploy y antes de reanudar el
  controlador**. Si se aplicara tras el ensayo llegaría tarde: el controlador lanza los runners en
  cuanto terminan los Jobs de ensayo (medido en la ronda 3: runner creado a 15:45:55Z, pod con el
  defecto a 15:46:47Z, y el runner recibió 409 del pod sano).
- El ensayo solo exige que el flujo de sondeo sea ejecutable (estado < 500): 201 y 409 valen. Los
  runners son estrictos: con el defecto fallan en `flow-1`, y el reporte declara `bug` citando esa
  evidencia real (`runs/<run>/flow-1/result.json`).
- La reversión del defecto (`$patch: delete` de `TARGET_APP_BUG`) se hace siempre en el trap.

## LLM real y LiteLLM (U8-T09)

LiteLLM (`ghcr.io/berriai/litellm:v1.106.0`, fijada en `deploy/flux/kind/litellm.yaml`) corre en `aqs-system`
como proxy compatible con OpenAI hacia el gateway del humano. Es el único componente con egress al
proveedor.

Insumos del humano, fuera del repo (modo 600): `${XDG_RUNTIME_DIR}/aqs-kind/deepseek.key` y
`${XDG_RUNTIME_DIR}/aqs-kind/litellm.env` (variables `LITELLM_UPSTREAM_IP`, `LITELLM_UPSTREAM_PORT`,
`LITELLM_UPSTREAM_MODEL`). `scripts/kind/secrets.sh` los valida antes de tocar el clúster y crea:

- Secret `litellm-upstream` (`config.yaml` con el modelo y la dirección del upstream; `upstream-api-key`).
  Se reaplica en cada ejecución.
- Secret `agents-llm-key` (`api-key`): clave maestra de LiteLLM, con prefijo `sk-`. Planner y reporter la
  presentan como Bearer.
- Secret `litellm-tls` (`ca.crt`, `tls.crt`, `tls.key`): TLS del servicio. Los agentes exigen https
  (`agents/*/llm_http.py`), así que confían en `ca.crt` con `SSL_CERT_FILE`.
- NetworkPolicy `litellm-egress-upstream`, renderizada en el clúster con `IP/32` y puerto de `litellm.env`
  (no va al repo). El overlay pone `litellm-egress-default-deny`.

Planner y reporter usan `LLM_BASE_URL=https://litellm.aqs-system.svc:4000/v1` y `LLM_MODEL=aqs-llm` (alias de
la config de LiteLLM; el nombre del modelo del proveedor no sale del Secret). Tope de salida 8000 y timeout
120 s: el modelo es de razonamiento y con `max_tokens` bajo el `content` llega vacío.

`scripts/kind/load-images.sh` descarga y carga la imagen de LiteLLM. `kind-smoke.sh` comprueba que no hay
claves en los logs de LiteLLM y de los agentes, que planner y reporter tienen `LLM_PROVIDER=http` y que
LiteLLM no alcanza `1.1.1.1:443`.

**Limitaciones conocidas.** El `base` de `deploy/flux/base/agents.yaml` usa `LLM_BASE_URL=http://...`, que
`check_base_url` rechaza (solo https o loopback). Solo kind está corregido. La medición de tokens por corrida
no es posible: ni LiteLLM sin base de datos ni los agentes persisten el `usage` (candidata).

**Webhook de GitHub real.** El paso del humano (port-forward de go-intake, túnel `cloudflared`, secreto del
webhook y push real) está en `docs/operaciones/webhook-github-cloudflared.md`. Comprobación:
`bash scripts/kind/github-webhook-check.sh <sha-40>`.

## NATS JetStream (U8-T01)

- `deploy/flux/base/nats/` despliega NATS con JetStream en `aqs-system` (una réplica, PVC) y el Job
  `nats-init`, que crea el stream `AQS_EVENTS` (`aqs.events.>`). `scripts/kind/deploy.sh` espera el
  StatefulSet y el Job (`OK|FALLA`).
- Aislamiento: solo los pods con `aqs.io/nats-client: "true"` alcanzan el 4222. Para comprobarlo:
  `bash scripts/kind/nats-netpol-check.sh` (exige el contexto `kind-aqs`).
- Comprobación manual de lectura del stream: ver `deploy/flux/base/nats/README.md`.

## Dashboard (U7-T05)

El build de `web/dashboard` se sirve en loopback contra la plataforma de kind, con el mismo origen
que usaría el navegador. Requiere la plataforma ya desplegada (secuencia de arriba hasta `deploy.sh`)
y `npm ci` hecho en `web/dashboard`.

```bash
bash scripts/kind/dashboard-up.sh     # build + port-forward a go-identity (18600), ui-api (18601), go-run-controller (18603) + vite preview en 127.0.0.1:18610
bash scripts/kind/dashboard-smoke.sh  # comprobaciones OK|FALLA contra http://127.0.0.1:18610 (cierra lo que abre)
bash scripts/kind/dashboard-down.sh   # para vite preview y los port-forwards
```

`dashboard-smoke.sh` arranca con `dashboard-up.sh` y para la pila siempre (trap). Comprueba por el
origen del dashboard: `index-servido`, `csp-estricta` (la misma política que `vite preview` en
`vite.config.ts`), `login` (usuario demo y sesión con rol `user`), `inbox` (`GET /api/notifications`),
`warm-estado` (`GET /api/warm` con un `state` del enum de WarmState), `sin-token-401` y
`logout-invalida-token`. Imprime `PENDIENTE confirmar-y-seguir-corrida (P1)`: confirmar una
notificación y seguir la corrida no se comprueba en kind hasta que P1 esté resuelto.

El proxy `/api` no se configura aquí: `dashboard-up.sh` exporta `AQS_IDENTITY_URL`,
`AQS_RUN_CONTROLLER_URL` y `AQS_UI_API_URL` hacia los port-forwards, y `vite.config.ts` no cambia.
El dashboard no corre dentro del clúster (sin imagen, Deployment ni ingress; C-95).

## Agentes en el clúster (U8-T05)

`agent-planner` y `agent-reporter` corren como Deployments en `aqs-system` (`deploy/flux/base/agents.yaml`),
con Service `:8080`, sondas, `securityContext` endurecido y NetworkPolicy: solo `go-run-controller` les habla;
el egreso es DNS, LiteLLM (`litellm.aqs-system.svc:4000`) y, el reporter, MinIO.

- **LLM en kind:** el overlay `deploy/flux/kind/` pone `LLM_PROVIDER=fake` con `PLANNER_ALLOW_FAKE=true` y
  `REPORTER_ALLOW_FAKE=1` (nunca `*_ENV=prod`). Los fixtures salen de ConfigMaps del overlay:
  - `agents-fixtures-planner`: la superficie de la app de referencia (`demo/target-app`) y el plan esperado.
  - `agents-fixtures-reporter`: el prompt exacto de la evidencia sembrada de `bug-ecom-01` y su respuesta.
    Se regenera con `PYTHONPATH=agents/agent-reporter/src agents/agent-reporter/.venv/bin/python scripts/kind/gen_agent_fixtures.py > deploy/flux/kind/agents/reporter/fixtures.json`.
    Como la entrada trae el `prompt` ya calculado, el arranque del reporter no lee MinIO.
- **Evidencia:** `REPORTER_EVIDENCE=s3` con `EVIDENCE_BUCKET=evidence`. Las credenciales son las del Secret
  `aqs-evidence-s3` (usuario `aqs-evidence`). `minio-init` lo crea con `provision-user.sh` usando el `mc` que
  trae la imagen de MinIO fijada por digest (no `minio/mc`, que no se puede descargar aquí).
- **Pruebas** (dentro de `flock`, sobre `kind-aqs`):

  ```bash
  bash scripts/test/u8-minio-user-kind.sh   # CA-2: OK put-evidence, get-evidence, sin-admin, otro-bucket-denegado; rc=0
  bash scripts/test/u8-agents-kind.sh       # CA-3: OK planner-plan-valido, reporter-lee-minio, otro-pod-bloqueado; rc=0
  ```

  Ambos crean sus pods de prueba (`u8-t05-*`) y los borran al salir. El primero prueba el usuario con
  `mc` y sus credenciales; el segundo siembra la evidencia, llama a los agentes desde un pod con la etiqueta
  de `go-run-controller` y valida cada respuesta contra `contracts/plans/*.schema.json` en el host (con el venv
  del planner, `agents/agent-planner/.venv`; otro intérprete con `U8_AGENTS_PY`).
- Un pod sin la etiqueta de `go-run-controller` no llega a los agentes: el fallo es un timeout de conexión
  (la NetworkPolicy descarta), no un fallo de DNS.
- Fuera de alcance: LiteLLM y la clave real (U8-T09), la llamada del controlador al reporter (U8-T06) y
  el evento `report.ready` por NATS.

Los comandos que tocan `aqs` se ejecutan dentro de `flock ${XDG_RUNTIME_DIR:-/tmp}/aqs-kind.lock`
cuando hay más de un codificador usando el clúster.
