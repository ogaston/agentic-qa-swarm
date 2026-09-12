# Requisitos — Agentic QA Swarm

> **Fuente**: `entradas/prd.md`, `entradas/pvd.md`, `docs/*` (overview, mercado, icp, critica, validacion) y `mockups/swarm-mock.html`.
> **Generado por**: Requirements Analysis (INCEPTION) con profundidad **Comprehensive**.
> **Estado**: Esperando aprobación. Todas las decisiones abiertas del PRD fueron resueltas por el usuario (6 preguntas de verificación + 8 de aclaración + 2 de seguimiento).

---

## 1. Resumen de Análisis de Intención

| Atributo | Valor |
|---|---|
| **Request** | "Lee entradas/prd.md y entradas/pvd.md y especifica el producto que describen. No escribas código: este trabajo se detiene al terminar el plan de tareas de cada unidad." |
| **Tipo de request** | New Project (greenfield) — especificación de producto completo (MVP) |
| **Claridad** | Alta (PRD+PVB excepcionalmente detallados); vacíos resueltos vía preguntas |
| **Estimación de alcance** | System-wide: 8 módulos (M1-M8), plataforma completa + sandbox efímero |
| **Estimación de complejidad** | Complejo — múltiples subsistemas, integración con GitHub/K8s/LLM, requisitos de seguridad y gobernanza críticos |
| **Estado de iteración** | Workflow se detiene al terminar el plan de tareas de cada unidad (no se genera código) |

---

## 2. Contexto del producto

### 2.1 One-liner

> **Agentic QA Swarm** convierte un evento de GitHub (commit, PR o tag/release) en una corrida de QA de caja negra contra una copia aislada de la app: el humano **confirma**, el sistema trae el artefacto, lo arranca en Docker **dentro de un namespace de prueba**, infiere los flujos a partir de la superficie externa, los **ensaya**, los ejecuta y entrega un **post-mortem de lógica de negocio**. Staging y producción del cliente **nunca son el blanco**.

### 2.2 JTBD

- **Primario (User — Platform/DevOps Engineer):** encontrar bugs de lógica de negocio antes de producción, en cada commit/PR/release, sin escribir ni mantener flujos de QA y **sin apuntar herramientas a staging**.
- **Secundario (Buyer — VP Eng / Head of Platform):** bajar el *Change Failure Rate* sin contratar SRE/QA dedicado, con aislamiento **verificablemente** aislado de staging/producción. Veto de confianza: *"¿Esto toca nuestro staging o producción?"* → respuesta exigible: **no**.

### 2.3 Principios de diseño no negociables (heredados del PRD)

1. **Cerebro fuera del bucle de ejecución**: el LLM solo planea y hace post-mortem; los runners ejecutan sin llamadas al modelo y sin credenciales de LLM.
2. **Ensayo obligatorio** antes de la corrida completa (gate condicionado a `ensayo_passed=true`), no relajable.
3. **Sandbox en namespace dedicado + teardown forzado**: SUT = app Docker + 1 DB (Postgres o Mongo) + 1 Redis por corrida, destruidos al terminar o abandonar; completitud de teardown 100%.
4. **Límite de autonomía verificable**: auto-**notify** sí, auto-**run** no; nunca staging/prod del cliente; mecanismos (GitHub App permisos mínimos, gate de confirm persistido/auditable, RBAC sin agenda fuera del test ns, NetworkPolicy namespace-only que bloquea LLM a runners, cero secrets de staging/prod montados) **en el MVP**, no opcionales.
5. **Flujos generados inspectables**: artefacto estándar ejecutable (k6 u otro motor), nunca formato propietario ilegible; el cliente puede auditor/versionar/ejecutar fuera de la plataforma.

---

## 3. Decisiones de usuario capturadas (resumen de respuestas)

