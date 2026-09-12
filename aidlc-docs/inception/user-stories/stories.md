# User Stories — Agentic QA Swarm (Must / MVP)

> Enfoque aprobado: épicas por módulo, una historia por ítem MoSCoW (sub-historias solo en ítems compuestos M7/M8), criterios en checklist verificable, alcance Must (M1-M10). Trazabilidad: [Req] → [UC] → [Journey PRD §7].

## US-M1 — Notificación GitHub sin auto-run

- **Épica**: M1 GitHub, artefacto y planificación. **Personas**: Marta (P1).
- **Narrativa**: Como Marta, quiero que cada commit/PR/tag de mi repo conectado cree una notificación en el inbox (evento, SHA/PR/tag, artefacto a levantar) para decidir qué confirmar, sin que nada corra solo.
- **Trazabilidad**: [M1] [UC1] [Journey 7.1 pasos 1-2].
- **Criterios de aceptación**:
  - [ ] Un push/PR/tag en repo conectado crea exactamente una notificación visible en el inbox con evento, SHA/PR/tag y artefacto.
  - [ ] Tras el evento, `kubectl get jobs -n <test-ns>` no muestra ningún Job nuevo (cero auto-run desde el webhook).
  - [ ] La GitHub App opera con permisos mínimos (solo lectura de eventos/contenido necesario para el artefacto).

## US-M2 — Pull del artefacto y boot aislado

- **Épica**: M1/M3 aprovisionamiento inicial. **Personas**: Marta (P1).
- **Narrativa**: Como Marta, quiero que tras mi confirmación el sistema traiga el artefacto (build-from-repo para commit/PR, imagen publicada para tag/release) y arranque la app en Docker dentro del namespace de prueba, para tener un SUT desechable sin tocar staging.
- **Trazabilidad**: [M2] [UC1] [Journey 7.1 paso 3] [V5].
- **Criterios de aceptación**:
  - [ ] Tras confirmar, el artefacto correcto se obtiene según el tipo de evento (repo para PR/commit, imagen para tag).
  - [ ] La app alcanza estado `Running` en el test ns (`kubectl get pods -n <test-ns>`), con 1 DB (Postgres o Mongo) + 1 Redis de esa corrida.
  - [ ] Si el boot falla 2 veces, el sistema se detiene fail-closed y genera handoff (ver US-M5-límite en M2/UC5): sin corrida completa y sin reintento infinito.
  - [ ] Ningún secret de staging/prod del cliente está montado en el sandbox (solo config/secrets sintéticos o declarados para el test ns).

## US-M3 — Inferencia de superficie externa

- **Épica**: M1 planificación. **Personas**: Marta (P1).
- **Narrativa**: Como Marta, quiero que el agente observe solo la superficie externa del sandbox (OpenAPI si está expuesta; si no, sondeo HTTP/UI de puertos publicados) y recomiende flujos de QA, sin leer el código fuente de mi app.
- **Trazabilidad**: [M3] [UC1, UC2] [Journey 7.1 paso 4].
- **Criterios de aceptación**:
  - [ ] La superficie inferida contiene únicamente endpoints/puertos observados desde fuera del contenedor (OpenAPI expuesta o sondeo de puertos publicados).
  - [ ] El plan de flujos generado referencia solo elementos de la superficie observada (auditable contra el artefacto de superficie).
  - [ ] Ningún paso del pipeline lee el código fuente del repo para generar tests (caja negra verificable en logs).

## US-M4 — Flujos de QA inspectables

