# Tareas candidatas

Trabajo que apareció durante las rondas y que **nadie pidió todavía**. No forma parte de ninguna tarea en curso. El humano decide cuáles se convierten en tarea y cuándo.

| # | Origen | Candidata | Notas |
|---|---|---|---|
| C-01 | U5-T01 ronda 1 | Acotar por rutas cualquier criterio que cuente archivos de todo el repo | Ya aplicado en las tareas de las olas 2 y 3 |
| C-02 | U5-T03 (tarea) | Firmar imágenes con `cosign` en `publish` | |
| C-03 | U5-T03 ronda 1, F-04 | `no-latest --strict`: fallar si `deploy/flux/prod` no existe o no tiene `*.yaml` | `deploy/flux/prod` ya tiene manifiestos desde el PR #2 |
| C-04 | U5-T03 ronda 1 | Que el guard `no-latest` detecte `images: - newTag: latest` de Kustomize | |
| C-05 | U5-T03 ronda 2 | Que `detect-lang.sh` soporte servicios Go bajo `go.work` sin `go.mod` propio | |
| C-06 | U5-T03 ronda 2, F-07 | `npm test --if-present` pasa en verde si un `package.json` no tiene script `test`; usar `npm test` a secas | Anotada por decisión del humano |
| C-07 | U5-T04 ronda 1, F-02 | Definir el `GitRepository` `agentic-qa-swarm` y el namespace `flux-system` del bootstrap de Flux | Sin ellos, los `Kustomization` de Flux no sincronizan |
| C-08 | U5-T04 ronda 1, F-02 | Dueño del Secret `warm-db-credentials` y confirmación de `idleScaleDownAfter`, `minReplicasIdle` y los schedules de los CronJobs | |
| C-09 | U5-T04, enmienda F-03 | Quitar los `env: TZ=UTC` que solo existían para el grep de CA-7 | El grep ya está enmendado en main |
| C-10 | U5-T02 ronda 1, F-03 | Acelerar `contracts/validate.sh` con una sola invocación de ajv (hoy tarda alrededor de 1 minuto) | |
| C-11 | U5-T02 ronda 1, F-04 | Alinear el contrato REST con el de eventos cuando U1 implemente el control plane | `Notification.artifact` ya está alineado |
| C-12 | Ola 3, redacción de U5-T06 | NetworkPolicy deny-by-default para `aqs-system` y `aqs-observability` | U5-T06 cubre solo `aqs-test` (test-ns-only, US-M8) |
