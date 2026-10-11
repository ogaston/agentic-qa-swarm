# Runbook: plataforma local en kind (U7-T04)

Clúster efímero `aqs` sobre podman, contexto de kubectl `kind-aqs`. Prerrequisitos y
detalles de instalación: `deploy/kind/README.md`. Todos los comandos se corren desde la
raíz del repositorio y no tocan otros clústeres (`aqs-poc`, `ckad`).

## Secuencia

```bash
bash scripts/kind/kind-up.sh        # crea el clúster aqs (o reutiliza) y deja kind-aqs
bash scripts/kind/build-images.sh   # construye las 9 imágenes de la plataforma
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
  `1.1.1.1:443` ni go-identity en `aqs-system`.
- `secrets-no-en-logs`: los logs de los 7 Deployments no contienen `password_hash`,
  `$argon2id$`, tokens de servicio ni la contraseña demo.

La sensibilidad de la comprobación de red se verifica borrando las NetworkPolicy de `aqs-test`:
`netpol-sin-egress-test` debe pasar a `FALLA` (requiere salida a Internet desde el nodo kind,
porque `1.1.1.1:443` es el destino que demuestra la falta de política).

## Qué funciona y qué queda pendiente en kind

| Parte del sistema | Estado en kind | Referencia |
|---|---|---|
| Arranque y salud de los 7 servicios del control plane | Funciona | `healthz-*`, `readyz-*` |
| Login con usuario demo e inbox de ui-api | Funciona | `login-demo`, `inbox-*` |
| RBAC de go-warm-manager y go-reset según la matriz | Funciona | `rbac-test-ns-only` |
| NetworkPolicy de aqs-test aplicada (kindnet) | Funciona | `netpol-*` |
| Logs sin secretos | Funciona | `secrets-no-en-logs` |
| Estado del warm desde ui-api (`GET /warm`) | Bloqueado: el Deployment de ui-api no define `WARM_URL` ni `UIAPI_WARM_TOKEN_FILE`; responde 503 `warm no configurado` | `warm-estado` (FALLA, defecto de manifiesto, ver bitácora U7-T04) |
| Ciclo de corrida webhook → notificación → confirm → run.confirmed → controlador | Pendiente | P1: los eventos viajan por archivos en el disco de cada pod |
| Planner y reporter | Pendiente | P2: los agentes no tienen Deployment; falta LiteLLM y egress limitado |
| Reset verificado del warm | Pendiente | C-104: `go-reset-baseline` en kind es un relleno `kind-stub` que no verifica nada |
| Evidencia en MinIO desde los agentes | Pendiente | P3 y C-107: el usuario de `aqs-evidence-s3` no está aprovisionado |

El smoke imprime estos pendientes como `PENDIENTE <paso> (<motivo>)`; no cuentan como fallo.
