# Unidades y Tareas — Agentic QA Swarm

> **Consolidado** generado desde:
> - `aidlc-docs/inception/application-design/unit-of-work.md` (5 unidades, responsabilidades, límites, estrategia monorepo)
> - `aidlc-docs/inception/application-design/unit-of-work-story-map.md` (13/13 historias Must asignadas)
> - `aidlc-docs/inception/application-design/unit-task-plans/U1.md` … `U5.md` (37 tareas con criterios verificables por comando)
> - `aidlc-docs/construction/plans/`: **no existe** — CONSTRUCTION quedó en SKIP por alcance declarado ("sin código"), por lo que no hay planes de tareas adicionales que consolidar desde allí.
>
> **Modelo warm (2026-09-17)**: el SUT es un **entorno warm propio de la plataforma** en un namespace de prueba de vida larga (app + 1 DB + 1 Redis pre-desplegados y **reutilizados entre corridas**), aislado de staging/prod. Cada corrida **despliega el artefacto sobre el warm**; entre corridas aplica **reset verificado** (`reset_verified=true`; cuarentena si falla); **scale-down en idle**; **rebuild/teardown periódico** como higiene; complejidad por **workflows de negocio** (cuotas/timeouts/aprobaciones). Gate añadido: `reset_verified=true` previo a cada corrida. KPI: **reset verificado 100% + higiene de rebuild** (reemplaza teardown 100%). Riesgo #1: contaminación entre corridas.
>
> **Reglas vigentes**: cada criterio se verifica con un comando (AUTONOMIA-02); ningún paso aplica cambios a clúster/nube sin aprobación humana registrada — los manifiestos son artefactos revisables en PR (AUTONOMIA-01).
>
> **Secuencia de construcción**: U5 → U1/U4 (paralelo) → U2 → U3. **Código**: monorepo `services/` `agents/` `contracts/` `deploy/flux/<base|dev|prod>` `.github/workflows/`. **Contratos primero**: cada unidad publica/implanta stubs de `contracts/` antes de integrarse.

## U5 — Plataforma & GitOps (habilitadora, primera)

- **Responsabilidad**: Flux, CI GitHub Actions (build/scan/publish pineado + SBOM), MinIO in-cluster, **manifiestos del entorno warm (app + DB + Redis + CronJobs de reset/rebuild + scale-down en idle) como workload GitOps**, RBAC/NetworkPolicy base, observabilidad base, backups Backup&Restore, secretos del producto.
- **Bounded context**: Plataforma. **Historias**: US-M10 (habilita a todas).
- **Desplegables**: manifiestos Flux por entorno (control plane + entorno warm), pipeline CI, MinIO (StatefulSet), stack de observabilidad.
- **Límites**: primera en la secuencia; ningún secret de staging/prod del cliente existe en ningún entorno.
- **Salida**: `contracts/` versionados, pipeline verde, manifiestos revisables (control plane **+ entorno warm**), MinIO y observabilidad definidos. Desbloquea U1/U4.

| # | Hecho | Tarea | Historias | Comando de verificación |
|---|---|---|---|---|
| U5-T1 | [x] | Esqueleto monorepo (`services/`, `agents/`, `contracts/`, `deploy/flux/<base\|dev\|prod>`, `.github/workflows/`) + README de layout | US-M10 | `ls` muestra el árbol; `git status` limpio tras commit inicial |
| U5-T2 | [x] | Contratos: OpenAPI del control plane (`contracts/openapi/control-plane.yaml`) + esquemas versionados de los 10 eventos del pipeline (`contracts/events/*.schema.json`, incl. `warm.ready`/`deploy.done`/`reset.verified`) + validador en CI | US-M10 | `ajv validate` (o equivalente) pasa los 10 esquemas; CI en rojo si un ejemplo los viola |
| U5-T3 | [x] | Pipeline GitHub Actions: build + test + `govulncheck`/scan + SBOM + publish con tags pineados (prohibido `latest` en prod) | US-M10 | `gh run list` en verde; `cosign`/SBOM adjunto al artefacto; `grep -r latest deploy/` vacío en prod |
| U5-T4 | [x] | Manifiestos Flux por entorno (Kustomize/HelmRelease por servicio + listos para reconciliar), **incluido el entorno warm** (app+DB+Redis + CronJobs `housekeeping`/`rebuild` + scale-down en idle) | US-M10 | `flux diff kustomization` sin errores; `kubeconform`/`kubeval` pasa todos los manifiestos (solo diff, sin apply) |
| U5-T5 | [x] | MinIO in-cluster (StatefulSet + bucket `evidence` + cifrado/TLS) como manifiestos revisables | US-M10 | `kubeconform` pasa; `mc alias list` + `mc ls` contra MinIO local de dev muestra el bucket (aprobación humana antes de cualquier apply) |
| U5-T6 | [x] | RBAC base + NetworkPolicy base (deny-by-default, test-ns-only) como manifiestos revisables + test de política | US-M10 | `conftest test` (o Kyverno CLI) pasa las políticas; `kubectl auth can-i --list` esperado documentado en el PR |
| U5-T7 | [x] | Observabilidad base: logging estructurado (timestamp/request-id/nivel/mensaje), métricas, dashboard y retención ≥90d (manifiestos + docs) | US-M10 | `promtool check rules` (o equivalente) pasa; retención configurada visible en el manifiesto (sin apply autónomo) |
| U5-T8 | [x] | Backups Backup&Restore (cron + retención + cifrado) + runbook de restore + propuesta de change mgmt ligero e IR/COE (AR2/AR7) | US-M10 | `kubeconform` pasa el CronJob; runbook + procesos documentados y enlazados en el repo |
| U5-T9 | [x] | CronJobs `housekeeping`/`rebuild` a `aqs-system` (SA `go-reset`) + política de aislamiento de `aqs-test` *(surgida en el loop: C-13)* | US-M10 | `tareas/U5-T09-cronjobs-al-control-plane.md` |
| U5-T10 | [x] | Política contra egress abierto en `aqs-test` *(surgida en el loop: C-19)* | US-M10 | `tareas/U5-T10-politica-egress.md` |
| U5-T11 | [x] | Namespace explícito en `minio-init` + política contra objetos sin namespace *(surgida en el loop: C-31, C-32)* | US-M10 | `tareas/U5-T11-namespace-explicito.md` |
| U5-T12 | [x] | La regla de `ipBlock` no se desactiva con `null` *(surgida en el loop: C-34)* | US-M10 | `tareas/U5-T12-ipblock-null.md` |

