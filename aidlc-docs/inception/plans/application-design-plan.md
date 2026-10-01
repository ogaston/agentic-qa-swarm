# Application Design Plan — Agentic QA Swarm

> **Nota (2026-10-01, realineación warm)**: las preguntas y opciones de abajo son el **registro histórico** de la fase (pre-pivote); su redacción "boot/sandbox/teardown" se refiere al modelo efímero entonces vigente. El diseño vigente usa **entorno warm reutilizable** (ver `components.md`/`services.md` realineados y `warm-realignment-plan.md`).
>
> Contexto analizado (Step 1): `requirements.md` (8 módulos M1-M8, principios #1-#6, decisiones V1-V9/AR1-AR9) + `stories.md` (13 historias Must) + `personas.md`. Dos planos: **control plane estable** (UI/API, identidad, planificación, gobernanza, post-mortem) y **entorno warm reutilizable** en el test namespace (app desplegada por corrida + DB + Redis pre-desplegados + Jobs/rollouts). Stack híbrido (Go ejecución + capa agentes), Flux, MinIO, GitHub Actions.

## Plan de diseño (checklist)

- [x] Paso 1: Confirmar decisiones de diseño a partir de las respuestas (Preguntas 1-5)
- [x] Paso 2: Generar `aidlc-docs/inception/application-design/components.md` (componentes + responsabilidades + interfaces)
- [x] Paso 3: Generar `aidlc-docs/inception/application-design/component-methods.md` (firmas de métodos, I/O; reglas de negocio detalladas quedan para Functional Design)
- [x] Paso 4: Generar `aidlc-docs/inception/application-design/services.md` (servicios + orquestación)
- [x] Paso 5: Generar `aidlc-docs/inception/application-design/component-dependency.md` (matriz + patrones de comunicación + flujos de datos)
- [x] Paso 6: Generar `aidlc-docs/inception/application-design/application-design.md` (consolidado)
- [x] Paso 7: Validar completitud y consistencia (trazabilidad M1-M8 → componentes → US-M*) + compliance de extensiones (Security/Resiliency/PBT/autonomía a nivel diseño)
- [x] Paso 8: Presentar artefactos para aprobación

## Artefactos obligatorios (incluidos en el plan)

- [x] components.md — definiciones de componentes y responsabilidades de alto nivel
- [x] component-methods.md — firmas de métodos (las reglas de negocio detalladas van después, en Functional Design)
- [x] services.md — definiciones de servicios y patrones de orquestación
- [x] component-dependency.md — relaciones de dependencia y patrones de comunicación
- [x] application-design.md — consolidado de los documentos anteriores
- [x] Validar completitud y consistencia del diseño

## Propuesta base (sobre la que preguntan las Preguntas 1-5)

- Componentes candidatos 1:1 con módulos: Ingesta GitHub + Inbox (M1), Ensayo (M2), Warm Environment Manager (M3), Runner QA (M4), Verified Reset & Housekeeping (M5), Post-mortem/Reporter (M6), Gobernanza/Policy (M7), Identidad (M8), Evidencia/MinIO (M6-infra).
- Orquestación candidata: Run Controller central con máquina de estados notify→confirm→warm ready→deploy sobre warm→infer→rehearse→run→reset verificado→report + rollouts/Jobs K8s por fase en el test ns.
- Comunicación candidata: REST sincrónico entre servicios del control plane; rollouts/Jobs/eventos K8s para el lifecycle de corridas.
- Despliegue candidato: un Deployment por servicio del control plane + entorno warm gestionado (rollouts/Jobs) + Flux.

---

## Pregunta 1: Límites de componentes (Component Identification)

¿Los componentes siguen 1:1 a los módulos M1-M8 o se consolidan?

A) **1:1 con M1-M8** (9 componentes candidatos de la propuesta base; máxima trazabilidad requisito→componente→unidad)

B) **Consolidado** — control plane en 4 componentes (Ingesta/Inbox, Planificación+Ensayo, Gobernanza+Identidad, Post-mortem/Reporter) + 1 lifecycle de sandbox (provision+run+teardown) (menos piezas, límites más gruesos)

C) **Híbrido** — control plane consolidado (B) pero warm lifecycle separado por fase (deploy sobre warm, ensayo, runners, reset como Jobs independientes) (Recomendado: equilibra operabilidad y granularidad de Jobs)

X) Otra (describe tras el tag [Answer])

[Answer]: A

---

## Pregunta 2: Orquestación del ciclo de corrida (Service Layer Design)

¿Quién orquesta la máquina de estados notify→confirm→boot→infer→rehearse→run→teardown→report?

A) **Run Controller central** con máquina de estados persistida (sesión/plan) que crea Jobs/rollouts por fase y aplica gates (confirm, `reset_verified`, `ensayo_passed`) (Recomendado: un solo lugar para gates y auditoría)

B) **Coreografía por eventos** — cada fase reacciona a eventos de la anterior sin orquestador central

C) **Híbrido** — controller central para gates/decisiones + controllers por fase para ejecución

X) Otra (describe tras el tag [Answer])

[Answer]: A

---

## Pregunta 3: Comunicación entre servicios (Component Dependencies)

¿Cómo se comunican los servicios del control plane entre sí y con el sandbox?

A) **REST sincrónico** entre servicios del control plane + **Jobs/eventos K8s** para el lifecycle de corridas (Recomendado: simple, observable con logs centralizados)

B) **Eventos/colas** (p. ej. NATS) para todo el desacoplo interno, REST solo hacia la UI

C) **Híbrido** — REST para UI/API y operaciones admin; eventos para el pipeline de corridas (notify, boot-done, ensayo-passed, run-done, teardown-done)

X) Otra (describe tras el tag [Answer])

[Answer]: C

---

## Pregunta 4: Granularidad de despliegue (Component Identification)

¿Cómo se empaquetan los componentes del control plane para Flux (Módulo 8)?

A) **Un Deployment por servicio** (UI/API, planner, governance, reporter, identity) + entorno warm gestionado (rollouts/Jobs) (máximo aislamiento y escalado independiente)

B) **Monolito modular** — un solo Deployment del control plane con módulos internos + entorno warm gestionado (menor sobrecarga operativa en el clúster del curso)

C) **Por lenguaje del híbrido** — servicio(s) Go para ejecución (controller, runners-operator, teardown) + servicio(s) de capa de agentes (planificación, post-mortem) (Recomendado: alinea despliegue con V9 y con el límite cerebro/músculo)

X) Otra (describe tras el tag [Answer])

[Answer]: A

---

## Pregunta 5: Estilo arquitectónico (Design Patterns)

¿Qué estilo guía el diseño del control plane? (Debe soportar: puertos GitHub/K8s/LLM/Slack/MinIO, rate limiting SECURITY-11, logging estructurado SECURITY-03, time-outs/circuit breakers RESILIENCY-10.)

A) **Hexagonal (puertos y adaptadores)** — núcleo de dominio (corridas, gates, políticas) con adaptadores por integración (Recomendado: aísla el dominio de GitHub/K8s/LLM/Slack/MinIO y facilita testeo)

B) **Capas clásicas** (handler → service → repository) por servicio

C) Sin estilo impuesto — se decide por servicio en Functional Design

X) Otra (describe tras el tag [Answer])

[Answer]: A