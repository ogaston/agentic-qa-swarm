# App objetivo: qué debe cumplir una app para sustituir a `demo/target-app`

La app de referencia (`demo/target-app`, U8-T04) es lo que el warm despliega por defecto en kind. La app
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
archivo montado, nunca en argumentos ni en logs). La referencia es `demo/target-app/baseline/baseline.sh`
con el listener `:9090` de la app.

## 3. Imagen con tag fijado
- Imagen construida desde un Dockerfile del repositorio o publicada, con tag explícito (nunca `latest`).
- Puerto HTTP: el nombrado `http` del `warm-app` (hoy `80`). Ejecución sin root si es posible.
- Las variables de conexión a la base del warm (`warm-db`) vienen del Secret `warm-db-credentials`.

## 4. Cómo apuntar `RUN_ARTIFACT_REF` a la app
- En `deploy/flux/base/control-plane.yaml`, `go-run-controller` lee `RUN_ARTIFACT_REF`
  (hoy `ghcr.io/ogaston/agentic-qa-swarm/target-app:0.0.0`). Cambiar ese valor a la imagen del cliente
  con tag fijado.
- El registro debe estar en la lista permitida de `go-warm-manager` (`DefaultAllowedRegistries`: `ghcr.io`).
- El `baseline.sh` del cliente sustituye al de `demo/target-app/baseline/` en `scripts/kind/secrets.sh`.
  Si ya existe un ConfigMap `go-reset-baseline` sin la etiqueta `aqs.io/kind-stub`, el script no lo toca.
