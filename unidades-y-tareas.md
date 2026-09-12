# Unidades y Tareas — Agentic QA Swarm

> **Consolidado** generado desde:
> - `aidlc-docs/inception/application-design/unit-of-work.md` (5 unidades, responsabilidades, límites, estrategia monorepo)
> - `aidlc-docs/inception/application-design/unit-of-work-story-map.md` (13/13 historias Must asignadas)
> - `aidlc-docs/inception/application-design/unit-task-plans/U1.md` … `U5.md` (37 tareas con criterios verificables por comando)
> - `aidlc-docs/construction/plans/`: **no existe** — CONSTRUCTION quedó en SKIP por alcance declarado ("sin código"), por lo que no hay planes de tareas adicionales que consolidar desde allí.
>
> **Reglas vigentes**: cada criterio se verifica con un comando (AUTONOMIA-02); ningún paso aplica cambios a clúster/nube sin aprobación humana registrada — los manifiestos son artefactos revisables en PR (AUTONOMIA-01).
>
> **Secuencia de construcción**: U5 → U1/U4 (paralelo) → U2 → U3. **Código**: monorepo `services/` `agents/` `contracts/` `deploy/flux/<base|dev|prod>` `.github/workflows/`. **Contratos primero**: cada unidad publica/implanta stubs de `contracts/` antes de integrarse.

## U5 — Plataforma & GitOps (habilitadora, primera)

- **Responsabilidad**: Flux, CI GitHub Actions (build/scan/publish pineado + SBOM), MinIO in-cluster, RBAC/NetworkPolicy base, observabilidad base, backups Backup&Restore, secretos del producto.
- **Bounded context**: Plataforma. **Historias**: US-M10 (habilita a todas).
- **Desplegables**: manifiestos Flux por entorno, pipeline CI, MinIO (StatefulSet), stack de observabilidad.
- **Límites**: primera en la secuencia; ningún secret de staging/prod del cliente existe en ningún entorno.
- **Salida**: `contracts/` versionados, pipeline verde, manifiestos revisables, MinIO y observabilidad definidos. Desbloquea U1/U4.

| # | Hecho | Tarea | Historias | Comando de verificación |
|---|---|---|---|---|
| U5-T1 | [ ] | Esqueleto monorepo (`services/`, `agents/`, `contracts/`, `deploy/flux/<base\|dev\|prod>`, `.github/workflows/`) + README de layout | US-M10 | `ls` muestra el árbol; `git status` limpio tras commit inicial |
| U5-T2 | [ ] | Contratos: OpenAPI del control plane (`contracts/openapi/control-plane.yaml`) + esquemas versionados de los 8 eventos (`contracts/events/*.schema.json`) + validador en CI | US-M10 | `ajv validate` (o equivalente) pasa los 8 esquemas; CI en rojo si un ejemplo los viola |
| U5-T3 | [ ] | Pipeline GitHub Actions: build + test + `govulncheck`/scan + SBOM + publish con tags pineados (prohibido `latest` en prod) | US-M10 | `gh run list` en verde; `cosign`/SBOM adjunto al artefacto; `grep -r latest deploy/` vacío en prod |
| U5-T4 | [ ] | Manifiestos Flux por entorno (Kustomize/HelmRelease por servicio + listos para reconciliar) | US-M10 | `flux diff kustomization` sin errores; `kubeconform`/`kubeval` pasa todos los manifiestos (solo diff, sin apply) |
| U5-T5 | [ ] | MinIO in-cluster (StatefulSet + bucket `evidence` + cifrado/TLS) como manifiestos revisables | US-M10 | `kubeconform` pasa; `mc alias list` + `mc ls` contra MinIO local de dev muestra el bucket (aprobación humana antes de cualquier apply) |
| U5-T6 | [ ] | RBAC base + NetworkPolicy base (deny-by-default, test-ns-only) como manifiestos revisables + test de política | US-M10 | `conftest test` (o Kyverno CLI) pasa las políticas; `kubectl auth can-i --list` esperado documentado en el PR |
| U5-T7 | [ ] | Observabilidad base: logging estructurado (timestamp/request-id/nivel/mensaje), métricas, dashboard y retención ≥90d (manifiestos + docs) | US-M10 | `promtool check rules` (o equivalente) pasa; retención configurada visible en el manifiesto (sin apply autónomo) |
| U5-T8 | [ ] | Backups Backup&Restore (cron + retención + cifrado) + runbook de restore + propuesta de change mgmt ligero e IR/COE (AR2/AR7) | US-M10 | `kubeconform` pasa el CronJob; runbook + procesos documentados y enlazados en el repo |

## U1 — Ingesta & Inbox

