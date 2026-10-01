# Unit of Work Dependencies — Agentic QA Swarm

> Matriz entre unidades (→ = depende de). Sin circulares. Secuencia de construcción (3A): U5 → U1/U4 → U2 → U3. Integración por contratos mockeados primero (2B).

## Matriz

| De \ A | U1 Ingesta | U2 Ejecución | U3 Agentes | U4 Gobierno | U5 Plataforma |
|---|---|---|---|---|---|
| **U1** | — | | | auth/roles (C8) | CI/CD, observabilidad, secretos |
| **U2** | confirm + plan (C1) | — | flujos (plan) | política/gates/cuotas (C7), auth (C8) | entorno warm (manifiestos), Jobs/RBAC/ns, MinIO, observabilidad |
| **U3** | | superficie+evidencia (C3/C4) | — | | LLM-egress permitido solo aquí, MinIO, CI |
| **U4** | | | | — | CI/CD, observabilidad, audit store |
| **U5** | | | | | — (base; sin dependencias internas) |

## Contratos por dependencia (stubs primero, 2B)

- **U1→U4**: `POST /auth/*` + middleware (stub: validador fake de tokens/roles).
- **U2→U1**: `run.confirmed`, `FlowPlan` (stub: confirmaciones y planes de ejemplo).
- **U2→U4**: gates `authorizeTransition` + cuotas + workflows (stub: política/workflow permisivo/denegatorio de prueba; incl. `reset_verified`).
- **U2→U3**: `FlowPlan` consumido / Flujos generados (stub: flujos sintéticos válidos).
- **U3→U2**: `SurfaceArtifact`, `EvidenceURIs` (stub: superficies y evidencias de ejemplo).
- **U3→proveedor LLM**: cliente con timeout/circuit breaker (stub: LLM fake determinista para tests).
- **Todas→U5**: esquemas de los eventos del pipeline (`contracts/events/*.schema.json`), OpenAPI del control plane, manifiestos del entorno warm, MinIO S3-compatible (stub: MinIO local o fake S3 en tests).
- **U4→U5**: audit store append-only (stub: log en fichero con retención simulada).

## Orden y paralelismo

1. **U5** primero (habilitadora): Flux, CI, MinIO, observabilidad, RBAC/NetworkPolicy base, contratos, **manifiestos del entorno warm**.
2. **U1 + U4 en paralelo** (tras U5): ingesta e identidad/gobierno contra stubs.
3. **U2** (tras U1/U4): controller + **lifecycle del entorno warm (deploy/reset/higiene)** contra stubs de U3 (flujos sintéticos) y política real de U4.
4. **U3** última (tras U2): agentes contra superficies/evidencias stub y luego reales.
5. Integración final: verificación cruzada de esquemas en CI + demo Sesión 16 (camino feliz, bloqueo de escape, fail-closed).