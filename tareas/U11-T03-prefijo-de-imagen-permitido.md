# U11-T03 — El deploy solo acepta imágenes de los repositorios permitidos

**Unidad:** U11 — Demo con repositorio y modelo reales
**Historias que implementa:** US-M8.3 (cero blancos fuera del alcance), US-M2.
**Depende de:** U11-T01 fusionada. Independiente de U11-T02 (puede ir en paralelo; toca otros archivos). **Tope: 3 rondas**.

---

## Origen

Hallazgo del revisor de U11-T01 (ronda 1) y de la revisión de seguridad automática del push: desde U11-T01 el controlador despliega
el artefacto que trae `run.confirmed` (`services/go-run-controller/cmd/go-run-controller/real.go`, `RealPhases.Artifact`), y
`go-warm-manager` (`ValidateArtifact`, `types.go`) solo comprueba el **registro** (`ghcr.io`), no el repositorio. Quien pudiera
publicar en el subject `run.confirmed` (la etiqueta `aqs.io/nats-client=true` es autoservicio, C-111) podría desplegar
`ghcr.io/<cualquiera>/<imagen>:tag` en el warm. Antes de U11-T01 la imagen era una variable de entorno fija, así que la superficie ha crecido.

## Alcance

**Dentro** (una línea, concreta):

> `go-warm-manager` solo acepta imágenes publicadas cuyo repositorio empiece por un prefijo de `WARM_ALLOWED_IMAGE_PREFIXES` (fail-closed si está vacío con artefactos de evento), el controlador valida lo mismo antes de pedir el deploy, y el uso del respaldo `RUN_ARTIFACT_REF` queda registrado.

Detalle:

1. **`go-warm-manager`:** nueva variable `WARM_ALLOWED_IMAGE_PREFIXES` (lista separada por comas; cada prefijo `registro/owner/repo`, comparación exacta del prefijo hasta el `:` o `@`, minúsculas; **no** vale `ghcr.io/ogaston` ni `ghcr.io` a secas). `ValidateArtifact` rechaza con el error nuevo `ErrImageNotAllowed` (HTTP 422 `image_not_allowed`) una `published-image` fuera de la lista. Vacía o ausente → todo `published-image` rechazado (fail-closed); un valor mal formado impide arrancar. Conserva la regla del registro permitido y la de tag explícito/sin `latest`.
2. **Manifiesto:** `deploy/flux/base/control-plane.yaml` cablea `WARM_ALLOWED_IMAGE_PREFIXES` con `ghcr.io/ogaston/demo-target-app-qa-swarm` y el valor de `RUN_ARTIFACT_REF` actual (`ghcr.io/ogaston/agentic-qa-swarm/target-app`, respaldo y `go-reset`/pruebas), de modo que el respaldo no se rechace.
3. **Controlador:** antes de pedir el deploy, `RealPhases.Artifact` valida con la misma regla el artefacto del evento (misma función compartida o copia con prueba de paridad, como `ValidateSurface`); si no pasa, la fase `deploy` falla cerrada con un mensaje que nombra la imagen rechazada, sin llamar a `go-warm-manager`.
4. **Respaldo visible:** cuando el controlador usa `RUN_ARTIFACT_REF` porque el evento no trae artefacto, registra un WARN estructurado una vez por corrida (`artifact_source=fallback`) y expone un contador `aqs_run_artifact_fallback_total` (Prometheus, mismo registro de métricas que usa el servicio).

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- NKeys/TLS de NATS y permisos de publicación por subject (C-111): endurecimiento propio, no se toca NATS ni sus NetworkPolicy.
- Cambiar el contrato `run.confirmed`, `go-intake`, `ui-api`, los scripts de kind y la documentación de la demo (U11-T02).
- Firmas de imágenes (cosign) o verificación de digest; pull secrets; `build-from-repo`.
- Reglas Rego nuevas.

---

## Archivos de contexto

- `services/go-warm-manager/types.go` (`ValidateArtifact`, `DefaultAllowedRegistries`), `service.go`, `cmd/go-warm-manager/main.go`, `internal/api/api.go`, `README.md`
- `services/go-run-controller/cmd/go-run-controller/real.go`, `adapters/phases.go`, `README.md`
- `deploy/flux/base/control-plane.yaml`
- `revisiones/U11-T01/ronda-1.md` (candidatas), `tareas/U11-T01-artefacto-del-commit.md`

---

## Criterios de aceptación

