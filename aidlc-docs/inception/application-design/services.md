# Services — Agentic QA Swarm

> Despliegue 1:1 por servicio (Q4=A): un Deployment por servicio del control plane + Jobs efímeros; reconciliado por Flux. Orquestación: Run Controller central (Q2=A). Comunicación híbrida (Q3=C): REST para UI/API/admin, eventos para el pipeline.

## Servicios del control plane (Deployments estables)

| Servicio | Componentes | Lenguaje (V9) | Expone / consume |
|---|---|---|---|
| `ui-api` | C1 (inbox UI/API Gateway) + lectura C6/C7/C9 | Go o TS (a decidir en NFR) | REST autenticada (C8); rate limiting; security headers |
| `go-intake` | C1 (webhook→notify) | Go | `POST /webhooks/github`; publica `notify.created` |
| `go-run-controller` | C9 (+ aplica C7/C8) | Go | REST `GET /runs/{id}`; consume/publica eventos del pipeline; crea Jobs |
| `go-provisioner` | C3 | Go | Job `provision-{run}`; publica `boot.done\|failed`, `surface.ready` |
| `go-teardown` | C5 | Go | Job `teardown-{run}` + CronJob `housekeeping`; publica `teardown.verified` |
| `go-governance` | C7 | Go | REST admin `PUT /policies/*`, `GET /audit`; evaluador de gates |
| `go-identity` | C8 | Go | REST `POST /auth/*`; middleware de auth para `ui-api` |
| `agent-planner` | C3-infer/generación (superficie→flujos) | Python o TS (capa de agentes) | Llama a LLM API off-cluster; sin acceso al test ns |
| `agent-reporter` | C6 | Python o TS (capa de agentes) | Llama a LLM API off-cluster; lee MinIO; redacta reportes |

## Jobs efímeros del test ns (viven y mueren con la corrida)

| Job | Componente | Notas |
|---|---|---|
| `rehearsal-{run}` (1 pod) | C2 | Sin credenciales de LLM; gate `ensayo_passed` |
| `runner-{run}-{flow}` | C4 | Motores enchufables (k6 un ejecutor); sin credenciales de LLM |
| `teardown-{run}` | C5 | Verificación de namespace limpio |
| app + DB (Postgres/Mongo) + Redis | SUT (C3) | Por corrida; destruidos por C5 |

## Orquestación (máquina de estados en `go-run-controller`)

```
notify.created → [confirm registrado?] → run.confirmed → boot.done (2 reintentos)
  → surface.ready → plan listo → rehearsal.passed (2 reintentos, gate duro)
  → run.done → teardown.verified → report.ready
Cualquier fallo → fail-closed: handoff o teardown (nunca reintento infinito).
```

## Comunicación (Q3=C híbrido)

- **REST sincrónico** (UI/API/admin): inbox, confirm, policies, audit, reports, runs, auth. Timeouts explícitos; rate limiting en públicas; security headers en HTML.
- **Eventos del pipeline** (control plane ↔ fases): `notify.created`, `run.confirmed`, `boot.done|failed`, `surface.ready`, `rehearsal.passed|failed`, `run.done`, `teardown.verified`, `report.ready`. Trazado distribuido entre servicios (RESILIENCY-05).
- **K8s nativo** para el plano efímero: Jobs, estado vía API de K8s, logs vía `kubectl logs` como insumo del post-mortem.