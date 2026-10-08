F-01: Corregido en 3357ac2: count/jobs.batch sube a 100 con comentario (Jobs sin TTL ni limpieza); comentario de dimensionado con los valores del codigo (runner 500m/256Mi en runner/job.go:101, ensayo 250m/128Mi en rehearsal/job.go:107).
F-02: Corregido en 3357ac2: go-warm-manager y go-run-controller montan minio-tls (key ca.crt, solo lectura) en /etc/aqs/minio-ca y fijan SSL_CERT_FILE; verificado en el build con yq (readOnlyRootFilesystem sigue true).
F-03: Fuera de alcance: tarea candidata propuesta (Rego/script de cableado, check-no-latest sobre deploy/ en ci.yml).
F-04: Corregido en 3357ac2: la bitacora ya no dice distroless (alpine con /bin/sh; falta el cliente de BD, limite real para U2-T08).
F-05: No es un defecto: sin hallazgo (scale no se usa; matriz RBAC OK).
F-06: Corregido en 3357ac2: startingDeadlineSeconds 300 y activeDeadlineSeconds 1800 en los 3 CronJobs; comentario de rebuild corregido y --- doble eliminado. Lease: candidata.
F-07: Fuera de alcance: tarea candidata propuesta; advertencia en la bitacora (no usar prod con estos Deployments hasta resolverlo).
F-08: Corregido en 3357ac2: anotacion description INACTIVA en AqsUpstreamCircuitOpen y casos de no disparo para AqsTestJobFailed y AqsWarmQuarantined; probar aqs-u2-rules desde policies.sh: candidata.
F-09: Fuera de alcance: tarea candidata propuesta (docs/operaciones/secrets.md).