| # | Decisión | Respuesta | Impacto |
|---|---|---|---|
| V1 | Extensión Resiliency | **Sí** (guía direccional) | Se aplican RESILIENCY-01..15; decisiones 2-8 abajo |
| V2 | Extensión Security | **Sí**, todas bloqueantes | SECURITY-01..15 son hallazgos bloqueantes en cada etapa |
| V3 | Extensión PBT | **Parcial** (solo PBT-02, 03, 07, 08, 09) | Enforced desde Functional Design / NFR / Code |
| V4 | Alcance de la especificación | **Solo MVP** (MoSCoW Must + Should); el resto como fuera de alcance | El plan de tareas cubre M1-M10 + S1-S5 |
| V5 | Fuente del artefacto | **Build-from-repo para commit/PR; imagen publicada para tags/releases** | Alcance del módulo M1/M2 |
| V6 | Almacenamiento de evidencia | **MinIO in-cluster** | El control plane persiste reportes/evidencia en MinIO dentro del clúster |
| V7 | Expiración de sesiones | **Sesión/plan persistido indefinidamente; sandbox con grace period** (24 h configurable) | Reconciliado: teardown 100% + reanudar sin repetir inferencia |
| V8 | Reintentos antes de fail-closed | **Boot: 2; Ensayo: 2** (después→handoff) | Límites duros a configurar en el Módulo M2 |
| V9 | Stack del control plane | **Híbrido: Go para control plane de ejecución (jobs/runners/teardown) + capa de agentes LLM** (Python o TypeScript) | Decision en la que se apoya NFR/Infra Design |
| AR1 | RTO/RPO y DR | **Horas — Backup & Restore** (coste mínimo) | DR del control plane (datos de plataforma y evidencias) |
| AR2 | Gestión de cambios | **Proponer proceso ligero** (registro de cambio + aprobación + nota de rollback) | El flujo propone y documenta el proceso |
| AR3 | CI/CD | **GitHub Actions** (pipeline del repo del producto) | Pipeline CI compatible con GitHub Actions |
| AR4 | Rollback | **Redeploy de la versión anterior** (version-pinned) | Mecanismo de rollback documentado |
| AR5 | Estilo de despliegue | **Directo / in-place** (en el entorno estable del producto) | Aceptable para el entorno de curso; multi-zona es meta de producción |
| AR6 | Topología regional | **Single-region, multi-zona** | Meta de producción; el laboratorio del módulo 8 es single-node → limitación documentada |
| AR7 | Respuesta a incidentes | **Proponer proceso ligero IR/COE** | El flujo propone IR/COE ligero |
| AR8 | CI/CD concreto | **GitHub Actions** | Id. AR3 |
| AR9 | Entrega GitOps | **Flux** (Kustomize/Helm controllers, reconciliación automática) | Control plane desplegado por Flux (Módulo 8) |

---

## 4. Requisitos funcionales (MoSCoW)

### 4.1 Must Have (MVP)

| ID | Requisito | Criterio de aceptación (KPI / comando) |
|---|---|---|
| **M1** | Integración GitHub: commit, PR y tag/release crean **notificación, no corrida** (GitHub App de permisos mínimos; inbox de notificaciones) | Antonimar verificado con `GET /notifications` devuelve las notificaciones creadas; NO se auto-ejecuta ninguna corrida en el webhook (`test: no auto-run en webhook`) |
| **M2** | Pull del artefacto **build-from-repo** (commit/PR) o **imagen publicada** (tag/release) + boot en **Docker dentro del namespace de prueba** dedicado | `kubectl get pods -n <test-ns>` → app `Running`; boot exitoso gated; boot fallido → fail-closed tras 2 reintentos |
| **M3** | Descubrir **solo la superficie externa** (OpenAPI si está expuesta; si no, sondeo HTTP/UI de puertos publicados) | Artefacto de superficie generado a partir de puertos/Service del test ns; sin lectura de código fuente |
| **M4** | Generar flujos de QA de caja negra (trabajo de un QA): familia funcional de negocio + concurrencia feliz camino | Flujos generados como artefacto estándar ejecutable (formato k6 u otro) y **inspectable** |
| **M5** | **Ensayo bloqueante** dentro del sandbox antes de la corrida completa | Gate de pipeline condicionado a `ensayo_passed=true`; ensayo = un flujo unitario exitoso (2xx/invariante mínima) en el sandbox |
| **M6** | Ejecutar los flujos contra ese sandbox (runners/Jobs en el mismo namespace) | `kubectl get jobs -n <test-ns>` completados; runners sin credenciales de LLM |
| **M7** | **Teardown forzado** de app + deps + runners (completitud 100%), incl. housekeeping de sesiones abandonadas (grace period) | Verificación de namespace limpio tras cada corrida; **Completitud de teardown = 100%** (KPI) |
| **M8** | **RBAC + NetworkPolicy**: solo el test namespace; **nunca staging/producción del cliente**; runners **sin acceso a LLM** | `kubectl auth can-i` desde runner → denegado fuera del test ns; NetworkPolicy test ns-only; escape de namespace → incidente bloqueante |
| **M9** | **Post-mortem de lógica de negocio** entregado al usuario (correlación de logs con causa de negocio) | Precisión del post-mortem >80% vs diagnóstico humano; evidencia cruda junto al resumen |
| **M10** | **Despliegue GitOps del producto** (entorno estable para operadores) para el módulo 8 | Control plane desplegado y reconciliado por **Flux**; `flux get kustomization` → Ready |

