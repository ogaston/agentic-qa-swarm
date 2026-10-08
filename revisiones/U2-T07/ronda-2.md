# Ronda 2 — U2-T07

VEREDICTO: VERDE

Los dos NARANJA de la ronda 1 (F-01 cuota de Jobs, F-02 CA de MinIO) están corregidos y verificados por mí en el build real de kustomize de dev y prod. No queda ningún ROJO ni NARANJA. Lo que queda es AMARILLO o candidata declarada, y es aceptable para fusionar en desarrollo. El humano fusiona, siempre.

El worktree está limpio (`git status` = 0 líneas), en el sha acccbb4. El diff contra el merge-base toca solo `deploy/flux/base/`, la bitácora y `revisiones/`: 0 archivos fuera de alcance y 0 Go.

## Criterios de aceptación, verificados por mí
Usé el build real de `deploy/flux/dev` y `deploy/flux/prod` con kustomize v5.4.3, leído con `yq` 4.44.3.

| # | Criterio | Resultado |
|---|---|---|
| 1 | `policies.sh` | pasa: 11 `OK` (kubeconform, conftest-test, conftest-combine y rbac-matrix para dev y prod; conftest-verify; promtool-rules; check-secrets) y 0 `FALLA` |
| 2 | Cuota y LimitRange | pasa: claves de la cuota correctas, `count/jobs.batch` = `"100"` en dev y prod, y el LimitRange da `true,true,true` con la variante `[...]\|join(",")` |
| 3 | Tres servicios | pasa: `/readyz /healthz true true` ×3, `RUN_PHASES=real` y ningún `FAKE` en el build |
| 4 | HPA y CronJobs | pasa: `go-run-controller 1 1`; `housekeeping`, `idle-check` y `rebuild` con `Forbid` y deadlines `300 1800` |
| 5 | Sin latest y Secrets cifrados | pasa: `check-no-latest.sh deploy` rc=0 y `check-secrets` OK |
| 6 | Alertas | pasa: `promtool test rules` y `check rules` dan SUCCESS (3 reglas) |
| 8 | Alcance | pasa: 0 fuera de alcance y 0 Go |

El texto de los CA-2 (segunda expresión) y CA-5 (el script exige el directorio `deploy`) sigue siendo defectuoso, y el codificador ya lo declaró en la bitácora. Eso es del orquestador, no un hallazgo de código.

## Verificación de los arreglos de la ronda 1
- **F-01 (cuota de Jobs).** `count/jobs.batch` es `100`, con un comentario que explica que los Jobs no llevan TTL ni se limpian. El comentario ya usa los valores reales de `runner/job.go` y `rehearsal/job.go`: runner 500m/256Mi, ensayo 250m/128Mi. La cuenta es 1,75 CPU/896Mi de Jobs más 1,75 CPU/1,1Gi de warm, igual a 3,5 CPU/2Gi, dentro de los 6 CPU/6Gi de la cuota. `warm-app`, `warm-db` y `warm-redis` siguen cabiendo, con sus límites actuales de `warm.yaml` y un surge.
- **F-02 (CA de MinIO).** `go-warm-manager` y `go-run-controller` montan el Secret `minio-tls`, clave `ca.crt`, en `/etc/aqs/minio-ca/ca.crt`, con `readOnly: true`. Ambos fijan `SSL_CERT_FILE=/etc/aqs/minio-ca/ca.crt`, y `readOnlyRootFilesystem` sigue en `true`. El volumen y el Secret están en el mismo namespace, `aqs-system`.
  - La clave y la ruta coinciden con `minio/statefulset.yaml` y `job-init.yaml`, que ya usan `ca.crt` de `minio-tls`.
  - `scripts/secrets/generate.sh` firma `tls.crt` con esa CA (`-CA ca.crt`) y los SAN `minio.aqs-system.svc` y `.cluster.local` coinciden con los endpoints. No es solo cert más key.
  - El Secret `minio-tls` no está en el repo; lo genera y cifra un humano con `generate.sh`, igual que ya hace falta para MinIO.
  - Al fijar `SSL_CERT_FILE` se reemplazan las raíces del sistema. Es correcto aquí, porque estos servicios solo usan TLS hacia MinIO y el resto es HTTP interno.
- **Amarillos.**
  - Los tres CronJobs llevan `startingDeadlineSeconds: 300` y `activeDeadlineSeconds: 1800`. 1800 s sobra: `go-reset` está acotado por `ReadyTimeout` (120 s), el script de baseline (60 s) y Redis (5 s).
  - El `---` doble ya no está.
  - El comentario de `rebuild` ya dice «housekeeping cada hora en :00».
  - La anotación `description: "INACTIVA..."` está en `AqsUpstreamCircuitOpen`.
  - La bitácora corrige lo de alpine y no distroless.
- **Alertas.** Hay casos de no disparo nuevos para `AqsTestJobFailed` y `AqsWarmQuarantined`, y todas las reglas tienen ahora al menos un caso de no disparo. Mis 4 mutaciones, una por regla, hacen fallar `promtool test`:
  - `increase(...) > 5` en `AqsHandoffRising`;
  - quitar el filtro de namespace con `> -1` en `AqsTestJobFailed`;
  - `>= 0` en `AqsWarmQuarantined`;
  - `>= 0` en `AqsUpstreamCircuitOpen`.

## Barrido de mutaciones (copia temporal, `policies.sh` completo)
`policies.sh` no detectó ninguna de estas mutaciones. Las 12 siguientes son del conjunto de la ronda 1 y de lo nuevo:

