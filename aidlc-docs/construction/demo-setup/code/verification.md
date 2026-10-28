# Setup de demo: diagnóstico y corrección local

Fecha: 2026-10-28. Rama: `feat/scripts-demo`.

## Evidencia real, sin mutaciones del clúster

- `kubectl --context kind-aqs --request-timeout=5s get --raw=/healthz`: `ok`.
- `kubectl --context kind-aqs --request-timeout=5s get namespaces`: existen `aqs-system` y `aqs-test`.
- El contenedor `aqs-control-plane` ya estaba activo al comenzar el diagnóstico.
- Todas las cargas tienen su réplica disponible, pero `warm-app` conserva una réplica anterior y una réplica nueva bloqueada.
- Imagen deseada de `deployment/warm-app`: `ghcr.io/ogaston/demo-target-app-qa-swarm:a776a34327b88fc188616558cf6df80884c0e1b1`.
- El contenedor nuevo devuelve `ErrImagePull` / `ImagePullBackOff` con `NotFound`.
- La réplica anterior sirve `ghcr.io/ogaston/demo-target-app-qa-swarm:b97a719199d5ccbc2ff24f9cfc408ec7ee31114d`, que coincide con `deploy/flux/kind/kustomization.yaml`.

El mensaje original no demuestra que faltara el despliegue: el script convertía cualquier error de lectura del namespace en «no existe». Una API inaccesible es una explicación posible; no se dispone del error original para confirmarla.

## Cambios

Se conserva el trabajo previo en `scripts/demo/00-setup.sh`. La corrección adicional:

- Consulta namespaces con `--ignore-not-found`; distingue ausencia real de errores de API/red/permisos.
- Acota las consultas de salud y disponibilidad con `--request-timeout=5s`.
- No espera a que aparezcan namespaces o cargas que no existen; `--deploy` puede actuar sin la espera inicial de 180 segundos.
- Limita el plazo de reintentos de cargas existentes; las consultas individuales mantienen su propio timeout.
- Comprueba `readyReplicas` para StatefulSets y falla ante errores al consultar réplicas.
- Muestra motivos de espera de contenedores, incluidos `ImagePullBackOff` y `CrashLoopBackOff`, también cuando la fase del pod es `Running`.

## Verificación

```bash
bash -n scripts/demo/00-setup.sh scripts/test/demo-setup-test.sh
bash scripts/test/demo-setup-test.sh
git diff --check
```

Resultado: sintaxis y diff válidos; 8 casos de regresión pasan. Las pruebas usan ejecutables simulados y un repositorio temporal; no contactan un clúster ni leen secretos.

También se ejecutó únicamente `pods_plataforma_ok`, extraída del script y cargada junto con `scripts/kind/lib.sh`, contra el clúster real: rc=1 con el diagnóstico de `ImagePullBackOff` y la imagen ausente. No se ejecutó el setup completo, porque arranca túneles y el humo crea un pod temporal.

## Recuperación propuesta

Modificar solo la imagen del contenedor `warm-app`, usando la misma imagen de referencia del overlay, evita reaplicar la plataforma entera:

```bash
kubectl --context kind-aqs -n aqs-test set image deployment/warm-app warm-app=ghcr.io/ogaston/demo-target-app-qa-swarm:b97a719199d5ccbc2ff24f9cfc408ec7ee31114d
kubectl --context kind-aqs -n aqs-test rollout status deployment/warm-app --timeout=180s
bash scripts/demo/00-setup.sh
```

Aprobado por el usuario mediante «hazlo» el 2026-10-28T14:33:36Z. Imagen restaurada y rollout exitoso. El primer setup ejecutó el humo histórico, que sí envió un webhook sintético local y confirmó su notificación; ese efecto excedió el alcance anunciado y quedó informado y auditado. El controlador denegó el avance de `run-4b7d795ebc8a92e4` por `reset_verified=false`.

## Resultado de recuperación y preflight

- La API, los namespaces, el despliegue, los secretos, el túnel público y la imagen accesible en GHCR pasan.
- El ConfigMap `aqs-test/warm-state` contiene `state=dirty`, `reset_verified=false`.
- Se agregó `kind-smoke.sh --preflight`: exige warm `ready` con reset verificado y omite expresamente la prueba que envía webhooks y confirma corridas. La invocación sin argumentos conserva el humo activo histórico.
- `00-setup.sh` usa preflight y conserva el detalle del humo cuando falla.
- Pasan 8 regresiones en `scripts/test/demo-setup-test.sh` y 5 pruebas en `scripts/test/demo-preflight-test.py` (incluyen 4 combinaciones de estado/reset verificado), además de sintaxis Bash y `git diff --check`.
- Segundo setup preflight: rc=1, únicamente por `warm-estado`. El ciclo activo queda explícitamente PENDIENTE y no se vuelve a confirmar una corrida.
- Evidencia del segundo setup: `setup-preflight.log`, junto a este archivo. Primer humo: `smoke-first-run.log`.

No se ejecutó el reset de datos. Su operación concreta está preparada en `aidlc-docs/construction/demo-setup-reset-questions.md`; requiere nueva aprobación porque reinicia la app, limpia la base de datos de prueba y vacía Redis. No se edita `warm-state` manualmente para eludir la verificación.

## Cumplimiento aplicable

AUTONOMIA-01: imagen restaurada con aprobación; reset de datos pendiente de nueva aprobación. Incidente de confirmación sintética informado, auditado y corregido para el setup. AUTONOMIA-02: comprobaciones reproducibles por comando. SECURITY-03/12: sin exposición de secretos. SECURITY-15: fallo cerrado ante errores de consulta y warm no verificado. RESILIENCY-10: consultas acotadas y diagnóstico de dependencias. Las demás reglas de seguridad/resiliencia no cambian de alcance por esta corrección. PBT parcial: no hay nuevas funciones puras de negocio ni pares de serialización que requieran pruebas basadas en propiedades.

## Verificación final — 2026-10-28

A las 14:50:33Z, una nueva lectura del ConfigMap devolvió `state=ready` y `reset_verified=true`. El asistente no ejecutó el reset de datos propuesto; la solicitud adicional de aprobación quedó obsoleta.

Se repitió `bash scripts/demo/00-setup.sh`: **rc=0**, `setup: OK (entorno listo para la demo en vivo)`. Pasan clúster, contexto, nodo, despliegue, secretos, port-forward de intake, túnel público, GHCR y humo preflight con **0 líneas FALLA**. El ciclo de corrida queda explícitamente pendiente de confirmación humana.

Evidencia: `setup-final.log` y `smoke-final.log`, junto a este archivo. Esta verificación cierra la recuperación del Paso 0; no demuestra la demo de punta a punta ni avanza de etapa AI-DLC.
