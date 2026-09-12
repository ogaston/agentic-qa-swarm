# Component Dependencies — Agentic QA Swarm

## Matriz de dependencias (→ = depende de)

| De \ A | C1 Intake | C2 Ensayo | C3 Provision | C4 Runner | C5 Teardown | C6 Reporter | C7 Governance | C8 Identity | C9 Controller | MinIO | Ext (GH/Slack/LLM) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| C1 | — | | | | | | | | | | GH (eventos) |
| C2 | plan (C1/C3) | — | sandbox | | | | gate | | estado | | |
| C3 | confirm (C1) | | — | | | | cuotas | | estado | | registry/LLM(superficie) |
| C4 | plan | gate ensayo | sandbox | — | | | gate | | estado | escribe | |
| C5 | | | sandbox | runners | — | | | | estado | | |
| C6 | | | | evidencia | | — | | | estado | lee | LLM |
| C7 | | | | | | | — | roles | | | |
| C8 | | | | | | | | — | | | SecretStore |
| C9 | | | | | | | política | auth | — | | |
| UI/API | C1/C6/C7/C9 lectura | | | | | | | auth | | | |

## Patrones de comunicación

- **Sincrónico REST** (control plane ↔ UI/admin): inbox, confirm, policies, audit, reports, runs, auth. Contrato: OpenAPI del control plane; validación de entrada (NF-SEG-05); auth en cada request (NF-SEG-08/12).
- **Eventos del pipeline** (fases → controller → siguiente fase): los 8 eventos de services.md; anuncios de estado, no comandos con lógica de negocio.
- **K8s Jobs** (controller → test ns): creación/observabilidad de Jobs por fase; RBAC confinado; NetworkPolicy namespace-only.
- **Puertos hexagonales** (Q5=A): `GitHub`, `ContainerRuntime`, `K8s`, `Executor`, `Evidence` (MinIO), `LLM`, `PolicyStore`, `SecretStore`, `Notify` (Slack S1). El dominio (corridas, gates, políticas) no conoce SDKs externos.

## Flujos de datos principales

1. **Notify**: GitHub → C1 → `notify.created` → C9 (estado) → UI inbox.
2. **Confirm→Boot**: UI → C1.confirm → `run.confirmed` → C9 → C3 provision → `boot.done` + `surface.ready` (+ plan de `agent-planner`).
3. **Ensayo→Run**: C9 → C2 Job → `rehearsal.passed` (gate) → C9 → C4 Jobs → `run.done` + evidencia a MinIO.
4. **Teardown→Report**: C9 → C5 Job → `teardown.verified` → C6 (`agent-reporter` + LLM) → `report.ready` → UI.
5. **Gobernanza**: Admin → C7 (política) → C9 la consulta en cada transición; todo al audit append-only.

## Acoplamiento y riesgos

- C9 es el hub: si cae, las corridas en curso quedan huérfanas → mitigación: estados persistidos + housekeeping de C5 + health deep + alertas (RESILIENCY-05/06/15).
- C4/C2 nunca hablan con LLM ni salen del test ns (aislamiento por construcción, no por promesa).
- C6 trata superficie/logs como dato, nunca como instrucción (red-team PRD §11 esc. 1-2).