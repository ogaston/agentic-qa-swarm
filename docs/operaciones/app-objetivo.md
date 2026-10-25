# App objetivo: qué debe cumplir una app para sustituir a la app de referencia

La app de referencia (repo `ogaston/demo-target-app-qa-swarm`, U8-T04; ya no vive en este repo, U11-T02) es lo que el warm despliega en kind. La app
definitiva del cliente la aporta el humano (U8-T09) y debe cumplir este contrato.

## 1. OpenAPI en una ruta conocida
- `GET /openapi.json` devuelve una especificación OpenAPI 3.x válida.
- `go-warm-manager` la busca en `/openapi.json`, `/v3/api-docs` y `/swagger.json`, en ese orden. Si no hay
  ninguna, sondea `/`, `/health`, `/healthz`, `/api`, `/status` y reporta solo los GET que responden.
- La superficie inferida (`POST /surface`) debe contener los endpoints de negocio que se quieran probar.

## 2. Contrato de baseline (`go-reset`)
`go-reset` ejecuta `RESET_BASELINE_SCRIPT` (ConfigMap `go-reset-baseline`, clave `baseline.sh`) con:
- `clean`: deja la base en el estado fijo de referencia. Rc 0 si sale bien.
- `verify`: imprime **solo** un entero: filas que difieren del baseline. `0` = limpia.
- `version`: imprime la versión del baseline (`RESET_BASELINE_VERSION` la fija si se configura).

El script corre dentro de la imagen de `go-reset` (alpine, busybox): solo `sh` y `wget`. Para hablar con
la app debe usar un canal de administración **solo dentro del clúster** y **autenticado** (token en
archivo montado, nunca en argumentos ni en logs). La referencia es `deploy/kind/target-app/baseline.sh`
(copia byte a byte de `baseline/baseline.sh` del repo de referencia en su SHA; ver `deploy/kind/target-app/PROCEDENCIA.md`)
con el listener `:9090` de la app.

## 3. Imagen con tag fijado
- Imagen construida desde un Dockerfile del repositorio o publicada, con tag explícito (nunca `latest`).
  La de referencia es `ghcr.io/ogaston/demo-target-app-qa-swarm:<sha40>`: el tag es el SHA del commit,
  publicado por la CI del repo objetivo en cada push a `main`. En kind el overlay `deploy/flux/kind` la fija.
- Puerto HTTP: el nombrado `http` del `warm-app` (hoy `80`). Ejecución sin root si es posible.
- Las variables de conexión a la base del warm (`warm-db`) vienen del Secret `warm-db-credentials`.

## 4. De dónde sale la imagen de la app
- En la demo la imagen **viene del evento**: el push a `main` del repo objetivo produce la notificación, y
  la imagen de ese commit (`ghcr.io/ogaston/demo-target-app-qa-swarm:<sha>`) es la que se despliega. Ya no se
  edita `RUN_ARTIFACT_REF` para cambiar de app ni de commit en la demo.
- Para una app del cliente, la imagen debe ser pública o accesible con pull anónimo, con tag fijado (ver §3).
  El valor `RUN_ARTIFACT_REF` de `deploy/flux/base/control-plane.yaml` (hoy `…/agentic-qa-swarm/target-app:0.0.0`) es
  un resto de U8 y se trata en la tarea candidata correspondiente; no se cambia en U11-T02.
- El registro debe estar en la lista permitida de `go-warm-manager` (`DefaultAllowedRegistries`: `ghcr.io`).
- El `baseline.sh` del cliente sustituye al vendorizado en `deploy/kind/target-app/` (vía `TARGET_APP_BASELINE_SCRIPT` en `scripts/kind/secrets.sh`).
  Si ya existe un ConfigMap `go-reset-baseline` sin la etiqueta `aqs.io/kind-stub`, el script no lo toca.
