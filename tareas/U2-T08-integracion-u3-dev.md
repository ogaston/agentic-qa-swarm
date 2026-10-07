# U2-T08 — Integración con U3 real: superficie → flujos → evidencia de extremo a extremo en dev

**Unidad:** U2 — Orquestación & Warm Sandbox Lifecycle
**Historias que implementa:** US-M5, US-M6 (camino feliz del journey 7.1, con gates, reset verificado y todo auditado)
**Depende de:** U2-T01 a U2-T07 **fusionadas**; **U3** con su fuente de flujos (`recommendFlows`) y su lectura de evidencia fusionadas (al menos las tareas que exponen `FlowPlan` desde `SurfaceArtifact`); y de la **aprobación humana del entorno dev** (cada paso que toca el clúster la requiere). Cierra U2. **No es despachable** mientras U3 no exista o falte la aprobación.

---

## Alcance

**Dentro** (una línea, concreta):

> Sustituir los stubs de U3 por la capa de agentes real en el borde del controlador (adaptador de `FlowSource` hacia el servicio de U3), añadir el workflow de contrato U2↔U3, y dejar escrito y probado el **procedimiento de demostración en dev** del recorrido completo notify → confirm → warm ready → deploy → ensayo → run → reset → estado final, con la verificación de reset 100% y del bloqueo de escape, que ejecuta el humano.

Detalle:

- **Adaptador `FlowSource`** (en `services/go-run-controller/`): cliente HTTP del servicio de U3 que, dado un `SurfaceArtifact` y un workflow, devuelve un `FlowPlan`. Valida la respuesta contra `contracts/plans/flow-plan.schema.json`; un plan inválido, vacío, con flujos que apuntan fuera del warm o un timeout → fallo de fase (nunca se ejecuta un plan sin validar). Solo `http(s)`, timeout 30 s, circuito (el de U2-T07). Selección por `RUN_FLOW_SOURCE=u3` (`U3_URL` obligatoria); `fake` sigue exigiendo `RUN_ALLOW_FAKE_PHASES=true` y rechazado con `RUN_ENV=prod`.
- **Contrato U2↔U3** en CI: workflow nuevo `.github/workflows/integration-u2-u3.yml` (en `push` a `main` y en `pull_request`; `permissions: contents: read`; acciones fijadas por SHA, **las mismas** que `ci.yml`; sin secretos del repositorio) que levanta el servicio de U3 y el controlador con fakes de Kubernetes y ejecuta `go test -tags contract -run 'Contract(U3)'`: la superficie del warm sembrado produce un `FlowPlan` válido contra su esquema, y la evidencia producida valida contra `evidence-uris.schema.json`.
- **Guion de la demostración en dev** `docs/demo-dev-u2.md` (nuevo): comandos exactos, en orden, con la salida esperada de cada uno, **sin credenciales** (referencias a Secrets por nombre), y con el marcador `APROBACIÓN HUMANA REQUERIDA` antes de cada comando que cambia algo en el clúster. Cubre: camino feliz (journey 7.1), **bloqueo de escape** (un flujo que intenta salir de `aqs-test` es bloqueado y se ve en el audit de `go-governance`), **fail-closed** (parar `go-governance` → ninguna transición avanza), **reset** (ensuciar la DB a mano → cuarentena y aviso) y un recorrido de 3 corridas seguidas donde la 2.ª y la 3.ª solo arrancan con `reset_verified=true`.
- **Script de verificación local** `scripts/test/u2-demo-local.sh` (nuevo): ejecuta la parte de la demostración que **no** requiere clúster (todo con fakes de Kubernetes y de MinIO local) e imprime `OK|FALLA <comprobación>` por cada una; es lo que el codificador y el revisor corren en el loop.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Ejecutar nada contra un clúster o la nube. **El humano ejecuta `docs/demo-dev-u2.md` en dev**, con su aprobación, y anota los resultados en la bitácora.
- Implementar o modificar el servicio de U3 (agentes, LLM, post-mortem) o los de U1/U4/U5; si falta algo en su API, es un bloqueo.
- Modificar `contracts/**`, `deploy/**`, `policy/**`, `ci.yml`, `contracts.yml`, `policies.yml`.
- El reporte post-mortem (`report.ready`, `getReport`) y la UI: U3/U1.
- Credenciales de LLM en el controlador, los Jobs o los manifiestos: el LLM vive solo en U3.