> **U5 terminada (2026-10-01)**, marcada por instrucción del humano tras fusionar los PRs #1-#12. Las tareas U5-T9..T12 surgieron de defectos encontrados durante las rondas de revisión. Los criterios de verificación finales de cada tarea están en `tareas/U5-T*.md`, la evidencia en `revisiones/U5-T*/` y las tareas candidatas pendientes en `tareas/candidatas.md`.

## U1 — Ingesta & Inbox

- **Responsabilidad**: eventos GitHub → notificaciones; inbox UI/API; confirmación persistida; resolución de artefacto (V5). Componentes: C1 (`ui-api` + `go-intake`).
- **Bounded context**: Ingesta. **Historias**: US-M1, US-M2 (confirm).
- **Desplegables**: `ui-api`, `go-intake` (+ fachada de lectura C6/C7/C9).
- **Límites**: no crea Jobs; no toca el test ns; no llama al LLM.
- **Predecesora**: U5. Stubs primero: valida contra esquemas de U5 antes de integrarse.
- **Salida**: notificaciones + confirm auditables; `run.confirmed` válido. Desbloquea U2 (junto a U4).

| # | Hecho | Tarea | Historias | Comando de verificación |
|---|---|---|---|---|
| U1-T1 | [x] | Stubs: validador fake de GitHub webhook (firma) + `Notification`/`ConfirmationReceipt` de ejemplo válidos contra `contracts/` | US-M1 | `go test ./...` en verde con fixtures que pasan `ajv validate` |
| U1-T2 | [x] | `go-intake`: `POST /webhooks/github` (verifica firma, crea notificación, publica `notify.created`) — nunca crea Jobs | US-M1 | test: un evento de prueba crea 1 notificación y `kubectl get jobs -n <test-ns>` sigue vacío en el entorno de test |
| U1-T3 | [x] | Resolución de artefacto V5: build-from-repo (commit/PR) vs imagen publicada (tag/release) + `ArtifactRef` | US-M2 | tests parametrizados por tipo de evento en verde; `go vet ./...` limpio |
| U1-T4 | [x] | `ui-api` inbox: `GET /notifications` + `POST /notifications/{id}/confirm` (auth vía stub U4, luego real) con rate limiting + security headers | US-M1, US-M2 | `curl` autenticado devuelve inbox; `curl -I` muestra CSP/HSTS/nosniff; `hey`/`k6` contra endpoint público respeta rate limit |
| U1-T5 | [x] | PBT parcial: round-trip de parseo/serialización de payloads GitHub (PBT-02) + generadores de dominio (PBT-07) + seed logueado (PBT-08); framework documentado (PBT-09: rapid) | US-M1 | `go test -run PBT` en verde con shrinking habilitado y seed visible en el log de fallo |
| U1-T6 | [x] | Logging estructurado + trazas + health shallow/deep + métricas (latencia/errores/throughput) | US-M1 | `curl /healthz` y `/readyz` 200; logs con timestamp/request-id/nivel; dashboard con el panel de U1 |
| U1-T7 | [x] | Integración contra U4 real (auth) y publicación de `run.confirmed` consumible por U2 (contrato verificado en CI) | US-M2 | CI verde con tests de contrato U1↔U4 y evento `run.confirmed` válido contra esquema |

