# U11-T02 — La plataforma deja de llevar la app: kind y pruebas contra el repositorio objetivo externo

**Unidad:** U11 — Demo con repositorio y modelo reales
**Historias que implementa:** US-M1, US-M2 (con el repositorio real), US-M10 (kind reproducible).
**Depende de:** U11-T01 **fusionada** (PR #134; esta rama parte de `tarea/U11-T01`). **Tope: 3 rondas**.

---

## Origen

La app objetivo vive ahora en `https://github.com/ogaston/demo-target-app-qa-swarm` (público; su CI publica
`ghcr.io/ogaston/demo-target-app-qa-swarm:<sha de 40 hex>`; la imagen de `b97a719199d5ccbc2ff24f9cfc408ec7ee31114d`
ya se descarga de forma anónima). **Decisión del humano (2026-10-25): se borra `demo/target-app` de este repo y la plataforma depende
del repositorio externo.** U11-T01 dejó rotos `kind-e2e.sh`, `kind-smoke.sh` y `dashboard-smoke.sh` (webhook de
`ogaston/agentic-qa-swarm` → 422 `repo_not_allowed`), y varias piezas leen `demo/target-app`:

- `scripts/kind/secrets.sh:103` toma el `baseline.sh` de `demo/target-app/baseline/`.
- `deploy/flux/kind/kustomization.yaml:337-341` arranca el warm con la imagen local `…/target-app:0.0.0` (construida desde `demo/target-app` por `build-images.sh`).
- `services/go-warm-manager` (`TestOpenAPIRequestBodyDeTargetApp`) lee `demo/target-app/openapi.json`.
- `services/aqs-runner/cmd/aqs-runner/main_test.go:562-565` compila el binario real desde `demo/target-app`.
- `scripts/test/u8-target-app-local.sh` y `u8-target-app-kind.sh` prueban esa app; `scripts/kind/github-webhook-check.sh` usa `git ls-remote origin` (el repo de la plataforma).

## Alcance

**Dentro** (una línea, concreta):

> Borrar `demo/target-app`, sustituir cada dependencia de él por el repositorio externo (baseline vendorizado con procedencia, imagen del warm de kind fijada por SHA, fixture de OpenAPI, prueba del runner con `TARGET_APP_DIR`), adaptar los scripts de kind al repo permitido con un SHA real, y documentar la demo con el repositorio real.

Detalle:

1. **Borrar** `demo/target-app/` completo (el humano ya lo tiene borrado sin confirmar en su árbol: el borrado aquí debe ser idéntico y no chocar). Borrar `scripts/test/u8-target-app-local.sh` (necesitaba el código fuente). Adaptar `scripts/test/u8-target-app-kind.sh` para que despliegue la imagen del repo externo (`${TARGET_IMAGE}`, ver 4).
2. **Baseline vendorizado:** `deploy/kind/target-app/baseline.sh` = copia **byte a byte** de `baseline/baseline.sh` del repo externo en el SHA `b97a719199d5ccbc2ff24f9cfc408ec7ee31114d`, más `deploy/kind/target-app/PROCEDENCIA.md` (repo, SHA, comando para verificar la copia). `scripts/kind/secrets.sh` lee `${TARGET_APP_BASELINE_SCRIPT:-deploy/kind/target-app/baseline.sh}`.
3. **`openapi.json` como fixture:** copia byte a byte en `services/go-warm-manager/testdata/target-app-openapi.json`; la prueba la lee de ahí.
4. **Imagen del warm en kind:** el overlay `deploy/flux/kind` arranca `warm-app` con `ghcr.io/ogaston/demo-target-app-qa-swarm:b97a719199d5ccbc2ff24f9cfc408ec7ee31114d` (sustituye el `images:` nginx → `…/target-app:0.0.0`). `build-images.sh`/`lib.sh` ya derivan las imágenes de demo del disco: sin `demo/*` no construyen nada de demo; actualizar el texto de `docs/operaciones/kind-local.md` («las 10 imágenes»).
5. **Prueba del runner:** el test de `aqs-runner` (CA-4 de U8-T09a) compila el binario desde `$TARGET_APP_DIR` (clon del repo externo) y hace `t.Skip` con un mensaje claro si no está definida. No se pierde el resto de pruebas del paquete.
6. **Scripts de kind al repo permitido:** `kind-e2e.sh`, `kind-smoke.sh` y `dashboard-smoke.sh` firman el webhook con `repository.full_name = ogaston/demo-target-app-qa-swarm`. `kind-e2e.sh` usa como SHA el `TARGET_SHA` si está definido; si no, `git ls-remote https://github.com/ogaston/demo-target-app-qa-swarm HEAD` (un commit real cuya imagen existe en GHCR); si no hay red o el SHA no tiene imagen, falla con un mensaje claro **antes** de confirmar. `github-webhook-check.sh` toma el HEAD remoto del repo objetivo (no de `origin`) y acepta `TARGET_REPO` para cambiarlo.
7. **Documentación:** `docs/operaciones/webhook-github-cloudflared.md` (repo nuevo, evento solo push, `Content-Type: application/json`, túnel con nombre recomendado), `docs/operaciones/app-objetivo.md` §3-§4 (la imagen viene del evento; ya no se edita `RUN_ARTIFACT_REF`), `docs/operaciones/kind-local.md`, y **`docs/demo-real.md`**: guion de la demo (precondiciones, comandos, qué se ve en cada paso, cómo se obtiene el defecto: un commit del repo objetivo que fije `ENV TARGET_APP_BUG=oversell` en su Dockerfile; un commit sano para el caso sin hallazgos; **solo push a `main`, los PR no tienen imagen**; qué hacer si la imagen aún no existe: esperar a la CI; tiempos medidos).

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Cualquier cambio de Go en `go-intake`, `ui-api`, `go-run-controller`, `go-warm-manager` (salvo la ruta del fixture en la prueba del punto 3), contratos y manifiestos de `deploy/flux/base`.
- Prefijo de repositorio de imagen permitido en `go-warm-manager`, métrica del respaldo, NKeys/TLS de NATS: U11-T03 y candidatas.
- El repositorio objetivo (su CI, su código) y el webhook real de GitHub: los hace el humano (`docs/demo-real.md`).
- Reintento de `kind-up.sh` (C-125), `build-from-repo` real, pull secrets.
- Cambiar RBAC, NetworkPolicy, agentes, dashboard.

---

## Archivos de contexto

- `tareas/U11-T01-artefacto-del-commit.md`, `revisiones/U11-T01/ronda-2.md` (F-06)
- `scripts/kind/secrets.sh`, `kind-e2e.sh`, `kind-smoke.sh`, `dashboard-smoke.sh`, `github-webhook-check.sh`, `build-images.sh`, `lib.sh`
- `deploy/flux/kind/kustomization.yaml`, `deploy/flux/base/warm.yaml`, `deploy/kind/README.md`
- `services/go-warm-manager/openapi_body_test.go`, `services/aqs-runner/cmd/aqs-runner/main_test.go`
- `scripts/test/u8-target-app-kind.sh`, `scripts/test/u8-target-app-local.sh`
- `docs/operaciones/webhook-github-cloudflared.md`, `app-objetivo.md`, `kind-local.md`
- Repo externo (solo lectura, SHA fijado): https://github.com/ogaston/demo-target-app-qa-swarm/tree/b97a719199d5ccbc2ff24f9cfc408ec7ee31114d ; clon local del humano: `../demo/target-app` (relativo al worktree)

---

## Criterios de aceptación

- [ ] **CA-1** — La app ya no vive en la plataforma y nada la referencia.
  ```bash
  test ! -d demo/target-app && echo SIN_APP; grep -rIn 'demo/target-app' scripts services deploy .github --include='*' | grep -v '^Binary' | wc -l
  ```
  Esperado: `SIN_APP` y `0` (los comentarios que la nombran se actualizan).

- [ ] **CA-2** — El baseline vendorizado es idéntico al del repo externo en el SHA fijado y `secrets.sh` lo usa.
  ```bash
  diff <(curl -fsS https://raw.githubusercontent.com/ogaston/demo-target-app-qa-swarm/b97a719199d5ccbc2ff24f9cfc408ec7ee31114d/baseline/baseline.sh) deploy/kind/target-app/baseline.sh && echo BASELINE_IGUAL; grep -n 'TARGET_APP_BASELINE_SCRIPT\|deploy/kind/target-app/baseline.sh' scripts/kind/secrets.sh | wc -l; test -s deploy/kind/target-app/PROCEDENCIA.md && echo PROCEDENCIA
  ```
  Esperado: `BASELINE_IGUAL`, un número ≥ 1 y `PROCEDENCIA`.

- [ ] **CA-3** — Las pruebas de Go no dependen del código de la app: pasan con el fixture y el runner se salta (o pasa con `TARGET_APP_DIR`).
  ```bash
  (cd services/go-warm-manager && GOTOOLCHAIN=auto go test ./... -count=1 2>&1 | tail -3); cmp <(curl -fsS https://raw.githubusercontent.com/ogaston/demo-target-app-qa-swarm/b97a719199d5ccbc2ff24f9cfc408ec7ee31114d/openapi.json) services/go-warm-manager/testdata/target-app-openapi.json && echo OPENAPI_IGUAL
  (cd services/aqs-runner && GOTOOLCHAIN=auto go test ./... -count=1 -v -run 'Orders' 2>&1 | grep -E '^(--- |ok|FAIL)'); (cd services/aqs-runner && AQS_PLANNER_PYTHON=${AQS_PLANNER_PYTHON:?un python con jsonschema (deps del planner)} TARGET_APP_DIR=$(cd ../../../demo/target-app && pwd) GOTOOLCHAIN=auto go test ./... -count=1 -v -run 'Orders.*(Cuerpo|Body)' 2>&1 | grep -E '^(--- |ok|FAIL)')
  ```
  Esperado: `ok` en `go-warm-manager` y `OPENAPI_IGUAL`; en `aqs-runner` sin la variable, la prueba del binario real sale `--- SKIP` y el resto `PASS`; con `TARGET_APP_DIR` apuntando al clon del repo externo **y `AQS_PLANNER_PYTHON` con un Python que tenga `jsonschema`** (deps del planner; sin él la prueba sigue saltándose por el planner), `--- PASS` (par falla-pasa: con la variable apuntando a un directorio vacío falla con el mensaje de compilación, no se salta en silencio).

- [ ] **CA-4** — El overlay de kind arranca el warm con la imagen del repo externo fijada por SHA y sigue válido.
  ```bash
  kubectl kustomize deploy/flux/kind | grep -n 'demo-target-app-qa-swarm'; bash scripts/ci/check-no-latest.sh deploy/flux/kind; echo "no-latest rc=$?"; bash scripts/ci/policies.sh 2>&1 | tail -3; echo "policies rc=${PIPESTATUS[0]}"
  ```
  Esperado: una línea `image: ghcr.io/ogaston/demo-target-app-qa-swarm:b97a719199d5ccbc2ff24f9cfc408ec7ee31114d` en el `warm-app`, `no-latest rc=0` y `policies rc=0` (si `conftest`/`kubeconform` no están en el host, `policies.sh` corre en docker; el codificador lo anota).

- [ ] **CA-5** — Los scripts de kind usan el repo permitido y un SHA real, y fallan pronto si no hay imagen.
  ```bash
  for f in kind-e2e kind-smoke dashboard-smoke github-webhook-check build-images secrets; do bash -n scripts/kind/$f.sh && echo "SINTAXIS_OK $f"; done; grep -rn 'ogaston/agentic-qa-swarm"' scripts/kind | wc -l; grep -ln 'TARGET_REPO_FULL' scripts/kind/kind-e2e.sh scripts/kind/kind-smoke.sh scripts/kind/dashboard-smoke.sh scripts/kind/github-webhook-check.sh | wc -l
  ```
  Esperado: seis `SINTAXIS_OK`, `0` y `4` (los cuatro scripts toman el repo objetivo de `TARGET_REPO_FULL` de `lib.sh`, ronda 2 / F-03; el literal solo vive allí). Más una prueba **sin clúster** (la escribe el codificador, p. ej. `scripts/test/u11-scripts-test.sh`, con `kubectl`/`curl` simulados en un directorio del `PATH`) que demuestre: (a) `kind-e2e.sh` con `TARGET_SHA` mal formado sale distinto de 0 antes de tocar el clúster; (b) sin red (`git ls-remote` falla) sale distinto de 0 con mensaje claro; (c) el cuerpo del webhook que firma lleva `ogaston/demo-target-app-qa-swarm` y el SHA elegido. Se ejecuta con `bash scripts/test/u11-scripts-test.sh` y termina con `OK`.

- [ ] **CA-6** — Lectura de vuelta en un clúster real (`kind-aqs`): humo y ciclo completo con el repositorio real. Requiere red a GHCR y al proveedor LLM del humano. **Estado tras la ronda 1:** el codificador lo corrió de verdad y encontró dos defectos que no son de esta tarea: el control positivo `llm-egress-acotado` falla (U11-T05) y el runner rechaza el nombre del Job por un `flow_id` inventado por el modelo (U11-T04). La parte propia de esta tarea ya quedó demostrada en esa corrida: `deploy: OK`, el `warm-app` arranca con `ghcr.io/ogaston/demo-target-app-qa-swarm:b97a719…`, y el e2e llega con el webhook del repo permitido hasta la fase `runners` (login, webhook 202, inbox, confirm, deploy, superficie, ensayo). **Por decisión del orquestador, el revisor de esta tarea evalúa CA-1..CA-5, CA-7 y CA-8 y comprueba que la evidencia de CA-6 de la bitácora es real y que los dos defectos están diagnosticados; la verificación completa de CA-6 (humo y e2e en verde, y la segunda lectura con un SHA distinto del fijado) se hace al cerrar U11-T04 y U11-T05, con el comando de abajo.**
  ```bash
  bash scripts/kind/kind-up.sh && bash scripts/kind/build-images.sh && bash scripts/kind/load-images.sh && bash scripts/kind/secrets.sh && bash scripts/kind/deploy.sh
  bash scripts/kind/kind-smoke.sh 2>&1 | grep -c '^FALLA'
  kubectl --context kind-aqs -n aqs-test get deploy warm-app -o jsonpath='{..image}'; echo
  TARGET_SHA=<sha de un commit del repo objetivo distinto de b97a719, con imagen publicada> bash scripts/kind/kind-e2e.sh --bug off 2>&1 | grep -c '^FALLA'; echo "rc=${PIPESTATUS[0]}"
  kubectl --context kind-aqs -n aqs-test get deploy warm-app -o jsonpath='{..image}'; echo
  ```
  Esperado: `0`; la primera lectura muestra `…:b97a719…`; el e2e da `0` y `rc=0`; la segunda lectura muestra la imagen del **`TARGET_SHA` usado**, distinta de la fijada (esa lectura discrimina; con el mismo SHA no prueba nada). Necesita un segundo commit con imagen en el repo objetivo: ver `docs/demo-real.md`.

- [ ] **CA-7** — Documentación presente y consistente.
  ```bash
  test -s docs/demo-real.md && echo DEMO_REAL; grep -c 'ENV TARGET_APP_BUG=oversell' docs/demo-real.md; grep -c 'demo-target-app-qa-swarm' docs/operaciones/webhook-github-cloudflared.md docs/operaciones/app-objetivo.md docs/demo-real.md; grep -n 'agentic-qa-swarm/settings/hooks' docs/operaciones/webhook-github-cloudflared.md | wc -l
  ```
  Esperado: `DEMO_REAL`, `≥1`, tres cuentas `≥1` y `0` (el enlace al webhook del repo de la plataforma desaparece).

- [ ] **CA-8** — Alcance.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/tarea/U11-T01); git diff --name-only $b | grep -v -E '^(demo/target-app/|scripts/(kind|test)/|deploy/(kind|flux/kind)/|services/go-warm-manager/(testdata/|openapi_body_test\.go)|services/aqs-runner/cmd/aqs-runner/main_test\.go|docs/|tareas/U11-T0[2-5]-|unidades-y-tareas\.md|deploy/flux/base/warm\.yaml|bitacoras/U11-T02\.md|revisiones/U11-T02/)' | wc -l; git diff --name-only $b -- services/go-intake services/ui-api services/go-run-controller deploy/flux/base contracts | grep -v '^deploy/flux/base/warm.yaml$' | wc -l; git diff $b -- deploy/flux/base/warm.yaml | grep -E '^[+-][^+-]' | grep -v -E '^[+-]\s*#' | wc -l
  ```
  Esperado: `0`, `0`, `0` y `0` (la única excepción en `deploy/flux/base` es `warm.yaml` y solo en líneas de comentario: el comentario que nombra `demo/target-app` se actualiza para cumplir CA-1).

---

## Plan de pruebas

- **Rojo primero:** antes de borrar nada, mostrar que con `demo/target-app` ausente fallan hoy `go test` de `go-warm-manager` y la prueba del runner, y `secrets.sh` aborta; pegar las salidas. Antes de adaptar los scripts, mostrar el 422 `repo_not_allowed` de `kind-e2e.sh` contra el go-intake de T01.
- Pruebas sin clúster de los scripts (CA-5) con `kubectl`/`curl` simulados: SHA mal formado, sin red, cuerpo firmado.
- Par falla-pasa de la copia del baseline: alterar un byte de `deploy/kind/target-app/baseline.sh` y comprobar que el `diff` de CA-2 falla.
- CA-6 es la prueba real; si el entorno del codificador no puede correr kind/podman o llegar a GHCR, lo declara en la bitácora y el **revisor o el humano lo corren** (no se da por cumplido).

---

## Notas

- Archivos que se **modifican en su sitio**; nada de duplicados con sufijo. Rama `tarea/U11-T02`, worktree `../agentic-qa-swarm-wt-U11-T02`, desde `tarea/U11-T01` (el PR apila sobre el #134).
- Orden de despliegue: controlador antes que `ui-api` (README de `go-run-controller`).
- El defecto sembrado en la demo **no** se inyecta con un `kubectl patch`: lo trae la imagen del commit (`ENV TARGET_APP_BUG=oversell`); el `--bug oversell` de `kind-e2e.sh` conserva su parche de entorno tras el ensayo, que sigue valiendo para el e2e. Si el rollout por commit pisa ese parche, se anota como hallazgo, no se arregla aquí.
- El `ping` de GitHub exige `Content-Type: application/json` (415 si no).
- Candidatas que surjan van a `tareas/candidatas.md`.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
