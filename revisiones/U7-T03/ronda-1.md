# Ronda 1 — U7-T03

VEREDICTO: NO-VERDE

Los 6 CA pasan en mi ejecución, pero hay un NARANJA en pie: el relleno de `baseline.sh` incumple el contrato de la errata E-2. Quedan además cuatro AMARILLOS que no bloquean.

Revisé el sha 511e0f8 (base 893cd84). Leí la tarea desde `origin/main` (blob e1f3cac, con errata). El worktree quedó limpio (`git status --short | wc -l` = 0) y no edité nada.

## Criterios de aceptación, verificados por mí
Corrí `kind-up.sh`, `build-images.sh` y `load-images.sh` (los tres con rc=0) sobre un clúster `aqs` nuevo y luego los comandos tal cual están en la tarea.

| # | Criterio | Resultado |
|---|---|---|
| 1 | Políticas sobre el overlay, sin relajarlas | pasa. Cuento 0 `FALLA` y 4 `OK .*kind` (`kubeconform`, `conftest-test`, `conftest-combine`, `rbac-matrix`). `policies.sh` sale con rc=0. Los archivos de `policy` y de `base`, `dev` y `prod` que cambian: 0. |
| 2 | El overlay deja fuera lo que kind no soporta | pasa. 0 HelmRelease/HelmRepository/ServiceMonitor/PrometheusRule y 19 Deployment/StatefulSet/NetworkPolicy. |
| 3 | Despliegue leído de vuelta | pasa. `deploy.sh` da rc=0 en 48 s, sin ninguna `FALLA`. Los 7 Deployments del control plane muestran `=1`. Los pods de `aqs-system` fuera de Running/Completed son 0. `kubectl get pods -A` muestra 0 reinicios y `minio-init` en Completed. |
| 4 | Secrets fuera del repo y sin fugas | pasa. 0 coincidencias en la salida de la corrida con generación y en la de la segunda corrida. `kind: Secret` en `deploy/flux/kind`: 0. `stat` da solo `600` (`admin-mfa-secret.txt`, `admin.txt` y `demo.txt`), en un directorio `drwx------`. |
| 5 | Idempotencia y guarda | pasa. Salen `rc=0`, `mismo` (mismo uid de `go-identity-users`) y `rc=3` con el contexto `aqs-guard-test`. El contexto quedó en `kind-aqs` y luego borré `aqs-guard-test`. |
| 6 | Alcance | pasa. `git status` da 0 y los archivos fuera de alcance son 0. El diff contra la base tiene solo 7 archivos: la bitácora, `kind-stubs.yaml`, `kustomization.yaml`, `deploy/kind/README.md`, `policies.sh`, `deploy.sh` y `secrets.sh`. |

Comprobaciones extra:
- **Secret desconocido:** hice una copia del repo en el scratchpad y le añadí una referencia a `bogus-secret`. `secrets.sh` falló con rc=1, nombró `bogus-secret` y no creó nada.
- **Carga de usuarios:** `go-identity` arrancó con `"users":2`. `ui-api` arrancó sano con `UIAPI_AUTH=identity`.
- **NetworkPolicy:** la ruta `ui-api` → `go-identity` pasa por `allow-same-namespace` (ingress) de `base`, y `aqs-system` no tiene egress restringido. Por eso la ausencia de una NetworkPolicy nueva es coherente con la errata. No probé la conexión con una petición real.
- **Shellcheck:** no está instalado aquí, así que no lo corrí.

## Hallazgos

### F-01 · NARANJA · `deploy/flux/kind/kind-stubs.yaml:13-18` · El `baseline.sh` de relleno incumple el contrato de la errata E-2
La errata exige este contrato:

| Subcomando | Lo que pide la errata | Lo que hace el relleno |
|---|---|---|
| `clean` | sale con 0 | sale con 0 |
| `verify` | imprime `0` | imprime `stub kind: verify sin efecto` |
| `version` | imprime `kind-stub` | imprime `v1` |