### 4.2 Should Have (MVP)

| ID | Requisito |
|---|---|
| S1 | Entrega por Slack (resumen post-corrida) |
| S2 | Segunda familia de flujos (p. ej. concurrencia además del feliz camino) |
| S3 | Traspaso a humano con contexto estructurado (boot fallido o ensayo fallido: logs, payloads intentados, hipótesis, acción sugerida) |
| S4 | Panel de políticas (eventos GitHub, cuotas de namespace, confirm-required), exclusivo Admin |
| S5 | Dashboard inbox de notificaciones pendientes |

### 4.3 Could Have (futuro)

C1 Soporte a ambos motores de DB · C2 Persistencia de sesiones interrumpidas · C3 Biblioteca histórica de flujos · C4 Interfaz de auditoría completa · C5 Pull de imagen de registry además de build desde repo (parcialmente cubierto por V5).

### 4.4 Fuera de alcance (Won't Have — MVP)

Ejecutar contra staging/prod del cliente (W1); motor de ejecución propietario ilegible (W2); pricing/billing multi-tier (W3); modo Enterprise en el clúster del cliente (W4); catálogo completo de 6 clases de ataque (W5); GraphQL/gRPC como contrato de primer nivel (W6); HA multi-región (W7); auto-run en cada evento GitHub (W8); certificaciones SOC2/ISO (W9); leer el código fuente para generar tests (W10); QA visual/a11y/scanners de seguridad como producto (W11).

---

## 5. Requisitos no funcionales

### 5.1 Seguridad (SECURITY-01..15 — bloqueantes, según V2)

| ID | Requisito (resumen) | Regla |
|---|---|---|
| NF-SEG-01 | Cifrado en reposo y en tránsito (TLS 1.2+) en todas las persistentes (Postgres/Mongo, Redis, MinIO, reportes) | SECURITY-01 |
| NF-SEG-02 | Access logging en intermediarios de red del control plane (API Gateway/LB) a almacén persistente | SECURITY-02 |
| NF-SEG-03 | Logging estructurado centralizado con timestamp/request-id/nivel/mensaje; sin secrets ni PII en logs | SECURITY-03 |
| NF-SEG-04 | HTTP security headers en todos los endpoints HTML del control plane | SECURITY-04 |
| NF-SEG-05 | Validación de entrada en toda API (tipos, límites, allowlists, sanitización XSS, queries parametrizadas) | SECURITY-05 |
| NF-SEG-06 | Mínimo privilegio: sin wildcards en políticas; roles scoped | SECURITY-06 |
| NF-SEG-07 | Configuración de red deny-by-default; sin `0.0.0.0/0` salvo LB público 80/443 | SECURITY-07 |
| NF-SEG-08 | Autorización a nivel de aplicación (deny-by-default, IDOR, función, CORS restringido, validación de tokens) | SECURITY-08 |
| NF-SEG-09 | Hardening: sin credenciales por defecto, errores genéricos, sin listado de directorios, versiones soportadas | SECURITY-09 |
| NF-SEG-10 | Supply chain: lock files, escaneo de vulnerabilidades, sin `latest` en producción, SBOM, fuentes oficiales | SECURITY-10 |
| NF-SEG-11 | Diseño seguro: módulos dedicados para auth/pagos (si aplica), defensa en profundidad, **rate limiting**, misuse cases | SECURITY-11 |
| NF-SEG-12 | Autenticación: password policy 8+, hashing adaptativo (bcrypt/argon2), **MFA soportado para admin**, sesiones con expiración y cookie `Secure;HttpOnly;SameSite`, anti-brute-force, sin credenciales hardcodeadas | SECURITY-12 |
| NF-SEG-13 | Integridad: deserialización segura, checksums, acceso controlado al pipeline, SRI, datos críticos auditables | SECURITY-13 |
| NF-SEG-14 | Alerting de eventos de seguridad (auth failures, escalada, accesos), log appende-only (el product no borra sus audit logs), retención ≥90 días, dashboard | SECURITY-14 |
| NF-SEG-15 | Manejo de excepciones: handlers explícitos, **fail-closed**, limpieza de recursos en error, errores genéricos, global handler | SECURITY-15 |

**Refuerzos específicos del dominio (derivados del PRD §11):** inyección de prompts vía superficie/logs tratada como **dato, no instrucción**; red-team: filtrado de patrones de secretos antes del LLM y antes de publicar; límite duro de tiempo/tokens de planificación; sin socket de Docker del host; sin privileged.

