# U8-T08 — Ciclo completo en kind: del webhook firmado al reporte, con reset verificado

**Unidad:** U8 — Producto demostrable (ciclo completo en kind)
**Historias que implementa:** US-M1 a US-M9 de punta a punta (journey 7.1, camino feliz y bug sembrado); US-M10.
**Depende de:** U8-T01 a U8-T07 **fusionadas**. **Ola 4**. **Tope: 3 rondas**.

---

## Alcance

**Dentro** (una línea, concreta):

> Añadir `scripts/kind/kind-e2e.sh`, que sobre la plataforma en `kind-aqs` envía a `go-intake` un webhook de GitHub **firmado** (HMAC con el secreto de `go-intake-webhook`), inicia sesión como `demo`, comprueba que la notificación llega al inbox, la confirma por `ui-api`, y sigue la corrida en `go-run-controller` hasta su estado final, verificando con lecturas de vuelta cada fase (deploy de la app de referencia, superficie, plan, ensayo, runners con evidencia en MinIO, reset con `reset_verified=true` real y reporte en MinIO); dos escenarios: `--bug off` (veredicto sin hallazgos) y `--bug oversell` (veredicto `bug` con un hallazgo que cita la invariante del stock y enlaza evidencia real); y convertir en `OK` los `PENDIENTE` de `kind-smoke.sh` que este ciclo cubre.

Detalle:

- Cada fase se comprueba leyendo el sistema, no el log del script: `GET /runs/{id}` (fases y `verdict`), `kubectl get jobs -l aqs.io/run-id=…`, objetos de MinIO listados con el usuario `aqs-evidence`, `GET /warm` (`reset_verified=true`, `state=ready`).
- El escenario `--bug oversell` despliega la app con `TARGET_APP_BUG=oversell` (variable del artefacto o segunda etiqueta de imagen; el codificador elige y lo documenta).
- Con `LLM_PROVIDER=fake` y fixtures de U8-T05: el reporte es determinista. Con la clave real (U8-T09) el script se repite sin cambios.
- Runbook `docs/operaciones/kind-local.md`: sección «Demo de punta a punta», con el guion de presentación (qué mostrar en el dashboard en cada paso).
- Limpia siempre: corridas de prueba, port-forwards y pods efímeros; deja el warm en `ready`.

**Fuera**:

- Webhook real de GitHub, LLM real y app del cliente (U8-T09).
- Corregir defectos de otros componentes: se registran como candidata y su comprobación queda `FALLA` documentada solo con aprobación del orquestador.

---

## Archivos de contexto

- `tareas/U8-T01…T07`, `scripts/kind/kind-smoke.sh`, `scripts/kind/lib.sh`, `docs/operaciones/kind-local.md`
- `services/go-intake/README.md` (firma del webhook, eventos aceptados), `contracts/events/examples/valid/`, `services/go-intake/` fixtures de GitHub
- `contracts/openapi/control-plane.yaml` (`/notifications`, confirmación, `/runs/{id}`, `/warm`)
- `services/go-run-controller/README.md` (fases y evidencia)

---

## Criterios de aceptación

Con la plataforma desplegada desde cero: `kind-up`, `build-images`, `load-images`, `deploy`.

- [ ] **CA-1** — Camino feliz de punta a punta.
  ```bash
  bash scripts/kind/kind-e2e.sh --bug off 2>&1 | grep -E '^(OK|FALLA)' | cut -d' ' -f1 | sort | uniq -c; echo "rc=${PIPESTATUS[0]}"
  ```
  Esperado: solo `OK` (al menos 12: webhook-202, inbox, confirm, deploy, superficie, plan, ensayo, runners, evidencia-en-minio, reset-verificado, reporte-en-minio, veredicto-sin-hallazgos), `rc=0`.

- [ ] **CA-2** — Bug sembrado detectado, con evidencia real.
  ```bash
  bash scripts/kind/kind-e2e.sh --bug oversell 2>&1 | grep -E '^(OK|FALLA) (veredicto|hallazgo)'
  ```
  Esperado: `OK veredicto-bug` y `OK hallazgo-cita-invariante-y-evidencia` (cada `evidence_uri` del hallazgo existe en MinIO).

- [ ] **CA-3** — Puerta humana: sin confirmación no hay corrida.
  ```bash
  bash scripts/kind/kind-e2e.sh --sin-confirmar 2>&1 | grep -E '^(OK|FALLA) sin-confirmar'
  ```
  Esperado: `OK sin-confirmar-no-crea-corrida` (pasado el plazo, ningún Job con la etiqueta de esa notificación y `GET /runs` sin ella).

- [ ] **CA-4** — El humo ya no declara pendiente lo que cubre el ciclo, y nada queda colgado.
  ```bash
  bash scripts/kind/kind-smoke.sh 2>&1 | grep -c -E '^PENDIENTE .*\((P1|P2|C-104)'
  ss -ltn | grep -c -E '127\.0\.0\.1:186'; kubectl get pods -A --no-headers | grep -c -i -E 'e2e|probe'
  ```
  Esperado: `0`, `0` y `0`.

- [ ] **CA-5** — Alcance.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(scripts/kind/|docs/operaciones/kind-local\.md|deploy/kind/README\.md|deploy/flux/kind/|bitacoras/U8-T08\.md|revisiones/U8-T08/)' | wc -l
  ```
  Esperado: `0` y `0`.

---

## Plan de pruebas

- **Rojo primero:** antes de U8-T01…T07 el ciclo se para en la notificación; pegar la salida del script en el primer `FALLA`.
- Sensibilidad: con la app sin defecto, `veredicto-bug` debe dar `FALLA` (CA-1 y CA-2 son el par).

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
