# Aprobación del reset verificado del warm

**Estado al 2026-10-28T14:50:33Z: solicitud obsoleta.** El warm ya devuelve `ready` con `reset_verified=true`. El asistente no ejecutó el reset propuesto; no hace falta responder esta pregunta para verificar el setup.

La imagen de referencia ya está restaurada y su rollout terminó correctamente. El setup detecta ahora el bloqueo real restante: `aqs-test/warm-state` contiene `state=dirty` y `reset_verified=false`. La plataforma deniega nuevas corridas por ese motivo.

## Pregunta 1

¿Apruebas ejecutar el reset verificado del warm local, que elimina los datos de prueba actuales y vacía su caché?

A) Aprobar: usar el servicio `go-reset` de `kind-aqs`, reiniciar la app warm, limpiar su base de datos de prueba, vaciar Redis y comprobar el baseline. Después, repetir `00-setup.sh` en modo preflight. No incluye confirmar nuevas corridas ni enviar webhooks.

B) Request Changes: conservar los datos actuales e indicar otra recuperación.

[Answer]:

## Operación concreta

1. Abrir un port-forward temporal y exclusivo en loopback: `kubectl --context kind-aqs -n aqs-system port-forward --address 127.0.0.1 svc/go-reset 18635:8080`.
2. Leer `go-reset-service-token` en memoria, sin imprimirlo ni pasarlo como argumento de proceso.
3. Enviar `POST http://127.0.0.1:18635/resets` con bearer de servicio y el cuerpo `{"run_id":"demo-setup-recovery-20261028","trace_id":"demo-setup-recovery-20261028"}`.
4. Exigir HTTP 200, `state=ready`, `reset_verified=true` y las comprobaciones `app-ready`, `db-baseline`, `cache-empty`. Cerrar el port-forward temporal.
5. Repetir `bash scripts/demo/00-setup.sh`, que ahora usa `kind-smoke.sh --preflight` y no confirma corridas.

La pérdida de los datos de prueba actuales es parte del reset. No se modifican staging/prod ni otros clústeres. No se editará manualmente el ConfigMap para fingir un reset verificado.

La aprobación anterior («hazlo») cubría restaurar la imagen y ejecutar setup; no autorizaba borrar datos. AUTONOMIA-01 exige aprobación registrada para esta nueva operación de infraestructura. La confirmación también es necesaria por la pérdida de datos indicada.

## Efecto observado del primer humo

El humo histórico usado por el primer setup sí envió y confirmó una notificación sintética (`run-4b7d795ebc8a92e4`), aunque el alcance anunciado excluía confirmaciones de QA. El controlador denegó su avance con `reset_verified=false`. Ese efecto se informó al usuario y quedó registrado en auditoría. El modo preflight agregado elimina ese comportamiento del Paso 0; el humo activo sigue disponible mediante su invocación histórica explícita.