### 5.2 Resiliencia (RESILIENCY-01..15 — guía direccional, según V1 + AR1-AR7)

| ID | Requisito | Regla |
|---|---|---|
| NF-RES-01 | Clasificación de criticalidad por workload (control plane vs sandbox) + impacto de indisponibilidad + mapa de dependencias | RESILIENCY-01 |
| NF-RES-02 | **RTO/RPO en horas; estrategia Backup & Restore** para la persistencia del control plane (evidencias, reportes, MinIO, DB de plataforma) | RESILIENCY-02/11/12 |
| NF-RES-03 | Proceso ligero de **gestión de cambios** propuesto por el flujo (registro + aprobación + nota de rollback) | RESILIENCY-03 |
| NF-RES-04 | **CI/CD = GitHub Actions**; **rollback = redeploy de versión anterior (version-pinned)**; **estilo de despliegue = directo/in-place** | RESILIENCY-04 |
| NF-RES-05 | Observabilidad 3 pilares: métricas (latencia/errores/throughput/saturación), logs centralizados, trazado distribuido, dashboard | RESILIENCY-05 |
| NF-RES-06 | Health checks shallow + deep (conectividad a deps) + integración con routing + synthetic canary | RESILIENCY-06 |
| NF-RES-07 | Alarmas de resiliencia (réplica única, lag, fallos de backup) y monitoreo de capacidad | RESILIENCY-07 |
| NF-RES-08 | **Topología: single-region multi-zona** (meta de producción). El laboratorio del módulo 8 es single-node → limitación documentada como deuda; minimizar superficie de fallo | RESILIENCY-08 |
| NF-RES-09 | Auto-scaling del control plane con límites mín/máx; cuotas y límites documentados (alarma al 80%) | RESILIENCY-09 |
| NF-RES-10 | Timeouts explícitos en todo call externo; circuit breakers; bulkheads; degradación graceful documentada | RESILIENCY-10 |
| NF-RES-11 | Estrategia DR **Backup & Restore** documentada con procedimientos de failover/failback (aplica a datos del control plane) | RESILIENCY-11 |
| NF-RES-12 | **Backups automatizados** de DB/evidencias + retención definida + cifrado + validación de restore | RESILIENCY-12 |
| NF-RES-13 | Runbooks de recovery y validación post-recovery | RESILIENCY-13 |
| NF-RES-14 | Aproximación de testing de resiliencia (game days / DR drills) — se capturan escenarios ahora, se ejecuta en Operations | RESILIENCY-14 |
| NF-RES-15 | Proceso ligero **IR/COE propuesto** (alertas → runbook → post-mortem/COE) | RESILIENCY-15 |

### 5.3 Autonomía del agente (AUTONOMIA-01/02 — siempre activas)

| ID | Regla |
|---|---|
| NF-AUT-01 | Ninguna tarea del plan aplica cambios a clúster/nube/entornos compartidos sin aprobación humana registrada; el destino del agente es un **PR con evidencia**, nunca `kubectl apply`/`terraform apply`/`helm install` autónomo; toda tarea que toque un entorno compartido produce un artefacto revisable | AUTONOMIA-01 |
| NF-AUT-02 | Todo criterio de aceptación de cada tarea se verifica **con un comando medible** (no "funciona correctamente") | AUTONOMIA-02 |

### 5.4 PBT parcial (PBT-02, 03, 07, 08, 09 — según V3)

| ID | Requisito |
|---|---|
| NF-PBT-01 | Round-trip: toda función con inversa lógica (serialización/parseo/encode-decode de la capa de datos del control plane) con PBT de round-trip | PBT-02 |
| NF-PBT-02 | Invariantes: operaciones con invariantes documentadas (p. ej. normalización de config/superficie, agrupación de hallazgos) con PBT de invariante | PBT-03 |
| NF-PBT-03 | Generadores de dominio (estructuras reales, rangos válidos, edge cases incluidos) reutilizables | PBT-07 |
| NF-PBT-04 | Shrinking habilitado; seed logueado en fallo; PBT en CI con seed fijo o logueado | PBT-08 |
| NF-PBT-05 | Framework PBT por lenguaje del stack (Go → rapid; y según capa del híbrido) documentado en el stack | PBT-09 |

### 5.5 Rendimiento y calidad (del PRD §10)

