# agent-reporter

Post-mortem de logica de negocio a partir de la evidencia de una corrida (U3-T04). Biblioteca estandar en
runtime; el LLM y la evidencia/almacen/eventos reales entran por puertos (adaptadores en U3-T07 / C-45).

**Aviso: logs como dato; la defensa es la validacion y la redaccion, no el prompt.** La evidencia va en un
bloque `<datos-evidencia>` en JSON canonico, pero un prompt no es una frontera de seguridad. El veredicto del
modelo se valida de forma determinista (`bug` exige un flujo fallido; `sin-hallazgos` exige todos pasados;
solo cita URIs recibidas; `finding_id` unicos; esquema `contracts/plans/report.schema.json`). Una violacion
produce `ReportRejected`, sin reparar.

## Flujo
`correlate_postmortem`: valida URIs -> lee con topes -> `redact_secrets` por objeto (antes de recortar y
antes del prompt) -> prompt -> LLM (`timeout_s`, `max_tokens`) -> validacion -> `redact_secrets` sobre el
Report -> guardar, leer de vuelta y comparar hash -> evento `report.ready` (solo tras guardar).

## Redaccion (`redact_secrets`, mejor esfuerzo, no garantia)
Reemplaza por `[REDACTED:<tipo>]`: ID de clave de acceso de AWS, tokens de GitHub (`gh[pousr]_`), JWT,
cabecera `Authorization:` y `Bearer`, bloques de clave privada PEM, credenciales en URL
(`scheme://user:pass@`) y asignaciones `password|passwd|secret|token|api_key|access_key` con `=` o `:`
(incluido JSON). Una lista de patrones no detecta cualquier secreto; los datos sensibles de negocio (PII) no
se redactan aqui.

## Configuracion
| Variable | Defecto | Efecto |
|---|---|---|
| `REPORTER_MAX_OBJECT_BYTES` | 65536 | tope por objeto; recorte cabeza+cola con marcador |
| `REPORTER_MAX_TOTAL_BYTES` | 262144 | tope total de evidencia en el prompt (`REPORTER_MAX_TOTAL_BYTES`) |
| `REPORTER_TIMEOUT_S` | 30 | plazo del LLM; excederlo es `budget_exceeded` |
| `REPORTER_MAX_OUTPUT_TOKENS` | 1024 | tope de salida del modelo |
| `REPORTER_MAX_INPUT_TOKENS` | 120000 | tope de entrada (estimado) |
| `LLM_PROVIDER` | - | solo `fake`; requiere `REPORTER_ALLOW_FAKE=1`, prohibido con `REPORTER_ENV=prod` |
| `REPORTER_EVIDENCE_DIR` | `/evidence` | directorio del lector de evidencia |

## API
`POST /v1/report` (`Content-Type: application/json`, cuerpo <= 64 KiB) con `{run_id, evidence_uris}`:
`200` Report | `422 {error: no_evidence|report_rejected|invalid_request}` | `504 {error: budget_exceeded}` |
`502 {error: llm_unavailable|evidence_unavailable|store_unavailable}` | `413` | `415`.
`GET /healthz`, `/readyz`, `/metrics` (`aqs_reporter_requests_total{result}`,
`aqs_reporter_redactions_total{type}`). `detail` y logs nunca llevan contenido de evidencia ni del modelo.

## Pruebas
`.venv/bin/python -m pytest -q`; `python -m agent_reporter.precision` (umbral 0.8);
`python -m agent_reporter.demo_redaction --out DIR`.

## Decisiones de redaccion (ronda 2)
- Una sola pasada de `redact_secrets`, sobre el objeto completo y ANTES de recortar: un secreto partido por el recorte
  no sobrevive (la prueba barre el desplazamiento por ambas fronteras y falla si se redacta despues de recortar).
- Costo lineal: patrones con lookbehind y cotas; hay pruebas de rendimiento con cota de tiempo (64 KiB).
- El tope de LECTURA (bytes traidos del almacen antes de redactar) lo impone el adaptador del lector: es de U3-T07.
  `EvidenceReader.get` es un puerto y devuelve el objeto completo; aqui solo se acota el costo de redactar.
- Las URIs de entrada vienen de la plataforma (U2) y van tal cual al prompt (candidata: redactar su query/credenciales).
