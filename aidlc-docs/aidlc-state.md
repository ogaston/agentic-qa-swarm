# AI-DLC State Tracking

## Project Information
- **Project Type**: Greenfield
- **Start Date**: 2026-09-12T02:35:55Z
- **Current Stage**: CONSTRUCTION (Code Generation, por unidad). Unidades U1–U11 cerradas según `unidades-y-tareas.md`; U10-T06 fusionada (PR #146, merge `ad500d1`) tras la ronda 4 VERDE y U11-T01..T06 fusionadas con CA-6 de U11-T02 verificado. Pendiente: ensayo real con webhook de GitHub, túnel y DeepSeek con el humano (`docs/demo-real.md`) y, después, la etapa Build and Test. Actualizado 2026-10-27. Artefactos de INCEPTION realineados al modelo warm el 2026-10-01.

## Workspace State
- **Existing Code**: Sí — monorepo en la raíz del workspace (`services/`, `agents/`, `contracts/`, `demo/`, `deploy/flux/<base|dev|prod|kind>`, `scripts/`, `policy/`, `.github/workflows/`), construido por unidades desde el 2026-10-03 y fusionado a `main`
- **Reverse Engineering Needed**: No (greenfield)
- **Workspace Root**: /home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm

## Code Location Rules
- **Application Code**: Workspace root (NEVER in aidlc-docs/)
- **Documentation**: aidlc-docs/ only
- **Structure patterns**: See code-generation.md Critical Rules

## Extension Configuration
| Extension | Enabled | Decided At |
|---|---|---|
| Límite de autonomía del agente (AUTONOMIA-01/02) | Sí — siempre activa (sin .opt-in.md) | Inicio de sesión |
| Resiliency Baseline | Sí — guía direccional de diseño (P1=A). Decisiones: RTO/RPO horas + Backup&Restore (AR1), change mgmt ligero a proponer (AR2), CI/CD GitHub Actions (AR3/AR8), rollback version-pinned (AR4), despliegue directo/in-place (AR5), single-region multi-zona (AR6), IR/COE ligero a proponer (AR7), GitOps Flux (AR9) | Requirements Analysis + follow-ups |
| Security Baseline | Sí — todas las reglas SECURITY bloqueantes (P2=A) | Requirements Analysis |
| Property-Based Testing | Parcial — solo PBT-02, PBT-03, PBT-07, PBT-08, PBT-09 (P3=B) | Requirements Analysis |

## Cambio de requisitos (fuera de etapa)
- **2026-09-17 — Pivote a entorno warm reutilizable**: detalle en `DECISIONES.md` §5 y `audit.md`. Fuente autoritativa: `specs/prd.md` (§4.2/§4.3, §6, §8 MoSCoW, §9 módulos). Propagado el 2026-10-01 a **todos** los artefactos de INCEPTION, a `unidades-y-tareas.md` y a este estado (plan: `inception/plans/warm-realignment-plan.md`).

## Stage Progress
### 🔵 INCEPTION PHASE
- [x] Workspace Detection (Greenfield)
- [x] Reverse Engineering (N/A — greenfield)
- [x] Requirements Analysis (aprobado; **realineado a warm** 2026-10-01)
- [x] User Stories (13 historias + 3 personas; **realineadas a warm**)
- [x] Workflow Planning (`execution-plan.md`; **realineado a warm**)
- [x] Application Design (5 artefactos; **realineados a warm**)
- [x] Units Generation (`unit-of-work` + dependencias + story-map + planes U1-U5; **realineados a warm** — PUNTO DE PARADA)

### 🟢 CONSTRUCTION PHASE
- Estado: **ACTIVA** desde 2026-10-03 por instrucción explícita del usuario. Etapas de diseño por unidad (Functional/NFR/Infrastructure Design) **omitidas**: el diseño de cada tarea vive en `tareas/U0X-T0N-*.md` (mismo criterio que en U5). Code Generation en curso por unidad.
- [x] U5 — Plataforma & GitOps
- [x] U1 — Ingesta & Inbox
- [x] U4 — Gobernanza & Identidad
- [x] U2 — Orquestación & Warm Sandbox Lifecycle
- [x] U3 — Agentes LLM
- [x] U6 — Frontend web (S5, primera unidad post-MVP)
- [x] U7 — Plataforma en kind local (podman)
- [x] U8 — Producto demostrable (ciclo completo en kind), cerrada 2026-10-21 según unidades-y-tareas.md.
- [x] U9 — Resultados visibles en el dashboard, cerrada 2026-10-22.
- [x] U10 — Fidelidad visual: T01..T06 fusionadas; T06 cerrada en PR #146 (merge `ad500d1`) tras ronda 4 VERDE.
- [x] U11 — T01..T06 fusionadas; CA-6 de T02 verificado (PR #144). Ensayo con webhook real y humano sigue pendiente según docs/demo-real.md.
- [ ] Build and Test (etapa de la fase, tras cerrar las unidades activas)

### 🟡 OPERATIONS PHASE
- Estado: **PLACEHOLDER** (no iniciado).