- **Épica**: M1/M4 generación y ejecución. **Personas**: Marta (P1).
- **Narrativa**: Como Marta, quiero seleccionar flujos recomendados y que el sistema genere artefactos de flujo estándar e inspectables (p. ej. k6), ejecutables y versionables fuera de la plataforma.
- **Trazabilidad**: [M4] [UC2] [Journey 7.1 pasos 4-6] [Principio #5].
- **Criterios de aceptación**:
  - [ ] Cada flujo generado es un archivo estándar ejecutable de forma independiente (no binario ni formato propietario ilegible).
  - [ ] El flujo puede descargarse, auditarse y versionarse fuera de la plataforma.
  - [ ] Los runners ejecutan el flujo sin llamadas al LLM y sin credenciales de modelo en el pod.

## US-M5 — Ensayo bloqueante

- **Épica**: M2 ensayo. **Personas**: Marta (P1, automático tras su selección).
- **Narrativa**: Como Marta, quiero que ningún plan escale a corrida completa sin un ensayo unitario exitoso dentro del sandbox, para no quemar compute en planes rotos.
- **Trazabilidad**: [M5] [UC1, UC2] [Journey 7.1 paso 5] [Principio #2].
- **Criterios de aceptación**:
  - [ ] El gate de pipeline exige `ensayo_passed=true` (ensayo = un flujo unitario con 2xx/invariante mínima dentro del sandbox) antes de autorizar la corrida completa.
  - [ ] Tras 2 ensayos fallidos, el sistema escala a humano con contexto (fail-closed, ver US de traspaso en S3 — fuera de este alcance Must; el handoff mínimo queda cubierto por el reporte de motivo).
  - [ ] El ensayo no puede omitirse por configuración ni a petición (gate condicionado técnicamente).

## US-M6 — Corrida aislada y recolección de evidencia

- **Épica**: M4 ejecución. **Personas**: Marta (P1).
- **Narrativa**: Como Marta, quiero que la corrida completa ejecute los flujos contra el sandbox y recoja logs/evidencia, persistidos en MinIO in-cluster para el post-mortem.
- **Trazabilidad**: [M6] [UC1, UC2] [Journey 7.1 paso 6] [V6].
- **Criterios de aceptación**:
  - [ ] Los Jobs de runners completan en el mismo test ns (`kubectl get jobs -n <test-ns>` en `Complete`/`Failed` registrado).
  - [ ] Logs y evidencia de la corrida quedan persistidos en MinIO in-cluster y referenciados desde el reporte.
  - [ ] La corrida solo existe tras confirm + ensayo (nunca directa desde el webhook).

## US-M7.1 — Teardown garantizado tras cada corrida

- **Épica**: M5 teardown. **Personas**: Julián (P2, automático).
- **Narrativa**: Como Julián, quiero que al terminar cada corrida (éxito, fallo o cancelación) el sistema destruya app + deps + runners de esa corrida y lo verifique, para que el clúster nunca quede sucio.
- **Trazabilidad**: [M7] [UC3] [Journey 7.3 pasos 2-4] [Principio #3].
- **Criterios de aceptación**:
  - [ ] Tras cada finalización, `kubectl get all -n <test-ns>` no lista workloads de esa corrida (app, DB, Redis, runners).
  - [ ] La verificación de "namespace limpio" queda registrada antes de notificar el fin.
  - [ ] Completitud de teardown = 100% (KPI; cualquier resto = incidente).

## US-M7.2 — Housekeeping de sesiones abandonadas

- **Épica**: M5 housekeeping. **Personas**: Julián (P2, automático).
- **Narrativa**: Como Julián, quiero que las sesiones interrumpidas o abandonadas apliquen el mismo teardown tras el grace period (24 h configurables) mientras el plan/sesión persiste para reanudar, para conciliar "indefinido" con teardown 100%.
- **Trazabilidad**: [M7] [UC3] [Journey 7.3 pasos 5-7] [V7].
- **Criterios de aceptación**:
  - [ ] Una sesión sin actividad más allá del grace period dispara teardown del sandbox sin acción del usuario.
  - [ ] El plan generado persiste como sesión "incompleta" reutilizable sin repetir inferencia válida.
  - [ ] Un proceso de housekeeping cierra sesiones colgadas y garantiza teardown aunque el usuario nunca vuelva.

## US-M8.1 — RBAC confinado al test namespace

- **Épica**: M7 gobernanza (guardrails). **Personas**: Julián (P2).
- **Narrativa**: Como Julián, quiero que ningún workload del producto pueda agendar o leer fuera del namespace de prueba, para que el aislamiento sea verificable y no una promesa.
- **Trazabilidad**: [M8] [UC4] [Principio #4] [NF-SEG-06].
- **Criterios de aceptación**:
  - [ ] `kubectl auth can-i --list` desde las ServiceAccounts de runners/ensayo/teardown niega todo fuera del test ns.
  - [ ] Ningún rol del producto incluye wildcards de acciones/recursos sin excepción documentada.
  - [ ] Un intento de salir del test ns queda bloqueado y auditado (demo Sesión 16).

## US-M8.2 — NetworkPolicy sin egress y sin LLM para runners

- **Épica**: M7 gobernanza (guardrails). **Personas**: Julián (P2).
- **Narrativa**: Como Julián, quiero que el tráfico del test ns no salga de ese namespace y que los runners no puedan alcanzar APIs de LLM, para que ni un modelo engañado pueda exfiltrar o escapar.
- **Trazabilidad**: [M8] [UC4] [Principio #4] [NF-SEG-07].
- **Criterios de aceptación**:
  - [ ] La NetworkPolicy del test ns niega egress fuera del namespace (verificable con un pod de prueba que intenta salir).
  - [ ] Los runners no resuelven ni alcanzan endpoints de LLM (bloqueo a nivel de red, no solo de configuración).
  - [ ] Regla deny-by-default: solo puertos/servicios requeridos abiertos dentro del test ns.

## US-M8.3 — Confirm-required y cero acceso a staging/prod

- **Épica**: M7 gobernanza (límite de autonomía). **Personas**: Julián (P2), VP Eng (P3, evidencia).
- **Narrativa**: Como Julián, quiero que auto-run esté apagado por defecto, que cada corrida exija confirmación persistida y auditable, y que staging/producción del cliente estén fuera de alcance por construcción, para superar el veto de confianza del buyer.
- **Trazabilidad**: [M8] [UC4] [Principio #4] [Journey 7.2 pasos 3-4].
- **Criterios de aceptación**:
  - [ ] Con auto-run apagado, un evento GitHub nunca crea Jobs (solo notificación).
  - [ ] Cada corrida referencia un registro de confirmación persistido y auditable.
  - [ ] Cero credenciales/allowlist de staging/prod del cliente en el clúster del producto (verificable en Secrets y políticas); el único allowlist es el Service del sandbox en el test ns.

## US-M9 — Post-mortem de lógica de negocio

- **Épica**: M6 reportería. **Personas**: Marta (P1), VP Eng (P3, evidencia).
- **Narrativa**: Como Marta, quiero recibir el post-mortem que correlacione logs con la causa de negocio (invariante violada o confirmación de resistencia) junto a la evidencia cruda, para corregir antes del merge/release sin haber escrito tests.
- **Trazabilidad**: [M9] [UC1, UC2] [Journey 7.1 pasos 7-9].
- **Criterios de aceptación**:
  - [ ] Cada corrida terminada produce un reporte con: flujos corridos, invariante evaluada, veredicto y enlaces a evidencia cruda (logs) en MinIO.
  - [ ] La causa raíz del reporte coincide con la causa real conocida en el dataset de evaluación (precisión >80%).
  - [ ] Los reportes filtran patrones de secretos antes de pasar por el LLM y antes de publicarse.

## US-M10 — Producto desplegado vía GitOps

- **Épica**: M10 plataforma estable. **Personas**: Julián (P2).
- **Narrativa**: Como Julián, quiero que el entorno estable del producto (UI/API, identidad, planificación, gobernanza, post-mortem) viva desplegado y reconciliado por Flux, con CI en GitHub Actions y rollback por redeploy de versión anterior.
- **Trazabilidad**: [M10] [Journey 7.2 paso 7] [AR3, AR4, AR5, AR9] [Módulo 8].
- **Criterios de aceptación**:
  - [ ] `flux get kustomizations` muestra los componentes del control plane en estado `Ready` y reconciliados desde el repo Git.
  - [ ] El pipeline de GitHub Actions construye, escanea y publica artefactos versionados (sin tags `latest` en producción).
  - [ ] Un despliegue fallido se revierte redesplegando la versión anterior fijada (rollback version-pinned documentado).