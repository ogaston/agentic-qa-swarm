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
  el testigo interno (ClusterIP de `kubernetes.default`, puerto 443, abierto sin políticas y sin
  depender de Internet) ni el testigo externo `1.1.1.1:443`. go-identity en `aqs-system` no sirve
  como testigo: lo bloquea el ingress de `aqs-system` aunque no haya políticas en `aqs-test`.
- `secrets-no-en-logs`: los logs de los 7 Deployments no contienen `password_hash`,
  `$argon2id$`, tokens de servicio ni la contraseña demo.

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
| Ciclo de corrida webhook → notificación → confirm → run.confirmed → controlador | Pendiente | P1: los eventos viajan por archivos en el disco de cada pod |
| Planner y reporter | Pendiente | P2: los agentes no tienen Deployment; falta LiteLLM y egress limitado |
| Reset verificado del warm | Pendiente | C-104: `go-reset-baseline` en kind es un relleno `kind-stub` que no verifica nada |
| Evidencia en MinIO desde los agentes | Pendiente | P3 y C-107: el usuario de `aqs-evidence-s3` no está aprovisionado |

El smoke imprime estos pendientes como `PENDIENTE <paso> (<motivo>)`; no cuentan como fallo.

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

Los comandos que tocan `aqs` se ejecutan dentro de `flock ${XDG_RUNTIME_DIR:-/tmp}/aqs-kind.lock`
cuando hay más de un codificador usando el clúster.
