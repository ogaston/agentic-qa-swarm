# Subir y bajar el entorno de la demo

Guía única para apagar, encender y comprobar todo el entorno local de la demo (clúster kind `aqs`, dashboard,
túnel y webhook de GitHub). Todo se corre desde la raíz del repositorio con el contexto de kubectl `kind-aqs`.

## Resumen de comandos

| Quiero… | Comando | Tarda |
|---|---|---|
| Saber si todo está bien | `bash scripts/demo/status.sh` | segundos |
| Encender todo (o repararlo a fondo) | `bash scripts/demo/up.sh` | 5 a 10 min |
| Dejar el warm limpio y verificado | `bash scripts/demo/up.sh --reset` | 1 a 2 min |
| Comprobar que la plataforma responde | `bash scripts/kind/kind-smoke.sh --preflight` | 1 a 2 min |
| Reparar el dashboard o el túnel sin reconstruir | `bash scripts/demo/reconnect.sh` | 30 s |
| Apagar todo | `bash scripts/demo/down.sh` | segundos |

`status.sh` solo lee. Cada línea `FALLA` trae debajo el comando que la arregla.

## Qué piezas hay

| Pieza | Dónde | La arranca | Se rompe cuando… |
|---|---|---|---|
| Nodo kind `aqs-control-plane` | contenedor de podman | `up.sh` | corres `down.sh` o reinicias la máquina |
| Plataforma (`aqs-system`) | clúster, reconciliada por Flux desde `main` | Flux y `up.sh` | un PR fusionado y sin `git pull` local deja imágenes viejas |
| Warm (`aqs-test`) | clúster | Flux, despertado por el preflight o por una corrida | pasan 30 min sin corridas (`idleScaleDownAfter: 30m`): queda en 0 réplicas |
| Dashboard | `vite preview` en `127.0.0.1:18610` y port-forward a identity (18600), ui-api (18601) y run-controller (18603) | `reconnect.sh` (lo llama `up.sh`) | se recrean los pods: el login da **500 sin cuerpo** |
| go-intake local | port-forward en `127.0.0.1:18622` | `reconnect.sh` (lo llama `up.sh`) | se recrean los pods: el webhook da **502** |
| Conector cloudflared | servicio de systemd `cloudflared` (con token) | systemd, al encender la máquina | casi nunca; si cae: `sudo systemctl start cloudflared` |
| URL pública | `https://aqs.ogaston.com` → `127.0.0.1:18622` | Cloudflare | falta el port-forward de go-intake (502) |
| Webhook de GitHub | `ogaston/demo-target-app-qa-swarm`, solo `push` | `up.sh` (crea o actualiza el hook) | el secreto en GitHub no es el que go-intake espera (**401**) |

Un `kubectl port-forward` queda pegado al pod que eligió al arrancar. Si ese pod se recrea, el proceso sigue
escuchando pero ya no responde. Por eso, después de `refresh-images.sh`, de un cambio de modelo o de borrar un
pod a mano, hay que correr `reconnect.sh`. `up.sh` ya lo hace al final.

## Encender

1. Trae `main` al día. `up.sh` construye las imágenes desde tu copia local, así que una copia atrasada corre
   código viejo aunque Flux muestre la revisión nueva:

   ```bash
   git switch main && git pull --ff-only
   ```

2. Enciende todo:

   ```bash
   bash scripts/demo/up.sh
   ```

   Hace esto, en orden: enciende el nodo si estaba parado y espera a que el kubelet recree todos los pods de `aqs-system` (unos 3 minutos tras un `down.sh`); despliega lo que falte (`demo-preflight.sh --deploy`);
   reconstruye, carga y recrea las imágenes de la plataforma (`refresh-images.sh`); rehace el port-forward de
   go-intake y el dashboard y comprueba el túnel (`reconnect.sh`); y reconcilia el webhook de GitHub. Al final
   imprime `Dashboard: http://127.0.0.1:18610` y el archivo con las credenciales de `demo`.

   El secreto del webhook depende de si el repo está registrado en el asistente de proyectos del dashboard
   (Repositorios). Si está registrado, go-intake verifica con el secreto derivado de su versión
   (`webhook_secret_version`); si no, con el de plataforma. `up.sh` lee el registro y pone el que corresponde. Si
   registras, borras o rotas el repo en el dashboard, vuelve a correr `up.sh`.

3. Deja el warm limpio y comprueba la plataforma:

   ```bash
   bash scripts/demo/up.sh --reset                 # «reset: OK (state=ready, reset_verified=true)»
   bash scripts/kind/kind-smoke.sh --preflight     # «smoke: OK (sin FALLA; …)»
   bash scripts/demo/status.sh                     # «status: OK (listo para grabar)»
   ```

   Si quieres que la grabación muestre Flux en `main`: `bash scripts/kind/flux-up.sh` y
   `bash scripts/kind/flux-status.sh` (pasos del [guion](../entrega/guion-grabacion.md)).

4. Graba o haz la demo **antes de que pasen 30 minutos sin corridas**. Si el warm se apagó, `status.sh` lo
   marca; repite `bash scripts/demo/up.sh --reset`.

## Apagar

```bash
bash scripts/demo/down.sh
```

Para el dashboard y sus port-forward, el port-forward de go-intake y el nodo kind. Conserva los datos del
clúster, los Secrets y el webhook de GitHub. El servicio `cloudflared` sigue vivo, pero mientras el clúster está
apagado la URL pública responde 502. Un push hecho en ese rato no llega: al encender, haz otro push o pulsa
**Redeliver** en GitHub (Settings → Webhooks → Recent Deliveries).

## Si algo falla

| Síntoma | Causa | Arreglo |
|---|---|---|
| Login del dashboard: `unexpected_response HTTP 500 sin cuerpo` | port-forward del dashboard pegado a un pod que ya no existe | `bash scripts/demo/reconnect.sh` |
| Entrega del webhook en GitHub con **502** | falta el port-forward de go-intake en 18622 | `bash scripts/demo/reconnect.sh` y luego Redeliver |
| Entrega del webhook con **401** (`invalid_signature` en el log de go-intake) | el secreto en GitHub no es el que go-intake espera (repo registrado, borrado o rotado en el asistente) | `bash scripts/demo/up.sh` y luego Redeliver |
| `warm-app` en 0 réplicas | 30 min sin corridas | `bash scripts/demo/up.sh --reset` |
| `up.sh` termina en `refresh-images: el pod … arrancó antes del reinicio` | un pod viejo sobrevivió al recreado | `bash scripts/kind/refresh-images.sh` y luego `bash scripts/demo/reconnect.sh` |
| El reset devuelve 409 con `cuarentena` | warm en `ImagePullBackOff` | `kubectl -n aqs-test delete deployment warm-app` (Flux lo recrea) y repite el reset |
| `reconnect.sh`: `127.0.0.1:18622 sigue ocupado por un proceso que esta demo no arrancó` | un port-forward abierto a mano | `ss -ltnp \| grep 18622`, mata ese pid y repite |
| `status.sh`: `conector cloudflared apagado` | el servicio de systemd cayó | `sudo systemctl start cloudflared` |
| `up.sh`: `aqs-system pods did not restart and become ready within 5 min` | algún pod no arranca tras encender el nodo | `kubectl -n aqs-system get pods` y `describe` del que falle; luego repite `up.sh` |
| Los pods corren código viejo | `main` local atrasado al correr `up.sh` | `git pull --ff-only` y `bash scripts/demo/up.sh` |

Para ver la causa exacta de un webhook rechazado:

```bash
kubectl -n aqs-system logs deploy/go-intake --since=10m | grep 'webhook rechazado'
```
