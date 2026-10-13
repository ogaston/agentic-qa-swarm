# AI-DLC State Tracking

## Project Information
- **Project Type**: Greenfield
- **Start Date**: 2026-09-12T02:35:55Z
- **Current Stage**: CONSTRUCTION (Code Generation, por unidad) — activada por instrucción explícita el 2026-10-03. **Terminadas: U5, U1, U4, U2, U3, U6 y U7.** En curso: **U8 — Producto demostrable (ciclo completo en kind)**, con 7 de sus 9 tareas terminadas (T01–T07); pendientes T08 y T09. Estado al día el 2026-10-13. Artefactos de INCEPTION **realineados al modelo warm** el 2026-10-01.

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
- [ ] U8 — Producto demostrable (ciclo completo en kind): T01–T07 hechas; pendientes T08 (`kind-e2e.sh` de punta a punta) y T09 (LiteLLM + DeepSeek, app definitiva y webhook real de GitHub, con insumos del humano)
- [ ] Build and Test (etapa de la fase, tras cerrar U8)

### 🟡 OPERATIONS PHASE
- Estado: **PLACEHOLDER** (no iniciado).
