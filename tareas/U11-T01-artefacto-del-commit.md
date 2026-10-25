# U11-T01 — La corrida despliega la imagen del commit empujado

**Unidad:** U11 — Demo con repositorio y modelo reales
**Historias que implementa:** US-M1 (evento real de GitHub), US-M2 (deploy sobre el warm del artefacto del evento).
**Depende de:** U8 y U9 cerradas (lo están). **Tope: 3 rondas**.

---

## Origen

El repositorio objetivo real es `ogaston/demo-target-app-qa-swarm`. Su CI publica
`ghcr.io/ogaston/demo-target-app-qa-swarm:<sha de 40 hex>` en cada push a `main`. Hoy un push real no
produce una corrida sobre ese commit:

- `services/go-intake/internal/artifact/artifact.go` resuelve `commit`/`pull_request` a `build-from-repo`,
  que `go-warm-manager` rechaza (`service.go`, `ErrBuildNotSupported`).
- `run.confirmed` no lleva el artefacto (`contracts/events/run.confirmed.schema.json`,
  `additionalProperties: false`).
- `services/go-run-controller/cmd/go-run-controller/real.go` fija `Artifact: func(string) … { return "published-image", ref, nil }`
  con `ref = RUN_ARTIFACT_REF`: todas las corridas despliegan la misma imagen.

**Decisión del humano (2026-10-25):** imagen por SHA en GHCR; **no** se construye en el clúster.

## Alcance

**Dentro** (una línea, concreta):

> Un commit/PR del repositorio permitido se resuelve a `published-image` `<registro>/<repo>:<sha>`, el artefacto viaja en `run.confirmed` y el controlador despliega **el artefacto del evento** (con `RUN_ARTIFACT_REF` solo como respaldo cuando el evento no lo trae); go-intake solo acepta repositorios de una lista permitida y responde 200 al `ping` de GitHub.

Detalle:

1. **Resolución (go-intake):** `commit` y `pull_request` (mismo repo, sin forks) → `Kind: published-image`,
   `Ref: <registry>/<owner/repo en minúsculas>:<sha>`. Los `tag` no cambian. Se elimina `build-from-repo`
   de la salida del resolvedor (el tipo sigue existiendo en el contrato).
2. **Lista de repositorios (go-intake):** variable `INTAKE_ALLOWED_REPOS` (lista separada por comas,
   comparación exacta sin distinguir mayúsculas). Vacía o ausente = **fail-closed** (todo 422
   `repo_not_allowed`). Cableada en `deploy/flux/base/control-plane.yaml` con
   `ogaston/demo-target-app-qa-swarm`.
3. **`ping` de GitHub (C-126):** `X-GitHub-Event: ping` con firma válida → `200`, sin notificación ni evento.
   Firma inválida sigue siendo rechazada como hoy.
4. **Contrato:** `run.confirmed` v1 admite `data.artifact` **opcional** `{kind, ref}` (mismas reglas que en
   `notify.created`). Ejemplos `valid`/`invalid` nuevos y validador de CI.
5. **ui-api:** al publicar `run.confirmed` incluye el `Artifact` de la notificación confirmada
   (`services/ui-api/inbox/publish.go`, `NewRunConfirmed`).
6. **go-run-controller:** `ParseRunConfirmed` (`adapters/nats.go`) acepta el campo opcional, lo guarda en la
   `Run` (persistido) y `RealPhases.Artifact` devuelve el de la corrida; si no hay, `RUN_ARTIFACT_REF`.
7. **Espera de imagen:** se documenta (README de `go-warm-manager`) que `WARM_DEPLOY_TIMEOUT` (10 min por
   defecto) cubre la carrera «el webhook llega antes de que la CI publique la imagen»; el mensaje de fallo
   del deploy por rollout incompleto nombra la imagen. **No** se añade lógica nueva de reintento.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Construir imágenes en el clúster (`build-from-repo` real), kaniko/buildah, credenciales de registro.
- `scripts/kind/` (excepto un caso en `kind-e2e.sh` solo si un CA lo exige), el repositorio objetivo, el
  webhook de GitHub, `docs/operaciones/webhook-github-cloudflared.md` (U11-T02).
