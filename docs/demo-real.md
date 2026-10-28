# Demo con el repositorio real (U11-T02)

Guion de la demo de punta a punta con el repositorio objetivo **real**, `ogaston/demo-target-app-qa-swarm`
(público), en lugar de la app que vivía en `demo/target-app` de este repositorio (ya borrada).

El flujo: un **push a `main`** del repo objetivo → GitHub entrega el webhook por el túnel → `go-intake` valida
la firma y crea la notificación → el humano confirma en el dashboard → el warm despliega **la imagen de ese
commit** (`ghcr.io/ogaston/demo-target-app-qa-swarm:<sha>`) → se infiere la superficie, se ensaya, se corren
los flujos, se resetea (reset verificado) y se publica el reporte.

## Estado conocido (lee esto primero)

No quedan bloqueos de plataforma conocidos para esta demo: U11-T04 (`flow_id` apto para Job), U11-T05 (egreso LLM)
y U11-T06 (cobertura del puerto LLM) ya están fusionadas. El Paso 0 debe salir limpio (`0` líneas `FALLA`); si no,
trátalo como regresión y no sigas hasta diagnosticarlo.

Lo que **sí se observó** en la corrida real de la ronda 1 de U11-T02 (`kind-e2e.sh --bug off`, commit
`b97a719…`): port-forwards, login, webhook `202`, inbox, confirmación, deploy sobre el warm (imagen del repo
objetivo), superficie inferida, ensayo y ensayo-job. Aquella corrida se detuvo en `runners` por el defecto ya
corregido en U11-T04; el resultado de los runners, el reset, el reporte y el veredicto siguen pendientes de
observarse en esta demo en vivo.

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

Las precondiciones 1–4 las comprueba (y en parte resuelve) `bash scripts/demo/00-setup.sh`; las 5 y 6 son
manualmente del humano.

1. Clúster `kind-aqs` desplegado: `bash scripts/kind/deploy.sh` termina con `deploy: OK`. Controlador
   (`go-run-controller`) desplegado **antes** que `ui-api` (README de `go-run-controller`).
2. Secretos generados, incluido `go-intake-webhook` (`bash scripts/kind/secrets.sh`).
3. Red a `ghcr.io` (la imagen se descarga de forma anónima: el paquete del repo objetivo debe ser **público**).
4. `cloudflared` en el host. Recomendado: túnel con nombre (`docs/operaciones/webhook-github-cloudflared.md`).
5. Webhook en el repo objetivo (`https://github.com/ogaston/demo-target-app-qa-swarm/settings/hooks`): solo
   evento `push`, `Content-Type: application/json`, secreto = el de `go-intake-webhook`.
6. Clon del repo objetivo en tu máquina, con permiso de push a `main`.

## Guion (con los scripts asistidos)