- **Responsabilidad**: eventos GitHub → notificaciones; inbox UI/API; confirmación persistida; resolución de artefacto (V5). Componentes: C1 (`ui-api` + `go-intake`).
- **Bounded context**: Ingesta. **Historias**: US-M1, US-M2 (confirm).
- **Desplegables**: `ui-api`, `go-intake` (+ fachada de lectura C6/C7/C9).
- **Límites**: no crea Jobs; no toca el test ns; no llama al LLM.
- **Predecesora**: U5. Stubs primero: valida contra esquemas de U5 antes de integrarse.
- **Salida**: notificaciones + confirm auditables; `run.confirmed` válido. Desbloquea U2 (junto a U4).

| # | Hecho | Tarea | Historias | Comando de verificación |
|---|---|---|---|---|
| U1-T1 | [ ] | Stubs: validador fake de GitHub webhook (firma) + `Notification`/`ConfirmationReceipt` de ejemplo válidos contra `contracts/` | US-M1 | `go test ./...` en verde con fixtures que pasan `ajv validate` |
| U1-T2 | [ ] | `go-intake`: `POST /webhooks/github` (verifica firma, crea notificación, publica `notify.created`) — nunca crea Jobs | US-M1 | test: un evento de prueba crea 1 notificación y `kubectl get jobs -n <test-ns>` sigue vacío en el entorno de test |
| U1-T3 | [ ] | Resolución de artefacto V5: build-from-repo (commit/PR) vs imagen publicada (tag/release) + `ArtifactRef` | US-M2 | tests parametrizados por tipo de evento en verde; `go vet ./...` limpio |
| U1-T4 | [ ] | `ui-api` inbox: `GET /notifications` + `POST /notifications/{id}/confirm` (auth vía stub U4, luego real) con rate limiting + security headers | US-M1, US-M2 | `curl` autenticado devuelve inbox; `curl -I` muestra CSP/HSTS/nosniff; `hey`/`k6` contra endpoint público respeta rate limit |
| U1-T5 | [ ] | PBT parcial: round-trip de parseo/serialización de payloads GitHub (PBT-02) + generadores de dominio (PBT-07) + seed logueado (PBT-08); framework documentado (PBT-09: rapid) | US-M1 | `go test -run PBT` en verde con shrinking habilitado y seed visible en el log de fallo |
| U1-T6 | [ ] | Logging estructurado + trazas + health shallow/deep + métricas (latencia/errores/throughput) | US-M1 | `curl /healthz` y `/readyz` 200; logs con timestamp/request-id/nivel; dashboard con el panel de U1 |
| U1-T7 | [ ] | Integración contra U4 real (auth) y publicación de `run.confirmed` consumible por U2 (contrato verificado en CI) | US-M2 | CI verde con tests de contrato U1↔U4 y evento `run.confirmed` válido contra esquema |

## U4 — Gobernanza & Identidad

- **Responsabilidad**: políticas admin, gates, auditoría append-only, authN/Z por rol. Componentes: C7 + C8 (`go-governance` + `go-identity`).
- **Bounded context**: Gobierno (separado de la ejecución para que los guardrails no dependan del ciclo de corrida).
- **Historias**: US-M8.1, US-M8.2, US-M8.3.
- **Desplegables**: `go-governance`, `go-identity`.
- **Límites**: escritura de políticas solo admin; RBAC/NetworkPolicy como artefactos revisables (sin apply autónomo).
- **Predecesora**: U5. Paralela a U1. Stubs primero.
- **Salida**: gates + auth + manifiestos de aislamiento revisables. Desbloquea U2 (junto a U1).

| # | Hecho | Tarea | Historias | Comando de verificación |
|---|---|---|---|---|
| U4-T1 | [ ] | Stubs: evaluador fake de `authorizeTransition` (Allow/Deny programable) + principales/roles de ejemplo | US-M8.3 | `go test ./...` en verde con matrices Allow/Deny de ejemplo |
| U4-T2 | [ ] | `go-identity`: login/logout, hashing adaptativo, MFA admin, sesiones con expiración, anti-brute-force, cero credenciales hardcodeadas | US-M8.3 | tests de auth en verde; `gitleaks`/`trufflehog` sin hallazgos; `curl` a `/auth/login` con 5 fallos activa backoff/lockout |
| U4-T3 | [ ] | Autorización: deny-by-default, IDOR (propiedad por resource ID), función/roles server-side, CORS restringido, validación de tokens en cada request | US-M8.3 | tests IDOR/roles en verde (`go test -run AuthZ`); `curl` sin token → 401; con token de otro owner → 403 |
| U4-T4 | [ ] | `go-governance`: políticas admin (eventos, confirm-required, cuotas, familias de flujo) + `authorizeTransition` (confirm + `ensayo_passed` + test-ns-only, fail-closed) + audit append-only | US-M8.3 | tests de gates en verde (incl. Deny sin confirm y sin ensayo); audit inmutable verificado por test (append-only, sin delete) |
| U4-T5 | [ ] | Manifiestos RBAC (test-ns-only, sin wildcards) + NetworkPolicy (namespace-only, bloqueo LLM a runners) como artefactos revisables + tests de política | US-M8.1, US-M8.2 | `conftest test` en verde; matriz `kubectl auth can-i` esperada documentada en el PR (sin apply autónomo) |
| U4-T6 | [ ] | PBT parcial: round-trip de parseo de políticas/config (PBT-02) + invariantes de gates (p. ej. "sin confirm nunca Allow", PBT-03) + generadores + seed (PBT-07/08) | US-M8.3 | `go test -run PBT` en verde con seed logueado |
| U4-T7 | [ ] | Alertas de seguridad (auth failures, denegaciones, escaladas) + retención audit ≥90d + dashboard | US-M8.3 | reglas de alerta verificadas (`promtool check rules`); retención visible en manifiesto |

