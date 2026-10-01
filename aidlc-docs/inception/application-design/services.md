# Services — Agentic QA Swarm

> Despliegue 1:1 por servicio (Q4=A): un Deployment por servicio del control plane + **entorno warm reutilizable** gestionado como workload; reconciliado por Flux. Orquestación: Run Controller central (Q2=A). Comunicación híbrida (Q3=C): REST para UI/API/admin, eventos para el pipeline.

## Servicios del control plane (Deployments estables)

| Servicio | Componentes | Lenguaje (V9) | Expone / consume |
|---|---|---|---|
| `ui-api` | C1 (inbox UI/API Gateway) + lectura C6/C7/C9 | Go o TS (a decidir en NFR) | REST autenticada (C8); rate limiting; security headers |
| `go-intake` | C1 (webhook→notify) | Go | `POST /webhooks/github`; publica `notify.created` |
| `go-run-controller` | C9 (+ aplica C7/C8) | Go | REST `GET /runs/{id}`; consume/publica eventos del pipeline; crea Jobs/rollouts |
| `go-warm-manager` | C3 | Go | Gestiona el entorno warm; rollout/Job `deploy-{run}`; publica `warm.ready`, `deploy.done\|failed`, `surface.ready` |
| `go-reset` | C5 | Go | Job `reset-{run}` + CronJobs `housekeeping`/`rebuild`; publica `reset.verified`, `teardown.verified`; cuarentena y scale-down idle |
| `go-governance` | C7 | Go | REST admin `PUT /policies/*`, `GET /audit`; evaluador de gates |
| `go-identity` | C8 | Go | REST `POST /auth/*`; middleware de auth para `ui-api` |
| `agent-planner` | C3-infer/generación (superficie→flujos) | Python o TS (capa de agentes) | Llama a LLM API off-cluster; sin acceso al test ns |
| `agent-reporter` | C6 | Python o TS (capa de agentes) | Llama a LLM API off-cluster; lee MinIO; redacta reportes |

## Entorno warm del test ns (reutilizable) + Jobs por corrida

| Recurso | Componente | Notas |
|---|---|---|
| app del cliente (Deployment/rollout `deploy-{run}`) | C3 | Desplegada **por corrida sobre el warm**; no se aprovisiona desde cero |
| DB warm (Postgres/Mongo) + Redis warm | SUT (C3) | **Pre-desplegados y reutilizados**; reseteados por C5 entre corridas |
| `rehearsal-{run}` (1 pod) | C2 | Sin credenciales de LLM; gate `ensayo_passed` |
| `runner-{run}-{flow}` | C4 | Motores enchufables (k6 un ejecutor); sin credenciales de LLM; cuota/timeout del workflow |
| `reset-{run}` | C5 | Reset verificado (restart + clean DB + flush cache + verificación → `reset_verified`) |
| `rebuild`/`teardown` (CronJob) | C5 | Higiene periódica: rebuild desde imagen base o teardown total + reprovisionamiento |

## Orquestación (máquina de estados en `go-run-controller`)

```
notify.created → [confirm registrado?] → run.confirmed
  → [warm ready? reset_verified + probes] → deploy.done sobre warm (2 reintentos)
  → surface.ready → plan (workflow) → rehearsal.passed (2 reintentos, gate duro)
  → run.done → reset.verified → report.ready
Fallo/fin de corrida → reset verificado; si falla → cuarentena + handoff.
idle → scale-down; cadencia de higiene → rebuild/teardown.
Cualquier fallo → fail-closed: handoff o reset/cuarentena (nunca reintento infinito).
```

## Comunicación (Q3=C híbrido)

- **REST sincrónico** (UI/API/admin): inbox, confirm, policies, audit, reports, runs, auth. Timeouts explícitos; rate limiting en públicas; security headers en HTML.
- **Eventos del pipeline** (control plane ↔ fases): `notify.created`, `run.confirmed`, `warm.ready`, `deploy.done|failed`, `surface.ready`, `rehearsal.passed|failed`, `run.done`, `reset.verified`, `teardown.verified` (higiene), `report.ready` (+ alerta `warm.quarantined`). Trazado distribuido entre servicios (RESILIENCY-05).
- **K8s nativo** para el entorno warm: rollouts/Jobs, estado vía API de K8s, logs vía `kubectl logs` como insumo del post-mortem.