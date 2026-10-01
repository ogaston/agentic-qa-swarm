# Warm Realignment Plan — Agentic QA Swarm

> **Motivo**: el pivote a **entorno warm reutilizable** (2026-09-17) quedó registrado solo en `specs/prd.md`, `entradas/prd.md`, `DECISIONES.md` y `audit.md`. El commit `42b790e` NO incluyó `unidades-y-tareas.md` ni `aidlc-state.md` (aunque el audit afirmaba haberlos tocado), y ningún artefacto de INCEPTION fue realineado. Este plan propaga el PRD autoritativo a todos los artefactos de INCEPTION + al consolidado + al estado.
>
> **Fuente autoritativa**: `specs/prd.md` (§4 principios 2/3/6, §5 UC1-UC4, §7 journeys 7.1-7.3, §8 MoSCoW M1-M10/S1-S5, §9 módulos M1-M8, KPIs §10).
>
> **Cambio semántico (resumen)**: SUT = entorno warm de la plataforma (namespace de prueba de vida larga, aislado de staging/prod) con app + 1 DB + 1 Redis pre-desplegados y **reutilizados**. Cada corrida **despliega el artefacto sobre el warm**; entre corridas aplica **reset verificado** (`reset_verified=true`; cuarentena si falla); **scale-down en idle**; **rebuild/teardown periódico** como higiene; **workflows de negocio** (complejidad/cuotas/timeouts/aprobaciones). Gates: confirm + `ensayo_passed` + `reset_verified` + test-ns-only. KPI: **reset verificado 100% + higiene de rebuild** (reemplaza teardown 100%). Riesgo #1: **contaminación entre corridas**.
>
> **Alcance**: sin código (CONSTRUCTION sigue en SKIP). Solo documentación.

## Plan de ejecución (checklist)

- [x] Paso 1: Crear este plan y registrar la auditoría de entrada
- [x] Paso 2: Realinear `requirements/requirements.md` (principios, V7, M1-M10, S3-S5, KPIs, UC3, arquitectura, entrega, resumen)
- [x] Paso 3: Realinear `application-design/components.md` (C3 → Warm Environment Manager, C5 → Verified Reset & Housekeeping)
- [x] Paso 4: Realinear `application-design/component-methods.md` (deployToWarm, resetVerified, quarantine, rebuild, workflows)
- [x] Paso 5: Realinear `application-design/services.md` (go-warm-manager, go-reset, Jobs warm, eventos, máquina de estados)
- [x] Paso 6: Realinear `application-design/component-dependency.md` (matriz, flujos, riesgos)
- [x] Paso 7: Realinear `application-design/application-design.md` (consolidado + trazabilidad + compliance)
- [x] Paso 8: Realinear `user-stories/stories.md` + `personas.md`
- [x] Paso 9: Realinear `application-design/unit-of-work.md`, `unit-of-work-dependency.md`, `unit-of-work-story-map.md`
- [x] Paso 10: Realinear planes `plans/*.md` (execution, application-design, unit-of-work, story-generation, user-stories-assessment)
- [x] Paso 11: Realinear planes de tareas por unidad `unit-task-plans/U1-U5.md`
- [x] Paso 12: Realinear consolidado `unidades-y-tareas.md` (raíz)
- [x] Paso 13: Corregir `aidlc-docs/aidlc-state.md` (duplicados, workspace root, etapa warm)
- [x] Paso 14: Completar contexto WHAT/WHY/HOW en `AGENTS.md`
- [x] Paso 15: Validar consistencia (grep de términos obsoletos) y registrar auditoría de salida

## Resultado

Realineados al modelo warm: `requirements.md`, `components.md`, `component-methods.md`, `services.md`, `component-dependency.md`, `application-design.md`, `stories.md`, `personas.md`, `unit-of-work.md`, `unit-of-work-dependency.md`, `unit-of-work-story-map.md`, `plans/execution-plan.md`, `plans/application-design-plan.md`, `plans/unit-of-work-plan.md`, `plans/story-generation-plan.md`, `plans/user-stories-assessment.md`, `unit-task-plans/U2-U5.md`, `unidades-y-tareas.md`, `aidlc-state.md` y `AGENTS.md`. Notas históricas añadidas en los ficheros de preguntas (pre-pivote). Sin código; CONSTRUCTION sigue en SKIP.