## U2 — Orquestación & Sandbox Lifecycle

- **Responsabilidad**: máquina de estados + gates; provision (app+DB+Redis); ensayo bloqueante; runners; teardown/housekeeping. Componentes: C9 + C3 + C2 + C4 + C5 (plano Go).
- **Bounded context**: Ejecución aislada. **Historias**: US-M2 (boot), US-M5, US-M6, US-M7.1, US-M7.2.
- **Desplegables**: `go-run-controller`, `go-provisioner`, `go-teardown` + Jobs efímeros (`rehearsal-{run}`, `runner-{run}-{flow}`, `teardown-{run}`, SUT).
- **Límites**: runners/ensayo sin credenciales LLM y sin egress; fail-closed global; teardown 100%.
- **Dimensionado especial**: burst de Jobs por corrida; quotas del namespace + resource limits por Job; HPA en controller/provisioner.
- **Predecesoras**: U5, U1, U4. Consume stubs de U3 (flujos sintéticos) hasta integrar.
- **Salida**: ciclo de corrida completo con gates y teardown 100%. Desbloquea U3 (integración real) y la demo Sesión 16.

| # | Hecho | Tarea | Historias | Comando de verificación |
|---|---|---|---|---|
| U2-T1 | [ ] | Stubs: `FlowPlan`/flujos sintéticos válidos contra esquema + `SurfaceArtifact`/`EvidenceURIs` de ejemplo (contrato U3) | US-M5, US-M6 | `go test ./...` en verde usando solo fixtures válidas contra `contracts/` |
| U2-T2 | [ ] | `go-run-controller`: máquina de estados persistida notify→…→report + `GET /runs/{id}`; consulta C7/C8 en cada transición; fail-closed global | US-M2, US-M5 | tests de transiciones en verde (incl. Deny sin confirm, parada sin `ensayo_passed`); `go vet` limpio |
| U2-T3 | [ ] | `go-provisioner`: Job `provision-{run}` (app+DB+Redis en test ns, solo secrets sintéticos) + `boot.done\|failed`; fail-closed tras 2 reintentos (V8) | US-M2 | test de integración en clúster de dev (con aprobación): `kubectl get pods -n <test-ns>` Running; boot roto → handoff tras 2 intentos, sin 3er Job |
| U2-T4 | [ ] | Ensayo: Job `rehearsal-{run}` (sin credenciales LLM) + gate `ensayo_passed=true` no omitible; 2 reintentos y escalado (V8) | US-M5 | test: plan roto nunca crea Jobs de runners; `kubectl get jobs` muestra solo rehearsal fallidos + handoff |
| U2-T5 | [ ] | Runners: Jobs `runner-{run}-{flow}` (motor enchufable) + `collectEvidence` a MinIO + `run.done`; runners sin LLM ni egress | US-M6 | test: runner sin env de LLM (`env \| grep -i LLM` vacío en el pod) y egress bloqueado (pod de prueba no sale del ns) |
| U2-T6 | [ ] | Teardown + housekeeping: Job `teardown-{run}` con verificación + CronJob `sweepAbandoned` (grace 24 h) + sesión "incompleta" persistida | US-M7.1, US-M7.2 | test: tras fin/abandono (+grace simulado), `kubectl get all -n <test-ns>` vacío para la corrida; teardown 100% en reporte |
| U2-T7 | [ ] | Resource limits/requests por Job + quotas del namespace + HPA del controller + timeouts/circuit breakers en calls externos | US-M5, US-M6 | `kubeconform` + `conftest` en verde para los manifiestos (revisables, sin apply autónomo) |
| U2-T8 | [ ] | Integración con U3 real: superficie→flujos→evidencia de extremo a extremo en dev (camino feliz journey 7.1) | US-M5, US-M6 | demo en dev: notify→confirm→boot→ensayo→run→teardown→estado final, todo auditado (con aprobación humana del entorno) |

