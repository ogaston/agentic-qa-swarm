# go-governance

Políticas, gates y auditoría append-only con cadena de hashes (U4-T04). Observabilidad y retención: U4-T07.

## Observabilidad
- Log JSON (`timestamp`, `level`, `message`, `request_id`, `trace_id`, `service`); cada entrada de auditoría se copia al log como `message:"audit"` (sin el detalle de hechos).
- `/healthz`, `/readyz` (`data_dir`, `policies`, `audit_chain`, `identity` si hay `IDENTITY_URL`) y `/metrics`, sin token.
- `GOVERNANCE_AUDIT_RETENTION_DAYS` (por defecto y mínimo `90`; el servicio no arranca con menos) se expone como `aqs_audit_retention_days`. Es una garantía de no borrado: el servicio nunca borra, rota ni compacta la auditoría.

## Asimetría de `aqs_policy_changes_total`
`result="rejected"` cuenta los PUT de un admin que no se guardan (404, 413, 400, 422). El 403 de una persona sin rol se audita como `policy.rejected` igual que antes, pero **no** cuenta en `aqs_policy_changes_total`: no fue un intento válido de cambio y ya figura en `aqs_http_requests_total{code="403",route="/policies/{name}"}`. Contar ambos daría un `rejected` mayor; es una decisión de diseño, no un defecto.
