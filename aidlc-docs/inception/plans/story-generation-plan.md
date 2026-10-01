# Story Generation Plan — Agentic QA Swarm

> Rol asumido: product owner. Metodología de conversión de requisitos en historias.
> Alcance de este plan: decisiones de estructura y formato de historias (NO priorización ni tareas de desarrollo).

## Plan de ejecución (checklist)

- [x] Paso 1: Confirmar enfoque de desglose y formato a partir de las respuestas (Preguntas 1-5)
- [x] Paso 2: Generar `aidlc-docs/inception/user-stories/personas.md` (arquetipos + características + mapeo a historias)
- [x] Paso 3: Generar `aidlc-docs/inception/user-stories/stories.md` (historias INVEST + criterios de aceptación)
- [x] Paso 4: Verificar INVEST (Independent, Negotiable, Valuable, Estimable, Small, Testable) y trazabilidad a requisitos (M1-M10, S1-S5, UC1-UC5)
- [x] Paso 5: Presentar artefactos para aprobación (Parte 2 completada)

## Artefactos obligatorios (incluidos en el plan)

- [x] Generar stories.md con historias siguiendo criterios INVEST
- [x] Generar personas.md con arquetipos y características
- [x] Asegurar historias Independent, Negotiable, Valuable, Estimable, Small, Testable
- [x] Incluir criterios de aceptación en cada historia
- [x] Mapear personas a historias relevantes

## Opciones de desglose (trade-offs)

- **User Journey-Based**: las historias siguen los journeys 7.1-7.4 del PRD (happy path usuario, operador/admin, interrupción, escalado a humano). Pro: fidelidad al flujo real. Contra: cruza módulos (una historia toca M1+M2+M4+M5).
- **Feature-Based**: historias organizadas por capacidad (notificaciones, sandbox, ensayo, teardown, post-mortem, gobernanza). Pro: alineado a M1-M8. Contra: pierde la narrativa por rol.
- **Persona-Based**: historias agrupadas por Marta (user), Julián (admin), buyer (VP Eng). Pro: clarifica permisos (M7 exclusivo admin). Contra: fragmenta flujos transversales.
- **Domain-Based**: historias por dominios (ingesta GitHub, ejecución aislada, gobernanza, reportería). Pro: bounded contexts claros. Contra: los gates (confirm/ensayo) quedan repartidos.
- **Epic-Based**: épicas jerárquicas (una por módulo M1-M8) con sub-historias. Pro: trazabilidad directa requisito → épica → unidad de trabajo. Contra: épicas grandes si no se subdividen.
- **Híbrido**: épicas por módulo + historias de journey que las cruzan, con mapeo explícito. Pro: combina trazabilidad y narrativa. Contra: más artefactos que mantener.

---

## Pregunta 1: Enfoque de desglose (Breakdown Approach)

¿Qué enfoque de organización deben seguir las historias?

A) **Epic-Based por módulo** — una épica por M1-M8 con sub-historias (máxima trazabilidad a requisitos y a unidades de trabajo)

B) **Journey-Based** — historias siguiendo los journeys 7.1-7.4 y UC1-UC5 (máxima fidelidad al flujo por rol)

C) **Híbrido** — épicas por módulo + historias de journey con mapeo explícito entre ambas (Recomendado: trazabilidad + narrativa)

D) **Persona-Based** — agrupadas por Marta / Julián / buyer

X) Otra (describe tras el tag [Answer])

[Answer]: A

---

## Pregunta 2: Granularidad (Story Granularity)

¿Qué tamaño deben tener las historias?

A) **Una historia por ítem MoSCoW** (M1-M10, S1-S5) con sub-historias solo donde el ítem sea compuesto (p. ej. M7)

B) **Fina** — cada capacidad verificable es una historia propia (p. ej. "crear notificación", "confirmar corrida", "pull de artefacto" por separado)

C) **Por caso de uso** — una historia por UC1-UC5 más historias de journeys de borde (7.3, 7.4)

X) Otra (describe tras el tag [Answer])

[Answer]: A

---

## Pregunta 3: Formato de criterios de aceptación (Acceptance Criteria)

¿Qué formato deben usar los criterios de aceptación? (Deben ser testeables; AUTONOMIA-02 exigirá comando verificable a nivel de tarea en Units Generation.)

A) **Checklist verificable** — cada criterio es comprobable por observación o comando (Recomendado: compatible con gates `reset_verified=true`, `ensayo_passed=true`, reset verificado 100%)

B) **Gherkin (Given/When/Then)** — formato BDD clásico por historia

C) **Mixto** — Gherkin para flujos de usuario (journeys) + checklist para gates técnicos (ensayo, teardown, RBAC)

X) Otra (describe tras el tag [Answer])

[Answer]: A

---

## Pregunta 4: Personas (User Personas)

¿Qué personas se documentan en personas.md? (El PRD ya define a Marta —Backend Lead—, Julián —Head of Platform— y al buyer VP Eng.)

A) **Las del PRD tal cual** — Marta (user), Julián (admin), VP Eng (buyer con veto de confianza) (Recomendado: sin inventar roles)

B) Las del PRD + **SRE/Auditor** como persona adicional (trazabilidad de auditoría y evidencia)

C) Solo **dos operativas** — Platform Engineer (user) y Head of Platform (admin); el buyer como stakeholder, no persona

X) Otra (describe tras el tag [Answer])

[Answer]: A

---

## Pregunta 5: Alcance de historias (Business Context)

¿Qué alcance cubren las historias generadas ahora? (El alcance global del trabajo es solo MVP por tu respuesta V4.)

A) **Must + Should** — historias para M1-M10 y S1-S5 (Recomendado: el plan de tareas por unidad debe contemplar todo el MVP)

B) **Solo Must** — historias para M1-M10; los Should se historian después

X) Otra (describe tras el tag [Answer])

[Answer]: B