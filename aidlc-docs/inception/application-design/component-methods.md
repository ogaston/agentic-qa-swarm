# Component Methods — Agentic QA Swarm

> Firmas de alto nivel (propósito + I/O). Las reglas de negocio detalladas se definen en Functional Design (CONSTRUCTION). Convención: `nombre(entradas: tipos) -> salida | evento`.

## C1 — GitHub Intake + Inbox

- `handleWebhook(event: GitHubEvent, signature: string) -> Notification | Rejected` — valida firma, crea notificación; nunca crea Jobs.
- `listNotifications(user: Principal, filter: InboxFilter) -> Notification[]` — inbox con evento/SHA/PR/tag/artefacto/estado.
- `confirmRun(notificationId: ID, user: Principal, flows: FlowSel) -> ConfirmationReceipt` — persiste confirmación auditable; publica `run.confirmed`.
- `resolveArtifact(event: GitHubEvent) -> ArtifactRef` — build-from-repo (commit/PR) o imagen publicada (tag/release). [V5]

## C2 — Rehearsal

- `runRehearsal(plan: FlowPlan, warm: WarmEnvRef) -> RehearsalResult{passed: bool, evidence: URI}` — un flujo unitario contra el entorno warm; publica `rehearsal.passed|failed`.
- `retryOrEscalate(attempt: int, max=2) -> RehearsalResult | Handoff` — fail-closed tras 2 intentos con reporte de motivo. [V8]

## C3 — Warm Environment Manager

- `ensureWarmReady(warm: WarmEnvRef) -> WarmState` — verifica `reset_verified=true` + probes; si `dirty`/`cuarentena`, delega el reset/rebuild en C5.
- `deployToWarm(artifact: ArtifactRef, warm: WarmEnvRef, workflow: WorkflowId) -> DeployResult | BootFailed` — despliega la versión del artefacto **sobre el warm** (sin aprovisionar DB/Redis desde cero); publica `deploy.done|failed`.
- `retryOrEscalate(attempt: int, max=2) -> DeployResult | Handoff` — fail-closed; handoff estructurado (logs de boot/deploy). [V8]
- `inferSurface(warm: WarmEnvRef) -> SurfaceArtifact` — solo superficie externa (OpenAPI expuesta o sondeo de puertos); sin leer fuente. [M3]
- `recommendFlows(surface: SurfaceArtifact, workflow: WorkflowId) -> FlowPlan` — flujos QA deterministas (capa de agentes, off-cluster) dentro del workflow.
- `setWarmState(warm: WarmEnvRef, state: WarmState) -> void` — publica transiciones `ready`/`dirty`/`cuarentena`/`idle-escalado`.

## C4 — QA Runner

- `executeFlows(plan: FlowPlan, warm: WarmEnvRef, gate: RehearsalPassed) -> RunResult` — requiere `ensayo_passed=true`; crea Jobs `runner-{run}-{flow}`; enforce de cuota/timeout del workflow.
- `collectEvidence(run: RunID) -> EvidenceURIs` — logs hacia MinIO vía puerto `Evidence`; publica `run.done`.

## C5 — Verified Reset & Housekeeping

- `resetVerified(run: RunID) -> ResetVerified` — restart de servicios + limpieza de DB + flush de cache + verificación (probes/checks); publica `reset.verified`; sin él no arranca la siguiente corrida. [M7]
- `quarantine(warm: WarmEnvRef, cause: Failure) -> WarmState` — si el reset falla, marca `cuarentena` y escala a humano.
- `scaleDownIdle(warm: WarmEnvRef) -> WarmState` — replicas mínimas / pausa de runners en idle (recorte de costo).
- `rebuildOrTeardown(warm: WarmEnvRef) -> TeardownVerified` — rebuild desde imagen base o teardown total + reprovisionamiento + verificación; publica `teardown.verified`.
- `sweepAbandoned(gracePeriod: Duration = 24h) -> SweepReport` — reset/limpieza de sesiones inactivas; persiste plan como "incompleta". [V7]

## C6 — Post-mortem & Reporter

- `correlatePostmortem(run: RunID, evidence: EvidenceURIs) -> Report` — causa de negocio + invariante + veredicto (LLM off-cluster, logs como dato).
- `redactSecrets(content: bytes) -> bytes` — filtra patrones de secretos pre-LLM y pre-publicación.
- `getReport(run: RunID, user: Principal) -> Report` — reporte + enlaces a evidencia cruda.

## C7 — Governance & Policy

- `setPolicy(scope: PolicyScope, value: Policy, admin: Principal) -> PolicyVersion` — exclusiva admin (eventos, confirm-required, cuotas del entorno warm, cadencia de higiene, **workflows de negocio**: complejidad, cuotas, timeouts, aprobaciones).
- `authorizeTransition(run: RunID, gate: Gate) -> Allow | Deny(audit)` — gates: confirm registrado + `reset_verified` previo + `ensayo_passed` + test-ns-only + workflow permitido (cuota/timeout/aprobación); fail-closed.
- `appendAudit(entry: AuditEntry) -> void` — log append-only (el producto no borra sus audit logs).

## C8 — Identity & Access

- `login(credentials: Creds) -> Session | AuthFailed` — policy 8+, hashing adaptativo, anti-brute-force, MFA para admin.
- `authorize(principal: Principal, resource: ResourceID, action: Action) -> Allow | Deny` — deny-by-default, IDOR, función, tokens validados server-side.
- `logout(session: Session) -> void` — invalida sesión server-side.

## C9 — Run Controller

- `startRun(confirmation: ConfirmationReceipt) -> RunID` — crea máquina de estados persistida; publica `run.started`.
- `advance(run: RunID, event: PipelineEvent) -> State` — transiciones notify→confirm→warm ready→deploy sobre warm→infer→rehearse→run→reset verificado→report; consulta C7/C8 y el estado del warm (C3/C5) en cada transición.
- `getRunState(run: RunID, user: Principal) -> RunState` — estado para UI (`GET /runs/{id}`).
- `handleFailure(run: RunID, cause: Failure) -> Handoff | ResetVerified` — fail-closed global: sin reintento infinito; siempre deja el warm en `ready` (reset verificado) o en cuarentena + handoff.