---

## Archivos de contexto

- `unidades-y-tareas.md` (U2 y U3) y los `unit-task-plans/U2.md` y `U3.md`
- `aidlc-docs/inception/application-design/unit-of-work-dependency.md` (contratos U2↔U3) y `services.md`
- `contracts/plans/*.schema.json` (U2-T01) y `contracts/events/*.schema.json`
- `services/go-run-controller/` (T02, T04, T05, T07), `tareas/U2-T02` a `U2-T07`
- Las tareas de U3 y su `README`/API (a leer cuando existan)
- `.github/workflows/ci.yml` (SHAs fijados) y `.github/workflows/integration-u1-u4.yml` (patrón de workflow de integración)
- `docs/` (formato de documentación existente)

---

## Criterios de aceptación

Desde la raíz del worktree.

- [ ] **CA-1** — El adaptador valida y falla cerrado (pruebas unitarias).
  ```bash
  cd services/go-run-controller && go test -race -run 'FlowSourceU3' -v ./... | grep -E '^(--- |ok|FAIL)'; go test -race ./... 2>&1 | grep -c FAIL
  ```
  Esperado: `--- PASS` en al menos seis pruebas (plan válido; plan inválido; plan vacío; plan con paso fuera de `/`; timeout; URL no `http(s)`) y `0`.

- [ ] **CA-2** — Contrato U2↔U3 contra el servicio de U3 real.
  ```bash
  up   # el codificador documenta la función en la bitácora: levanta U3 y el controlador con fakes de Kubernetes
  cd services/go-run-controller && U3_URL=http://127.0.0.1:18400 go test -tags contract -run 'Contract(U3)' -v ./... | grep -E '^\s*--- (PASS|FAIL|SKIP)|^(ok|FAIL)'
  down
  ```
  Esperado: al menos 3 `--- PASS` (superficie → `FlowPlan` válido; `FlowPlan` inválido de U3 → fallo de fase; U3 detenido → fallo de fase) y ningún `FAIL`/`SKIP`.

- [ ] **CA-3** — El recorrido completo, sin clúster, con todos los gates y reset 100%.
  ```bash
  bash scripts/test/u2-demo-local.sh 2>&1 | grep -E '^(OK|FALLA)'
  ```
  Esperado: solo líneas `OK`, al menos estas cinco: `camino-feliz`, `reset-entre-corridas` (3 corridas seguidas; la 2.ª y la 3.ª arrancan solo con `reset_verified=true`), `fail-closed-gobernanza`, `cuarentena-por-db-sucia`, `bloqueo-fuera-de-aqs-test`; ninguna `FALLA`. Antes de la tarea: el script no existe (rojo inicial).

- [ ] **CA-4** — El guion de dev es completo y nada en él es una orden autónoma.
  ```bash
  grep -c 'APROBACIÓN HUMANA REQUERIDA' docs/demo-dev-u2.md
  grep -c -E '^\s*(kubectl (apply|delete|create|patch)|flux reconcile|helm )' docs/demo-dev-u2.md
  grep -c -i -E 'password=|token=[A-Za-z0-9]|apikey' docs/demo-dev-u2.md
  for s in 'camino feliz' 'bloqueo de escape' 'fail-closed' 'reset' '3 corridas'; do grep -c -i "$s" docs/demo-dev-u2.md; done | paste -sd' '
  ```
  Esperado: un número ≥ `6`; el segundo, el de órdenes que cambian el clúster **sin** marcador, debe ser `0` (el codificador estructura el documento para que cada orden de ese tipo esté en una línea precedida inmediatamente por el marcador, y el revisor lo verifica leyendo); `0` credenciales; y cinco números ≥ `1`.