- Cambiar RBAC, NetworkPolicy, `go-governance`, `go-reset`, agentes, dashboard.
- Soporte de varios `RUN_ARTIFACT_REF` o de repos distintos al objetivo con baseline propio.
- Cabeceras de caché, reintentos nuevos, pull secrets.

---

## Archivos de contexto

- `services/go-intake/internal/artifact/artifact.go`, `internal/intake/model.go`, `handler.go`, `cmd/go-intake/main.go`
- `contracts/events/run.confirmed.schema.json`, `contracts/events/notify.created.schema.json`, `contracts/events/examples/`
- `services/ui-api/inbox/publish.go`, `store.go`, `event.go`
- `services/go-run-controller/adapters/nats.go`, `adapters/phases.go`, `cmd/go-run-controller/real.go`, `runctl/ports.go`
- `services/go-warm-manager/types.go` (`ValidateArtifact`, `DefaultAllowedRegistries`), `README.md`
- `deploy/flux/base/control-plane.yaml`
- `docs/operaciones/app-objetivo.md`

---

## Criterios de aceptación

- [ ] **CA-1** — Un commit y un PR del mismo repo se resuelven a imagen publicada con el SHA como tag; un fork o un SHA inválido siguen rechazados.
  ```bash
  cd services/go-intake && go test ./... -count=1 -run 'Artifact|Resolve' -v 2>&1 | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `ok`, con pruebas que comprueban `published-image` y `ghcr.io/ogaston/demo-target-app-qa-swarm:<sha>` para commit y PR, y que ninguna salida es `build-from-repo`. Las pruebas PBT existentes se actualizan, no se borran.

- [ ] **CA-2** — La lista de repos es fail-closed y el `ping` responde 200.
  ```bash
  cd services/go-intake && go test ./... -count=1 -run 'Allowed|Repo|Ping' -v 2>&1 | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `ok` con pruebas de: repo fuera de la lista → 422 `repo_not_allowed` y ninguna notificación; lista vacía → 422; repo permitido en otra capitalización → 202; `ping` firmado → 200 sin notificación ni evento; `ping` con firma inválida → rechazado.

- [ ] **CA-3** — El contrato admite el artefacto opcional y rechaza lo malformado.
  ```bash
  S=contracts/events/run.confirmed.schema.json; V="npx --yes -p ajv-cli@5.0.0 -p ajv-formats@3.0.1 ajv validate --spec=draft2020 -c ajv-formats -s $S"
  for f in contracts/events/examples/valid/run.confirmed*.json; do $V -d "$f" >/dev/null 2>&1 && echo "OK $f" || echo "FALLA $f"; done
  for f in contracts/events/examples/invalid/run.confirmed*.json; do $V -d "$f" >/dev/null 2>&1 && echo "FALLA(acepta) $f" || echo "OK(rechaza) $f"; done
  ```
  Esperado: todo `valid` imprime `OK` (incluye un ejemplo **con** `artifact` y otro sin él) y todo `invalid` imprime `OK(rechaza)` (incluye `kind` desconocido, `ref` vacío y campo extra dentro de `artifact`); ninguna línea `FALLA`. Es el mismo ajv que usa `.github/workflows/integration-u1-u4.yml`.

- [ ] **CA-4** — ui-api publica el artefacto y el controlador despliega el del evento, no el estático.
  ```bash
  (cd services/ui-api && go test ./... -count=1 -run 'RunConfirmed|Publish|Artifact' -v 2>&1 | grep -E '^(--- |ok|FAIL)'); (cd services/go-run-controller && go test ./... -count=1 -run 'Artifact' -v 2>&1 | grep -E '^(--- |ok|FAIL)')
  ```
  Esperado: `ok` en ambos. Pruebas que demuestran: (a) el `run.confirmed` publicado por ui-api tras confirmar lleva `data.artifact` igual al de la notificación y valida contra el esquema; (b) con `RUN_ARTIFACT_REF=ghcr.io/x/estatica:1` y un evento con `ghcr.io/ogaston/demo-target-app-qa-swarm:<sha>`, el deploy recibe la segunda; (c) sin `artifact` en el evento, recibe la estática; (d) el artefacto sobrevive a un reinicio del controlador (se lee del estado persistido); (e) un evento `run.confirmed` con `artifact` malformado se rechaza.

