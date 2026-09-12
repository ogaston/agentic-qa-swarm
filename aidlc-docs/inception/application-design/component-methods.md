# Component Methods — Agentic QA Swarm

> Firmas de alto nivel (propósito + I/O). Las reglas de negocio detalladas se definen en Functional Design (CONSTRUCTION). Convención: `nombre(entradas: tipos) -> salida | evento`.

## C1 — GitHub Intake + Inbox

- `handleWebhook(event: GitHubEvent, signature: string) -> Notification | Rejected` — valida firma, crea notificación; nunca crea Jobs.
- `listNotifications(user: Principal, filter: InboxFilter) -> Notification[]` — inbox con evento/SHA/PR/tag/artefacto/estado.
- `confirmRun(notificationId: ID, user: Principal, flows: FlowSel) -> ConfirmationReceipt` — persiste confirmación auditable; publica `run.confirmed`.
- `resolveArtifact(event: GitHubEvent) -> ArtifactRef` — build-from-repo (commit/PR) o imagen publicada (tag/release). [V5]

## C2 — Rehearsal

- `runRehearsal(plan: FlowPlan, sandbox: SandboxRef) -> RehearsalResult{passed: bool, evidence: URI}` — un flujo unitario en el sandbox; publica `rehearsal.passed|failed`.
- `retryOrEscalate(attempt: int, max=2) -> RehearsalResult | Handoff` — fail-closed tras 2 intentos con reporte de motivo. [V8]

## C3 — Sandbox Provisioner

- `provisionSandbox(artifact: ArtifactRef, policy: NamespaceQuota) -> SandboxRef | BootFailed` — app Docker + 1 DB + 1 Redis en el test ns; publica `boot.done|failed`.
- `retryOrEscalate(attempt: int, max=2) -> SandboxRef | Handoff` — fail-closed; handoff estructurado (logs de boot). [V8]
- `inferSurface(sandbox: SandboxRef) -> SurfaceArtifact` — solo superficie externa (OpenAPI expuesta o sondeo de puertos); sin leer fuente. [M3]
- `recommendFlows(surface: SurfaceArtifact) -> FlowPlan` — flujos QA deterministas (capa de agentes, off-cluster).

## C4 — QA Runner

- `executeFlows(plan: FlowPlan, sandbox: SandboxRef, gate: RehearsalPassed) -> RunResult` — requiere `ensayo_passed=true`; crea Jobs `runner-{run}-{flow}`.
- `collectEvidence(run: RunID) -> EvidenceURIs` — logs hacia MinIO vía puerto `Evidence`; publica `run.done`.

## C5 — Teardown & Housekeeping

- `teardownRun(run: RunID) -> TeardownVerified` — destruye app+deps+runners; verifica `kubectl get all -n <test-ns>` vacío para la corrida.
- `sweepAbandoned(gracePeriod: Duration = 24h) -> SweepReport` — teardown de sesiones inactivas; persiste plan como "incompleta". [V7]
- `verifyNamespaceClean(testNs: Namespace) -> bool` — verificación registrada antes de notificar el fin.

## C6 — Post-mortem & Reporter

- `correlatePostmortem(run: RunID, evidence: EvidenceURIs) -> Report` — causa de negocio + invariante + veredicto (LLM off-cluster, logs como dato).
- `redactSecrets(content: bytes) -> bytes` — filtra patrones de secretos pre-LLM y pre-publicación.
- `getReport(run: RunID, user: Principal) -> Report` — reporte + enlaces a evidencia cruda.

## C7 — Governance & Policy

- `setPolicy(scope: PolicyScope, value: Policy, admin: Principal) -> PolicyVersion` — exclusiva admin (eventos, confirm-required, cuotas, familias de flujo).
- `authorizeTransition(run: RunID, gate: Gate) -> Allow | Deny(audit)` — gates: confirm registrado + `ensayo_passed` + test-ns-only; fail-closed.
- `appendAudit(entry: AuditEntry) -> void` — log append-only (el producto no borra sus audit logs).

## C8 — Identity & Access

- `login(credentials: Creds) -> Session | AuthFailed` — policy 8+, hashing adaptativo, anti-brute-force, MFA para admin.
- `authorize(principal: Principal, resource: ResourceID, action: Action) -> Allow | Deny` — deny-by-default, IDOR, función, tokens validados server-side.
- `logout(session: Session) -> void` — invalida sesión server-side.

## C9 — Run Controller

- `startRun(confirmation: ConfirmationReceipt) -> RunID` — crea máquina de estados persistida; publica `run.started`.
- `advance(run: RunID, event: PipelineEvent) -> State` — transiciones notify→confirm→boot→infer→rehearse→run→teardown→report; consulta C7/C8 en cada transición.
- `getRunState(run: RunID, user: Principal) -> RunState` — estado para UI (`GET /runs/{id}`).
- `handleFailure(run: RunID, cause: Failure) -> Handoff | TeardownVerified` — fail-closed global: sin reintento infinito, siempre con teardown o handoff.