Leí el script de vuelta del clúster (`kubectl get cm go-reset-baseline -n aqs-system -o jsonpath=...`) y lo ejecuté:
```
verify  -> "stub kind: verify sin efecto"   rc=0
version -> "v1"
```
- **Por qué importa `verify`:** `services/go-reset/internal/adapters/script/script.go:59-67` hace `strconv.Atoi(strings.TrimSpace(out))`. Una salida no numérica produce el error `salida de verify inválida`. Cualquier `verify` real (`housekeeping`, `idle-check`, `rebuild` o una llamada de U7-T04) fallaría por el relleno, y no por el entorno. Ningún CA lo detecta, porque `go-reset` arranca sin llamar a `verify`.
- **Por qué importa `version`:** `v1` imita una versión de baseline real. La errata pide un marcador que se vea como relleno (`kind-stub`). Hoy no tiene efecto, porque `RESET_BASELINE_VERSION=v1` está fijo en los manifiestos y tiene prioridad. Pero queda desviado de la errata.
- **Qué corregir:** `verify` debe imprimir `0` y `version` debe imprimir `kind-stub`.
- **Bitácora:** dice «contrato clean|verify|version» y no registra esta desviación.

### F-02 · AMARILLO · `deploy/flux/kind/kind-stubs.yaml` · El relleno vive en el overlay y no en `secrets.sh`
Juzgo esto como desviación de la errata y no como defecto, porque el efecto práctico es pequeño. No lo mezclo con F-01.
- **A favor:** el ConfigMap pasa por `kubeconform` y `conftest` en `policies.sh`, es declarativo y lo gestiona `apply -k`.
- **En contra:** `deploy.sh` hace `apply -k` en cada corrida y pisa cualquier `go-reset-baseline` real que alguien aplique a mano. Un `secrets.sh` con «si existe, no lo regenera» lo habría respetado. La errata decía «`secrets.sh` crea en el clúster».
- **Qué hacer:** que el orquestador decida. O se acepta con la desviación anotada en la bitácora y el README, o se mueve a `secrets.sh`. No bloquea por sí solo.

### F-03 · AMARILLO · `bitacoras/U7-T03.md` · Evidencia incompleta
- Un `kind-up` falló y su salida no se capturó. Se anotó como «transitorio, no reproducido». En mi corrida salió bien.
- El CA-6 de la ronda se declara «se verifica después», sin salida pegada posterior al commit. Yo lo verifiqué y da 0 y 0.
- La sección «Rojo primero» tiene una línea duplicada (`y, antes, ...`).

### F-04 · AMARILLO · `scripts/kind/secrets.sh` · Aviso `Warning` inocuo
`kubectl apply -k` imprime un `Warning ... missing last-applied-configuration` para `aqs-system` y `aqs-test`. Los namespaces los crea `secrets.sh` con `kubectl create`. Es la candidata C-F del propio codificador. Se arregla creándolos con `--save-config` o con `kubectl apply`.

### F-05 · AMARILLO · `scripts/kind/secrets.sh` · Dependencia implícita de `podman`
`gen_go_identity_users` llama a `podman run` directamente. Es coherente con U7-T01 y U7-T02, que usan podman. Conviene documentarlo en el README como prerrequisito.

## Tareas candidatas (fuera de alcance)
- C-E del codificador: el usuario MinIO de `aqs-evidence-s3` no se aprovisiona en ningún lado. Debe decidirse antes de la prueba de humo de U7-T04.
- U7-T04 debe declarar `PENDIENTE` el reset verificado en kind, porque el relleno no verifica nada. Es el aviso de la errata.
- Ya registradas por el humano: C-103 a C-106.

## Limpieza
- `bash scripts/kind/kind-down.sh` dio rc=0 y borró `aqs`.
- Después ejecuté `kubectl config use-context kind-ckad`, y el contexto actual es `kind-ckad`.
- Quedan `aqs-poc` y `ckad`, sin tocar.
- Mis salidas largas están en el scratchpad (`pol.out`, `deploy1.out`, `u7t03.out`). Los archivos de contraseñas siguen en `/run/user/1000/aqs-kind/`, con permisos 600.

## Rutas relevantes
- `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U7-T03/deploy/flux/kind/kind-stubs.yaml`
- `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U7-T03/deploy/flux/kind/kustomization.yaml`
- `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U7-T03/scripts/kind/secrets.sh`
- `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U7-T03/scripts/kind/deploy.sh`
- `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U7-T03/services/go-reset/internal/adapters/script/script.go`

VEREDICTO: NO-VERDE
NARANJA|deploy/flux/kind/kind-stubs.yaml:13-18|baseline.sh de relleno incumple el contrato E-2 (verify no imprime 0; version no imprime kind-stub)
INFORME: revisiones/U7-T03/ronda-1.md