> **Tareas redactadas (2026-10-03)**: el detalle de cada tarea de U1 (alcance, fuera de alcance, criterios con comando, plan de pruebas) está en `tareas/U1-T01-stubs.md` … `tareas/U1-T07-integracion-u4-run-confirmed.md`. Orden: T01 → T02 → T03 → T04 → (T05, T06) → T07; T07 exige U4-T02 y U4-T03 fusionadas. Decisiones abiertas que surgieron al redactar: C-45…C-48, C-53 en `tareas/candidatas.md`.

## U4 — Gobernanza & Identidad

- **Responsabilidad**: políticas admin (**workflows de negocio**: complejidad, cuotas, timeouts, aprobaciones), gates, auditoría append-only, authN/Z por rol. Componentes: C7 + C8 (`go-governance` + `go-identity`).
- **Bounded context**: Gobierno (separado de la ejecución para que los guardrails no dependan del ciclo de corrida).
- **Historias**: US-M8.1, US-M8.2, US-M8.3.
- **Desplegables**: `go-governance`, `go-identity`.
- **Límites**: escritura de políticas solo admin; RBAC/NetworkPolicy como artefactos revisables (sin apply autónomo).
- **Predecesora**: U5. Paralela a U1. Stubs primero.
- **Salida**: gates + auth + manifiestos de aislamiento revisables. Desbloquea U2 (junto a U1).

| # | Hecho | Tarea | Historias | Comando de verificación |
|---|---|---|---|---|
| U4-T1 | [x] | Stubs: evaluador fake de `authorizeTransition` (Allow/Deny programable) + principales/roles de ejemplo | US-M8.3 | `go test ./...` en verde con matrices Allow/Deny de ejemplo |
| U4-T2 | [x] | `go-identity`: login/logout, hashing adaptativo, MFA admin, sesiones con expiración, anti-brute-force, cero credenciales hardcodeadas | US-M8.3 | tests de auth en verde; `gitleaks`/`trufflehog` sin hallazgos; `curl` a `/auth/login` con 5 fallos activa backoff/lockout |
| U4-T3 | [x] | Autorización: deny-by-default, IDOR (propiedad por resource ID), función/roles server-side, CORS restringido, validación de tokens en cada request | US-M8.3 | tests IDOR/roles en verde (`go test -run AuthZ`); `curl` sin token → 401; con token de otro owner → 403 |
| U4-T4 | [x] | `go-governance`: políticas admin (eventos, confirm-required, cuotas/cadencia del warm, **workflows**: complejidad/cuotas/timeouts/aprobaciones) + `authorizeTransition` (confirm + `reset_verified` + `ensayo_passed` + test-ns-only + workflow permitido, fail-closed) + audit append-only | US-M8.3 | tests de gates en verde (incl. Deny sin confirm, sin `reset_verified` y sin ensayo); audit inmutable verificado por test (append-only, sin delete) |
| U4-T5 | [x] | Manifiestos RBAC (test-ns-only, sin wildcards) + NetworkPolicy (namespace-only, bloqueo LLM a runners) como artefactos revisables + tests de política | US-M8.1, US-M8.2 | `conftest test` en verde; matriz `kubectl auth can-i` esperada documentada en el PR (sin apply autónomo) |
| U4-T6 | [x] | PBT parcial: round-trip de parseo de políticas/config/workflows (PBT-02) + invariantes de gates (p. ej. "sin confirm nunca Allow", "sin `reset_verified` nunca Allow", PBT-03) + generadores + seed (PBT-07/08) | US-M8.3 | `go test -run PBT` en verde con seed logueado |
| U4-T7 | [x] | Alertas de seguridad (auth failures, denegaciones, escaladas) + retención audit ≥90d + dashboard | US-M8.3 | reglas de alerta verificadas (`promtool check rules`); retención visible en manifiesto |
| U4-T8 | [x] | Cableado de despliegue de `go-identity` y `go-governance`: entorno, referencias a Secrets por nombre, `fsGroup`, `Recreate`, política contra valores literales sensibles y procedimiento SOPS documentado *(surgida al cerrar U4: C-53, C-70)* | US-M8.3, US-M10 | `tareas/U4-T08-cableado-despliegue.md` |

> **Tareas redactadas (2026-10-03)**: el detalle de cada tarea de U4 está en `tareas/U4-T01-stubs.md`, `U4-T02-go-identity-autenticacion.md`, `U4-T03-autorizacion.md`, `U4-T04-go-governance.md`, `U4-T05-rbac-networkpolicy-politicas.md`, `U4-T06-pbt-parcial.md` y `U4-T07-alertas-retencion-dashboard.md`. Orden: T01 → T02 → T03 → T04 → (T06, T07); T05 solo depende de U5 y puede ir desde la primera ola. Candidatas nuevas: C-47, C-49…C-53.