Los pasos 0–2 se hacen con los scripts de `scripts/demo/` (detalle en `scripts/demo/README.md`).
Los comandos manuales equivalentes quedan en la sección [Guion manual (referencia)](#guion-manual-referencia)
por si hay que diagnosticar algo.

### Paso 0. Comprobar el entorno

```bash
bash scripts/demo/00-setup.sh            # o con --deploy si la plataforma no está desplegada
```

Esperado: todas las comprobaciones en `OK` (clúster, despliegue, secretos, túnel, ghcr, humo sin `FALLA`)
y al final la URL pública del webhook y la del dashboard, más los pasos manuales pendientes (precondiciones
5 y 6). Además:

```bash
kubectl --context kind-aqs -n aqs-test get deploy warm-app -o jsonpath='{..image}'; echo
```

Esperado: la imagen del `warm-app` es la del último despliegue. Antes de la demo, la imagen de referencia es
`ghcr.io/ogaston/demo-target-app-qa-swarm:b97a719199d5ccbc2ff24f9cfc408ec7ee31114d`.

### Paso 1. Commit sano (caso sin hallazgos) — resultado ESPERADO, no observado

```bash
bash scripts/demo/10-push-commit.sh --repo-dir /ruta/al/clon --bug off
# imprime SHA=<sha40> y OK imagen-publicada cuando la CI ya publicó la imagen
bash scripts/demo/20-run-and-watch.sh <SHA_SANO>
```

El primer script empuja el commit vacío a `main` y espera a que la imagen exista en GHCR (no hay que vigilar
la pestaña *Actions*). El segundo empuja el webhook, espera la confirmación humana en el dashboard (o confirma
él mismo con `--confirm-auto`), vigila la corrida hasta `done` y cierra con el resumen. Esperado:
**`OK demo-corrida-completa` con veredicto sin hallazgos** (todos los flujos pasan), imagen desplegada
`…:<SHA_SANO>` y `reset_verified=true`.

### Paso 2. Commit con el defecto (caso con hallazgo) — resultado ESPERADO, no observado

```bash
bash scripts/demo/10-push-commit.sh --repo-dir /ruta/al/clon --bug oversell
# añade ENV TARGET_APP_BUG=oversell al Dockerfile (idempotente), empuja y espera la imagen
bash scripts/demo/20-run-and-watch.sh <SHA_DEFECTO>
```

`--bug oversell` hace exactamente la edición del paso manual (la línea `ENV` junto a la que ya existe, en ese
orden: editar → add → commit → push). Esperado: el warm despliega `…:<SHA_DEFECTO>` y el ensayo/los runners
fallan en el flujo de compra porque la app vende más stock del que tiene; el reporte declara **bug** citando
esa evidencia.

### Paso 2b. Segundo commit con imagen para la lectura de vuelta de CA-6 — ya verificado

CA-6 de U11-T02 quedó verificado el 2026-10-26 en un clúster real con el segundo commit `d000bf8` (humo 0
FALLA, e2e rc=0 con 23 checks, segunda lectura de imagen distinta de la fijada; ver `bitacoras/U11-T02.md`).
Si se repite la demo, la comprobación sigue siendo la misma: la lectura de vuelta solo discrimina si el SHA
desplegado **no** es el fijado (`b97a719…`). Usa `SHA_SANO` o `SHA_DEFECTO` (cualquiera con imagen publicada,
distinto de `b97a719…`) como `TARGET_SHA` en el e2e, y verifica después:

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
| estado de la corrida | dashboard, por `run_id` | fases hasta `done` |
| reset | dashboard (vista del warm, que lee `GET /warm` de ui-api: `reset_verified`) | `true` tras la corrida |
| reporte | dashboard, por `run_id` | veredicto (`bug` en el caso con defecto) |

El `run_id` sale del **recibo de confirmación** que muestra el dashboard al confirmar la notificación (la
respuesta `Receipt {run_id}` de `POST /notifications/{id}/confirm`). Es el identificador de la corrida en el
dashboard y en el nombre de sus Jobs (`aqs.io/run-id`).

Lectura de la imagen (lo que demuestra qué commit se desplegó):

```bash
kubectl --context kind-aqs -n aqs-test get deploy warm-app -o jsonpath='{..image}'; echo
```

## Guion manual (referencia)

Lo que hacen los scripts por dentro, por si hay que diagnosticar un paso.

### Paso 0 manual

```bash
bash scripts/kind/kind-smoke.sh 2>&1 | grep -c '^FALLA'      # esperado: 0; si da otro valor, ver Estado conocido
```

### Paso 1 manual (commit sano)

En el clon del repo objetivo, sin `ENV TARGET_APP_BUG` en el `Dockerfile`:

```bash
git commit --allow-empty -m "demo: commit sano"
git push origin HEAD:refs/heads/main
git rev-parse HEAD            # anota el SHA de 40 caracteres: SHA_SANO
```

Espera a que la CI del repo objetivo termine en verde (pestaña *Actions* del repo). La imagen
`ghcr.io/ogaston/demo-target-app-qa-swarm:<SHA_SANO>` existe solo entonces. Luego, en la plataforma:

```bash
bash scripts/kind/github-webhook-check.sh <SHA_SANO>
```

Esperado: `OK webhook-github-recibido` y una línea `notificación=<id> evento=commit repo=ogaston/demo-target-app-qa-swarm estado=pending`.
En el dashboard aparece la notificación en *pending*. Confírmala: el dashboard muestra el recibo con el `run_id`
y el warm despliega `…:<SHA_SANO>`. Esperado al final: **veredicto sin hallazgos**.

### Paso 2 manual (commit con el defecto)

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

## Si la imagen aún no existe

No confirmes ni empujes el webhook todavía. Espera a que la CI del repo objetivo termine en verde y comprueba que
la imagen existe. Si la CI falla, arregla el commit en el repo objetivo (no en la plataforma). Los scripts de la
demo ya hacen esta comprobación: `10-push-commit.sh` no devuelve el SHA hasta que la imagen existe (sale con
código 2 si no aparece y 3 si no puede comprobarla), y `20-run-and-watch.sh` la vuelve a exigir antes de empujar
el webhook o confirmar, distinguiendo «la imagen no existe» de «no pude comprobarla» (token anónimo, red o límite
de GHCR), igual que el e2e automatizado (`scripts/kind/kind-e2e.sh`).

## Tiempos

Pendiente de medir en la primera corrida real completa de la demo. Se anotarán aquí los tiempos observados por
paso (push → imagen publicada; webhook → notificación; confirmación → `done`; ciclo completo). No se dan cifras
sin medir.

## Qué no hace esta demo

- No modifica el repositorio objetivo desde la plataforma: los commits los hace el humano.
- No usa `kubectl patch` para inyectar el defecto (el `--bug oversell` de `kind-e2e.sh` es un flujo de prueba aparte).
- No cubre PR: los PR no publican imagen.
