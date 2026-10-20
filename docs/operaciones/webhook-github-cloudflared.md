# Webhook real de GitHub hacia el inbox (U8-T09, CA-3)

Paso exacto para que un **push real de GitHub** llegue a `go-intake` a través de un túnel `cloudflared`
y aparezca en el inbox de `ui-api` del clúster kind `aqs`. Lo ejecuta el **humano**: el codificador no
empuja commits a GitHub ni configura el webhook del repositorio.

Ningún valor de secreto ni URL de túnel va al repositorio. La clave del HMAC vive en el Secret
`go-intake-webhook` del clúster (la crea `scripts/kind/secrets.sh`) y se copia al portapapeles, nunca a
una salida de terminal ni a un archivo.

## Requisitos previos

- Clúster `kind-aqs` desplegado: `bash scripts/kind/deploy.sh` terminó con `deploy: OK`.
- `cloudflared` instalado en el host. El repositorio es `ogaston/agentic-qa-swarm`.
- Permisos de administrador del repositorio en GitHub (para crear el webhook).

## Paso 1. Port-forward de go-intake (terminal A, se queda abierto)

```bash
kubectl --context kind-aqs -n aqs-system port-forward --address 127.0.0.1 svc/go-intake 18622:8080
```

Puerto `18622` en loopback. No choca con el humo (`18600-18606`), el e2e (`18615-18619`) ni el dashboard.

## Paso 2. Túnel cloudflared (terminal B, se queda abierto)

```bash
cloudflared tunnel --url http://127.0.0.1:18622
```

`cloudflared` imprime una URL pública `https://<nombre>.trycloudflare.com`. Esa URL es la del paso 3. Un
túnel rápido cambia de URL en cada arranque: si lo reinicias, actualiza el campo *Payload URL* en GitHub.
Un túnel con nombre propio es equivalente y da una URL estable; elige el que prefieras.

## Paso 3. Secreto del webhook: copiar al portapapeles, sin imprimir

GitHub firma cada entrega con HMAC-SHA256 usando un secreto que tú configuras. Debe ser **el mismo** valor
que tiene el Secret del clúster. Cópialo sin mostrarlo en la terminal:

```bash
# Wayland
kubectl --context kind-aqs -n aqs-system get secret go-intake-webhook -o jsonpath='{.data.secret}' | base64 -d | wl-copy
# X11
kubectl --context kind-aqs -n aqs-system get secret go-intake-webhook -o jsonpath='{.data.secret}' | base64 -d | xclip -selection clipboard
```

## Paso 4. Webhook en GitHub

En `https://github.com/ogaston/agentic-qa-swarm/settings/hooks` → **Add webhook**:

| Campo | Valor |
|---|---|
| Payload URL | `https://<nombre>.trycloudflare.com/webhooks/github` (URL del paso 2 más la ruta) |
| Content type | `application/json` |
| Secret | pega el contenido del portapapeles (paso 3) |
| Which events | **Just the push event** |
| Active | marcado |

Pulsa **Add webhook**. GitHub envía un `ping` inicial; en **Recent Deliveries** debe aparecer con código `2xx`
(`ping` no crea notificación; lo que cuenta es el push del paso 5).

## Paso 5. Empujar un commit real

Desde tu clon del repositorio, en una rama que tenga webhook activo (la de prueba que prefieras):

```bash
git commit --allow-empty -m "test(U8-T09): entrega real de GitHub para CA-3"
git push origin HEAD:refs/heads/<rama>
git rev-parse HEAD        # anota este SHA de 40 caracteres
```

## Paso 6. Comprobar que llegó al inbox

```bash
bash scripts/kind/github-webhook-check.sh <SHA-de-40-caracteres>
```

Salida esperada (y `rc=0`):

```
sha esperado: <SHA> (rama main)
OK webhook-github-recibido
  notificación=<id> evento=commit repo=ogaston/agentic-qa-swarm estado=pending sha=<SHA>
```

Sin el argumento, el script toma el HEAD remoto de `WEBHOOK_BRANCH` (por defecto `main`). Espera hasta
`WEBHOOK_TIMEOUT_S` (por defecto 600 s).

## Si falla

| Síntoma en GitHub (Recent Deliveries) | Causa probable | Qué hacer |
|---|---|---|
| `401` | el secreto del webhook no coincide con el Secret del clúster | repite el paso 3 y pega de nuevo |
| `4xx` distinto de 401, o sin conexión | la Payload URL no apunta al túnel o al port-forward | revisa la URL del paso 2 y que el port-forward del paso 1 siga abierto |
| `202` pero el script sale con `FALLA` | la notificación no llegó al inbox (transporte NATS) | `kubectl -n aqs-system logs deploy/go-intake` y `deploy/ui-api` (sin valores) |

El script no ve la entrega de GitHub: solo comprueba el inbox. Un `FALLA` no distingue por sí solo entre
un secreto mal copiado y un túnel caído; la pestaña **Recent Deliveries** lo distingue.

## Qué no hace este paso

- No fusiona ni modifica ramas de trabajo. El push es solo para la prueba.
- No confirma la notificación (no crea corrida). La confirmación humana es el paso 3 del guion de demo
  (`docs/operaciones/kind-local.md`).
- No expone el clúster: el túnel apunta solo a un port-forward de `go-intake` en loopback.
