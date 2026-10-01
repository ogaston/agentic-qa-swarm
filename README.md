# Agentic QA Swarm — layout del monorepo

- `services/` — servicios del control plane (Go). Vacío hasta las tareas de U1-U4.
- `agents/` — capa de agentes LLM. Vacío hasta las tareas de U2/U3.
- `contracts/openapi/` — contratos OpenAPI (contenido en U5-T02).
- `contracts/events/` — esquemas de eventos (contenido en U5-T02).
- `deploy/flux/base/` — manifiestos Flux/Kustomize base (U5-T04).
- `deploy/flux/dev/` — overlay del entorno dev (U5-T04).
- `deploy/flux/prod/` — overlay del entorno prod (U5-T04).
- `.github/workflows/` — workflows de GitHub Actions (U5-T03).