- [ ] **CA-1** — `go-warm-manager` rechaza imágenes fuera de los prefijos, incluidos los trucos de nombre, y falla cerrado.
  ```bash
  cd services/go-warm-manager && GOTOOLCHAIN=auto go test ./... -count=1 -run 'Prefix|ImageNotAllowed|ValidateArtifact' -v 2>&1 | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `ok`, con pruebas parametrizadas: aceptada `ghcr.io/ogaston/demo-target-app-qa-swarm:<sha>` y `…@sha256:<64hex>`; rechazadas `ghcr.io/evil/x:1`, `ghcr.io/ogaston/demo-target-app-qa-swarm-evil:1` (prefijo sin límite), `ghcr.io/ogaston/demo-target-app-qa-swarm/../x:1`, `ghcr.io/OGASTON/…` (mayúsculas), `docker.io/…`, tag `latest`, sin tag; lista vacía → todo rechazado; prefijo `ghcr.io` o `ghcr.io/ogaston` en la configuración → el servicio no arranca; el API responde `422 image_not_allowed` y no parchea nada (lectura de vuelta con el cliente de Kubernetes falso: ninguna acción de patch).

- [ ] **CA-2** — El controlador no pide el deploy de una imagen no permitida y registra el respaldo.
  ```bash
  cd services/go-run-controller && GOTOOLCHAIN=auto go test ./... -count=1 -run 'Artifact|Fallback|ImageNotAllowed' -v 2>&1 | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `ok`, con pruebas de: evento con `ghcr.io/evil/x:1` → la fase `deploy` falla cerrada y el servidor falso de `go-warm-manager` **no recibe** ninguna petición; evento permitido → recibe el deploy con esa imagen; evento sin artefacto → usa `RUN_ARTIFACT_REF`, emite un único WARN `artifact_source=fallback` por corrida y el contador `aqs_run_artifact_fallback_total` sube en 1.

- [ ] **CA-3** — Paridad entre la regla del controlador y la de `go-warm-manager`.
  ```bash
  (cd services/go-warm-manager && GOTOOLCHAIN=auto go test ./... -count=1 -run 'Paridad|Parity' -v 2>&1 | grep -E '^(--- |ok|FAIL)'); (cd services/go-run-controller && GOTOOLCHAIN=auto go test ./... -count=1 -run 'Paridad|Parity' -v 2>&1 | grep -E '^(--- |ok|FAIL)')
  ```
  Esperado: `ok` en ambos, con un conjunto común de casos (archivo de casos idéntico en los dos módulos o prueba que compara las dos copias) que decide igual en cada caso.

- [ ] **CA-4** — El manifiesto cablea la variable, es válido y las políticas siguen en verde.
  ```bash
  grep -n -A1 'WARM_ALLOWED_IMAGE_PREFIXES' deploy/flux/base/control-plane.yaml; python3 -c "import yaml;list(yaml.safe_load_all(open('deploy/flux/base/control-plane.yaml')));print('YAML OK')"; bash scripts/ci/policies.sh 2>&1 | tail -3; echo "policies rc=${PIPESTATUS[0]}"
  ```
  Esperado: aparece la variable con `ghcr.io/ogaston/demo-target-app-qa-swarm` y el prefijo del respaldo, `YAML OK` y `policies rc=0` (`policies.sh` corre en docker si faltan binarios).

- [ ] **CA-5** — Módulos y alcance.
  ```bash
  for d in services/go-warm-manager services/go-run-controller; do (cd $d && GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -count=1 2>&1 | grep -v 'no test files' | grep -c '^FAIL'); done; git status --short | wc -l; b=$(git merge-base HEAD origin/tarea/U11-T01); git diff --name-only $b | grep -v -E '^(services/(go-warm-manager|go-run-controller)/|deploy/flux/base/control-plane\.yaml|tareas/U11-T0[23]-|unidades-y-tareas\.md|bitacoras/U11-T03\.md|revisiones/U11-T03/)' | wc -l
  ```
  Esperado: `0` y `0` por cada módulo (cero líneas `FAIL`; `go vet` sin salida), luego `0` y `0` (árbol limpio y alcance). Lo que cuenta es la ausencia de `FAIL`, no un `ok` final (hay paquetes sin pruebas).

---

## Plan de pruebas

- **Rojo primero:** una prueba que pida desplegar `ghcr.io/evil/x:1` y muestre que hoy `go-warm-manager` lo acepta y parchea; y otra con el controlador (el servidor falso recibe la petición). Pegar las salidas.
- Negativas: prefijo con comodín o demasiado corto en la configuración, mayúsculas, `..`, prefijo sin límite de nombre (`…-evil`), lista vacía, `docker.io`, `latest`.
- PBT (si el módulo ya los usa): ningún `published-image` aceptado cuyo repositorio no empiece por un prefijo permitido + `/`, `:` o `@`; semilla registrada.
- Par falla-pasa de sensibilidad: quitar la validación del controlador deja pasar la imagen no permitida a `go-warm-manager`, y la prueba de CA-2 lo detecta.

---

## Notas

- Archivos que se **modifican en su sitio**; nada de duplicados con sufijo. Rama `tarea/U11-T03`, worktree `../agentic-qa-swarm-wt-U11-T03`, desde `tarea/U11-T01`.
- Esta tarea no sustituye a C-111 (NKeys): reduce el daño de un publicador no autorizado, no lo impide.
- Despliegue: `go-warm-manager` con la variable antes (o a la vez) que el controlador nuevo; sin ella rechazaría todo.
- Candidatas que surjan van a `tareas/candidatas.md`.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