| Mutación | ¿`policies.sh` la detecta? |
|---|---|
| Quitar `RESET_REDIS_ADDR` | no |
| Nombre de Secret mal escrito (`go-reset-service-tokn`) | no |
| Borrar el PVC `go-reset-data` | no |
| Quitar `strategy: Recreate` | no |
| `readOnlyRootFilesystem: false` | no |
| `RUN_PHASES=fake` + `RUN_ALLOW_FAKE_PHASES=true` | no |
| Quitar el `volumeMount` de la CA (warm-manager) | no |
| Quitar `SSL_CERT_FILE` | no |
| Bajar `count/jobs.batch` a 1 | no |
| `secretName: minio-tlss` en el volumen de la CA | no |
| Clave equivocada (`tls.crt` en lugar de `ca.crt`) | no |
| Quitar `startingDeadlineSeconds` | no |

Esto es la misma clase «cableado sin prueba» de la ronda 1. Las únicas barreras automáticas son `secretrefs.rego` (token con valor literal) y `check-secrets` (Secret en claro). Un humano atrapa lo demás leyendo el build con los CA-2 y CA-3. Cerrar la brecha exige Rego o un script nuevo, y los dos están fuera del alcance de esta tarea. Es candidata y no bloquea el merge a desarrollo: el cableado coincide con el código, como verifiqué en la ronda 1 y no cambió.

## Hallazgos que quedan
- **F-A1 · AMARILLO · Credenciales S3 sin aprovisionar.** `init-bucket.sh` solo crea el bucket `evidence`. Nada crea un usuario MinIO para `aqs-evidence-s3`, y el comentario de cabecera no dice que ese par debe existir en MinIO. Un humano debe crearlo o reutilizar las credenciales de MinIO. Es candidata de documentación o provisión para U2-T08.
- **F-A2 · AMARILLO · Brechas de CI heredadas, ya declaradas.**
  - `ci.yml:198` corre `check-no-latest.sh deploy/flux/prod`, que no escanea `base/`. Un `image: ...:latest` en `base/` solo lo atrapa `check-no-latest.sh deploy` a mano.
  - `policies.sh` no prueba `aqs-u2-rules`.
  - Prod hereda `replicas: 2` sobre servicios con PVC RWO. La bitácora ya advierte de no usar prod.

## Dependencias de ejecución sin aportar (punto 6)
No encontré ninguna que bloquee desarrollo.
- **NetworkPolicy.**
  - `aqs-system` solo tiene reglas de ingress: no hay default-deny de egress, así que el acceso a la API de Kubernetes y a MinIO (mismo namespace) no está bloqueado. C-51 no es un bloqueo hoy; sería un endurecimiento futuro.
  - `aqs-test` tiene `allow-from-control-plane` (ingress desde `aqs-system`) y DNS permitido, así que el acceso a `warm-app` y `warm-redis` desde el control plane pasa.
  - Los runners hablan con `warm-app` en el mismo namespace. Es el controlador quien sube la evidencia a MinIO, no los runners.
- **Services y puertos.** Todos coinciden con las URL de entorno: 8080 para los servicios de `aqs-system`, `minio` 9000, `warm-app` 80 y `warm-redis` 6379. El `ServiceMonitor` usa el puerto `http` y el kube-state-metrics del chart cubre `kube_job_status_failed`.
- **Pod Security.** Los namespaces no llevan etiquetas de Pod Security Admission, así que no exigen `seccompProfile`.

Siguen siendo candidatas declaradas y no bloquean:
- No hay transporte de eventos hacia `RUN_EVENTS_FILE`, que vive en el PVC del controlador y nadie alimenta (C-45/C-49). Sin eso no hay corrida de extremo a extremo.
- Faltan los Secrets y el ConfigMap `go-reset-baseline` (los crea un humano con SOPS).
- Las imágenes `target-app`, `rehearsal` y `runner` `:0.0.0` son marcadores.
- La imagen de `go-reset` es alpine, así que el `baseline.sh` corre, pero le falta el cliente de base de datos (`psql`).
- Los Jobs no tienen TTL ni limpieza, y los Jobs de ensayo y runner son código fuera de alcance.

## Tareas candidatas
- Rego o script de cableado que cubra: fakes prohibidos; referencias colgantes a Secret, PVC y ConfigMap; `ResourceQuota` y `LimitRange` en `aqs-test`; `Recreate` y `runAsNonRoot` en los Deployments con PVC; `SSL_CERT_FILE` con su mount.
- Probar `aqs-u2-rules` desde `policies.sh`.
- Apuntar `check-no-latest.sh` en `ci.yml` a `deploy/`, en vez de solo `deploy/flux/prod`.
- Documentar el usuario MinIO de `aqs-evidence-s3` en `docs/operaciones/secrets.md`.
- Añadir `ttlSecondsAfterFinished` o limpieza de Jobs terminados.
- Parchear prod con `replicas: 1` para los servicios con PVC.
- Reconciliar el texto de los CA-2 (segunda expresión de yq) y CA-5 de la tarea.

VEREDICTO: VERDE
AMARILLO|deploy/flux/base/control-plane.yaml (aqs-evidence-s3)|Falta documentar/aprovisionar el usuario MinIO de aqs-evidence-s3
AMARILLO|scripts/ci/policies.sh y ci.yml|12 mutaciones de cableado no detectadas; no-latest de CI no escanea base/ (candidatas)
INFORME: revisiones/U2-T07/ronda-2.md
