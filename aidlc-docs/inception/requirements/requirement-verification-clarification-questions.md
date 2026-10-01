# Preguntas de Clarificación — Requirements Analysis

> **Nota (2026-10-01, realineación warm)**: documento **histórico** de la fase de Requirements Analysis (pre-pivote). La contradicción #1 se resolvió entonces con "sandbox + grace period / teardown 100%"; el **pivote a entorno warm (2026-09-17)** sustituyó ese modelo por **entorno warm reutilizable + reset verificado + scale-down en idle + rebuild/teardown periódico** (KPI "reset verificado 100% + higiene de rebuild"). La respuesta vigente es **V7 en `requirements.md`**. Las respuestas de resiliencia (2-8) siguen vigentes. Se conserva el texto original como registro del proceso.

Tras analizar tus respuestas detecté **1 contradicción** en la Pregunta 7 y, al activar la
extensión **Resiliency Baseline**, esta extensión **obliga a preguntarte** (no me permite
decidir por ti) varias decisiones de arquitectura. Responde tras cada `[Answer]:`.

---

## Contradicción 1: Expiración de sesiones (Q7 = C) vs. teardown forzado (PRD §6.3, §7.3, KPI §10)

Respondiste **C) Indefinido hasta cierre explícito** ("el sandbox vive hasta que el usuario o un
admin lo cierra"). Esto **contradice** tres reglas del PRD:
- Principio no negociable #3: teardown forzado — "destruye los workloads al terminar **o abandonar**".
- Edge case 7.3: "Si nunca vuelve, un proceso de housekeeping cierra sesiones colgadas y garantiza teardown".
- KPI de calidad: **Completitud de teardown 100%** (y riesgo #1: sandbox leftover).

Si un sandbox abandonado vive indefinidamente, el leftover es un incidente de confianza esperable.

### Pregunta de clarificación 1
¿Cómo reconciliar la preferencia "indefinido" con el teardown 100%?

A) **Sesión/plan persistido indefinidamente, sandbox con grace period** — el plan generado queda guardado para reanudar sin repetir inferencia, pero el sandbox (app + deps + runners) se destruye tras un tiempo de inactividad acotado (p. ej. 24 h). Satisfacer el edge case 7.3 y el KPI. (Recomendado)

B) Literal C: sandbox **y** sesión viven hasta cierre explícito del usuario/admin — se acepta el riesgo de leftover; el KPI de teardown queda supeditado al cierre manual.

C) Cambiar a 24 h con notificación previa (opción B original).

X) Otra (describe tras el tag [Answer])

[Answer]: A

---

## Pregunta de clarificación 2: RTO/RPO y estrategia de Disaster Recovery (RESILIENCY-02)

¿Cuáles son tus objetivos de Recovery Time Objective (RTO) y Recovery Point Objective (RPO)? Determinantes de la estrategia de DR y de la redundancia.

A) RPO/RTO en horas — estrategia Backup & Restore (coste más bajo; datos respaldados, servicios sin desplegar; se redespliega desde IaC y se restaura en fallo)

B) RPO/RTO en decenas de minutos — Pilot Light (coste $$; datos vivos, servicios ociosos)

C) RPO/RTO en minutos — Warm Standby (coste $$$; datos vivos, servicios a capacidad reducida)

D) RPO/RTO casi en tiempo real — Multi-site Active/Active (coste $$$$; requiere multi-región)

E) N/A — despliegue single-region aceptable; se confía en disponibilidad multi-zona

X) Otra (describe tras el tag [Answer])

[Answer]: A

---

## Pregunta de clarificación 3: Proceso de gestión de cambios (RESILIENCY-03)

¿Cómo deben gobernarse los cambios de producción de este workload? (El flujo conformará el diseño a tu respuesta, no inventará un proceso.)

A) Usar el proceso de gestión de cambios existente de la organización — indica el nombre/herramienta. El flujo lo referenciará y hará que los artefactos encajen (registros de cambio, gates de aprobación).

B) No existe un proceso formal — el flujo debe proponer un proceso ligero (registro de cambio + aprobación + nota de rollback).

C) N/A — este workload está exento de gestión de cambios formal (p. ej. herramienta interna). Documentar la exención.

X) Otra (describe tras el tag [Answer])

[Answer]: B

---

## Pregunta de clarificación 4: CI/CD y tooling de despliegue (RESILIENCY-04)

¿Qué tooling CI/CD y proceso de despliegue debe usar este workload?

A) Usar el pipeline de CI/CD existente — indica la herramienta (p. ej. GitHub Actions, GitLab CI, Jenkins). El flujo producirá artefactos compatibles.

B) No existe pipeline — el flujo debe proponer una definición de pipeline acorde al IaC y runtime elegidos.

X) Otra (describe tras el tag [Answer])

[Answer]: A

---

## Pregunta de clarificación 5: Mecanismo de rollback (RESILIENCY-04)

¿Cómo debe revertirse un despliegue de producción fallido?

A) Redesplegar la versión anterior del artefacto/IaC (rollback por versión fijada)

B) Blue/green: volver a cambiar al entorno anterior

C) Canary con auto-rollback ante regresión de salud/métricas

D) Rollback sensible a base de datos (requiere inversión de migración de esquema/datos) — marcar para diseño explícito

E) Usar el procedimiento de rollback existente de la organización — indica la referencia

X) Otra (describe tras el tag [Answer])

[Answer]: A

---

## Pregunta de clarificación 6: Estilo de despliegue (RESILIENCY-04)

¿Qué estrategia de despliegue es aceptable para el perfil de riesgo de este workload?

A) Directo / in-place (menor coste, mayor blast radius) — aceptable para workloads no críticos

B) Rolling (reemplazo gradual de instancias)

C) Blue/green (cutover sin downtime, mayor coste)

D) Canary (cambio de tráfico progresivo con rollback automático)

X) Otra (describe tras el tag [Answer])

[Answer]: A

---

## Pregunta de clarificación 7: Topología regional (RESILIENCY-08)

¿Este workload requiere despliegue multi-región, o basta single-region con redundancia multi-zona? (El PRD, MVP, excluye HA multi-región; la extensión requiere tu decisión explícita.)

A) Single-region multi-zona — tolera fallo de zona, no de región completa. Coste menor. (Alineado con RTO/RPO opciones A/B/E.)

B) Multi-región activo-pasivo — sobrevive a un fallo de región con failover. Coste mayor.

C) Multi-región activo-activo — sobrevive a fallo de región sin downtime. Coste máximo.

X) Otra (describe tras el tag [Answer])

[Answer]: A

---

## Pregunta de clarificación 8: Proceso de respuesta a incidentes (RESILIENCY-15)

¿Cómo se manejan los incidentes de producción de este workload?

A) Usar el proceso de respuesta a incidentes existente — indica la referencia (p. ej. runbooks de PagerDuty, proceso IR/on-call interno). El flujo alineará alertas y runbooks.

B) No existe proceso formal — el flujo debe proponer un proceso ligero de respuesta a incidentes y Correction of Errors (COE).

X) Otra (describe tras el tag [Answer])

[Answer]: B