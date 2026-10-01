# Component Dependencies — Agentic QA Swarm

## Matriz de dependencias (→ = depende de)

| De \ A | C1 Intake | C2 Ensayo | C3 Warm Mgr | C4 Runner | C5 Reset | C6 Reporter | C7 Governance | C8 Identity | C9 Controller | MinIO | Ext (GH/Slack/LLM) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| C1 | — | | | | | | | | | | GH (eventos) |
| C2 | plan (C1/C3) | — | warm | | | | gate | | estado | | |
| C3 | confirm (C1) | | — | | reset/rebuild (C5) | | cuotas/cadencia | | estado | | registry/LLM(superficie) |
| C4 | plan | gate ensayo | warm | — | | | gate/workflow | | estado | escribe | |
| C5 | | | warm | runners | — | | | | estado | | |
| C6 | | | | evidencia | | — | | | estado | lee | LLM |
| C7 | | | | | | | — | roles | | | |
| C8 | | | | | | | | — | | | SecretStore |
| C9 | | | | | | | política | auth | — | | |
| UI/API | C1/C6/C7/C9 lectura | | | | | | | auth | | | |

## Patrones de comunicación

- **Sincrónico REST** (control plane ↔ UI/admin): inbox, confirm, policies, audit, reports, runs, auth. Contrato: OpenAPI del control plane; validación de entrada (NF-SEG-05); auth en cada request (NF-SEG-08/12).
- **Eventos del pipeline** (fases → controller → siguiente fase): los eventos de services.md (10, incl. `warm.ready`/`deploy.done`/`reset.verified`); anuncios de estado, no comandos con lógica de negocio.
- **K8s Jobs/rollouts** (controller → test ns): creación/observabilidad de rollouts/Jobs por fase sobre el entorno warm; RBAC confinado; NetworkPolicy namespace-only.
- **Puertos hexagonales** (Q5=A): `GitHub`, `ContainerRuntime`, `K8s`, `WarmEnv` (entorno warm: deploy/health/reset), `Executor`, `Evidence` (MinIO), `LLM`, `PolicyStore`, `SecretStore`, `Notify` (Slack S1). El dominio (corridas, gates, políticas, warm) no conoce SDKs externos.

## Flujos de datos principales

1. **Notify**: GitHub → C1 → `notify.created` → C9 (estado) → UI inbox.
2. **Confirm→Warm ready→Deploy**: UI → C1.confirm → `run.confirmed` → C9 → C3 `ensureWarmReady` (`warm.ready`) → C3 `deployToWarm` → `deploy.done` + `surface.ready` (+ plan de `agent-planner` según workflow).
3. **Ensayo→Run**: C9 → C2 Job contra el warm → `rehearsal.passed` (gate) → C9 → C4 Jobs → `run.done` + evidencia a MinIO.
4. **Reset→Report**: C9 → C5 Job → `reset.verified` → C6 (`agent-reporter` + LLM) → `report.ready` → UI. Higiene periódica: C5 → `teardown.verified` (rebuild/teardown); idle → scale-down.
5. **Gobernanza**: Admin → C7 (política/workflows) → C9 la consulta en cada transición; todo al audit append-only.

## Acoplamiento y riesgos

- C9 es el hub: si cae, las corridas en curso quedan huérfanas → mitigación: estados persistidos + housekeeping de C5 + health deep + alertas (RESILIENCY-05/06/15).
- C4/C2 nunca hablan con LLM ni salen del test ns (aislamiento por construcción, no por promesa).
- C3/C5 son los guardianes del warm: reuso **solo** con `reset_verified=true`; sin él, cuarentena. Riesgo #1 = **contaminación de estado entre corridas** → mitigado por reset verificado + rebuild periódico.
- C6 trata superficie/logs como dato, nunca como instrucción (red-team PRD §11 esc. 1-2).