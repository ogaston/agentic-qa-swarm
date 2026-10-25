# Demo con el repositorio real (U11-T02)

Guion de la demo de punta a punta con el repositorio objetivo **real**, `ogaston/demo-target-app-qa-swarm`
(público), en lugar de la app que vivía en `demo/target-app` de este repositorio (ya borrada).

El flujo: un **push a `main`** del repo objetivo → GitHub entrega el webhook por el túnel → `go-intake` valida
la firma y crea la notificación → el humano confirma en el dashboard → el warm despliega **la imagen de ese
commit** (`ghcr.io/ogaston/demo-target-app-qa-swarm:<sha>`) → se infiere la superficie, se ensaya, se corren
los flujos, se resetea (reset verificado) y se publica el reporte.

## Estado conocido (lee esto primero)

Bloqueos pendientes, fuera de esta tarea:

- **U11-T05 — egreso LLM fijado por IP.** El control positivo `llm-egress-acotado` de `kind-smoke.sh` falla porque
  el proveedor LLM rota su IP y el planner no alcanza el gateway. Mientras U11-T05 no se fusione, el **Paso 0 puede
  dar `1`** (`FALLA llm-egress-acotado`). Ese único `FALLA` no impide la demo hasta el ensayo, pero el paso de
  comprobación no sale limpio.
- **U11-T04 — `flow_id` inválido para el nombre del Job.** El modelo a veces inventa `flow_id` con formas que el
  runner no admite como nombre de Job (`run_id/flow_id no aptos para un nombre de Job`). Con ids largos o raros la
  corrida se queda en la fase `runners` (`running`) y no llega a `done`. Mientras U11-T04 no se fusione, **la demo
  puede bloquearse en `runners`**.

Lo que **sí se observó** en la corrida real de la ronda 1 de U11-T02 (`kind-e2e.sh --bug off`, commit
`b97a719…`): port-forwards, login, webhook `202`, inbox, confirmación, deploy sobre el warm (imagen del repo
objetivo), superficie inferida, ensayo y ensayo-job. **No observado:** el resultado de los runners, el reset, el
reporte y el veredicto. El comando del e2e falla en `runners` con `rc=1`.

## Lo que importa saber antes

- **Solo los push a `main` tienen imagen.** La CI del repo objetivo publica `ghcr.io/ogaston/demo-target-app-qa-swarm:<sha>`
  en cada push a `main`. Un push a otra rama o un PR **no** publica imagen, así que la plataforma no puede
  desplegarlo.
- **La imagen tarda en aparecer.** Tras el push, espera a que la CI del repo objetivo termine en verde (y la
  imagen exista) **antes de empujar el webhook y de confirmar**. Si confirmas antes, el deploy falla porque la
  imagen aún no existe.
- **El defecto lo trae el commit, no un parche.** El defecto sembrado es `TARGET_APP_BUG=oversell` (la invariante
  «el stock nunca es negativo» deja de aplicarse). Se activa con una línea `ENV` en el `Dockerfile` del repo
  objetivo, dentro del commit que se empuja. No se inyecta con `kubectl patch`.
- **El `--bug oversell` de `kind-e2e.sh` es otra cosa**: parchea el `warm-app` tras el deploy para el ensayo
  automatizado. Para la demo en vivo usa el commit con `ENV`, no ese flag.

## Precondiciones

1. Clúster `kind-aqs` desplegado: `bash scripts/kind/deploy.sh` termina con `deploy: OK`. Controlador
   (`go-run-controller`) desplegado **antes** que `ui-api` (README de `go-run-controller`).
2. Secretos generados, incluido `go-intake-webhook` (`bash scripts/kind/secrets.sh`).
3. Red a `ghcr.io` (la imagen se descarga de forma anónima: el paquete del repo objetivo debe ser **público**).
4. `cloudflared` en el host. Recomendado: túnel con nombre (`docs/operaciones/webhook-github-cloudflared.md`).
5. Webhook en el repo objetivo (`https://github.com/ogaston/demo-target-app-qa-swarm/settings/hooks`): solo
   evento `push`, `Content-Type: application/json`, secreto = el de `go-intake-webhook`.
6. Clon del repo objetivo en tu máquina, con permiso de push a `main`.

## Guion

### Paso 0. Comprobar el entorno

```bash
bash scripts/kind/kind-smoke.sh 2>&1 | grep -c '^FALLA'      # esperado: 0 (ESPERADO, no observado; hoy puede dar 1, ver Estado conocido)
kubectl --context kind-aqs -n aqs-test get deploy warm-app -o jsonpath='{..image}'; echo
```

Esperado: la imagen del `warm-app` es la del último despliegue. Antes de la demo, la imagen de referencia es
`ghcr.io/ogaston/demo-target-app-qa-swarm:b97a719199d5ccbc2ff24f9cfc408ec7ee31114d`.

### Paso 1. Commit sano (caso sin hallazgos) — resultado ESPERADO, no observado

En el clon del repo objetivo, sin `ENV TARGET_APP_BUG` en el `Dockerfile`:

```bash
git commit --allow-empty -m "demo: commit sano"
git push origin HEAD:refs/heads/main
git rev-parse HEAD            # anota el SHA de 40 caracteres: SHA_SANO
```