- [ ] **CA-5** — Lectura de vuelta del manifiesto: lista el repo permitido, es YAML válido y las políticas del repo siguen en verde.
  ```bash
  grep -n -A1 'INTAKE_ALLOWED_REPOS' deploy/flux/base/control-plane.yaml; python3 -c "import yaml;list(yaml.safe_load_all(open('deploy/flux/base/control-plane.yaml')));print('YAML OK')"; bash scripts/ci/policies.sh 2>&1 | tail -3
  ```
  Esperado: aparece `INTAKE_ALLOWED_REPOS` con `ogaston/demo-target-app-qa-swarm`, `YAML OK`, y `scripts/ci/policies.sh` termina con rc 0. Si `conftest`/`kubeconform` no están instalados en la máquina, el codificador lo declara en su bitácora y el revisor lo corre donde estén (la CI `policies.yml` es la fuente).

- [ ] **CA-6** — Pruebas de todos los módulos tocados y alcance.
  ```bash
  for d in services/go-intake services/ui-api services/go-run-controller services/go-warm-manager; do (cd $d && go vet ./... && go test ./... -count=1 2>&1 | tail -1); done; git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/(go-intake|ui-api|go-run-controller|go-warm-manager)/|contracts/(events|openapi)/|deploy/flux/base/control-plane\.yaml|tareas/U11-T01-|unidades-y-tareas\.md|bitacoras/U11-T01\.md|revisiones/U11-T01/)' | wc -l
  ```
  Esperado: cuatro `ok`, `0` y `0`.

---

## Plan de pruebas

- **Rojo primero:** antes de cambiar el resolvedor, correr una prueba que pida `published-image` para un commit y pegar el fallo actual (`build-from-repo`); antes de tocar el controlador, una prueba que despliegue el artefacto del evento y falle porque hoy sale el estático.
- Negativas: repo fuera de la lista, lista vacía, fork, SHA corto, `artifact` con `kind` desconocido, `ref` vacío, campo extra.
- Par falla-pasa de sensibilidad: quitando la propagación del artefacto en ui-api, la prueba (a) de CA-4 falla.
- PBT: el round-trip de `run.confirmed` con y sin `artifact` (PBT-02) y la invariante «un commit nunca resuelve a build-from-repo» (PBT-03), con semilla registrada.

---

## Notas

- Archivos que se **modifican en su sitio**; nada de duplicados con sufijo. Rama `tarea/U11-T01`, worktree `../agentic-qa-swarm-wt-U11-T01`, desde `main`.
- **Corrección (ronda 2, F-06; error de la especificación):** NO es cierto que `kind-e2e.sh` siga funcionando. Con esta tarea, un push del repositorio permitido
  siempre produce un artefacto del evento, y `kind-e2e.sh`, `kind-smoke.sh` y `dashboard-smoke.sh` firman el webhook con `ogaston/agentic-qa-swarm` (422 `repo_not_allowed`).
  Esos scripts **quedan rotos hasta U11-T02**, que los adapta (webhook del repo permitido y una imagen local con el tag del SHA firmado). Decisión del orquestador, a revisar por el humano
  en el PR: no se tocan aquí `scripts/kind/` ni el overlay kind. El codificador lo declara en la bitácora y en el cuerpo del PR; `RUN_ARTIFACT_REF` queda como respaldo solo para eventos sin artefacto.
- Regla `ValidateArtifact` de `go-warm-manager`: solo `ghcr.io` con tag explícito (nunca `latest`). El SHA de 40 hex cumple.
- El tag de imagen es el SHA completo en minúsculas; el nombre del repo se pasa a minúsculas (GHCR lo exige).
- Candidatas que surjan (p. ej. `build-from-repo` real, pull secrets para registros privados) van a `tareas/candidatas.md`, no a esta tarea.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
