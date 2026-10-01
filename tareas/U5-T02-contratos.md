# U5-T02 — Contratos: eventos del pipeline + OpenAPI del control plane

**Unidad:** U5 — Plataforma & GitOps
**Historias que implementa:** US-M10
**Depende de:** U5-T01 (árbol `contracts/openapi/`, `contracts/events/`). Ola 2, en paralelo con U5-T03 y U5-T04.

---

## Alcance

**Dentro** (una línea, concreta):

> Publicar los 10 esquemas JSON Schema (draft 2020-12, versión `v1`) de los eventos del pipeline en `contracts/events/`, con un ejemplo válido y al menos un ejemplo inválido por evento, el OpenAPI 3.1 del control plane en `contracts/openapi/control-plane.yaml`, un validador local `contracts/validate.sh` y el workflow `.github/workflows/contracts.yml` que lo corre en cada push/PR.

Los 10 eventos (fuente: `aidlc-docs/inception/application-design/services.md`, línea 45), un archivo por evento:

| Archivo | Valores de `type` |
|---|---|
| `notify.created.schema.json` | `notify.created` |
| `run.confirmed.schema.json` | `run.confirmed` |
| `warm.ready.schema.json` | `warm.ready` |
| `deploy.schema.json` | `deploy.done`, `deploy.failed` |
| `surface.ready.schema.json` | `surface.ready` |
| `rehearsal.schema.json` | `rehearsal.passed`, `rehearsal.failed` |
| `run.done.schema.json` | `run.done` |
| `reset.verified.schema.json` | `reset.verified` |
| `teardown.verified.schema.json` | `teardown.verified` |
| `report.ready.schema.json` | `report.ready` |

Envoltorio común obligatorio en los 10: `event_id` (uuid), `type`, `version` (const `1`), `occurred_at` (date-time), `trace_id` (trazado distribuido, RESILIENCY-05) y `data` (objeto específico del evento, con `additionalProperties: false`). `$id` contiene `/v1/`.

Endpoints mínimos del OpenAPI (fuente: `components.md` y `services.md`): `POST /webhooks/github`, `GET /notifications`, `POST /notifications/{id}/confirm`, `GET /runs/{id}`, `GET /reports/{run}`, `PUT /policies/{name}`, `GET /audit`, `POST /auth/login`, `POST /auth/logout`. Basta con la forma: request/response con esquemas y códigos de estado, sin lógica.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Los eventos `warm.quarantined` (alerta) y `run.started`: no forman parte de los 10. Si los ve necesarios, los propone como tarea candidata.
- Implementar productores o consumidores de eventos, o servidores del API: son U1–U4.
- `.github/workflows/ci.yml` y cualquier otro workflow distinto de `contracts.yml`: es **U5-T03**.
- Cualquier archivo bajo `deploy/`: es **U5-T04**.
- Modificar `README.md` de la raíz (lo dejó U5-T01).
- Añadir `package.json`, `node_modules/` o toolchain al repo. Las herramientas se invocan con `npx` y versión fijada.

---

## Archivos de contexto

Rutas, no contenido pegado:

- `unidades-y-tareas.md`
- `aidlc-docs/inception/application-design/unit-task-plans/U5.md`
- `aidlc-docs/inception/application-design/services.md` (lista de eventos, línea 45; servicios y endpoints)
- `aidlc-docs/inception/application-design/components.md` (interfaces REST por componente)
- `aidlc-docs/inception/application-design/component-methods.md` (qué publica cada método)
- `aidlc-docs/inception/application-design/component-dependency.md` (flujo de eventos)
- `aidlc-docs/inception/application-design/unit-of-work.md` (layout de `contracts/`)

---

## Criterios de aceptación

Cada uno con **su comando**. El revisor los va a correr él mismo, uno por uno, desde la raíz del worktree.

- [ ] **CA-1** — Existen los 10 esquemas con los nombres exactos.
  ```bash
  for e in notify.created run.confirmed warm.ready deploy surface.ready rehearsal run.done reset.verified teardown.verified report.ready; do test -f "contracts/events/$e.schema.json" || { echo "FALTA $e"; exit 1; }; done; ls contracts/events/*.schema.json | wc -l
  ```
  Esperado: `10`. Antes de la tarea: `FALTA notify.created` (rojo inicial).

- [ ] **CA-2** — Los 10 esquemas compilan en draft 2020-12.
  ```bash
  npx --yes -p ajv-cli@5.0.0 -p ajv-formats@3.0.1 ajv compile --spec=draft2020 -c ajv-formats -s 'contracts/events/*.schema.json' 2>&1 | grep -c 'is valid'
  ```
  Esperado: `10`.

