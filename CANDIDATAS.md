# Tareas candidatas — trabajo real que apareció y NO se hizo

> Una línea por candidata. Sale de una bitácora o de un informe del revisor durante una tarea, y
> **el orquestador decide** si se convierte en tarea nueva. El alcance que crece solo es la forma
> más común de que un proyecto de semestre no se termine: va aquí, nunca al diff de la tarea en curso.

| # | Fecha | Sale de (tarea / ronda) | Descripción | Decisión |
|---|---|---|---|---|
| C-136 | 2026-10-31 | Diagnóstico del orquestador tras U12-T02 (fusionada) | `scripts/demo/up.sh` solo ejecuta `build-images.sh` y `load-images.sh` si el clúster `aqs` es nuevo (`fresh=1`). Con un clúster existente las imágenes `:0.0.0` quedan viejas y los pods no se reinician, así que el código fusionado no llega al clúster (pasó con `/run-evidence` y ahora con la confirmación de ejecución de U12-T02). Propuesta: reconstruir y cargar siempre (o cuando la imagen sea anterior a `HEAD`) y hacer `kubectl rollout restart` de los deployments afectados; el humo debe comprobar que la corrida queda en `awaiting_execution`. | Convertida en U12-T03 (2026-10-31, a pedido del humano) |
| C-137 | 2026-10-31 | Separada de C-136 al redactar U12-T03 | El humo (`kind-smoke.sh`) debe comprobar que una corrida con ensayo aprobado queda en `awaiting_execution:true` y no pasa a `running` sin `POST /runs/{id}/execute`. | Pendiente |
