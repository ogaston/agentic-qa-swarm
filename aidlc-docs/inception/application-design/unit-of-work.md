# Unit of Work — Agentic QA Swarm

> Descomposición aprobada (Preguntas 1-6): 5 unidades, monorepo, secuencia U5→U1/U4→U2→U3, contratos mockeados primero, U2/U3 con dimensionado especial.

## U1 — Ingesta & Inbox

- **Responsabilidad**: eventos GitHub → notificaciones; inbox UI/API; confirmación persistida; resolución de artefacto (V5). Componentes: C1 (ui-api + go-intake).
- **Bounded context**: Ingesta. **Historias**: US-M1, US-M2 (confirm).
- **Desplegables**: `ui-api`, `go-intake` (+ resto de UI/API Gateway como fachada de lectura C6/C7/C9).
- **Límites**: no crea Jobs; no toca el test ns; no llama al LLM.

## U2 — Orquestación & Sandbox Lifecycle

- **Responsabilidad**: máquina de estados + gates; provision (app+DB+Redis); ensayo bloqueante; runners; teardown/housekeeping. Componentes: C9 + C3 + C2 + C4 + C5 (plano Go).
- **Bounded context**: Ejecución aislada. **Historias**: US-M2 (boot), US-M5, US-M6, US-M7.1, US-M7.2.
- **Desplegables**: `go-run-controller`, `go-provisioner`, `go-teardown` + Jobs efímeros (`rehearsal-{run}`, `runner-{run}-{flow}`, `teardown-{run}`, SUT).
- **Límites**: runners/ensayo sin credenciales LLM y sin egress; fail-closed global; teardown 100%.
- **Dimensionado especial (4A)**: burst de Jobs por corrida; quotas del namespace + resource limits por Job; HPA en controller/provisioner.

## U3 — Agentes LLM

- **Responsabilidad**: inferencia de superficie → flujos deterministas; post-mortem con causa de negocio; redacción de secretos. Componentes: agent-planner + agent-reporter (C3-infer/gen + C6).
- **Bounded context**: Inteligencia. **Historias**: US-M3, US-M4 (generación), US-M9.
- **Desplegables**: `agent-planner`, `agent-reporter` (Python o TS, a fijar en NFR; PBT-09).
- **Límites**: LLM off-cluster; sin acceso al test ns; superficie/logs como dato, nunca instrucción; límite duro de tiempo/tokens de planificación.
- **Dimensionado especial (4A)**: costo/latencia de inferencia; colas + timeouts + circuit breaker hacia el proveedor LLM.

## U4 — Gobernanza & Identidad

- **Responsabilidad**: políticas admin, gates, auditoría append-only, authN/Z por rol. Componentes: C7 + C8 (go-governance + go-identity).
- **Bounded context**: Gobierno (separado de la ejecución para que los guardrails no dependan del ciclo de corrida).
- **Historias**: US-M8.1, US-M8.2, US-M8.3.
- **Desplegables**: `go-governance`, `go-identity`.
- **Límites**: escritura de políticas solo admin;RBAC/NetworkPolicy como artefactos revisables (sin apply autónomo).

## U5 — Plataforma & GitOps

- **Responsabilidad transversal habilitadora**: Flux (reconciliación), CI GitHub Actions (build/scan/publish pineado + SBOM), MinIO in-cluster, RBAC/NetworkPolicy base, observabilidad base (logging centralizado, métricas, dashboard), backups (Backup&Restore, AR1), secretos del producto.
- **Bounded context**: Plataforma. **Historias**: US-M10 (habilita a todas).
- **Desplegables**: manifiestos Flux por entorno, pipeline CI, MinIO (StatefulSet), stack de observabilidad.
- **Límites**: primera en la secuencia (3A); ningún secret de staging/prod del cliente existe en ningún entorno.

## Estrategia de organización de código (greenfield, 6A monorepo)

```
/
  services/<ui-api|go-intake|go-run-controller|go-provisioner|go-teardown|go-governance|go-identity>/
    cmd/ internal/ api/openapi.yaml Dockerfile
  agents/<planner|reporter>/
    src/ tests/ requirements.txt|package.json Dockerfile
  contracts/
    events/*.schema.json        # esquemas versionados de los 8 eventos (stubs primero, 2B)
    openapi/control-plane.yaml
  deploy/flux/<base|dev|prod>/
    <svc>/ helmrelease|kustomization.yaml + NetworkPolicy + RBAC (revisables, sin apply autónomo)
  .github/workflows/           # CI: build, scan, sbom, publish pineado
  aidlc-docs/                  # documentación (este flujo)
```

- Contratos primero (2B): cada unidad publica/implanta stubs de `contracts/` antes de integrarse; la CI verifica compatibilidad de esquemas.
- Cada servicio Go: `go.mod` propio o workspace; agentes: Hypothesis (Python) o fast-check (TS) según NFR (PBT-09); lock files siempre (SEC-10).