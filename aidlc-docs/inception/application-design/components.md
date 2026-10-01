# Components — Agentic QA Swarm

> Decisiones aplicadas: 1:1 con M1-M8 + Run Controller central (Q1=A, Q2=A); estilo hexagonal con puertos (Q5=A). Plano estable = C1, C6-C9 (+ C8); **entorno warm reutilizable** = C3 (gestión del warm + deploy por corrida) + C5 (reset verificado/higiene) + C2/C4 como Jobs por corrida dentro del namespace warm. Trazabilidad a requisitos y historias.

## C1 — GitHub Intake + Inbox (M1)

- **Responsabilidades**: recibir eventos GitHub (commit/PR/tag) vía GitHub App de permisos mínimos; crear notificaciones (evento, SHA/PR/tag, artefacto); exponer inbox (UI/API) con estados; registrar confirmación del usuario (persistida/auditable). Nunca crea Jobs.
- **Interfaces**: `POST /webhooks/github` (externa, GitHub); `GET /notifications`, `POST /notifications/{id}/confirm` (UI/API, autenticadas); puerto `ArtifactRef` (repo/imagen según evento).
- **Trazabilidad**: [M1] [US-M1] [V5].

## C2 — Rehearsal / Ensayo (M2)

- **Responsabilidades**: ejecutar un flujo unitario **contra el entorno warm**; verificar 2xx/invariante mínima; publicar `rehearsal.passed|failed`; reintentar máx. 2 veces y escalar a handoff.
- **Interfaces**: Job `rehearsal-{run}` (1 pod) en el test ns warm; lee plan + superficie; escribe resultado al Run Controller. Gate técnico no omitible.
- **Trazabilidad**: [M5-req] [US-M5] [V8] [Principio #2].

## C3 — Warm Environment Manager (M3)

- **Responsabilidades**: gestionar el **entorno warm reutilizable** del namespace de prueba (app + 1 DB (Postgres o Mongo) + 1 Redis **pre-desplegados y reutilizados entre corridas**): health/probes, estados `ready`/`dirty`/`cuarentena`/`idle-escalado`, deploy de la versión del artefacto **por corrida sobre el warm**, inferir la superficie externa y recomendar flujos; publicar `warm.ready` y `deploy.done|failed` (fail-closed tras 2 intentos).
- **Interfaces**: servicio `go-warm-manager`; rollout/Job `deploy-{run}` sobre el warm; puerto `ContainerRuntime` (build/pull); puerto `K8s`; puerto `PolicyStore` (cuotas, cadencia); sin secrets de staging/prod (solo sintéticos/declarados).
- **Trazabilidad**: [M2/M3] [US-M2, US-M3, US-M4] [V5] [Principio #3].

## C4 — QA Runner (M4)

- **Responsabilidades**: ejecutar flujos (artefacto estándar, p. ej. k6) como Jobs contra el Service del SUT **en el entorno warm**; recoger logs/evidencia hacia el Evidence Store; sin llamadas al LLM y sin credenciales de modelo.
- **Interfaces**: Jobs `runner-{run}-{flow}` en el test ns; puerto `Executor` (motor enchufable; k6 un ejecutor posible); puerto `Evidence` (escritura MinIO); enforcement de workflow (cuota/timeout).
- **Trazabilidad**: [M6] [US-M4, US-M6] [Principio #1].

## C5 — Verified Reset & Housekeeping (M5)

- **Responsabilidades**: **reset verificado entre corridas** (restart de servicios + limpieza de DB + flush de cache + job de verificación con probes/checks → `reset_verified=true`); **cuarentena** del warm si el reset falla (escala a humano); **scale-down en idle** (replicas mínimas / pausa de runners); **rebuild periódico o teardown total + reprovisionamiento** como higiene; housekeeping de sesiones abandonadas (grace period 24 h configurable) y persistencia de sesión/plan "incompleta" para reanudar.
- **Interfaces**: Job `reset-{run}` + CronJob `housekeeping`/`rebuild`; publica `reset.verified` (entre corridas) y `teardown.verified` (rebuild/teardown periódico); escribe estado del warm y de sesión.
- **Trazabilidad**: [M7] [US-M7.1, US-M7.2] [V7] [Principio #3].

## C6 — Post-mortem & Reporter (M6)

- **Responsabilidades**: correlacionar logs/evidencia con causa de negocio (capa de agentes, LLM off-cluster); redactar reporte (flujos, invariante, veredicto, enlaces a evidencia cruda); filtrar secretos pre-LLM y pre-publicación; entregar (dashboard; Slack en S1 fuera de este Must).
- **Interfaces**: servicio `agent-reporter`; puerto `LLM` (solo planeación/post-mortem); puerto `Evidence` (lectura MinIO); `GET /reports/{run}` (UI/API).
- **Trazabilidad**: [M9-req/M6] [US-M9] [Principio #1].

## C7 — Governance & Policy (M7)

- **Responsabilidades**: políticas exclusivas de admin (repos/eventos, confirm-required, cuotas de namespace, familias de flujo habilitadas, bloqueo staging/prod); gates (confirm + `ensayo_passed` + dentro del test ns); log de auditoría append-only.
- **Interfaces**: `PUT /policies/*` (admin, autenticado + rol); puerto `PolicyStore`; `GET /audit` (lectura; escritura solo sistema).
- **Trazabilidad**: [M8-req/M7] [US-M8.1, US-M8.2, US-M8.3, UC4].

## C8 — Identity & Access (M8)

- **Responsabilidades**: autenticación (password policy 8+, hashing adaptativo, MFA para admin, sesiones con expiración, anti-brute-force), autorización por rol (deny-by-default, IDOR, función, CORS restringido, validación de tokens), sin credenciales hardcodeadas.
- **Interfaces**: `POST /auth/login|logout`, middleware de auth en UI/API; puerto `SecretStore` (solo secrets del producto).
- **Trazabilidad**: [M8] [NF-SEG-08, NF-SEG-12].

## C9 — Run Controller (orquestador central)

- **Responsabilidades**: máquina de estados persistida por corrida (notify→confirm→**warm ready**→deploy sobre warm→infer→rehearse→run→**reset verificado**→report); crear Jobs por fase; aplicar gates (confirm registrado, `reset_verified=true` previo, `ensayo_passed=true`, test-ns-only, workflow permitido); publicar eventos del pipeline; exponer estado (`GET /runs/{id}`).
- **Interfaces**: servicio `go-run-controller`; consume eventos; invoca puertos K8s/Jobs; consulta C7 (política/workflow) y C8 (auth) en cada transición, y a C3/C5 para el estado del warm (fail-closed ante error).
- **Trazabilidad**: [Transversal M1-M7] [UC1-UC4] [Q2=A].

## Infraestructura compartida (no componentes de dominio)

- **Evidence Store**: MinIO in-cluster (persistencia de logs/reportes/evidencia) — accedido vía puerto `Evidence` por C4 (escritura) y C6 (lectura). [V6].
- **Entorno warm (SUT)**: app del cliente desplegada por corrida sobre 1 DB + 1 Redis pre-desplegados y reutilizados en el test namespace — gestionado por C3, reseteado por C5.
- **Externos**: GitHub App, Slack API (S1), LLM API (solo C-planificador de C1/C3/C4-generación y C6), registry opcional.
- **Cross-cutting**: logging estructurado centralizado (timestamp/request-id/nivel/mensaje, sin secrets/PII); rate limiting en endpoints públicos; timeouts explícitos + circuit breakers + degradación documentada; health shallow+deep; métricas y dashboard.