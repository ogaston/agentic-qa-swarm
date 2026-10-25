# Demo con el repositorio real (U11-T02)

Guion de la demo de punta a punta con el repositorio objetivo **real**, `ogaston/demo-target-app-qa-swarm`
(público), en lugar de la app que vivía en `demo/target-app` de este repositorio (ya borrada).

El flujo: un **push a `main`** del repo objetivo → GitHub entrega el webhook por el túnel → `go-intake` valida
la firma y crea la notificación → el humano confirma en el dashboard → el warm despliega **la imagen de ese
commit** (`ghcr.io/ogaston/demo-target-app-qa-swarm:<sha>`) → se infiere la superficie, se ensaya, se corren
los flujos, se resetea (reset verificado) y se publica el reporte.

## Lo que importa saber antes

- **Solo los push a `main` tienen imagen.** La CI del repo objetivo publica `ghcr.io/…:<sha>` en cada push a
  `main`. Un push a otra rama o un PR **no** publica imagen, así que la plataforma no puede desplegarlo.
- **La imagen tarda en aparecer.** Tras el push, espera a que la CI del repo objetivo termine en verde antes de
  confirmar la notificación. Si confirmas antes, el deploy falla con la imagen aún inexistente.
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
bash scripts/kind/kind-smoke.sh 2>&1 | grep -c '^FALLA'      # esperado: 0
kubectl --context kind-aqs -n aqs-test get deploy warm-app -o jsonpath='{..image}'; echo
```

Se ve: la imagen de referencia del warm antes de la demo (la del último despliegue).

### Paso 1. Commit sano (caso sin hallazgos)

En el clon del repo objetivo, sin `ENV TARGET_APP_BUG` en el `Dockerfile`:

```bash
git commit --allow-empty -m "demo: commit sano"
git push origin HEAD:refs/heads/main
git rev-parse HEAD            # anota el SHA de 40 caracteres: SHA_SANO
```

Espera a que la CI del repo objetivo publique `ghcr.io/ogaston/demo-target-app-qa-swarm:<SHA_SANO>`
(pestaña *Actions* del repo, y/o `docker manifest inspect` si lo tienes a mano).

Luego, en la plataforma:

```bash
bash scripts/kind/github-webhook-check.sh <SHA_SANO>
```

Se ve: `OK webhook-github-recibido` y una línea `notificación=<id> evento=commit repo=ogaston/demo-target-app-qa-swarm estado=pending`.
En el dashboard aparece la notificación en *pending*. Confírmala: se crea la corrida y el warm despliega
`…:<SHA_SANO>`. El ciclo termina con **veredicto sin hallazgos** (todos los flujos pasan).

### Paso 2. Commit con el defecto (caso con hallazgo)

En el mismo repo, un commit que fija el defecto en su `Dockerfile` (en la etapa final, junto al `ENV` existente):

```dockerfile
ENV LISTEN_ADDR=:80 BASELINE_LISTEN_ADDR=:9090
ENV TARGET_APP_BUG=oversell
```

```bash
git commit -am "demo: fija el defecto oversell"
git push origin HEAD:refs/heads/main
git rev-parse HEAD            # SHA_DEFECTO
```

Espera la imagen de `SHA_DEFECTO` (paso 1), repite `github-webhook-check.sh <SHA_DEFECTO>` y confirma la
notificación. Se ve: el warm despliega `…:<SHA_DEFECTO>` y el ensayo/los runners fallan en el flujo de
compra porque la app vende más stock del que tiene. El reporte declara **bug** citando esa evidencia.

Lectura de vuelta (obligatoria, para demostrar que se desplegó el commit y no otra imagen):

```bash
kubectl --context kind-aqs -n aqs-test get deploy warm-app -o jsonpath='{..image}'; echo
# debe terminar en :<SHA_DEFECTO>
```

### Paso 3. Qué mirar en cada paso

| Paso | Dónde | Qué se ve |
|---|---|---|
| webhook | `github-webhook-check.sh` / Recent Deliveries | `202`, notificación `pending` |
| confirmación | dashboard | recibo con `run_id` |
| deploy | `GET /deploys/{run_id}` | `done` con la imagen del SHA |
| superficie | `POST /surface` | endpoints inferidos del OpenAPI de la app |
| ensayo y runners | Jobs en `aqs-test` | pasan (sano) o fallan en el flujo de compra (defecto) |
| reset | `GET /warm` | `reset_verified: true` |
| reporte | MinIO `s3://evidence/runs/<run>/` | `report-*.json`, veredicto |

## Si la imagen aún no existe

No confirmes. Espera a que la CI del repo objetivo termine en verde y vuelve a comprobar que el manifiesto existe.
Si la CI falla, arregla el commit en el repo objetivo (no en la plataforma). El e2e automatizado
(`scripts/kind/kind-e2e.sh`) hace esta comprobación antes de confirmar y sale con código 2 y un mensaje claro si no hay imagen.

## Tiempos

Pendiente de medir en la primera corrida real de la demo. Se anotan aquí tiempos observados por paso
(push → imagen publicada; webhook → notificación; confirmación → `done`; ciclo completo). No se dan
cifras sin medir.

## Qué no hace esta demo

- No modifica el repositorio objetivo desde la plataforma: los commits los hace el humano.
- No usa `kubectl patch` para inyectar el defecto (el `--bug oversell` de `kind-e2e.sh` es un flujo de prueba aparte).
- No cubre PR: los PR no publican imagen.
