# Application Design — Agentic QA Swarm (consolidado)

> Consolida: components.md, component-methods.md, services.md, component-dependency.md. Decisiones: Q1=A (1:1 M1-M8 + Run Controller), Q2=A (controller central), Q3=C (REST UI/admin + eventos pipeline), Q4=A (un Deployment por servicio + Jobs + Flux), Q5=A (hexagonal).

## Componentes (9 + infra compartida)

- **C1** GitHub Intake + Inbox (M1) · **C2** Rehearsal (M2) · **C3** Warm Environment Manager (M3) · **C4** QA Runner (M4) · **C5** Verified Reset & Housekeeping (M5) · **C6** Post-mortem & Reporter (M6) · **C7** Governance & Policy (M7) · **C8** Identity & Access (M8) · **C9** Run Controller (orquestador, máquina de estados persistida). Infra: MinIO in-cluster (Evidence) + **entorno warm (SUT) en el test ns**; externos: GitHub App, Slack (S1), LLM API (solo planner/reporter), registry opcional.

## Servicios (un Deployment por servicio + entorno warm gestionado)

`ui-api`, `go-intake`, `go-run-controller`, `go-warm-manager`, `go-reset`, `go-governance`, `go-identity`, `agent-planner`, `agent-reporter` (Go ejecución + Python/TS agentes, V9); entorno warm (app desplegada por corrida + DB + Redis reutilizados) con rollouts/Jobs `deploy-{run}`, `rehearsal-{run}`, `runner-{run}-{flow}`, `reset-{run}` y CronJobs `housekeeping`/`rebuild`; todo el control plane reconciliado por Flux (AR9), CI GitHub Actions (AR3/AR8), rollback version-pinned (AR4), despliegue directo (AR5).

## Orquestación y comunicación

Máquina de estados en C9: notify→confirm→warm ready→deploy sobre warm→infer→rehearse→run→reset verificado→report, con gates (confirm registrado, `reset_verified=true` previo, `ensayo_passed=true`, test-ns-only, workflow permitido) y fail-closed global (2 reintentos deploy/ensayo, V8). REST para UI/API/admin; eventos del pipeline (incl. `warm.ready`, `reset.verified`); K8s para el entorno warm.

## Dependencias y puertos

C9 hub (política/workflows C7 + auth C8 en cada transición; estado del warm vía C3/C5); C2/C4 aislados (sin LLM, sin egress); C3/C5 guardianes del warm (reuso solo con `reset_verified`; cuarentena si falla); C6 trata superficie/logs como dato. Puertos: GitHub, ContainerRuntime, K8s, WarmEnv, Executor, Evidence, LLM, PolicyStore, SecretStore, Notify.

## Trazabilidad M → componente → historias

M1→C1→US-M1 · M2(deploy sobre warm)→C3+C9→US-M2 · M3→C3→US-M3 · M4→C3(gen)+C4→US-M4 · M5→C2→US-M5 · M6→C4→US-M6 · M7(reset/higiene)→C5→US-M7.1/M7.2 · M8→C7(+C9/C8)→US-M8.1/M8.2/M8.3 · M9→C6→US-M9 · M10→todos (estable + warm)→US-M10.

## Compliance de extensiones (nivel diseño)

- **Security (bloqueante)**: authN/Z (C8, SEC-08/12), validación de entrada + rate limiting + headers (UI/API, SEC-04/05/11), RBAC mínimo + NetworkPolicy deny-by-default (C7/C5/C9, SEC-06/07), logging estructurado sin secrets (todos, SEC-03), supply chain pineada (CI, SEC-10), fail-closed + handlers + global handler (todos, SEC-15), audit append-only + retención ≥90d + alertas (C7, SEC-14), cifrado reposo/tránsito en persistentes y MinIO (SEC-01), access logging en intermediarios (SEC-02), hardening (SEC-09), integridad/deserialización segura/SRI/auditoría de cambios críticos (SEC-13). Sin *non-compliant* a nivel diseño; la verificación fina (código/IaC) corresponde a CONSTRUCTION (fuera de alcance).
- **Resiliency (direccional)**: RTO/RPO horas + Backup&Restore para persistentes del control plane (AR1); change mgmt ligero a proponer (AR2); GitHub Actions + rollback version-pinned + directo/in-place (AR3-5); observabilidad 3 pilares + health + alarmas (C9/UI); single-region multi-zona como meta, laboratorio single-node como limitación documentada (AR6); timeouts/circuit breakers/degradación (todos los calls); backups automatizados + runbooks + IR/COE ligero a proponer (AR7).
- **PBT parcial (02/03/07/08/09)**: aplica desde Functional Design/NFR (serialización, invariantes, generadores, shrinking/seed, frameworks por lenguaje). N/A en este artefacto.
- **Autonomía (siempre)**: notify≠run, confirm + `reset_verified` + ensayo gates, test-ns-only, reset verificado 100% + higiene de rebuild, runners sin LLM — todos reflejados en C9/C7/C5/C3/C2/C4; ningún paso de diseño propone `apply` autónomo (AUTONOMIA-01 N/A: sin tareas de infra en este documento).