- [ ] **CA-5** — El workflow existe, es válido, pinea por SHA y no usa secretos.
  ```bash
  docker run --rm -v "$PWD":/repo:z -w /repo rhysd/actionlint:1.7.1 .github/workflows/integration-u2-u3.yml; echo "actionlint rc=$?"
  grep -E '^\s+(- )?uses:' .github/workflows/integration-u2-u3.yml | grep -v -E '@[0-9a-f]{40}' | wc -l
  grep -c -E 'secrets\.' .github/workflows/integration-u2-u3.yml
  grep -E 'uses:' .github/workflows/integration-u2-u3.yml | sed -E 's/.*uses: *//; s/ *#.*//' | sort -u | while read -r u; do grep -q -F "$u" .github/workflows/ci.yml || echo "ACCION NUEVA: $u"; done
  ```
  Esperado: `actionlint rc=0`, `0`, `0` y ninguna línea `ACCION NUEVA`.

- [ ] **CA-6** — Sin credenciales de LLM fuera de U3 y las políticas siguen en verde.
  ```bash
  grep -r -l -i -E 'OPENAI|ANTHROPIC|LLM_API' services/go-run-controller services/go-warm-manager services/go-reset deploy 2>/dev/null | wc -l
  bash scripts/ci/policies.sh 2>&1 | grep -c '^FALLA'
  ```
  Esperado: `0` y `0`.

- [ ] **CA-7** — Higiene y alcance.
  ```bash
  (cd services/go-run-controller && go vet ./... && go vet -tags contract ./... && test -z "$(gofmt -l .)" && echo ok); git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/go-run-controller/|\.github/workflows/integration-u2-u3\.yml|docs/demo-dev-u2\.md|scripts/test/u2-demo-local\.sh|bitacoras/U2-T08\.md|revisiones/U2-T08/)' | wc -l
  ```
  Esperado: `ok`, `0` y `0`.

- [ ] **CA-8** — **Validación en dev (humano, con aprobación).** El humano ejecuta `docs/demo-dev-u2.md` en el clúster de dev y anota en `bitacoras/U2-T08.md`, para cada sección, la salida real. Esperado: camino feliz completo con `reset.verified` entre corridas; el escape bloqueado visible en `GET /audit`; con `go-governance` detenido ninguna transición avanza; el warm sucio a mano termina en cuarentena con aviso; 3 corridas seguidas con reset verificado 100%. **Este criterio lo cierra el humano, no el revisor.**

---

## Plan de pruebas

- Unitarias del adaptador con `httptest` (plan válido, inválido, vacío, lento, redirección).
- Contrato U2↔U3 en CI (CA-2, CA-5) y recorrido local con fakes (CA-3).
- Negativas: un `FlowPlan` con una URL fuera del warm es bloqueado antes de crear Jobs; un `FlowPlan` sin flujos no avanza del ensayo.

**Rojo primero:** registrar que `scripts/test/u2-demo-local.sh` y `docs/demo-dev-u2.md` no existen y que `RUN_FLOW_SOURCE=u3` no arranca.

---

## Notas

- **Esta tarea no se despacha hasta que U3 exista.** El orquestador, al verla sin U3 fusionada, reporta el hueco al humano y no despacha. Si la API de U3 difiere de lo descrito aquí, se actualiza este archivo antes de despachar (es una especificación, no un contrato con U3).
- **Go y workflows.** `go 1.26.8`; acciones fijadas por SHA, las mismas de `ci.yml`. Podman: montajes con `:z`.
- **Informes del loop.** El diff de `revisiones/<tarea>/` no cuenta como desborde.
- Ningún comando contra un clúster ni la nube en el loop; CA-8 es del humano.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
