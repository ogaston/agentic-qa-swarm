# Preguntas de Verificación de Requisitos — Agentic QA Swarm

Análisis de requisitos realizado sobre `entradas/prd.md` y `entradas/pvd.md`. El PRD está
excepcionalmente completo (MoSCoW, journeys, KPIs, riesgos, especificación por módulos M1-M8).
Las preguntas siguientes resuelven únicamente: (a) los **opt-ins obligatorios de extensiones**
del flujo AI-DLC y (b) las **decisiones abiertas (TBD)** y vacíos reales que el PRD deja sin
cerrar y que afectan la especificación del producto y el plan de tareas por unidad.

Marca tu respuesta tras `[Answer]:`. Si ninguna opción encaja, elige la última y describe.

---

## Pregunta 1: Extensión — Resiliency Baseline
¿Se aplica la línea base de resiliencia a este proyecto?

**Qué es esta extensión.** Activarla aplica un conjunto de **buenas prácticas direccionales de diseño** (derivadas del pilar Reliability de AWS Well-Architected) para construir sistemas resilientes: tolerancia a fallos, alta disponibilidad, observabilidad y recuperabilidad, en 15 áreas de práctica.

**Qué NO es.** No vuelve el workload "production-ready" ni certifica un objetivo de disponibilidad/RTO/RPO. Es un **punto de partida** — un primer borrador fundamentado de postura de resiliencia para validar y endurecer — no un sustituto de un Well-Architected Review formal.

A) Sí — aplicar la línea base de resiliencia como guía direccional de diseño (recomendado para un sistema con namespace de prueba, Jobs y teardown del sandbox)

B) No — omitir la línea base de resiliencia (adecuado para PoCs y prototipos)

X) Otra (describe tras el tag [Answer])

[Answer]: A

---

## Pregunta 2: Extensión — Security Baseline
¿Se aplican las reglas de extensión de seguridad a este proyecto?

A) Sí — exigir todas las reglas SECURITY como restricciones bloqueantes (recomendado: este producto existe para garantizar aislamiento y límites de autonomía)

B) No — omitir todas las reglas SECURITY (adecuado para PoCs)

X) Otra (describe tras el tag [Answer])

[Answer]: A

---

## Pregunta 3: Extensión — Property-Based Testing
¿Se aplican reglas de testing basado en propiedades (PBT) a este proyecto?

A) Sí — exigir todas las reglas PBT como restricciones bloqueantes (recomendado: hay lógica de negocio — inferencia de superficie, generación de flujos, correlación de post-mortem)

B) Parcial — exigir PBT solo para funciones puras y round-trips de serialización

C) No — omitir todas las reglas PBT

X) Otra (describe tras el tag [Answer])

[Answer]: B

---

## Pregunta 4: Alcance de la especificación y del plan de tareas
El PRD define un alcance MVP (MoSCoW: M1-M10 Must, S1-S5 Should, C1-C5 Could, W1-W11 Won't) y el PVB describe la visión completa (p. ej. catálogo de 6 clases de ataque, modo Enterprise en el clúster del cliente, SSO, billing multi-tier).

¿Qué alcance debe especificar el producto y el plan de tareas de cada unidad?

A) Solo el MVP (MoSCoW Must + Should), el resto marcado como fuera de alcance (recomendado: alineado a lo construible en el semestre y a la Sesión 16)

B) MVP + visión completa documentada (MVP detallado, visión como roadmap)

C) Visión completa del PVB en el mismo nivel de detalle

X) Otra (describe tras el tag [Answer])

[Answer]: A

---

## Pregunta 5: Fuente del artefacto para PRs en el MVP (PRD §9 TBD)
El PRD deja abierto cómo se obtiene el artefacto por tipo de evento: para PRs se sugiere build-from-repo; para tags puede ser imagen publicada (posible opción C5 del MVP).

¿Qué camino soporta el MVP primero?

A) Build-from-repo para PR y commits; imagen publicada para tags/releases

B) Solo build-from-repo (máxima simplicidad de entrada)

C) Solo imagen publicada desde un registry

D) Ambos caminos para todos los eventos (incluye C5)

X) Otra (describe tras el tag [Answer])

[Answer]: A

---

## Pregunta 6: Almacenamiento de evidencia y reportes (PRD §9 TBD)
El PRD deja abierto dónde se guardan reportes/evidencia: "MinIO in-cluster vs. bucket externo, sin definir en los docs".

¿Dónde debe persistirse la evidencia de las corridas en el MVP?

A) MinIO in-cluster (todo queda dentro del clúster del producto; sin deps externas de objeto) (recomendado: coherente con "nuestro cluster, nuestro test namespace" del MVP)

B) Bucket de objetos externo (S3-compatible) — separa almacenamiento del clúster

C) Ambos: in-cluster como primario, bucket como respaldo/export

X) Otra (describe tras el tag [Answer])

[Answer]: A

---

## Pregunta 7: Expiración de sesiones interrumpidas (PRD §7.3 TBD)
El PRD deja abierta la política de expiración para sesiones colgadas (usuario confirma, el boot/plan no se completa y el usuario no vuelve). Opciones anotadas en el PRD.

¿Qué política de expiración adoptar para el MVP (y el housekeeping de teardown)?

A) 24 h sin notificación adicional (el teardown se aplica al límite, sin avisar) (recomendado: simple y seguro)

B) 24 h con notificación previa (aviso antes de destruir el sandbox)

C) Indefinido hasta cierre explícito (el sandbox vive hasta que el usuario o un admin lo cierra)

X) Otra (describe tras el tag [Answer])

[Answer]: C

---

## Pregunta 8: Límite de reintentos antes de fail-closed (PRD §7.4 TBD)
El PRD deja abierto el número exacto de reintentos para boot fallido y ensayo fallido antes de escalar a humano (fail-closed).

¿Qué límite de reintentos adopta el MVP?

A) Boot: 2 reintentos; Ensayo: 2 reintentos (después → handoff) (recomendado: acotado y barato)

B) Boot: 3; Ensayo: 3 (más tolerante con artefactos lentos)

C) Configurable por administrador vía política (M7), con valores por defecto acotados

X) Otra (describe tras el tag [Answer])

[Answer]: A

---

## Pregunta 9: Stack tecnológico del control plane de la plataforma (M1/M6/M7/M8)
Ni el PRD ni el PVB definen el lenguaje/framework del entorno estable del producto (UI/API Gateway, identidad, gobernanza, post-mortem). Esto afecta el detalle del plan de tareas por unidad.

¿Qué stack para el control plane de la plataforma?

A) Go (ecosistema Kubernetes nativo, un binario por servicio, ideal para runners/control plane) + UI web (recomendado)

B) Python (FastAPI): rapidez para la capa de agentes/LLM y post-mortem + UI web

C) Node.js/TypeScript: tipos compartidos con la UI, ecosistema LLM maduro

D) Híbrido: Go para el control plane de ejecución (jobs/runners/teardown) + Python o TypeScript para la capa de agentes LLM

X) Otra (describe tras el tag [Answer])

[Answer]: D
