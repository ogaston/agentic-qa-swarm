# Ronda 2 — U7-T03

VEREDICTO: VERDE

Revisé el sha c0cca60 (base 893cd84, `git merge-base` confirmado tras `git fetch`). Corrí `kind-up.sh`, `build-images.sh` y `load-images.sh` sobre un clúster `aqs` nuevo y los tres dieron rc=0. No edité nada. El worktree quedó limpio (`git status --short | wc -l` = 0).

## Criterios de aceptación, verificados por mí
| # | Criterio | Resultado |
|---|---|---|
| 1 | Políticas sobre el overlay, sin relajarlas | pasa. 0 `FALLA`, 4 `OK .*kind` y `policies.sh` con rc=0. Archivos de `policy`, `base`, `dev` y `prod` que cambian: 0. Shellcheck v0.10.0 sobre `scripts/kind/*.sh` da rc=0. |
| 2 | El overlay deja fuera lo que kind no soporta | pasa. 0 HelmRelease/HelmRepository/ServiceMonitor/PrometheusRule y 19 Deployment/StatefulSet/NetworkPolicy. |
| 3 | Despliegue leído de vuelta | pasa. `deploy.sh` da rc=0 con 13 líneas `OK` y ninguna `FALLA`. Los 7 Deployments del control plane muestran `=1`. Los pods de `aqs-system` fuera de Running/Completed son 0. No hay pods con reinicios. |
| 4 | Secrets fuera del repo y sin fugas | pasa. 0 coincidencias de fuga, 0 `kind: Secret` en `deploy/flux/kind`, y `stat` da solo `600`. |
| 5 | Idempotencia y guarda | pasa. Salen `rc=0`, `mismo` y `rc=3` con el contexto `aqs-guard-test`. El contexto quedó en `kind-aqs` y borré `aqs-guard-test`. |
| 6 | Alcance | pasa. `git status` da 0 y los archivos fuera de alcance son 0. El diff contra la base tiene 7 archivos: la bitácora, `kustomization.yaml`, `deploy/kind/README.md`, `revisiones/U7-T03/ronda-1.md`, `policies.sh`, `deploy.sh` y `secrets.sh`. |

## Cierre de los hallazgos de la ronda 1
- **F-01 (NARANJA) — cerrado.** Leí el script de vuelta del clúster (`kubectl get cm go-reset-baseline -n aqs-system -o jsonpath='{.data.baseline\.sh}'`) y lo ejecuté:

  | Subcomando | Resultado |
  |---|---|
  | `clean` | rc=0 |
  | `verify` | imprime `0`, y es un entero válido para el `Atoi` de `script.go` |
  | `version` | imprime `kind-stub` |
  | argumento desconocido | rc=2 |

  El ConfigMap lleva la etiqueta `aqs.io/kind-stub=true`.
- **F-02 (AMARILLO, arbitrado) — cerrado.**
  - `kind-stubs.yaml` se borró y el relleno lo crea `ensure_baseline_stub` en `secrets.sh`, solo si no existe.
  - Prueba de no-pisado: sustituí el contenido del ConfigMap por `echo REAL` y le quité la etiqueta. Corrí `deploy.sh` de nuevo y dio rc=0 con «go-reset-baseline ya existe; no se toca». El contenido quedó en `REAL`, sin etiqueta, y con el mismo uid antes y después (`dca085dd-…`). La segunda corrida no lo pisó.
- **F-03 (AMARILLO) — cerrado.** La línea duplicada de la bitácora ya no está. Solo queda la mención en la respuesta del codificador. El CA-6 posterior al commit está pegado y yo lo verifiqué: 0 y 0.
- **F-04 (AMARILLO) — cerrado.** Los namespaces se crean con `create --dry-run | apply`. La salida de `deploy.sh` tiene 0 `Warning`.
- **F-05 (AMARILLO) — cerrado.** El README lista podman, `kubectl`, `openssl`, `jq` y `base32` como prerrequisitos del despliegue.

## Hallazgos en pie
Ninguno ROJO ni NARANJA.

## Tareas candidatas (fuera de alcance)
- C-E: el usuario MinIO de `aqs-evidence-s3` no se aprovisiona en ningún lado. Debe decidirse antes de la prueba de humo de U7-T04.
- U7-T04 debe declarar el reset verificado como `PENDIENTE` en kind, porque el relleno no verifica nada (`verify` siempre imprime `0`).
- Las candidatas C-103 a C-106 ya están registradas por el humano.

## Limpieza
- `bash scripts/kind/kind-down.sh` dio rc=0 y borró `aqs`.
- Ejecuté `kubectl config use-context kind-ckad` y el contexto actual es `kind-ckad`.
- Quedan `aqs-poc` y `ckad`, sin tocar.
- Mis salidas largas están en el scratchpad (`pol2.out`, `d2.out`, `d3.out`, `u7t03.out`).

Informe: `revisiones/U7-T03/ronda-2.md`

VEREDICTO: VERDE
INFORME: revisiones/U7-T03/ronda-2.md