| KPI | Meta |
|---|---|
| Time-to-first-isolated-run | **< 5 minutos** (desde confirm de evento hasta primera corrida en sandbox que pasa ensayo) |
| Tasa de hallazgos lógicos confirmados (North Star) | >30% de las APIs del beachhead en 90 días |
| Ensayo a la primera | >70% al mes 6; <40% = "generador de 400s" |
| Boot del artefacto | Fallos de boot = handoff (2 reintentos), no reintento infinito |
| Completitud de teardown | **100%** |
| Precisión de post-mortem | >80% vs diagnóstico humano |
| Incidentes de límite de autonomía violado | **0** (circuit-breaker, no gradual) |
| Tasa de ruido de hallazgos | <20% saludable; >50% = "modo ruido" |

---

## 6. Casos de uso (top 5, del PRD §5)

UC1 Primera corrida aislada desde notificación · UC2 Flujo de lógica de negocio (carrera/estado) · UC3 Teardown y purga tras corrida interrumpida/abandonada · UC4 Configuración de límites de autonomía por el Head of Platform · UC5 Artefacto que no arranca / ensayo fallido → escalado a humano (fail-closed). Journeys detallados en PRD §7 (7.1 happy path usuario, 7.2 operador/admin, 7.3 edge case interrupción, 7.4 edge case escalado a humano).

---

## 7. Arquitectura de referencia (resumen del PRD §9)

- **Entorno estable del producto** (control plane, lo usan Marta y Julián): UI/API Gateway (Deployment), Identidad y Acceso (M8), GitHub+planificación (M1), Gobernanza (M7), Post-mortem (M6). Desplegado por **Flux** (Módulo 8).
- **Namespace de prueba (SUT, vive y muere con la corrida):** app Docker del cliente + DB efímera (Postgres o Mongo) + Redis + ensayo (Job) + runners (Jobs) + teardown (Job). Aislado por RBAC/NetworkPolicy.
- **Externos:** LLM API (solo planeación y post-mortem), Slack API, GitHub App, registry opcional.
- **Persistencia de evidencia/reportes:** MinIO **in-cluster** (V6).
- **Decisiones de stack:** control plane ejecución en **Go** + capa de agentes LLM en **Python o TypeScript** (V9); CI GitHub Actions; GitOps Flux; despliegue directo/in-place; rollback version-pinned; sandbox provisionado por el producto en el namespace de prueba (app + deps propias).

---

## 8. Restricciones del plan de entrega (PRD §13)

- Kubernetes es el orquestador (namespaces, Jobs, NetworkPolicy, RBAC); el **blanco** es el test namespace, no staging.
- Módulos 4-5 (CKA): test ns + Jobs de ensayo + SUT Docker + StatefulSet/PVC de DB/Redis + primeros Deployments del control plane.
- Módulo 6 (CKAD): Deployments M1/M6/M7/M8 estables; ConfigMaps/Secrets sintéticos; Jobs de runners/teardown con resource limits; probes del SUT.
- Módulo 7 (CKS): RBAC mínimo, NetworkPolicy no sale del test ns y bloquea LLM a runners, admisión de políticas (Kyverno/OPA) reforzando test ns-only, escaneo de imágenes.
- Módulo 8 (producción): producto desplegado vía **GitOps Flux**; se miden KPIs del segmento 5.5; incidentes de autonomía = 0.
- Sesión 16: demo en vivo (camino feliz, bloqueo de escape del test ns, fail-closed, métricas reales) + reconocimiento de alcance.

---

## 9. Resumen de requisitos clave

- **Qué:** plataforma **Agent (no Autonomous)** que notifica desde GitHub, y tras confirmación humana levanta la app en un namespace de prueba desechable, infiere la superficie externa, genera/ensaya/ejecuta flujos de QA de caja negra, destruye el sandbox (teardown 100%) y entrega un post-mortem de lógica de negocio. Nunca toca staging/prod del cliente.
- **MVP:** M1-M10 Must + S1-S5 Should; W1-W11 explícitamente fuera de alcance.
- **Decisiones cerradas:** build-from-repo (commit/PR) + imagen (tag); MinIO in-cluster; sesión indefinida + sandbox con grace period 24 h; reintentos boot/ensayo = 2; control plane Go + capa agentes (Python/TS); DR Backup&Restore (horas); change mgmt light + IR/COE light propuestos; GitHub Actions; rollback version-pinned; despliegue directo; single-region multi-zona; GitOps **Flux**.
- **NFR habilitadores de confianza:** límite de autonomía verificable, ensayo bloqueante, seguridad por defecto (SECURITY-01..15 bloqueantes), resiliencia direccional (RESILIENCY-01..15), PBT parcial (02/03/07/08/09).