> **U4 terminada (2026-10-05)**, marcada por instrucción del humano tras fusionar los PRs #17-#28. La tarea U4-T8 surgió al cerrar la unidad (cableado de despliegue: C-53 y C-70). Los criterios finales de cada tarea están en `tareas/U4-T*.md`, la evidencia en `revisiones/U4-T*/` y las candidatas pendientes en `tareas/candidatas.md` (entre ellas C-50, C-64, C-69, C-71..C-74).

## U2 — Orquestación & Warm Sandbox Lifecycle

- **Responsabilidad**: máquina de estados + gates; gestión del **entorno warm** (deploy del artefacto por corrida sobre app+DB+Redis reutilizados, health/estados); ensayo bloqueante; runners; **reset verificado, cuarentena, scale-down en idle y rebuild/teardown periódico** + housekeeping. Componentes: C9 + C3 + C2 + C4 + C5 (plano Go).
- **Bounded context**: Ejecución aislada. **Historias**: US-M2 (deploy sobre warm), US-M5, US-M6, US-M7.1, US-M7.2.
- **Desplegables**: `go-run-controller`, `go-warm-manager`, `go-reset` + entorno warm (rollout/Jobs `deploy-{run}`, `rehearsal-{run}`, `runner-{run}-{flow}`, `reset-{run}`; CronJobs `housekeeping`/`rebuild`).
- **Límites**: runners/ensayo sin credenciales LLM y sin egress; fail-closed global; reuso del warm solo con `reset_verified=true`; reset verificado 100%.
- **Dimensionado especial**: burst de Jobs por corrida; quotas del namespace + resource limits por Job; HPA en controller/warm-manager; scale-down del warm en idle.
- **Predecesoras**: U5, U1, U4. Consume stubs de U3 (flujos sintéticos) hasta integrar.
- **Salida**: ciclo de corrida completo sobre el entorno warm con gates (confirm + `reset_verified` + ensayo) y reset verificado 100% + higiene de rebuild. Desbloquea U3 (integración real) y la demo Sesión 16.

| # | Hecho | Tarea | Historias | Comando de verificación |
|---|---|---|---|---|
| U2-T1 | [x] | Stubs: `FlowPlan`/flujos sintéticos válidos contra esquema + `SurfaceArtifact`/`EvidenceURIs`/`WarmState` de ejemplo (contrato U3) | US-M5, US-M6 | `go test ./...` en verde usando solo fixtures válidas contra `contracts/` |
| U2-T2 | [x] | `go-run-controller`: máquina de estados persistida notify→confirm→warm ready→deploy→infer→rehearse→run→reset→report + `GET /runs/{id}`; consulta C7/C8 y estado del warm (C3/C5) en cada transición; fail-closed global | US-M2, US-M5 | tests de transiciones en verde (incl. Deny sin confirm, sin `reset_verified`, sin `ensayo_passed`); `go vet` limpio |
| U2-T3 | [x] | `go-warm-manager`: gestión del **entorno warm** (app+DB+Redis pre-desplegados y reutilizados) + rollout/Job `deploy-{run}` del artefacto **sobre el warm** + `warm.ready`/`deploy.done\|failed`; fail-closed tras 2 reintentos (V8) | US-M2 | test de integración en clúster de dev (con aprobación): `kubectl get pods -n <test-ns>` Running sobre el warm; deploy roto → handoff tras 2 intentos, sin 3er Job |
| U2-T4 | [x] | Ensayo: Job `rehearsal-{run}` **contra el entorno warm** (sin credenciales LLM) + gate `ensayo_passed=true` no omitible; 2 reintentos y escalado (V8) | US-M5 | test: plan roto nunca crea Jobs de runners; `kubectl get jobs` muestra solo rehearsal fallidos + handoff |
| U2-T5 | [x] | Runners: Jobs `runner-{run}-{flow}` (motor enchufable) contra el warm + `collectEvidence` a MinIO + `run.done`; cuota/timeout del workflow; runners sin LLM ni egress | US-M6 | test: runner sin env de LLM (`env \| grep -i LLM` vacío en el pod) y egress bloqueado (pod de prueba no sale del ns) |
| U2-T6 | [x] | Reset verificado + higiene: Job `reset-{run}` (restart + clean DB + flush cache + verificación → `reset_verified`); cuarentena si falla; **scale-down en idle**; CronJobs `housekeeping` (grace 24 h)/`rebuild`; sesión "incompleta" persistida | US-M7.1, US-M7.2 | test: sin `reset_verified=true` no arranca la siguiente corrida; reset fallido → cuarentena + aviso; tras fin/abandono (+grace simulado) el warm queda `ready` o en cuarentena; rebuild/teardown periódico ejecutado |
| U2-T7 | [x] | Resource limits/requests por Job + quotas del namespace + HPA del controller + scale-down del warm en idle + timeouts/circuit breakers en calls externos | US-M5, US-M6 | `kubeconform` + `conftest` en verde para los manifiestos (revisables, sin apply autónomo) |
| U2-T8 | [x] | Integración con U3 real: superficie→flujos→evidencia de extremo a extremo en dev (camino feliz journey 7.1) | US-M5, US-M6 | demo en dev: notify→confirm→warm ready→deploy→ensayo→run→reset→estado final, todo auditado (con aprobación humana del entorno) |

