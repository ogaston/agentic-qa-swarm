# Ronda 1 — U9-T01

VEREDICTO: VERDE

Corrí yo mismo los seis criterios, todos con evidencia fresca de este diff (código en 55f2649, HEAD 8cd6dab solo agrega la bitácora). No queda ningún ROJO ni NARANJA. Worktree limpio al terminar (`git status --short | wc -l` da 0). Mi venv de Python y mis logs están en el scratchpad, fuera del repo.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Contratos y tipos | `bash contracts/validate.sh` y `npm run -s check:api` | rc=0 y rc=0 |
| 2 | ui-api | `go vet ./... && go test -count=1 ./... \| grep -c FAIL` y `go test -run Report -v` | vet limpio, 0 FAIL, 15 pruebas `Report*` PASS (pedía 9). Cubren 200, 401, 403 y 404 sin llamadas salientes, `report_not_ready`, 503 por cada URL ausente, controlador caído 503 con `Retry-After`, reporter caído 502, `run_id` cruzado 502, run mal formado 400, admin y cortacircuitos |
| 3 | agent-reporter | `pytest -q` completo y `pytest -k get_report` | 325 passed, 3 deselected (los mismos que en la base); `get_report`: 13 passed (pedía 4) |
| 4 | Políticas | `bash scripts/ci/policies.sh` | rc=0, todo OK (kubeconform, conftest, rbac-matrix, promtool, check-secrets) |
| 5 | kind | Bajo `flock`: `AQS_IMAGES="ui-api agent-reporter"` build y load, `rollout restart` solo de esos dos Deployments, luego `kind-e2e.sh --bug oversell` | `OK reporte-en-ui-api` y `OK reporte-inexistente-404`. 22 checks OK; los únicos FALLA son `veredicto-bug` (verdict=sin-hallazgos) y `hallazgo-cita-invariante-y-evidencia`, los dos excluidos por el arbitraje 2. Ningún otro FALLA. |
| 6 | Alcance | los dos comandos de CA-6 | 0 y 0 |

CA-5: no toqué go-run-controller (imagen de U9-T02 intacta, Running), ni Secrets, credenciales ni PVCs. Solo ui-api y agent-reporter.

## Revisión específica pedida
- **Propiedad.** `reports.go:117` busca el recibo con `ReceiptByRun`. Sin recibo da 404, con recibo de otro usuario da 403 y el admin ve todo. Es el mismo criterio que `/confirmations` (`api.go:320`). Ambos casos responden antes de cualquier llamada saliente; las pruebas lo comprueban con el contador de peticiones del doble. En kind la brecha es real: solo existen demo y admin, y el admin ve todo, así que el 403 solo lo cubren las pruebas unitarias. Está declarada en `kind-e2e.sh` y en la bitácora; no la considero bloqueante.
- **NetworkPolicy `ui-api-egress`.** Contrasté con lo que ui-api usa de verdad: NATS 4222 (`NATS_URL`), go-identity 8080 (`IDENTITY_URL`), go-warm-manager 8080 (`WARM_URL`) y DNS en kube-system.
  - Las etiquetas de nats, go-identity y go-warm-manager coinciden con las de los manifiestos, y el overlay `kind` no parchea ui-api.
  - No rompe nada que ui-api ya usara. En el clúster, `warm-verificado`, `login-demo`, `inbox` y `confirm` pasaron con la política aplicada, y el log de ui-api no tiene errores.
  - Las políticas `ui-api-egress` y `agent-reporter-ingress-from-ui-api` están aplicadas.
  - `agents-ingress-from-controller` y `allow-same-namespace` no interfieren.
- **`Report` de la OpenAPI.** Cargué ambos YAML/JSON y comparé: el esquema `Report` es idéntico al de `report.schema.json` salvo `description`, `$schema`, `$id` y `title`. Incluye el `allOf` if/then/else de `verdict`. OpenAPI es 3.1.0, así que el `if/then/else` es válido.
- **Errores del reporter.**
  - El reporter responde 400 `invalid_uri` sin tocar el store y 502 `store_unavailable`.
  - ui-api traduce todo fallo del reporter a 502 `reporter_unavailable`, y a 502 `report_mismatch` si el `run_id` no coincide.
  - Un controlador caído o con un status inesperado da 503 con `Retry-After`; sin las URLs da 503 `reports_unavailable`.
  - Las pruebas lo cubren.

## Hallazgos
### F-01 · AMARILLO · `services/ui-api/internal/httpapi/reports.go:3-5` · Candidata del token del reporter sin registrar
La tarea pide documentar la falta de token de servicio en el reporter y dejarla como candidata. Hoy no existe ningún token ahí. El comentario del código lo dice y remite a «candidatas de U9-T01», pero ni la bitácora ni `CANDIDATAS.md` la registran. El orquestador debería añadirla a `CANDIDATAS.md`. La mitigación actual es la NetworkPolicy: solo ui-api y go-run-controller llegan al reporter.

### F-02 · AMARILLO · `deploy/flux/base/agents.yaml:~251` · Separador `---` duplicado
Queda un documento YAML vacío entre dos políticas. kubeconform y conftest pasan; es solo estético.

### F-03 · AMARILLO · `services/ui-api/inbox/store.go` (`ReceiptByRun`) · Búsqueda lineal por recibos
Recorre todo el mapa de recibos en cada petición. Es irrelevante a la escala actual; si los recibos crecen, conviene indexar por `RunID`.

### F-04 · AMARILLO · `control-plane.yaml` (OpenAPI `Report`) · Paridad con `report.schema.json` sin prueba automática
La copia es fiel hoy, pero solo la protege un comentario («si cambia uno, cambia el otro»). `contracts/validate.sh` no la compara. Candidata: una comprobación de paridad.

## Tareas candidatas (defectos reales fuera de alcance)
- `docs/seguridad/aislamiento.md` y la cabecera de `networkpolicies-system.yaml` dicen «solo INGRESS, sin egress». Ahora hay una excepción (`ui-api-egress`). La cabecera ya se actualizó; el documento no, y `docs/` está fuera del alcance de CA-6.
- Token de servicio para `GET /v1/reports` del reporter (ver F-01).
- Ya conocidas y fuera del conteo por el arbitraje 2: `veredicto-bug` y `hallazgo-cita-invariante-y-evidencia` fallan porque el planner con LLM real no siempre genera el probe de stock.

## Observaciones
- `kind-e2e.sh` imprimió el aviso «lock tomado por residuos del bringup». No afectó la corrida.
- La bitácora de la ronda 1b/1c es coherente con lo que reproduje: el CrashLoop de go-run-controller venía de cargar todas las imágenes sin filtro, no de U9-T01.

## Rutas de evidencia
- Salida del e2e: `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/7c206815-66cc-47fb-9c25-52913a74a0bf/scratchpad/e2e.log`
- Políticas: `.../scratchpad/pol.log` (misma carpeta)
- Despliegue del paso 5: `.../scratchpad/ca5.out`, `build.log`, `load.log` (misma carpeta)

VEREDICTO: VERDE
INFORME: revisiones/U9-T01/ronda-1.md
