# Continuidad del setup de demo local

## Contexto y alcance

Reanudación solicitada el 2026-10-28 sobre `feat/scripts-demo`, dentro de CONSTRUCTION activa. Se conservan los cambios previos de `scripts/demo/00-setup.sh` y el directorio no rastreado ajeno a esta corrección. Referencias: U7 (kind), U11-T02 y Paso 0 del guion existente `docs/demo-real.md`.

El diagnóstico real encuentra la API sana, ambos namespaces existentes y un rollout de `warm-app` bloqueado por una imagen ausente: `ghcr.io/ogaston/demo-target-app-qa-swarm:a776a34327b88fc188616558cf6df80884c0e1b1`. El overlay fija la imagen de referencia `b97a719199d5ccbc2ff24f9cfc408ec7ee31114d`.

No se requieren etapas nuevas de diseño: no cambian arquitectura, contratos ni lógica de negocio. La petición de continuar autoriza el diagnóstico y la corrección local dentro del trabajo existente; cualquier cambio efectivo al clúster requiere aprobación registrada según AUTONOMIA-01.

## Pasos

- [x] 1. Leer estado, reglas activas y cambios existentes; verificar API, namespaces, cargas y eventos sin modificar infraestructura.
- [x] 2. Corregir `scripts/demo/00-setup.sh`: errores de API diferenciados de namespace ausente, tiempos de espera acotados, disponibilidad correcta de StatefulSets y diagnóstico del motivo de espera de contenedores. No esperar 180 segundos cuando faltan namespaces.
- [x] 3. Verificar con `bash -n scripts/demo/00-setup.sh` y pruebas aisladas de regresión en `scripts/test/demo-setup-test.sh`; ninguna prueba de regresión modifica el clúster. Pasan 8 casos y `git diff --check`; diagnóstico real comprobado mediante la función de disponibilidad, sin ejecutar setup completo.
- [x] 4. Documentar evidencia, actualizar estado y preparar una acción concreta de recuperación revisable para aprobación humana. Evidencia en `aidlc-docs/construction/demo-setup/code/verification.md`; aprobación pendiente en `aidlc-docs/construction/demo-setup-recovery-questions.md`.
- [x] 5. Tras aprobación humana registrada («hazlo», 2026-10-28T14:33:36Z), restaurar únicamente la imagen de `warm-app` a la referencia del overlay mediante el comando `kubectl set image` documentado y verificar rollout. Resultado: `deployment "warm-app" successfully rolled out`. El lock estaba heredado por un port-forward y no había otro script de prueba activo; se siguió la pauta existente del setup.
- [x] 6. Repetir el Paso 0 completo y registrar resultado. Primer setup rc=1 por ciclo-webhook-confirm-corrida; se descubrió que el humo histórico enviaba y confirmaba una notificación sintética, contrario al alcance anunciado. Se informó el efecto y se registró en auditoría. El controlador denegó el avance por reset_verified=false.
- [x] 7. Separar preflight y ciclo activo: `scripts/kind/kind-smoke.sh --preflight` no confirma corridas y exige warm ready/reset verificado; `00-setup.sh` usa ese modo y conserva el detalle de los fallos. Pasan 8 regresiones de setup y 5 pruebas de preflight. Segundo setup rc=1, exclusivamente por warm-estado: dirty/reset_verified=false; ciclo activo PENDIENTE, sin nueva confirmación.
- [x] 8. Comprobar de nuevo el bloqueo antes de ejecutar un reset. A las 2026-10-28T14:50:33Z el warm ya estaba ready/reset_verified=true; no se ejecutó el reset propuesto y ya no es necesario. El origen de ese cambio no se atribuye al asistente. La pregunta de aprobación queda obsoleta.
- [x] 9. Ejecutar la verificación final de `00-setup.sh` preflight con el warm ya verificado y registrar resultado. Resultado: rc=0, setup OK y humo con 0 líneas FALLA. Evidencia en `aidlc-docs/construction/demo-setup/code/setup-final.log` y `smoke-final.log`. Paso 0 recuperado; demo real pendiente de confirmación humana.

## Restricciones y validación

AUTONOMIA-01: recuperación de imagen aprobada; el reset propuesto no fue ejecutado y dejó de ser necesario al observar warm ready/reset_verified=true. El incidente de confirmación sintética del primer humo se registró y corrigió para futuras ejecuciones del setup. AUTONOMIA-02: cada resultado tiene comando verificable. SECURITY-03/12: no imprimir credenciales. SECURITY-15 y RESILIENCY-10: distinguir fallos de lectura, exigir reset verificado y acotar llamadas a la API. El resto de reglas de seguridad no cambia en esta corrección. PBT parcial: no se agregan serializadores, algoritmos ni invariantes de negocio; no aplica una nueva suite PBT.