> **Tareas redactadas (2026-10-06)**: el detalle de cada tarea de U2 está en `tareas/U2-T01-stubs.md` … `tareas/U2-T08-integracion-u3-dev.md`. Orden: T01 → (T02, T03, T06 en paralelo) → T04 → T05 → T07 → T08. T04 exige T02, T03 y T06 fusionadas; T05 exige T04 (ambas modifican `go-run-controller`); T07 exige T02–T06; **T08 no es despachable hasta que U3 exista y el humano apruebe el entorno dev**. Decisiones tomadas al redactar: T01 **añade** esquemas nuevos en `contracts/plans/` (no existían para `FlowPlan`, `SurfaceArtifact`, `EvidenceURIs`, `WarmState`); el controlador habla con `go-warm-manager` y `go-reset` por REST servicio a servicio (la API vive en sus README hasta llevarla al OpenAPI); el HPA del controlador queda en `maxReplicas: 1` hasta resolver C-45/C-49. Todos los criterios se verifican sin clúster (cliente de Kubernetes falso y MinIO local); la validación en dev es del humano.

> **Ola 2 (2026-10-07) cerrada por decisión del humano, sin VERDE del revisor**: U2-T02, U2-T03 y U2-T06 agotaron el tope de 3 rondas con hallazgos abiertos y se fusionan «tal cual» (PR #40, #41, #42), con los hallazgos pasando a tareas de seguimiento propias, cada una con sus 3 rondas: `tareas/U2-T02b-diario-y-eventos-en-disco.md`, `tareas/U2-T03b-deploy-en-proceso.md`, `tareas/U2-T06b-estado-ilegible-e-idle.md`. **No desplegar en un entorno compartido antes de U2-T02b** (un disco lleno puede impedir el reinicio del controlador). **Decisión del humano (2026-10-07), opción A**: el deploy y el reset se ejecutan **en proceso** con la ServiceAccount del servicio (sin Jobs sin token, sin relajar `isolation.rego`); U2-T03b se re-especifica como «deploy en proceso» y U2-T07 cablea el RBAC existente. Marcar `[x]` en la tabla de U2 cuando cada PR esté fusionado.

> **Ola 3 (2026-10-08)**: U2-T02b, T03b, T04 y T06b fusionadas con VERDE del revisor. **U2-T05 cerrada por decisión del humano, sin VERDE** (ronda 1 NO-VERDE, ronda 2 interrumpida por tiempo): se fusiona «tal cual» (PR #50) y sus hallazgos abiertos pasan a `tareas/U2-T05b-plazos-y-lanzamiento-parcial.md`. Es seguro fusionar porque en modo `real` la fase `run` falla cerrada («sin flujos») hasta que U3 aporte `FlowPlan`; **U2-T05b debe estar fusionada antes de U2-T08 / integración con U3**. Marcar `[x]` U2-T4 y U2-T5 al fusionar sus PR.

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
| U3-T1 | [x] | Stubs: LLM fake determinista (respuestas fijadas por prompt-hash) + superficies/evidencias de ejemplo del dataset (incl. subset trampa y no-arranca) | US-M3, US-M9 | suite offline en verde sin red (`pytest`/`vitest` con VCR/cassette o fake; cero llamadas reales en tests) |
| U3-T2 | [x] | `agent-planner`: superficie→`FlowPlan` determinista (sin llamadas en runners) + límite duro de tiempo/tokens + superficie tratada como dato | US-M3, US-M4 | test: plan válido contra esquema; test de prompt-injection (descripción maliciosa en superficie) no altera instrucciones; timeout documentado y probado |
| U3-T3 | [x] | Formato de flujos inspectable (artefacto estándar ejecutable, p. ej. k6) + validación contra esquema antes de publicar | US-M4 | `k6 lint`/validador del formato en verde para cada flujo generado de ejemplo |
| U3-T4 | [x] | `agent-reporter`: logs→post-mortem (causa de negocio + invariante + veredicto + enlaces a evidencia) + `redactSecrets` pre-LLM y pre-publicación | US-M9 | test: reporte de ejemplo con secretos sembrados sale redactado (`grep` de patrones antes/después); precisión medida contra subset golden del dataset |
| U3-T5 | [x] | Evaluación offline contra dataset inicial (10-15 artefactos: golden, bugs sembrados, trampa de esquema, no-arranca) + bucle de regresión por falso positivo | US-M9 | `eval/run` reporta: factualidad, adherencia (reset verificado/nunca fuera del ns/tope de intentos), ruido; umbrales del §10 verificados en el reporte |
| U3-T6 | [x] | PBT parcial (si Python: Hypothesis; si TS: fast-check): round-trip de serialización de planes/reportes (PBT-02) + invariantes (p. ej. "todo flujo cita superficie observada", PBT-03) + generadores + seed (PBT-07/08) | US-M3, US-M9 | suite PBT en verde con seed logueado; framework fijado en dependencias (PBT-09) |
| U3-T7 | [x] | Integración **local** U2↔U3: adaptador LLM compatible con OpenAI (probado en loopback) + `u3-up`/`u3-down` + recorrido local (contrato de U2 contra el planner real → reporte desde evidencia en disco) *(simplificada 2026-10-10)* | US-M4, US-M9 | `tareas/U3-T07-integracion-u2-real.md` (`bash scripts/test/u3-demo-local.sh` solo `OK`) |

> **Tareas redactadas (2026-10-07)**: el detalle de cada tarea de U3 está en `tareas/U3-T01-stubs.md` … `tareas/U3-T07-integracion-u2-real.md`. Orden: T01 → (T02, T03, T04 en paralelo) → T05 → T06 → T07. T05 exige T02 y T04 fusionadas; T06 exige T02, T03 y T04 (y va después de T05 por orden); **T07 no es despachable hasta que U2-T08 (y U2-T05b) estén fusionadas, el humano apruebe el entorno dev y resuelva las decisiones C-82 (proveedor LLM), C-83 (despliegue y egress de los agentes) y C-45 (transporte de eventos)**. Decisiones tomadas al redactar: lenguaje **Python** con Hypothesis (decisión del humano; servicios `agents/agent-planner` y `agents/agent-reporter`, evaluación en `agents/eval`, dataset en `agents/dataset`); flujos en **k6** generados desde el `FlowPlan` con una plantilla fija (la validación de capa 3 usa `k6 inspect` con la imagen `grafana/k6:0.55.0` por podman, porque k6 no está instalado y esa versión no tiene `k6 lint`); `Report` es un esquema nuevo en `contracts/plans/` (solo se añaden archivos); las pruebas son offline con un LLM fake por hash de prompt y una guarda que bloquea todo socket no local; las mediciones de T05 con el fake validan el pipeline, no la calidad de un modelo real (C-85). Todos los criterios se verifican sin clúster ni proveedor LLM (MinIO local y servidores simulados en loopback); la validación en dev es del humano.

> **Simplificación (2026-10-10, decisión del humano)**: U2-T08 (PR #63) y U3-T06 (PR #61) están fusionadas; queda **solo U3-T07**, redefinida como integración **local** (sin clúster, proveedor LLM real, NATS, LiteLLM, MinIO/S3 ni medición de costo). Es despachable ya y cierra el MVP. Lo retirado pasa al backlog post-MVP (P1–P6, abajo). Las decisiones C-45, C-82 y C-83 siguen vigentes para cuando se reactive.

> **U3 terminada (2026-10-10)**: U3-T07 fusionada (PR #67, VERDE en ronda 1, `revisiones/U3-T07/ronda-1.md`). Con ella quedan cerradas las 5 unidades del MVP simplificado; lo pendiente es el backlog post-MVP (S1–S5, P1–P6).

## U6 — Frontend web (S5, primera unidad post-MVP)

- **Responsabilidad**: dashboard web real del operador: login, inbox de notificaciones con confirmación explícita, seguimiento de la corrida y estado del entorno warm. Consume `ui-api`, `go-identity` y `go-run-controller` a través del OpenAPI (`contracts/openapi/control-plane.yaml`); `mockups/swarm-mock.html` es solo referencia visual.
- **Bounded context**: Interfaz. **Historias**: S5 (reactivada por decisión del humano, 2026-10-10), sobre US-M1, US-M2, US-M7.1 y US-M8.3.
- **Desplegables**: `web/dashboard/` (Vite + React + TypeScript). En esta unidad solo se construye y se prueba en local; imagen, servidor estático y manifiestos quedan en el backlog.
- **Límites**: el navegador nunca recibe el token de servicio del warm (lo usa `ui-api`); token de persona solo en memoria; ninguna acción del dashboard cambia estado salvo la confirmación explícita; CSP estricta sin código en línea.
- **Predecesoras**: U1, U2, U4 (APIs) y U5 (CI). **Posición**: tras el cierre del MVP.
- **Salida**: dashboard funcionando en local contra los servicios reales (recorrido `u6-demo-local.sh`).

| # | Hecho | Tarea | Historias | Comando de verificación |
|---|---|---|---|---|
| U6-T1 | [x] | Esqueleto `web/dashboard` (Vite + React + TS estricto), tipos generados desde el OpenAPI, cliente API tipado, proxy `/api` y workflow `web.yml` | S5 | `tareas/U6-T01-esqueleto-web.md` |
| U6-T2 | [x] | `ui-api`: `GET /warm` (token de servicio hacia `go-warm-manager`) y `GET /confirmations` (con control de propiedad) + OpenAPI | S5, US-M7.1 | `tareas/U6-T02-ui-api-lectura-warm-confirmaciones.md` |
| U6-T3 | [ ] | Login contra `go-identity`, sesión solo en memoria, logout, guarda de rutas y errores comunes (401/429/503) | S5, US-M8.3 | `tareas/U6-T03-login-sesion.md` |
| U6-T4 | [ ] | Inbox con filtros y confirmación explícita (`POST /notifications/{id}/confirm` → `/runs/{run_id}`) | S5, US-M1, US-M2 | `tareas/U6-T04-inbox-confirmacion.md` |
| U6-T5 | [ ] | Vista de corrida con sondeo hasta estado terminal, «Mis corridas» y estado del warm | S5, US-M2, US-M7.1 | `tareas/U6-T05-corrida-y-warm.md` |
| U6-T6 | [ ] | Recorrido local con servicios reales en loopback (`u6-demo-local.sh`) y CSP estricta del build | S5 | `tareas/U6-T06-recorrido-local-csp.md` |

> **Tareas redactadas (2026-10-10)**: orden T01 ∥ T02 → T03 → (T04 ∥ T05) → T06. T03 exige T01; T05 exige T02 y T03; T06 exige T04 y T05. Decisiones tomadas al redactar (el humano puede revertirlas): stack Vite + React 18 + TypeScript con versiones exactas y sin librerías de UI/estado; tipos generados con `openapi-typescript` y control de deriva en CI; el frontend llama a `/api/*` y el proxy de Vite enruta a identidad, controlador o `ui-api` (en producción lo hará el ingress); el estado del warm se lee a través de `ui-api` porque `go-warm-manager` solo acepta token de servicio; familias de flujos fijas en `['happy-path']` (S2 sigue fuera); sin navegador real en las pruebas (Testing Library + MSW, y `curl` en el recorrido). Todo se verifica sin clúster. Candidatas nuevas: C-94, C-95.

## U7 — Plataforma en kind local (podman)

- **Responsabilidad**: levantar la plataforma en un clúster kind **efímero y local** sobre podman, con las imágenes construidas en la máquina, el overlay `deploy/flux/kind` y Secrets aleatorios creados en el clúster; verificar con servicios reales que arrancan, que login/inbox/warm responden y que RBAC y NetworkPolicy se **aplican** (kindnet), no solo que pasan `conftest`.
- **Bounded context**: Plataforma/operación local. **Historias**: US-M10, sobre US-M8.1, US-M8.2 y S5 (U6). Primer paso de P5 sin depender de un entorno dev compartido.
- **Desplegables**: `deploy/kind/` (configuración del clúster), `deploy/flux/kind/` (overlay), `scripts/kind/` (up/down, imágenes, secretos, despliegue, humo) y `docs/operaciones/kind-local.md`.
- **Límites**: todo script que hable con el clúster exige el contexto `kind-aqs` (sale `3` con cualquier otro); nunca se apunta a dev/prod; Secrets solo en el clúster (nunca en el repo); el overlay no relaja ninguna política (`policy/*.rego` y `deploy/flux/base|dev|prod` no cambian); observabilidad y backups quedan fuera de kind (exigen CRDs y un S3 externo).
- **Predecesoras**: U5 (manifiestos, políticas), U1/U2/U4 (servicios); U7-T05 además U6-T06. **Gate**: decisión **C-96** aprobada por el humano antes de despachar U7-T01.
- **Salida**: `kind-up → build-images → load-images → deploy → kind-smoke → kind-down` en verde. **No** incluye el ciclo de corrida completo: los eventos viajan por archivos del disco de cada pod y los agentes no tienen Deployment; eso exige P1–P3 (sería una unidad siguiente) y el humo lo declara como `PENDIENTE`.

| # | Hecho | Tarea | Historias | Comando de verificación |
|---|---|---|---|---|
| U7-T1 | [ ] | Clúster kind `aqs` sobre podman (imagen de nodo por digest), `kind-up`/`kind-down` idempotentes y guarda de contexto `kind-aqs` | US-M10 | `tareas/U7-T01-cluster-kind.md` |
| U7-T2 | [ ] | Construir con podman las 9 imágenes (`:0.0.0`, contexto igual que la CI) y cargarlas con `kind load image-archive` | US-M10 | `tareas/U7-T02-imagenes-locales.md` |
| U7-T3 | [ ] | Overlay `deploy/flux/kind` (sin observabilidad ni backups), Secrets aleatorios en el clúster, `deploy.sh` con espera de disponibilidad y overlay `kind` en `policies.sh` | US-M10, US-M8.1, US-M8.2 | `tareas/U7-T03-overlay-kind-y-despliegue.md` |
| U7-T4 | [ ] | `kind-smoke.sh`: salud de los 7 servicios, login/inbox/warm reales, RBAC y NetworkPolicy aplicados (con prueba de sensibilidad) y pendientes P1/P2 declarados; runbook | US-M10, US-M8.1, US-M8.2, US-M1 | `tareas/U7-T04-humo-y-aislamiento.md` |
| U7-T5 | [ ] | Dashboard (`vite preview`) contra la plataforma en kind: login → inbox → warm → logout por el origen del dashboard | S5 | `tareas/U7-T05-dashboard-contra-kind.md` |

> **Tareas redactadas (2026-10-10)**, a pedido del humano («can we use a cluster created with kind for this project? and run everything in docker/podman» → «yes, draft the U7 kind tasks»). Orden estrictamente secuencial T01 → T02 → T03 → T04 → T05; T05 exige además U6-T06. Decisiones tomadas al redactar (el humano puede revertirlas): kind con proveedor podman y un solo nodo; kindnet como CNI porque aplica NetworkPolicy; acceso solo por `kubectl port-forward` en loopback (sin `extraPortMappings` ni ingress); imágenes cargadas con `podman save` + `kind load image-archive` (sin registro local); el overlay referencia piezas de `base` para excluir `observability` y `backup`; Secrets generados por script y nunca versionados (no se usa SOPS en kind). **No despachable hasta que el humano resuelva C-96** (autorizar a codificador y revisor a crear/destruir el clúster `aqs` y aplicar manifiestos solo en el contexto `kind-aqs`). Candidatas nuevas: C-96, C-97, C-98.

## Cobertura historias → unidades (13/13 Must, 0 sin asignar)

| Historia | Unidad(es) |
|---|---|
| US-M1 Notificación sin auto-run | U1 |
| US-M2 Deploy sobre el entorno warm (confirm) | U1 (confirm) + U2 (deploy sobre warm) |
| US-M3 Superficie externa | U3 |
| US-M4 Flujos inspectables (generación) | U3 (genera; U2 ejecuta en US-M6) |
| US-M5 Ensayo bloqueante | U2 |
| US-M6 Corrida + evidencia | U2 (MinIO provisto por U5) |
| US-M7.1 Reset verificado tras corrida | U2 |
| US-M7.2 Housekeeping abandonadas | U2 |
| US-M8.1 RBAC test-ns-only | U4 (manifiestos base en U5) |
| US-M8.2 NetworkPolicy sin egress/LLM | U4 (manifiestos base en U5) |
| US-M8.3 Confirm-required + cero staging/prod | U4 (registro en U1) |
| US-M9 Post-mortem | U3 |
| US-M10 GitOps del producto | U5 |

**Totales**: 5 unidades · 37 tareas (U5: 8, U1: 7, U4: 7, U2: 8, U3: 7) · 13 historias Must cubiertas · Should (S1-S5) **fuera del alcance actual (backlog post-MVP; no historizados por decisión de scope, 2026-10-01)**. Modelo warm propagado (U2 lifecycle, U4 workflows + gate `reset_verified`, U5 manifiestos warm, U3 adherencia por reset).

## Backlog post-MVP (Should — fuera de alcance)

No forman parte del MVP ni del plan de tareas (alcance = Must M1-M10). Se reactivarían reabriendo User Stories + asignación a unidades + tareas.

| ID | Item | Unidad candidata | Depende de |
|---|---|---|---|
| S1 | Entrega por Slack del resumen post-corrida | U3 (C6 reporter) | `Notify` port ya en el diseño |
| S2 | Segunda familia de flujos (p. ej. concurrencia) | U3 (agent-planner) | M4/M6 |
| S3 | Traspaso a humano con contexto estructurado (boot/deploy, reset o ensayo fallido) | U2 | Fail-closed de M2/M5 |
| S4 | Panel de políticas (eventos, cuotas del warm, workflows, confirm-required) | U4 (C7) | M7 |
| S5 | **En curso como U6 (2026-10-10).** Dashboard inbox + estado del warm (`ready`/`dirty`/`cuarentena`/`idle-escalado`) — **Frontend web real** (consumiendo `ui-api` y OpenAPI; el archivo `mockups/swarm-mock.html` es únicamente un prototipo exploratorio de diseño, no la UI final) | U1/U2/Frontend | M1, M3, U1-T04 |

### Retirado del MVP en la simplificación (2026-10-10)

| ID | Item | Origen | Depende de |
|---|---|---|---|
| P1 | Transporte de eventos NATS JetStream (reemplaza el outbox JSONL) | C-45 | — |
| P2 | LiteLLM + Deployments de planner/reporter con egress limitado y Secrets como archivo | C-82, C-83, C-51 | P1 opcional |
| P3 | Adaptadores S3 de MinIO en los agentes (evidencia, reporte, k6) | U3-T07 original | — |
| P4 | Medición de costo por corrida (inferencia + cómputo) | U3-T07 original | P2 |
| P5 | Demos en dev con aprobación humana (`docs/demo-dev-u3.md`; CA-8 de U2-T08) | U2-T08, U3-T07 originales | P1, P2, P3 |
| P6 | Umbrales del §10 medidos con modelo real | C-85 | P2 |
