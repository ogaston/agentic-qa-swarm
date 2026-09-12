# Unit of Work Plan — Agentic QA Swarm

> Descomposición del sistema en unidades de trabajo (Units Generation, Parte 1). Punto de parada declarado del trabajo: **plan de tareas de cada unidad** (incluido como pasos del plan). Términos: "Unit of Work" = agrupación de historias para desarrollo; cada unidad puede contener uno o más servicios desplegables.

## Propuesta base de unidades (a confirmar en Preguntas 1-6)

- **U1 Ingesta & Inbox** — C1 (ui-api + go-intake): webhooks, notificaciones, confirm. Historias: US-M1, US-M2 (confirm).
- **U2 Orquestación & Sandbox Lifecycle** — C9 + C3 + C2 + C4 + C5 (plano de ejecución Go): controller, provision, ensayo, runners, teardown/housekeeping. Historias: US-M2 (boot), US-M5, US-M6, US-M7.1, US-M7.2.
- **U3 Agentes LLM** — agent-planner + agent-reporter (C3-infer/gen + C6): superficie→flujos, post-mortem. Historias: US-M3, US-M4 (generación), US-M9.
- **U4 Gobernanza & Identidad** — C7 + C8 (go-governance + go-identity): políticas, gates, auditoría, auth. Historias: US-M8.1, US-M8.2, US-M8.3.
- **U5 Plataforma & GitOps** — M10 transversal: Flux, CI GitHub Actions, MinIO, observabilidad base,esmaltado de red/RBAC base. Historias: US-M10 (+ habilita a todas).

## Plan de ejecución (checklist)

- [x] Paso 1: Confirmar descomposición y estrategia a partir de las respuestas (Preguntas 1-6)
- [x] Paso 2: Generar `aidlc-docs/inception/application-design/unit-of-work.md` (definiciones + responsabilidades + estrategia de organización de código greenfield)
- [x] Paso 3: Generar `aidlc-docs/inception/application-design/unit-of-work-dependency.md` (matriz de dependencias entre unidades)
- [x] Paso 4: Generar `aidlc-docs/inception/application-design/unit-of-work-story-map.md` (todas las historias asignadas a unidades)
- [x] Paso 5: Generar planes de tareas por unidad `aidlc-docs/inception/application-design/unit-task-plans/U1.md` … `U5.md` (tareas con criterio de aceptación verificable por comando, AUTONOMIA-02; sin `apply` autónomo, AUTONOMIA-01) — **punto de parada del trabajo**
- [x] Paso 6: Validar límites de unidades y dependencias (todas las historias asignadas; sin dependencias circulares no justificadas)
- [x] Paso 7: Presentar artefactos para aprobación

## Artefactos obligatorios (incluidos en el plan)

- [x] unit-of-work.md — definiciones y responsabilidades de unidades
- [x] unit-of-work-dependency.md — matriz de dependencias
- [x] unit-of-work-story-map.md — mapeo historias → unidades
- [x] Estrategia de organización de código greenfield documentada en unit-of-work.md
- [x] Validar límites y dependencias de unidades
- [x] Asegurar que todas las historias están asignadas a unidades
- [x] Planes de tareas por unidad (punto de parada declarado; criterios verificables por comando)

---

## Pregunta 1: Agrupación (Story Grouping)

¿La descomposición en 5 unidades (U1 ingesta, U2 ejecución, U3 agentes, U4 gobierno, U5 plataforma) es correcta?

A) **Sí, las 5 propuestas** (Recomendado: separa el plano de ejecución Go, la capa de agentes y el gobierno; U5 transversal habilita)

B) **8 unidades 1:1 con M1-M8** (máxima granularidad; más sobrecarga de coordinación)

C) **3 unidades** — U-A Control plane (U1+U4+U5), U-B Ejecución (U2), U-C Agentes (U3) (mínima coordinación; límites más gruesos)

X) Otra (describe tras el tag [Answer])

[Answer]: A

---

## Pregunta 2: Dependencias e integración (Dependencies)

¿Se confirma el esquema de integración entre unidades (REST para UI/admin + eventos para el pipeline + Jobs K8s, con contratos OpenAPI del control plane y esquemas de eventos versionados)?

A) **Sí, como está diseñado** (contratos: OpenAPI + esquemas de los 8 eventos versionados; U2 depende de U4/U1, U3 de U2/U5, todas de U5-plataforma)

B) Sí, pero con **contratos mockeados primero** (cada unidad define sus stubs antes de integrarse)

C) Cambiar el esquema (describe qué cambiar)

X) Otra (describe tras el tag [Answer])

[Answer]: B

---

## Pregunta 3: Equipo (Team Alignment)

¿Cómo se alinea el trabajo por unidades con el equipo (contexto de curso, módulos 4-8)?

A) **Un equipo, unidades en secuencia** (U5 primero como habilitadora, luego U1/U4, U2, U3) (Recomendado para equipo único)

B) **Subequipos en paralelo** por unidad (indica cuántos frentes: p. ej. 2-3)

C) **Por módulo del curso** (CKA→U2/U5, CKAD→U1/U4, CKS→U4-endurecido, M8→U5-cierre)

X) Otra (describe tras el tag [Answer])

[Answer]: A

---

## Pregunta 4: Diferencias técnicas por unidad (Technical Considerations)

¿Alguna unidad requiere consideraciones especiales de escalado/despliegue distintas al resto?

A) **Sí: U2 (burst de Jobs por corrida) y U3 (costo/latencia de LLM) dimensionan aparte**; U1/U4/U5 despliegue estándar con HPA básico (Recomendado)

B) No, todas con el mismo perfil (un Deployment + HPA + limits por servicio, Jobs con quotas del namespace)

X) Otra (describe tras el tag [Answer])

[Answer]: A

---

## Pregunta 5: Dominios de negocio (Business Domain)

¿Los bounded contexts son: Ingesta (U1), Ejecución aislada (U2), Inteligencia (U3), Gobierno (U4), Plataforma (U5)?

A) **Sí** (Recomendado: el gobierno (U4) queda separado de la ejecución (U2) para que los guardrails no dependan del ciclo de corrida)

B) Fusionar Gobierno con Control plane de ingesta (U1+U4)

C) Fusionar Inteligencia con Ejecución (U2+U3)

X) Otra (describe tras el tag [Answer])

[Answer]: A

---

## Pregunta 6: Organización de código (Code Organization, greenfield)

¿Monorepo o multirepo, y con qué layout? (Aplica a Go + capa de agentes + manifiestos Flux + CI.)

A) **Monorepo** `/{services|agents|deploy|docs}` — `services/<svc-go>/`, `agents/<planner|reporter>/`, `deploy/flux/<env>/`, `.github/workflows/` (Recomendado: un solo pipeline, contratos y SBOM centralizados)

B) **Multirepo** — un repo por unidad (U1..U5) + repo `platform` (U5)

C) **Monorepo por lenguaje** — repo Go, repo agentes, repo deploy

X) Otra (describe tras el tag [Answer])

[Answer]: A