## U3 — Agentes LLM

- **Responsabilidad**: inferencia de superficie → flujos deterministas; post-mortem con causa de negocio; redacción de secretos. Componentes: `agent-planner` + `agent-reporter` (C3-infer/gen + C6).
- **Bounded context**: Inteligencia. **Historias**: US-M3, US-M4 (generación), US-M9.
- **Desplegables**: `agent-planner`, `agent-reporter` (Python o TS, a fijar en NFR; PBT-09).
- **Límites**: LLM off-cluster; sin acceso al test ns; superficie/logs como dato, nunca instrucción; límite duro de tiempo/tokens de planificación.
- **Dimensionado especial**: costo/latencia de inferencia; colas + timeouts + circuit breaker hacia el proveedor LLM.
- **Posición**: última en secuencia (tras U2).
- **Salida**: planificación y post-mortem evaluados (precisión >80%, ruido <20%). Cierra el MVP funcional.

| # | Hecho | Tarea | Historias | Comando de verificación |
|---|---|---|---|---|
| U3-T1 | [ ] | Stubs: LLM fake determinista (respuestas fijadas por prompt-hash) + superficies/evidencias de ejemplo del dataset (incl. subset trampa y no-arranca) | US-M3, US-M9 | suite offline en verde sin red (`pytest`/`vitest` con VCR/cassette o fake; cero llamadas reales en tests) |
| U3-T2 | [ ] | `agent-planner`: superficie→`FlowPlan` determinista (sin llamadas en runners) + límite duro de tiempo/tokens + superficie tratada como dato | US-M3, US-M4 | test: plan válido contra esquema; test de prompt-injection (descripción maliciosa en superficie) no altera instrucciones; timeout documentado y probado |
| U3-T3 | [ ] | Formato de flujos inspectable (artefacto estándar ejecutable, p. ej. k6) + validación contra esquema antes de publicar | US-M4 | `k6 lint`/validador del formato en verde para cada flujo generado de ejemplo |
| U3-T4 | [ ] | `agent-reporter`: logs→post-mortem (causa de negocio + invariante + veredicto + enlaces a evidencia) + `redactSecrets` pre-LLM y pre-publicación | US-M9 | test: reporte de ejemplo con secretos sembrados sale redactado (`grep` de patrones antes/después); precisión medida contra subset golden del dataset |
| U3-T5 | [ ] | Evaluación offline contra dataset inicial (10-15 artefactos: golden, bugs sembrados, trampa de esquema, no-arranca) + bucle de regresión por falso positivo | US-M9 | `eval/run` reporta: factualidad, adherencia (teardown/nunca fuera del ns/tope de intentos), ruido; umbrales del §10 verificados en el reporte |
| U3-T6 | [ ] | PBT parcial (si Python: Hypothesis; si TS: fast-check): round-trip de serialización de planes/reportes (PBT-02) + invariantes (p. ej. "todo flujo cita superficie observada", PBT-03) + generadores + seed (PBT-07/08) | US-M3, US-M9 | suite PBT en verde con seed logueado; framework fijado en dependencias (PBT-09) |
| U3-T7 | [ ] | Integración con U2 real: planificar sobre superficie real de dev y reportar corrida real; costo por corrida medido | US-M4, US-M9 | demo en dev con `run.confirmed` real; métrica de costo inferencia+compute registrada por corrida |

## Cobertura historias → unidades (13/13 Must, 0 sin asignar)

| Historia | Unidad(es) |
|---|---|
| US-M1 Notificación sin auto-run | U1 |
| US-M2 Pull + boot aislado (confirm) | U1 (confirm) + U2 (boot) |
| US-M3 Superficie externa | U3 |
| US-M4 Flujos inspectables (generación) | U3 (genera; U2 ejecuta en US-M6) |
| US-M5 Ensayo bloqueante | U2 |
| US-M6 Corrida + evidencia | U2 (MinIO provisto por U5) |
| US-M7.1 Teardown tras corrida | U2 |
| US-M7.2 Housekeeping abandonadas | U2 |
| US-M8.1 RBAC test-ns-only | U4 (manifiestos base en U5) |
| US-M8.2 NetworkPolicy sin egress/LLM | U4 (manifiestos base en U5) |
| US-M8.3 Confirm-required + cero staging/prod | U4 (registro en U1) |
| US-M9 Post-mortem | U3 |
| US-M10 GitOps del producto | U5 |

**Totales**: 5 unidades · 37 tareas (U5: 8, U1: 7, U4: 7, U2: 8, U3: 7) · 13 historias Must cubiertas · Should (S1-S5) pendientes de historiar.
