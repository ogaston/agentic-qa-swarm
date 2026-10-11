# U8-T01 — respuesta del codificador a la ronda 1

F-01: Corregido en el commit de la ronda 2: `require_kind_context` pasa antes de `trap cleanup EXIT` en `scripts/kind/nats-netpol-check.sh`. Prueba con kubectl falso (bitácora, Ronda 2): rojo con `other-cluster` ejecutaba `kubectl delete pod`; verde solo ejecuta `kubectl config current-context` y sale con rc=3.
F-02: Corregido: `deploy/flux/base/nats/README.md` documenta la variante verificada (pod etiquetado, `wait` a `Succeeded`, `kubectl logs | jq`, borrado del pod). Salida real en la bitácora, Ronda 2, sección CA-3 variante.
F-03: Corregido: la bitácora tiene el rojo de `nats stream info` antes del despliegue, la salida de CA-5 y la nota de los reinicios de `nats-init` (3 reinicios, Job `Succeeded`).
F-04: Corregido: sección «NATS JetStream (U8-T01)» en `docs/operaciones/kind-local.md`.
Tareas candidatas del informe: se mantienen como candidatas (ninguna hecha aquí). Actualizar `docs/seguridad/aislamiento.md` queda fuera de la cerca de CA-5.
