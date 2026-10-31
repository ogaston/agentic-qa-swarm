# AI-DLC State Tracking

## Project Information
- **Project Type**: Greenfield
- **Start Date**: 2026-09-12T02:35:55Z
- **Current Stage**: CONSTRUCTION (Code Generation, por unidad). Unidades U1–U11 cerradas según `unidades-y-tareas.md`; U10-T06 fusionada (PR #146, merge `ad500d1`) tras la ronda 4 VERDE y U11-T01..T06 fusionadas con CA-6 de U11-T02 verificado. Caso sano de la demo real verificado el 2026-10-28: run-d3fdad3c9118b08a done/sin-hallazgos, 7/7 flujos y reset verificado. Pendiente: caso con defecto oversell del guion (`docs/demo-real.md`) y, después, la etapa Build and Test. No se avanza de etapa por esta revisión. Artefactos de INCEPTION realineados al modelo warm el 2026-10-01.

## Workspace State

- **Run summary changes (2026-10-28)**: Timeline, flow outcomes, runner evidence downloads, waiting panel, and summary copy updated. Dashboard build/lint and 271 tests pass; ui-api 324 Go tests pass; reporter artifact validation passes. Browser and deployed storage verification remain pending. Other S3 evidence references retain copy fallback. See `construction/run-summary/code/verification.md`. No deployment or stage transition.
- **Login persistence and Inbox updates (2026-10-28)**: Approved plan implemented: versioned localStorage session with server validation on restore and cleanup on logout/expiration/401; table-header tabs and global statistics from unfiltered notifications; relative receipt time from persisted event timestamps via additive REST created_at; seven-character artifact suffix with hover/focus tooltip; GitHub repository/commit links in both modal steps with external arrows and keyboard focus trapping. All 270 dashboard tests, lint, build, API type check and ui-api Go tests pass. Fixture-backed production browser checks at 1440/390/375px pass under the existing CSP, including login/reload/logout, tooltip viewport bounds and modal navigation. Evidence: `construction/login-inbox-updates/code/verification.md`. Other concurrent demo/script changes preserved. Ready for review; no deployment or global stage transition.
- **Live tests in run detail (2026-10-28)**: Inspected request for `run-7e56e11a98a8bce7`: accessible loading indicator for the current phase and immediate visibility of test flows while running. Plan approved with “go ahead” and implemented. Current phase now shows an accessible spinner; tests appear expanded before the pending report while running. Plan readiness retries are bounded and renewed immediately on entering execution. All 250 dashboard tests, lint and build pass; fixture-backed browser checks at 1440/390/375px pass with no overflow or page errors. Evidence: `construction/run-live-tests/code/verification.md`. Existing local changes preserved; ready for review, no deployment or stage transition.
- **Demo warm wake**: Setup wakes idle warm via warm-manager before preflight; setup regression tests and live wake passed. No reset performed.
- **Interactive demo launcher (2026-10-28)**: Approved plan implemented in `scripts/demo/demo.sh`; setup, dashboard credentials and URLs, existing SHA or commit, and human-confirmed run. Bash syntax and mocked SHA, commit, invalid SHA and display checks pass. No cluster deployment or push performed.
- **Customer run view (2026-10-28)**: Approved plan implemented: customer copy, internal-detail removal, evidence copying and responsive visual polish. All 242 dashboard tests, lint and build pass. Fixture-backed browser verification at 1440/390/375px: no overflow or page errors; expanded flows and failed state checked. Evidence: `construction/run-customer-ui/code/verification.md`. Inner flow table additionally simplified per user screenshot: open layout, quiet sentence-case headers and plain code; build and desktop/mobile browser checks pass. Ready for review; existing local changes preserved; no overall stage transition.
- **Run creation race (2026-10-28)**: Approved correction implemented in RunPage: bounded initial-404 retries, accessible waiting state, five-failure exhaustion with manual retry, and corrected header. Regression demonstrated before implementation (5 failures); all 17 RunPage tests and 241 dashboard tests now pass, plus lint, API schema check, production build and diff whitespace check. Evidence: `construction/run-creation-race/code/verification.md`; all plan steps complete. Ready for code review; existing local changes preserved. No deployment or global stage transition.
- **Modal «Revisar notificación» (2026-10-28)**: Rediseño aplicado a ambos pasos con el sistema editorial vigente: jerarquía de cabecera, progreso, detalles separados, selección de flujos y pie de acciones adaptable. Build, lint y 237 pruebas pasan; verificación en navegador a 1440, 390 y 375 px sin desbordamiento ni errores de página. Cambios locales anteriores preservados; sin despliegue ni transición de etapa global.
- **Dashboard customer UI (2026-10-28)**: Plan aprobado con «hazlo ahora!!!!» y completado. Cabecera, tarjetas de Inbox, estado vacío de Mis corridas y contenido de Entorno implementados. Build, lint y 234 pruebas pasan; navegador local a 1440/390 px sin desbordamiento ni pageerror, con navegación, cierre de sesión, tabla móvil y estados del entorno verificados. Evidencia: `construction/dashboard-customer-ui/code/verification.md`. Cambios locales anteriores preservados; sin despliegue ni transición de etapa global.
- **Landing preview labels and containers (2026-10-28)**: Removed the two hero floating label containers and environment badge as requested. User clarified removal of width/padding wrappers everywhere: removed all seven `ed-container` class usages and their CSS rules, preserving section contents. Moved decorative corner marks inside horizontal boundaries to avoid overflow. Build and 3 existing landing tests pass; browser checks at 1440px and 390px confirm no horizontal overflow or page errors. No overall stage transition.
- **Landing CTA background (2026-10-28)**: Added a self-hosted orbital background to “Ship without the QA backlog”. Dashboard build and 3 existing landing tests pass; desktop (1440px) and mobile (390px) screenshots inspected, with no horizontal overflow or browser page errors. Plan: `construction/plans/landing-cta-background-plan.md`; evidence: `construction/landing-cta-background/code/verification.md`. Ready for review; no overall stage transition.
- **Corrida sana verificada (2026-10-28T15:10:48Z)**: `run-d3fdad3c9118b08a`, SHA `460b66b36e0db2ad12e7a7437fbfa43039541692`, done/sin-hallazgos; 7/7 runners ok, ensayo completo, imagen coincidente y warm ready/reset_verified=true. Evidencia: `construction/demo-setup/code/run-d3fdad3c9118b08a-verification.md`. Caso oversell no ejecutado en esta revisión; sin avance de etapa.
- **Continuidad de demo (2026-10-28)**: Paso 0 recuperado en `feat/scripts-demo`. Imagen de referencia de `warm-app` (`b97a719…`) restaurada y rollout exitoso tras aprobación «hazlo». El primer humo confirmó una notificación sintética, efecto no advertido del script histórico; incidente informado y auditado. Setup ahora usa `kind-smoke.sh --preflight`, sin confirmaciones, y pasan 13 pruebas locales. Verificación final: setup rc=0 y humo con 0 FALLA; warm ready/reset_verified=true. El asistente no ejecutó el reset propuesto; la solicitud adicional quedó obsoleta. Plan completado: `construction/plans/demo-setup-continuity-plan.md`; evidencia: `construction/demo-setup/code/verification.md`, `setup-final.log` y `smoke-final.log` en ese directorio. Demo real pendiente; no se ha avanzado de etapa.
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