Espera a que la CI del repo objetivo termine en verde (pestaña *Actions* del repo). La imagen
`ghcr.io/ogaston/demo-target-app-qa-swarm:<SHA_SANO>` existe solo entonces.

Luego, en la plataforma:

```bash
bash scripts/kind/github-webhook-check.sh <SHA_SANO>
```

Esperado: `OK webhook-github-recibido` y una línea `notificación=<id> evento=commit repo=ogaston/demo-target-app-qa-swarm estado=pending`.
En el dashboard aparece la notificación en *pending*. Confírmala: el dashboard muestra el recibo con el `run_id`
y el warm despliega `…:<SHA_SANO>`. Esperado al final: **veredicto sin hallazgos** (todos los flujos pasan).

### Paso 2. Commit con el defecto (caso con hallazgo) — resultado ESPERADO, no observado

**Antes del commit, edita el Dockerfile del repo objetivo** (`Dockerfile`, etapa final): añade la línea
`ENV TARGET_APP_BUG=oversell` junto al `ENV` que ya existe. El bloque completo queda así:

```dockerfile
FROM alpine:3.24
# …(sin cambios)…
EXPOSE 80 9090
ENV LISTEN_ADDR=:80 BASELINE_LISTEN_ADDR=:9090
ENV TARGET_APP_BUG=oversell
ENTRYPOINT ["/usr/local/bin/target-app"]
```

Luego, y solo luego, el commit y el push (**no** uses `git commit -am` antes de editar el Dockerfile):

```bash
git add Dockerfile
git commit -m "demo: fija el defecto oversell"
git push origin HEAD:refs/heads/main
git rev-parse HEAD            # SHA_DEFECTO (distinto de b97a719 y de SHA_SANO)
```

Espera a que la CI publique `ghcr.io/ogaston/demo-target-app-qa-swarm:<SHA_DEFECTO>` **antes** de empujar el
webhook de la plataforma o de confirmar. Luego `github-webhook-check.sh <SHA_DEFECTO>` y confirma la notificación.
Esperado: el warm despliega `…:<SHA_DEFECTO>` y el ensayo/los runners fallan en el flujo de compra porque la app
vende más stock del que tiene; el reporte declara **bug** citando esa evidencia. Hoy el paso puede bloquearse en
`runners` (U11-T04).

### Paso 2b. Segundo commit con imagen para la lectura de vuelta de CA-6

La lectura de vuelta solo discrimina si el SHA desplegado **no** es el fijado (`b97a719…`). Usa `SHA_SANO` o
`SHA_DEFECTO` (cualquiera con imagen publicada, distinto de `b97a719…`) como `TARGET_SHA` en el e2e, y verifica
después:

```bash
kubectl --context kind-aqs -n aqs-test get deploy warm-app -o jsonpath='{..image}'; echo
# debe terminar en :<TARGET_SHA usado>, no en :b97a719…
```

### Paso 3. Qué mirar en cada paso y dónde

| Paso | Dónde se ve | Qué se espera |
|---|---|---|
| webhook | `github-webhook-check.sh` y, en GitHub, *Recent Deliveries* del repo objetivo | `202`; notificación `pending` |
| confirmación | dashboard (ui-api) | recibo con `run_id` |
| imagen desplegada | `kubectl --context kind-aqs -n aqs-test get deploy warm-app -o jsonpath='{..image}'; echo` | la imagen del SHA elegido |
| estado de la corrida | dashboard, por `run_id` | fases hasta `done` (hoy se detiene en `runners`, U11-T04) |
| reset | dashboard (vista del warm, que lee `GET /warm` de ui-api: `reset_verified`) | `true` tras la corrida |
| reporte | dashboard, por `run_id` | veredicto (`bug` en el caso con defecto) |

El `run_id` sale del **recibo de confirmación** que muestra el dashboard al confirmar la notificación (la
respuesta `Receipt {run_id}` de `POST /notifications/{id}/confirm`). Es el identificador de la corrida en el
dashboard y en el nombre de sus Jobs (`aqs.io/run-id`).

Lectura de la imagen (lo que demuestra qué commit se desplegó):

```bash
kubectl --context kind-aqs -n aqs-test get deploy warm-app -o jsonpath='{..image}'; echo
```

## Si la imagen aún no existe

No confirmes ni empujes el webhook todavía. Espera a que la CI del repo objetivo termine en verde y comprueba que
la imagen existe. Si la CI falla, arregla el commit en el repo objetivo (no en la plataforma). El e2e automatizado
(`scripts/kind/kind-e2e.sh`) hace esta comprobación antes de confirmar: sale con código 2 y un mensaje claro si no
hay imagen, y distingue «la imagen no existe» de «no pude comprobarla» (token anónimo, red o límite de GHCR).

## Tiempos

Pendiente de medir en la primera corrida real completa de la demo. Se anotarán aquí los tiempos observados por
paso (push → imagen publicada; webhook → notificación; confirmación → `done`; ciclo completo). No se dan cifras
sin medir.

## Qué no hace esta demo

- No modifica el repositorio objetivo desde la plataforma: los commits los hace el humano.
- No usa `kubectl patch` para inyectar el defecto (el `--bug oversell` de `kind-e2e.sh` es un flujo de prueba aparte).
- No cubre PR: los PR no publican imagen.