- [ ] **CA-3** — Todos llevan el envoltorio común y un `$id` versionado.
  ```bash
  for f in contracts/events/*.schema.json; do jq -e '(.required | index("event_id") and index("type") and index("version") and index("occurred_at") and index("trace_id") and index("data")) and (."$id" | test("/v1/"))' "$f" >/dev/null || echo "MAL $f"; done; echo FIN
  ```
  Esperado: solo `FIN`.

- [ ] **CA-4** — El validador acepta todos los ejemplos válidos y rechaza todos los inválidos.
  ```bash
  bash contracts/validate.sh; echo "rc=$?"
  ```
  Esperado: `rc=0`. El script recorre `contracts/events/examples/valid/*.json` (deben validar) y `contracts/events/examples/invalid/*.json` (deben fallar), y sale distinto de 0 si cualquiera de los dos conjuntos se comporta al revés. Hay al menos un válido y un inválido por evento:
  ```bash
  ls contracts/events/examples/valid/*.json | wc -l; ls contracts/events/examples/invalid/*.json | wc -l
  ```
  Esperado: dos números ≥ 10.

- [ ] **CA-5** — El validador se pone en rojo si un ejemplo válido viola su esquema (prueba negativa sobre una copia, sin tocar el worktree).
  ```bash
  t=$(mktemp -d); git archive HEAD | tar -x -C "$t"; f=$(ls "$t"/contracts/events/examples/valid/*.json | head -1); jq 'del(.event_id)' "$f" > "$f.tmp" && mv "$f.tmp" "$f"; (cd "$t" && bash contracts/validate.sh >/dev/null 2>&1); echo "rc=$?"; rm -rf "$t"
  ```
  Esperado: `rc=` distinto de `0`.

- [ ] **CA-6** — El OpenAPI es válido y cubre los 9 endpoints.
  ```bash
  npx --yes @redocly/cli@1.25.0 lint contracts/openapi/control-plane.yaml >/dev/null 2>&1; echo "rc=$?"
  grep -c -E '^  /(webhooks/github|notifications|notifications/\{id\}/confirm|runs/\{id\}|reports/\{run\}|policies/\{name\}|audit|auth/login|auth/logout):' contracts/openapi/control-plane.yaml
  ```
  Esperado: `rc=0` y `9`.

- [ ] **CA-7** — El workflow de contratos corre el validador, pasa `actionlint` y sus acciones están fijadas por SHA.
  ```bash
  grep -c 'contracts/validate.sh' .github/workflows/contracts.yml
  docker run --rm --security-opt label=disable -v "$PWD":/repo -w /repo rhysd/actionlint:1.7.7 -color .github/workflows/contracts.yml; echo "rc=$?"
  grep -hE '^\s*-?\s*uses:' .github/workflows/contracts.yml | grep -v -E '@[0-9a-f]{40}' | wc -l
  ```
  Esperado: `≥1`, `rc=0` sin salida de actionlint, y `0`.

- [ ] **CA-8** — Árbol limpio tras el commit y sin `.gitkeep` sobrantes en los directorios que ahora tienen contenido.
  ```bash
  git status --short | wc -l; git ls-files contracts | grep -c '\.gitkeep$'
  ```
  Esperado: `0` y `0`.

---

## Plan de pruebas

Qué pruebas se escriben **en esta tarea**, no después:

- Rojo inicial de CA-1 (`FALTA notify.created`) registrado en la bitácora antes de crear nada.
- Por cada evento, un ejemplo válido y al menos un inválido que falle por una razón distinta del envoltorio (por ejemplo, un campo obligatorio de `data` ausente o un `type` que no pertenece al evento). Así se demuestra que `data` está realmente restringido.
- CA-5 es la prueba negativa del validador.

**Rojo primero:** el codificador reproduce `FALTA notify.created` y lo registra en su bitácora, con el comando literal, antes de crear nada.

---

## Notas

- `contracts/validate.sh` usa la misma invocación fijada que CA-2 (`npx --yes -p ajv-cli@5.0.0 -p ajv-formats@3.0.1 ajv ...`). No instala nada en el repo.
- El ejemplo válido de cada evento se nombra `examples/valid/<archivo-sin-.schema.json>.json`; los inválidos, `examples/invalid/<evento>.<motivo>.json`.
- Al añadir contenido a `contracts/openapi/` y `contracts/events/`, se borra su `.gitkeep` (regla de U5-T01: `.gitkeep` solo en directorios vacíos).
- La bitácora pega el **comando literal** de cada criterio y su salida, no abreviaturas como `$ CA-1` (hallazgos F-02 y F-03 de U5-T01).
- Esta tarea no ejecuta ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
