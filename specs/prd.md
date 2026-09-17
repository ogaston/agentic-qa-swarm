# PRD — Agentic QA Swarm

> Documento construido en co-creación iterativa. Cada afirmación de datos respeta las etiquetas `[INTERNO]` (proyección propia) y `[VERIFICAR]` (afirmación sin fuente confirmada de forma independiente). Ningún `[VERIFICAR]` se presenta como hecho.

**Documentos fuente (`docs/`):**

| Referencia usada | Archivo |
|---|---|
| Product Vision Board | `docs/pvd.md` |
| Panorama del dominio | `docs/overview.md` |
| Análisis de mercado | `docs/mercado.md` |
| Perfil de cliente ideal (ICP) | `docs/icp.md` |
| Investigación de crítica adversarial | `docs/critica.md` |
| Investigación de validación | `docs/validacion.md` |
| Mockup de UI | `docs/mockups/swarm-mock.html` |

---

## Paso 0 — Resolución de conflictos entre documentos

Antes de redactar el PRD se cruzaron los 6 documentos y se identificaron conflictos/vacíos. Decisiones tomadas:

| # | Conflicto | Resolución |
|---|---|---|
| 1 | Cifras de mercado sin fuente (~$24B en `mercado.md`; predicción de Gartner sin cita) | Se reemplazan por fuentes nombradas y verificables: **Global Market Insights** ("Automation Testing Market Size": $20B en 2022 → proyección >$40B para 2032) y **Gartner** (Mark O'Neill, Dionisio Zumerle, *"How to Build an Effective API Security Strategy"*, 2017/2019: *"Para 2022, los abusos de API pasarán de ser poco frecuentes a convertirse en el vector de ataque más frecuente..."*). La afirmación "OWASP confirma que se cumplió" queda `[VERIFICAR]`. Se deja constancia de que el $24B original de `mercado.md` no coincide exactamente con el $20B de la fuente nueva. |
| 2 | SOM definido como headcount (50-500 empleados, `icp.md`) vs. ronda de inversión (Series A-C, `mercado.md`) | No son excluyentes. El criterio real es cualitativo: empresas enfocadas en **crecer** más que en construir capacidades internas de testing. Se usa como señal de fit, no como filtro firmográfico duro. |
| 3 | Tensión entre `validacion.md` (optimista: "de semanas a minutos") y `critica.md` (riesgos que exigen arquitectura de seguridad no trivial) | El PRD se construye sobre el **escenario de fricción de `critica.md`** como caso base. "Minutos" queda como meta aspiracional, no como promesa del MVP. |
| 4 | El ICP carece de SRE dedicado, pero el artefacto necesita deps (DB, cache) para arrancar | El MVP usa un **entorno tibio (warm) propio de la plataforma**: un namespace de prueba de vida larga, aislado de staging/prod del cliente, con app + 1 base de datos (Postgres o Mongo) + 1 Redis **pre-desplegados y reutilizados entre corridas**. Cada corrida despliega la versión del artefacto sobre ese entorno y aplica un **reset de estado verificado** (reinicio de servicios, limpieza de DB, flush de cache, verificación) en lugar de reconstruir todo desde cero. Rebuild periódico y teardown total programado como higiene. |
| 5 | Meta de "Time-to-first-isolated-run < 5 min" sin baseline numérico | Baseline se mantiene **cualitativo** ("semanas", `[VERIFICAR]`), sin inventar una cifra numérica de partida. |
| 6 | Geografía y madurez de Kubernetes del ICP no explícitas | Geografía: **LatAm y US**. Madurez de Kubernetes: inferida como intermedia-alta, marcada explícitamente como inferencia, no dato. El cluster es necesario para el **namespace de prueba**, no para atacar el staging del cliente. |
| 7 | Numeración OWASP API6 (Mass Assignment en 2019, SSRF en 2023) | El PRD evita anclar el argumento a un número de categoría fijo; usa el concepto "fallas de lógica de negocio" y aclara la edición cuando es relevante. |
| 8 | Distinción buyer (VP Eng/Head of Platform) vs. user (Platform/DevOps Engineer) | `icp.md` se usa como fuente autoritativa; es consistente con el "veto de confianza" que define `pvd.md`. |
| 9 | Auto-disparo en GitHub vs. confirmación humana | **Auto-notify sí, auto-run no.** Commit, PR y tag/release crean una notificación. La corrida solo arranca tras confirmación explícita. |

---

## 1. One-liner + Job to be Done

### One-liner

> **Agentic QA Swarm** convierte un evento de GitHub (commit, PR o tag/release) en una corrida de QA de caja negra contra un **entorno tibio propio de la plataforma**: el humano confirma, el sistema despliega el artefacto sobre el entorno warm reutilizable dentro de un namespace de prueba aislado, genera los flujos a partir de lo que la app expone hacia afuera, los ensaya, los ejecuta, aplica reset de estado verificado entre corridas y entrega un post-mortem de lógica de negocio. Staging y producción del cliente no son el blanco.

### Job to be Done

**JTBD primario (usuario diario — Platform/DevOps Engineer):**

> Cuando llega un commit, un PR o un release y sé que un bug de lógica de negocio puede costarle dinero a la empresa, pero no tengo tiempo para escribir ni mantener flujos de QA —y no quiero apuntar herramientas a staging—, quiero confirmar la notificación, dejar que el sistema despliegue mi artefacto sobre un entorno de prueba tibio ya caliente, genere los flujos como lo haría un QA y me explique qué invariante falló, para encontrar esas fallas antes de producción sin bloquear el trabajo de valor de mi equipo.

**JTBD secundario (comprador — VP of Engineering / Head of Platform):**

> Cuando necesito bajar el *Change Failure Rate* de mis releases transaccionales sin contratar un equipo de SRE/QA dedicado, quiero una herramienta que pruebe el artefacto de forma autónoma pero **verificablemente aislada de mi staging y de producción**, para mostrar reducción de incidentes sin asumir el riesgo de que la herramienta misma toque los entornos que usa gente.

### Misión del producto

> Eliminar el trabajo manual de diseñar, escribir y mantener QA de caja negra para apps transaccionales, cerrando el ciclo completo —evento GitHub → notify → confirm → deploy sobre entorno warm → inferir superficie → flujos QA → ensayo → run → reset verificado → post-mortem (con scale-down en idle y rebuild/teardown periódico del entorno)— dentro de límites de autonomía verificables, para que un equipo de 3 a 15 ingenieros obtenga cobertura de fallas de lógica de negocio que hoy solo un equipo de QA/SRE dedicado podría producir.

---

## 2. Contexto y problema

### 2.1 Dolores del mercado (con datos y fuente)

**Dolor macro (seguridad):**

> *"Para 2022, los abusos de API pasarán de ser poco frecuentes a convertirse en el vector de ataque más frecuente, resultando en brechas de datos para aplicaciones web empresariales."*
> — Gartner, *"How to Build an Effective API Security Strategy"* (Mark O'Neill, Dionisio Zumerle, 2017/2019).

La afirmación de que "hoy OWASP confirma que esta predicción se cumplió" queda `[VERIFICAR]` por no tener documento específico citado.

**Tamaño de mercado (contexto de categoría, no validación de demanda del producto):**

> *"El mercado global de Automation Testing superó los $20 mil millones de dólares en 2022 y se proyecta que alcance más de $40 mil millones para 2032, impulsado por la adopción de metodologías ágiles y DevOps."*
> — Global Market Insights, *Automation Testing Market Size*.

⚠️ `docs/mercado.md` original menciona *"~$24B USD"* para la misma categoría, sin fuente — cifra distinta a la anterior, consistente en orden de magnitud pero no idéntica.

**Dolor específico del ICP:**

* `icp.md`: *"Cada vez que lanzamos una feature transaccional, descubrimos bugs de lógica de negocio en producción que cuestan dinero. No puedo justificar 3 ingenieros de QA, y no voy a dejar que una herramienta pegue contra nuestro staging."* (buyer)
* `icp.md`: *"CI ya corre en cada PR, pero solo ejecuta lo que el repo ya trae (o apunta a staging). Mantener flujos de QA de caja negra a mano es tedioso y se rompe cuando cambia la superficie externa."* (user)
* `pvd.md`: *"Escribir y mantener flujos de QA (funcionales, de estado, de concurrencia) toma **semanas, no horas**. El 80% es plomería de autenticación, setup y teardown, no diseño de prueba."* `[VERIFICAR]`
* `validacion.md`: *"Los equipos de plataforma (3-15 ingenieros) suelen abandonar el mantenimiento de flujos complejos de QA/Chaos porque el tiempo de ingeniería no escala, y rechazan herramientas que usen su staging como blanco."* `[VERIFICAR]`
* Las fallas más caras (condiciones de carrera, invariantes de negocio, categoría de *Mass Assignment*/lógica de negocio de OWASP API Security Top 10) no las detectan SAST/DAST ni tests unitarios — solo flujos reales contra la app corriendo.

### 2.2 ¿Por qué ahora?

* `overview.md`: GitHub ya dispara CI en cada commit/PR/release, pero ese pipeline prueba **lo que el repo ya contiene** o apunta a **staging compartido**. El QA de caja negra contra un entorno de prueba aislado sigue siendo manual, y reconstruirlo desde cero en cada corrida desperdicia minutos y compute.
* `overview.md`: *"Los LLMs con razonamiento profundo pueden, por primera vez, observar una app en ejecución y comportarse como un QA: entender lo que es visible desde fuera (OpenAPI si existe, HTTP, UI expuesta) e inferir los flujos a ejercitar."* Este es el habilitador técnico real: sin razonamiento sobre la superficie externa, el producto no es viable.
* Convergencia de madurez: el ICP ya usa GitHub, Docker y (o puede usar) Kubernetes — la pieza que faltaba era la capa de razonamiento más el entorno warm reutilizable, no el webhook.

### 2.3 Alternativas actuales del ICP (y por qué no alcanzan)

| Alternativa | Qué resuelve hoy | Por qué no alcanza |
|---|---|---|
| **GitHub Actions / CI genérico** | Corre la suite del repo en cada evento | White-box o contra staging; no genera QA de caja negra ni post-mortem de lógica de negocio (`mercado.md`) |
| **k6 / Gatling / JMeter** (k6 es open source) | Motor de carga como código | Determinista; semanas de programación manual; no arranca el artefacto (`mercado.md`) |
| **Gremlin / Chaos Mesh** (Chaos Mesh es CNCF) | Chaos de infraestructura, Capa 4 | No ataca lógica de negocio en Capa 7 (`mercado.md`, `validacion.md`) `[VERIFICAR: roadmaps L7]` |
| **Postman (IA)** | Aserciones de un endpoint aislado | No levanta el artefacto ni cubre el trabajo de un QA `[VERIFICAR]` |
| **Escáneres SAST/DAST** | Vulnerabilidades sintácticas | No detectan problemas de estado (`pvd.md`) |
| **Nada** (statu quo más común) | — | El equipo abandona el mantenimiento y asume el riesgo en producción (`validacion.md`) `[VERIFICAR]` |

### 2.4 Qué ya está resuelto gratis por open source / CNCF

* **Disparo de eventos: resuelto gratis.** GitHub webhooks.
* **Correr el artefacto: resuelto gratis.** Docker / compose.
* **Aislamiento y Jobs: resuelto gratis.** Namespaces de Kubernetes + `Jobs` nativos.
* **Motor de carga (opcional): resuelto gratis.** k6 es open source. *"No competimos: podemos orquestarlos como un ejecutor"* (`pvd.md` sec. 4).
* **Caos de infraestructura: resuelto gratis.** Chaos Mesh (CNCF).

**Lo que no está resuelto gratis:** inferir la superficie externa de un artefacto desplegado sobre un entorno warm, generar el trabajo de un QA (no solo un ataque), el bucle de gobernanza (notify ≠ run, ensayo obligatorio, **reset de estado verificado entre corridas** + rebuild/teardown periódico del **entorno reutilizable**), y el post-mortem correlacionado con causa de negocio.

**Consecuencia honesta:** un ingeniero suficientemente disponible puede lograr el mismo resultado técnico gratis a mano. El producto vende de vuelta el tiempo de ingeniería y la disciplina de gobernanza — no una capacidad inexistente en el mercado.

```mermaid
flowchart LR
    subgraph GRATIS["Ya resuelto gratis (CNCF / open source)"]
        A["GitHub webhooks<br/>(eventos)"]
        B["Docker / compose<br/>(artefacto)"]
        C["K8s namespaces + Jobs<br/>(aislamiento y ejecucion)"]
        D["k6 / Gatling<br/>(un ejecutor, opcional)"]
    end
    subgraph VACIO["El vacio que nadie automatiza gratis"]
        E["Inferir superficie externa<br/>del artifact en entorno warm"]
        F["Generar flujos de QA de caja negra<br/>+ ensayo + confirm"]
        G["Reset verificado entre corridas<br/>+ rebuild periodico + post-mortem"]
    end
    A --> E
    B --> E
    E --> F
    F --> C
    C --> G
    D -.opcional.-> F
```

---

## 3. ICP detallado

### 3.1 Firmographics

| Dimensión | Valor | Fuente |
|---|---|---|
| **Tamaño de empresa** | Scale-ups y medianas empresas tecnológicas, 50-500 empleados | `icp.md` |
| **Tamaño de equipo comprador** | 3 a 15 ingenieros | `icp.md`, `pvd.md` |
| **Verticales** | E-commerce, Fintech, Logística, SaaS B2B transaccional | `icp.md`, `pvd.md` |
| **Geografía** | LatAm, US | Decisión Paso 0 (#6) |
| **Stack técnico** | GitHub; artefacto Docker-runnable; K8s para el namespace de prueba; HTTP/UI al arrancar (OpenAPI es ventaja, no gate) | `icp.md` |
| **Madurez en Kubernetes** | Inferida como intermedia-alta | Inferencia — `TBD` a validar con clientes piloto |
| **Señal de fit adicional** | Foco en crecer, no en construir capacidades internas de testing/SRE; rechazo a herramientas que usen staging como blanco | Decisión Paso 0 (#2), `icp.md` |
| **Anti-perfil** | No puede correr la app en Docker; sin GitHub; QA de caja negra aislado ya maduro y staffed | `pvd.md` |

> ⚠️ La madurez de Kubernetes es una inferencia, no un dato de tus documentos — validar en las primeras 5-10 entrevistas de descubrimiento. OpenAPI ausente **no** descalifica si la app expone HTTP/UI al arrancar.

### 3.2 Buyer personas y veto de confianza

| | Buyer (firma el cheque) | User (usa a diario) |
|---|---|---|
| **Rol** | VP of Engineering / Director of Platform Engineering | Platform Engineer, DevOps Engineer, Backend Lead |
| **Qué mide** | *Change Failure Rate*, presupuesto de SRE | Tiempo no perdido en mantenimiento de flujos de QA; no tocar staging |
| **Veto de confianza** | Sí | No |

> *"¿Esto toca nuestro staging o producción?"* — Si la respuesta no es *"no: solo el entorno warm propio de la plataforma en su namespace de prueba, aislado de tu staging/prod, reutilizado entre corridas con reset de estado verificado y rebuild/teardown periódico"*, la conversación se acaba. (`pvd.md`)

```mermaid
flowchart TD
    U["User: Platform/DevOps Engineer<br/>siente el dolor a diario"] -->|"prueba / evalua"| P["Producto"]
    P -->|"resultado y evidencia de aislamiento"| B["Buyer: VP Eng / Head of Platform<br/>veto de confianza"]
    B -->|"'Esto toca staging o produccion?'"| G{Solo entorno warm propio<br/>en test namespace + reset verificado}
    G -->|Si, demostrable| APPROVE["Aprueba compra"]
    G -->|No lo puede garantizar| REJECT["Rechaza / no evalua mas"]
```

### 3.3 Pains

* Buyer: *"Cada vez que lanzamos una feature transaccional, descubrimos bugs de lógica de negocio en producción que cuestan dinero. No puedo justificar 3 ingenieros de QA, y no voy a dejar que una herramienta pegue contra nuestro staging."* (`icp.md`)
* User: *"CI ya corre en cada PR, pero solo ejecuta lo que el repo ya trae (o apunta a staging)."* (`icp.md`)
* Transversal: *"No tienen un equipo de SRE/QA dedicado; el VP de Engineering no justifica contratarlo."* (`pvd.md`)

### 3.4 Triggers de compra

1. Incidente reciente en producción por race condition o corrupción de estado que ya costó dinero.
2. Lanzamiento inminente de una feature transaccional de alto riesgo.
3. Un ingeniero ya escribió y abandonó scripts de k6/Gatling/Playwright.
4. Presión ejecutiva por bajar el *Change Failure Rate* sin aumentar headcount.
5. PRs/releases que solo tienen unitarias y nadie se fía de mergear.
6. Empresa en fase de foco en crecer rápido, no en construir capacidades internas de testing.

### 3.5 Objeciones probables y respuestas

| Objeción | Respuesta |
|---|---|
| "Esto toca nuestro staging o producción." | No. El SUT es el **entorno warm propio de la plataforma** en un namespace de prueba dedicado, aislado de tu staging/prod. Cero credenciales de staging/prod del cliente. |
| "Ya tenemos GitHub Actions, ¿para qué pagar?" | Actions corre lo que el repo ya trae. Esto despliega sobre un entorno warm reutilizable, genera QA de caja negra y entrega un post-mortem de lógica de negocio. |
| "No voy a darle a un agente de IA acceso autónomo a mi infraestructura." | Paradigma *Agent*, no *Autonomous*: auto-notify sí, auto-run no. Confirmación humana + workflows con cuotas/timeouts/políticas de aprobación. Lista explícita de qué nunca hace. |
| "Ya tenemos k6/Gatling, ¿para qué pagar?" | No reemplaza el motor; puede usarlo como un ejecutor. El valor es el entorno warm gestionado, inferir, generar, ensayar, reset verificado y explicar. |
| "¿Cómo controlan el costo de LLM?" | Arquitectura cerebro/músculo: LLM off-cluster, runners sin credenciales de modelo. El entorno warm además recorta startup y compute por corrida. |
| "¿Van a copiar secretos de nuestro staging para que la app arranque?" | No. El entorno warm usa config/secrets **sintéticos o declarados para el test ns**. Si el artefacto no bootea, fail-closed + handoff. |
| "Operamos en fintech, ¿qué pasa con datos sensibles?" | **`TBD`** — MVP con datos 100% sintéticos en el entorno warm, nunca PII real; el reset verificado evita fuga de estado entre corridas. |

---

## 4. Propuesta de valor única y diferenciadores

### 4.1 Qué problema resuelve, para quién, cómo

Para equipos de plataforma/backend (3-15 ingenieros) en scale-ups de 50-500 empleados sin SRE/QA dedicado: resuelve la incapacidad de descubrir fallas de lógica de negocio en cada commit/PR/release sin escribir flujos a mano y **sin usar staging como blanco**, cerrando el ciclo `evento GitHub → notify → confirm → deploy sobre entorno warm → inferir superficie → flujos QA → ensayo → run → reset verificado → post-mortem`, con complejidad configurable por workflows de negocio (cuotas, timeouts, políticas de aprobación).

### 4.2 Qué brecha real llena

> *"El vacío no es otro motor de k6. Es: nadie recibe el evento de GitHub, despliega el artefacto sobre un entorno warm aislado y reutilizable, observa solo la superficie externa, genera los flujos de QA, verifica el reset entre corridas y explica qué invariante de negocio se rompió —sin usar staging ni leer el código fuente para inventar las pruebas."* — `overview.md`

### 4.3 Diferenciación frente a competidores (incluidos los gratuitos)

| Competidor | Qué resuelve | Por qué no llena la brecha | Relación |
|---|---|---|---|
| GitHub Actions / CI | Suite del repo en cada evento | White-box o staging; no genera QA de caja negra | Alternativa real |
| k6 / Gatling / JMeter | Motor de carga | Determinista, manual; no arranca el artefacto | Un ejecutor posible |
| Gremlin / Chaos Mesh | Chaos de infraestructura | No ataca Capa 7 `[VERIFICAR]` | Adyacente |
| Postman (IA) | Aserciones de endpoint | No arranca el artefacto ni cubre un QA `[VERIFICAR]` | Adyacente |
| Kubernetes Jobs / namespaces (gratuitos) | Aislamiento y ejecución | Resuelven el músculo, no el cerebro | Base sobre la que construimos |

El diferenciador no es el agente (replicable), es el bucle completo medido —confirm, entorno warm aislado con reset verificado, rebuild/teardown periódico, post-mortem de lógica de negocio— más la biblioteca de flujos que ya encontraron bugs reales (`pvd.md` sec. 3, 9). Las clases de ataque son **una familia** de flujos, no el producto.

### 4.4 Matriz de posicionamiento 2x2

```mermaid
quadrantChart
    title Caja negra de logica de negocio vs. autonomia de diseno
    x-axis White-box / infra --> Caja negra de logica sobre artifact aislado
    y-axis Diseno manual --> Diseno autonomo
    quadrant-1 Punto ciego del mercado
    quadrant-2 Automatizacion de infraestructura
    quadrant-3 Manuales de bajo nivel
    quadrant-4 CI y motores scripting
    GitHub Actions: [0.22, 0.55]
    k6 / Gatling / JMeter: [0.55, 0.15]
    Gremlin / Chaos Mesh: [0.18, 0.25]
    Postman AI: [0.48, 0.53]
    Agentic QA Swarm: [0.88, 0.82]
```

> ⚠️ La ubicación de Postman AI y GitHub Actions es una estimación editorial, no un dato medido.

---

## 5. Casos de uso (top 5)

### UC1 — Primera corrida aislada desde una notificación de GitHub (sobre entorno warm)
* **Actor:** Platform/DevOps Engineer. **Trigger:** commit, PR o tag/release en un repo conectado.
* **Pasos:** evento → notificación en inbox → usuario confirma → deploy del artefacto sobre el **entorno warm** del namespace de prueba (reutilizado, ya caliente) → inferir superficie externa → generar flujos QA → ensayo → corrida → reset verificado → reporte.
* **Resultado:** corrida completada sin scripting manual, sin reconstruir infraestructura y sin tocar staging. **KPI:** Time-to-first-isolated-run (reducido por warm start).

### UC2 — Flujo de lógica de negocio (p. ej. condición de carrera o estado inválido)
* **Actor:** Backend Lead. **Trigger:** validar invariantes antes de merge/release.
* **Pasos:** el agente recomienda flujos a partir de la superficie (incluye, entre otros, carrera y multi-estado) → usuario selecciona dentro del **workflow de negocio configurado** (complejidad, cuota, timeout, aprobación) → ensayo unitario → corrida en el entorno warm → captura de violaciones de invariantes → reset verificado.
* **Resultado:** evidencia reproducible de violación (o confirmación de resistencia), sin contaminar la siguiente corrida. **KPI:** Tasa de hallazgo lógico.

### UC3 — Reset verificado, scale-down en idle y rebuild/teardown periódico del entorno warm
* **Actor:** Sistema. **Trigger:** fin de corrida (éxito, fallo o abandono), idle prolongado, o ventana de higiene programada.
* **Pasos:** tras cada corrida → reiniciar servicios, limpiar DBs, hacer flush de caches, **verificar el reset** (probes + job de verificación) → marcar entorno `ready` o en cuarentena si falla; en idle → escalar hacia abajo (replicas mínimas / pausa de runners) para recortar costo; periódicamente → rebuild desde imagen base o teardown total + reprovisionamiento → verificar → notificar.
* **Resultado:** entorno reutilizable sin fuga de estado entre corridas, barato en idle e higiénico en el tiempo. **KPI:** Completitud de reset verificado (meta 100%), tiempo de rebuild, costo en idle.

### UC4 — Configuración de límites de autonomía por el Head of Platform
* **Actor:** VP Eng/Head of Platform. **Trigger:** habilitar el producto por primera vez.
* **Pasos:** instalar GitHub App → elegir repo y qué eventos notifican (commit / PR / tag) → confirmar que auto-run está apagado → definir cuotas del entorno warm y del namespace de prueba → definir **workflows de negocio** (complejidad de prueba, cuotas, timeouts, políticas de aprobación) → **bloquear staging y producción del cliente**.
* **Resultado:** entorno con guardrails auditables. **KPI:** % de cuentas con política configurada antes de la primera corrida `[INTERNO]`.

### UC5 — El artefacto no arranca o el agente no logra un flujo válido y escala a humano
* **Actor:** Agente → Platform Engineer. **Trigger:** boot falla, o el ensayo falla repetidamente.
* **Pasos:** reintentar hasta límite → detenerse **fail-closed** (no corrida completa, no reintento infinito) → generar reporte de motivo → usuario resuelve manualmente.
* **Resultado:** cero compute desperdiciado de más, contexto claro para intervenir. **KPI:** Ensayo a la primera / tasa de boot del artefacto / tasa de escalamiento a humano.

---

## 6. Principios de diseño no negociables

| # | Principio | Origen |
|---|---|---|
| 1 | Cerebro fuera del bucle de ejecución | `pvd.md`, `critica.md` Riesgo 3 |
| 2 | Ensayo obligatorio antes de la corrida completa | `pvd.md`, `critica.md` Riesgo 2 |
| 3 | Entorno warm aislado + reset verificado + rebuild periódico | `pvd.md`, `critica.md` Riesgo 1 |
| 4 | **Límite de autonomía verificable** (notify ≠ run; nunca staging/prod del cliente) | `pvd.md` sec. 5 |
| 5 | Flujos generados inspectables (no runtime propietario) | `pvd.md` sec. 3, 4, 9 |
| 6 | Complejidad configurable por workflows de negocio | Nuevo (este cambio) |

### 1. Cerebro fuera del bucle de ejecución
* **(a)** El LLM solo interviene en planeación y post-mortem, nunca durante la ejecución.
* **(b)** Los *runners* ejecutan flujos deterministas sin llamadas adicionales al modelo.
* **(c) Prohibido:** credenciales de LLM en pods runner; llamar al modelo por cada request de la corrida.

### 2. Ensayo obligatorio antes de la corrida completa
* **(a)** Ningún plan escala a corrida completa sin un flujo unitario exitoso **contra el entorno warm**.
* **(b)** Fase de ensayo bloqueante, condicionada técnicamente, no solo por política.
* **(c) Prohibido:** saltarse el ensayo, incluso si el cliente lo pide explícitamente (`pvd.md`: *"ni siquiera cuando el cliente pida 'salta el ensayo, que tengo prisa'"*).

### 3. Entorno warm aislado + reset verificado + rebuild periódico
* **(a)** El SUT es siempre el **entorno warm propio de la plataforma**: app en Docker + deps (DB + Redis) pre-desplegados en el namespace de prueba, **reutilizados entre corridas**. Staging y producción del cliente **no son el blanco**.
* **(b)** Entre corridas se aplica un **reset de estado obligatorio y verificado**: reinicio de servicios, limpieza de bases de datos, flush de caches y job de verificación (probes + checks). Sin `reset_verified=true` no arranca la siguiente corrida; si falla, el entorno entra en cuarentena y escala a humano.
* **(c)** El entorno **escala hacia abajo en idle** (replicas mínimas / pausa de runners) y se somete a **rebuild periódico o teardown total + reprovisionamiento** como higiene (rotación de imagen, deriva de config, leftovers).
* **(d) Prohibido:** reusar el entorno sin reset verificado; dejar workloads huérfanos; usar credenciales de staging/prod del cliente; operar un entorno warm que nunca se reconstruye.

### 4. Límite de autonomía verificable (el agente es *Agent*, no *Autonomous*)

| Sin aprobación extra (permitido) | Nunca (prohibido) |
|---|---|
| Crear notificación desde GitHub (commit, PR, tag/release) | Auto-ejecutar en el webhook |
| Tras confirm: deploy del artefacto sobre el entorno warm, inferir superficie **externa**, redactar flujos | Leer el código fuente de la app para inventar tests |
| Ensayar, correr, reset verificado, post-mortem | Apuntar a staging o producción del cliente |
| Recoger logs y redactar post-mortem | Llamar al LLM por cada request; omitir el reset; reusar sin `reset_verified=true` |

**Mecanismos verificables (no promesas):** GitHub App con permisos mínimos; gate de confirmación persistido y auditable; RBAC de Kubernetes que no agenda fuera del test namespace; `NetworkPolicy` que no sale de ese namespace y que bloquea a los runners el acceso a APIs de LLM; gate de pipeline condicionado a `ensayo_passed=true` (y a boot/deploy exitoso) **más** gate de `reset_verified=true` entre corridas; resource limits + quotas + timeouts por workflow; cero secrets de staging/prod del cliente montados en el entorno warm.

> ⚠️ Este principio solo es verificable si estos mecanismos están en el MVP, no como configuración opcional.

```mermaid
flowchart TD
    GH["GitHub: commit / PR / tag"] --> N["Notificacion en inbox"]
    N --> CONF{"Usuario confirma?"}
    CONF -- No --> WAIT["Espera / expira (warm sigue en idle escalado)"]
    CONF -- Si --> CHK{"Entorno warm ready?<br/>(reset_verified + probes)"}
    CHK -- "No: dirty o cuarentena" --> RST["Reset / rebuild + verificacion"]
    RST --> CHK
    CHK -- Si --> DEP["Deploy artefacto sobre warm<br/>en test namespace"]
    DEP --> BOOT{"App arranca?"}
    BOOT -- No --> H1["Fail-closed: handoff a humano (UC5)"]
    BOOT -- Si --> A["Agente: infiere superficie externa"]
    A --> B["Genera flujos QA deterministas<br/>(segun workflow: cuota/timeout/aprobacion)"]
    B --> C{"Ensayo unitario en warm"}
    C -- "Falla tras N intentos" --> H["Escala a humano (UC5)"]
    C -- "Exitoso: ensayo_passed=true" --> E["Corrida en test namespace<br/>(runners sin credenciales de LLM)"]
    E --> R["Reset verificado: restart + clean DB<br/>+ flush cache + reset_verified"]
    R --> G["Post-mortem (LLM, off-cluster)"]
    R -. "idle --> scale-down<br/>periodo --> rebuild/teardown" .-> WARM["Gestion del warm"]
```

### 6. Complejidad configurable por workflows de negocio
* **(a)** La complejidad de prueba no es un flag suelto: son **workflows definidos por el negocio** (p. ej. `smoke`, `standard`, `deep`), cada uno con familias de flujo habilitadas, cuotas de compute, timeouts y políticas de aprobación (p. ej. `deep` exige aprobación admin por corrida).
* **(b)** El admin versiona los workflows; cada corrida cita el workflow usado y sus límites se enforzan técnicamente (quotas, timeouts, gates), no solo por política.
* **(c) Prohibido:** correr fuera de un workflow declarado; exceder cuota/timeout del workflow; escalar complejidad sin la aprobación que el workflow exige.

### 5. Flujos generados inspectables
* **(a)** El código generado es un artefacto estándar (p. ej. k6, o el formato que el MVP elija) ejecutable independientemente.
* **(b)** El cliente puede auditar, versionar y ejecutar el flujo fuera de la plataforma.
* **(c) Prohibido:** un formato propietario ilegible. k6 es **un** ejecutor posible, no un Must de identidad del producto.

---

## 7. User journeys

### 7.1 Happy path del usuario final
1. Marta (Backend Lead, fintech) tiene el repo `payments-api` conectado. GitHub dispara un PR (o un commit, o un tag).
2. El dashboard muestra una notificación: evento, SHA/PR/tag, artefacto a desplegar sobre el entorno warm. **Nada corre todavía.**
3. Marta confirma. El sistema verifica que el entorno warm está `ready` (`reset_verified=true` + probes) y despliega el artefacto sobre él (ya caliente: sin aprovisionar DB/Redis desde cero).
4. El agente observa solo la superficie externa, infiere flujos de QA y los recomienda dentro del workflow configurado (p. ej. `standard`). Marta selecciona cuáles correr.
5. El sistema ejecuta el ensayo: un flujo completo contra el entorno warm.
6. El ensayo pasa; se muestra el plan y, si el workflow lo exige, se pide aprobación adicional para la corrida completa (nunca en silencio desde el webhook).
7. Al terminar, se ejecuta el **reset verificado** (restart + clean DB + flush cache + verificación) y el agente de post-mortem correlaciona los logs.
8. Marta recibe en Slack el resumen: flujos corridos, invariante violada (si aplica), enlace al reporte con evidencia.
9. Marta corrige el bug antes del merge/release, sin escribir una línea de test y sin que staging se enterara.

### 7.2 Happy path del operador/administrador
1. Julián (Head of Platform) recibe la solicitud de habilitar el producto.
2. Revisa la documentación de límites de autonomía (Segmento 6).
3. Instala el GitHub App, selecciona el repo, habilita notificaciones de commit / PR / tag-release. Auto-run permanece apagado.
4. Define cuotas del entorno warm y del namespace de prueba (CPU/memoria/tiempo), política de scale-down en idle y cadencia de rebuild/teardown, y confirma que staging y producción del cliente están fuera de alcance.
5. Define los **workflows de negocio** (`smoke` / `standard` / `deep` u otros): familias de flujo habilitadas, cuotas, timeouts y aprobaciones por workflow.
6. Aprueba la configuración; su equipo opera dentro de esos límites sin aprobación por corrida **más allá del confirm** (salvo que el workflow exija aprobación extra).
7. El dashboard de la **plataforma** vive en un entorno estable (el que usan Marta y Julián). Ese entorno no es el SUT; el SUT es el entorno warm en su namespace.
8. Un mes después revisa el panel agregado (corridas, hallazgos, boot success, completitud de reset, costo/idle, higiene de rebuild) para justificar el gasto ante su VP.

### 7.3 Edge case: el flujo se interrumpe o el usuario abandona
1. Marta ve la notificación, confirma, el deploy sobre el warm empieza, pero la interrumpen y cierra la pestaña.
2. El sistema detecta la interrupción (sin completar dentro de un tiempo límite).
3. Como todo corría en el namespace de prueba del entorno warm, no hay datos ni entornos del cliente comprometidos.
4. El sistema marca el entorno como `dirty`, ejecuta el **reset verificado** (mismo que tras una corrida completa) sin esperar acción de Marta, y solo lo marca `ready` si la verificación pasa; si no, cuarentena + aviso.
5. El plan generado (si existía) queda guardado como sesión "incompleta" para no repetir inferencia.
6. Si Marta vuelve, encuentra el plan esperando con aviso del estado del entorno (listo o en higiene).
7. Si nunca vuelve, el entorno warm cae a **scale-down por idle** y un proceso de housekeeping cierra sesiones colgadas; el rebuild/teardown periódico sigue su cadencia.

> ⚠️ `TBD`: tiempo de expiración de sesión no definido en los docs. Opciones: (a) 24h sin notificación, (b) 24h con notificación previa, (c) indefinido hasta cierre explícito.

### 7.4 Edge case: el agente no puede resolver la tarea y escala a un humano
1. El artefacto no arranca (Dockerfile roto, PR sin imagen, env faltante) **o** el agente prueba un payload que la superficie rechaza con `400`.
2. El agente reintenta dentro de un límite (`TBD` el número exacto).
3. Tras agotar los reintentos, se detiene **fail-closed**: no autoriza corrida completa, no reintenta forever.
4. El sistema arma el paquete de traspaso: logs de boot o fragmento de la superficie en conflicto, payloads intentados y errores, hipótesis en lenguaje natural, acción sugerida concreta.
5. Marta recibe este contexto completo en Slack/dashboard.
6. Marta corrige cómo se expone o cómo se arranca la app y vuelve a confirmar; el agente retoma sin repetir el trabajo ya válido.

---

## 8. Alcance del MVP (MoSCoW)

**Restricción real:** construible en lo que queda del semestre, desplegable en Kubernetes para el módulo 8.

### Must Have
M1. Integración GitHub: commit, PR y tag/release crean **notificación, no corrida**.
M2. Deploy del artefacto (repo y/o imagen) **sobre el entorno warm** del namespace de prueba dedicado (reutilizable, ya caliente).
M3. Descubrir **solo la superficie externa** (OpenAPI si está expuesta; si no, sondeo HTTP/UI de los puertos publicados).
M4. Generar flujos de QA de caja negra (el trabajo de un QA; las clases de ataque son una familia, no el catálogo entero), dentro del **workflow de negocio** seleccionado (complejidad, cuotas, timeouts, aprobaciones).
M5. Ensayo bloqueante contra el entorno warm antes de la corrida completa.
M6. Ejecutar los flujos contra ese entorno warm (runners en el mismo namespace o en namespace adyacente autorizado, vía Service del SUT).
M7. **Reset de estado verificado entre corridas** (restart + clean DB + flush cache + verificación, completitud 100%; sin `reset_verified=true` no hay siguiente corrida) + **scale-down en idle** + **rebuild o teardown total periódico** del entorno warm.
M8. RBAC + NetworkPolicy: solo el test namespace (+ acceso autorizado de runners al Service del SUT); **nunca staging ni producción del cliente**; runners sin acceso a LLM; resource limits y quotas por workflow.
M9. Post-mortem de lógica de negocio entregado al usuario.
M10. Despliegue GitOps del **producto** (entorno estable para operadores) para el módulo 8, **incluido el entorno warm como workload gestionado por GitOps**.

### Should Have
S1. Entrega por Slack. S2. Segunda familia de flujos (p. ej. concurrencia además del feliz camino). S3. Traspaso a humano con contexto estructurado (boot fallido, reset fallido o ensayo fallido). S4. Panel de políticas (eventos GitHub, cuotas del entorno warm, workflows con timeouts y aprobaciones, confirm-required). S5. Dashboard inbox de notificaciones pendientes + estado del warm (`ready`/`dirty`/`cuarentena`/`idle-escalado).

### Could Have
C1. Soporte para ambos motores de DB. C2. Persistencia de sesiones interrumpidas. C3. Biblioteca histórica de flujos. C4. Interfaz de auditoría completa. C5. Pull de imagen de registry además de build desde repo.

### Won't Have (y por qué)
W1. Ejecutar contra producción **o staging** del cliente — excluido por principio de diseño permanente. El único entorno estable es el de la plataforma (UI/API de operadores).
W2. Motor de ejecución propietario ilegible — excluido permanentemente (principio #5).
W3. Modelo de pricing/billing multi-tier — problema de negocio, no del MVP técnico.
W4. Modo Enterprise "corre en el clúster del cliente" como producto empacado — no alcanzable en un semestre; el MVP usa *nuestro* cluster, *nuestro* test namespace y *nuestro* entorno warm.
W5. Cobertura de las 6 clases de ataque meta a mes 6 (`pvd.md` sec. 6) — meta de producto madurado; el MVP genera flujos de QA de lógica de negocio, no el catálogo completo.
W6. GraphQL/gRPC como contrato de primer nivel — MVP: lo que el contenedor expone por HTTP/UI.
W7. Alta disponibilidad multi-región — fuera del objetivo del módulo 8.
W8. **Auto-run** en cada evento de GitHub — contradice el confirm. Auto-**notify** **sí** es Must (M1).
W9. Certificaciones de compliance (SOC2, ISO 27001) — no alcanzable en un semestre académico.
W10. Leer el código fuente de la app para generar tests — caja negra permanente.
W11. QA visual, a11y y scanners de seguridad como producto — fuera del MVP (`pvd.md` sec. 6).

---

## 9. Especificación funcional: módulos y features

| Módulo | Features principales | Roles y permisos | Pantallas/flujos |
|---|---|---|---|
| **M1 GitHub, artefacto y planificación** | Recibir commit/PR/tag, inbox de notificaciones, resolución de artefacto, deploy sobre entorno warm, inferir superficie externa, recomendar/generar flujos QA según workflow | User: ve inbox, confirma, selecciona flujos. Admin: solo lectura | "Inbox" → "Warm ready" → "Flujos recomendados" |
| **M2 Ensayo** | Ejecutar flujo unitario contra el entorno warm, validar éxito, reintentos acotados, bloquear corrida completa, generar contexto de traspaso | Automático | "Ensayando..." → éxito/fallo |
| **M3 Entorno warm (lifecycle)** | Gestionar en el **test namespace** el entorno reutilizable (app + 1 DB + 1 Redis pre-desplegados): health, scale-down en idle, rebuild/teardown periódico, deploy de versiones por corrida | Automático; Admin define cuotas, cadencia de higiene y workflows | Estado del warm: ready/dirty/cuarentena/idle |
| **M4 Ejecución de QA** | Runners/Jobs contra el entorno warm, recolección de logs, RBAC/NetworkPolicy, enforcement de workflow (cuota/timeout/aprobación) | Tras confirm (+ ensayo + `reset_verified` previo). Nunca desde el webhook solo | "Corrida en curso" |
| **M5 Reset verificado y limpieza** | Reset entre corridas (restart + clean DB + flush cache + verificación `reset_verified`), cuarentena si falla, housekeeping de sesiones abandonadas, rebuild/teardown periódico | Automático | Confirmación "Reset verificado / warm ready" |
| **M6 Post-mortem y reportería** | Correlación de logs con causa de negocio, resumen, envío a Slack, evidencia histórica | User consume su reporte; Admin ve agregados | "Reporte de corrida" |
| **M7 Gobernanza y control de autonomía** | GitHub App, tipos de evento, confirm-required, cuotas del warm, **workflows de negocio** (complejidad, cuotas, timeouts, aprobaciones), bloqueo de staging/prod, log de auditoría | **Exclusivo de Admin** (escritura); User solo lectura | "Repositorio conectado", "Workflows", "Log de auditoría" |
| **M8 Identidad y acceso** | Autenticación, autorización por rol, SSO (futuro) | Todos | Login |

```mermaid
flowchart TB
    subgraph EXT["Servicios externos (fuera del cluster)"]
        LLM["API de LLM<br/>(planeacion + post-mortem)"]
        SLACK["Slack API"]
        GH["GitHub App<br/>(commit / PR / tag)"]
        REG["Registry de imagen<br/>(opcional)"]
    end

    subgraph K8S["Kubernetes"]
        subgraph STABLE["Entorno estable del producto (lo que usan Marta y Julian)"]
            UI["UI / API Gateway<br/>(Deployment)"]
            AUTH["M8 Identidad y Acceso"]
            PLAN["M1 GitHub + planificacion"]
            GOV["M7 Gobernanza"]
            REPORT["M6 Post-mortem"]
        end

        subgraph TESTNS["Namespace de prueba (SUT: entorno warm reutilizable)"]
            APP["App del cliente (Docker)<br/>deploy por corrida sobre warm"]
            REHEARSAL["M2 Ensayo (Job, 1 pod)"]
            RUNNERS["M4 Runners de QA (Jobs)"]
            RESET["M5 Reset verificado (Job)<br/>+ rebuild/teardown periodico"]
            DB[("DB warm: Postgres o Mongo<br/>(reset entre corridas)")]
            CACHE[("Redis warm<br/>(flush entre corridas)")]
        end
    end

    GH -->|"notify, nunca run"| UI
    UI --> AUTH
    UI --> PLAN
    PLAN -->|"tras confirm: deploy"| GH
    PLAN -->|"opcional: pull image"| REG
    PLAN -->|"infiere superficie, genera flujos"| LLM
    PLAN -->|"deploy version + workflow"| APP
    APP --> DB
    APP --> CACHE
    PLAN --> REHEARSAL
    REHEARSAL --> APP
    REHEARSAL --> GOV
    GOV -->|"confirm + ensayo_passed + reset_verified<br/>+ workflow (cuota/timeout/aprobacion)?"| RUNNERS
    RUNNERS --> APP
    RUNNERS --> RESET
    RESET --> REPORT
    RESET -.->|"idle: scale-down<br/>higiene: rebuild/teardown"| APP
    REPORT -->|"correlaciona logs"| LLM
    REPORT --> SLACK
```

**Qué corre en el entorno estable (producto):** UI/API Gateway, Identidad, Planificación (M1), Gobernanza (M7), Post-mortem (M6).
**Qué corre en el namespace de prueba (SUT):** entorno warm reutilizable (app Docker del cliente + deps), ensayo (M2), runners (M4), reset verificado + rebuild/teardown periódico (M5).
**Qué es servicio externo:** API de LLM (principio #1), Slack API, GitHub, registry opcional.

> ⚠️ `TBD`: almacenamiento de evidencia/reportes — MinIO in-cluster vs. bucket externo, sin definir en los docs.
> ⚠️ `TBD`: para PRs, la fuente del artefacto es build-from-repo; para tags, puede ser imagen publicada. El MVP debe documentar cuál camino soporta primero (Should: C5).

---

## 10. Métricas de éxito

### North Star
> **Tasa de hallazgos lógicos confirmados por el usuario (no ruido):** % de corridas que producen al menos un hallazgo marcado como real/accionable. Baseline: no existe (`TBD`). Meta: >30% de las APIs del beachhead en 90 días `[INTERNO]` (ancla en `pvd.md` sec. 8, con el matiz de "confirmado").

### KPIs de activación
| KPI | Baseline | Meta |
|---|---|---|
| Time-to-first-isolated-run | Cualitativo: "semanas" `[VERIFICAR]` | < 5 minutos `[INTERNO]` |
| % de cuentas con política de autonomía (GitHub + confirm-required + test ns) antes de la primera corrida | No existe `TBD` | 100% `[INTERNO]` |

### KPIs de retención
| KPI | Baseline | Meta |
|---|---|---|
| Corridas recurrentes por cliente activo/mes | No existe `TBD` | Propuesta: 2-3/mes `[INTERNO]`, a validar |
| Change Failure Rate del cliente | No existe `TBD` | Sin meta fija — lagging y multi-causal, se reporta como evidencia cualitativa |

### KPIs de calidad
| KPI | Meta |
|---|---|
| Ensayo a la primera | >70% al mes 6; <40% = *"generador de 400s"* `[INTERNO]` |
| Boot/deploy del artefacto sobre warm | Fallos de deploy = handoff, no reintento infinito `[INTERNO]` |
| Completitud de reset verificado | 100% `[INTERNO]` (sin `reset_verified=true` no hay siguiente corrida) |
| Higiene del warm (rebuild/teardown en cadencia) | 100% de ventanas cumplidas `[INTERNO]` |
| Precisión del post-mortem | >80% vs. diagnóstico humano `[INTERNO]` |

### Métricas específicas de IA
| Dimensión | Métrica | Meta |
|---|---|---|
| Precisión | Precisión del post-mortem / Ensayo a la primera | >80% / >70% |
| Utilidad | North Star | >30% en 90 días |
| Seguridad | Completitud de reset verificado + higiene de rebuild | 100% / 100% |
| Seguridad | Incidentes de límite de autonomía violado (escape de namespace, auto-run, target fuera del test ns, corrida sin reset verificado) | **0** (circuit-breaker, no meta gradual) |

### Métrica de fallo por ruido
> **Tasa de ruido de hallazgos** = (falsos positivos + hallazgos ignorados 7 días) ÷ total de hallazgos reportados. Umbral propuesto: <20% saludable, >50% = "modo ruido" que bloquea escalar volumen `[INTERNO]`. Señal complementaria: tiempo hasta la primera interacción del usuario con el reporte.

---

## 11. Plan de evaluación de la IA

### Dataset inicial
10-15 **artefactos de referencia** (e-commerce, fintech, logística): app containerizable + superficie externa documentada o descubrible, con: anotación "golden" de los flujos de negocio, subset con bugs de lógica sembrados deliberadamente, subset "trampa" de validación de esquema (formato/regex no evidente), subset que **no arranca** (para evaluar fail-closed). `TBD`: 100% sintético vs. artefactos reales anonimizados con permiso.

No es un dataset de "10 archivos OpenAPI sueltos": el agente ve la app **corriendo**, no un YAML que el usuario pegó.

### Criterios de calidad
* **Factualidad:** la causa raíz del post-mortem debe coincidir con la real conocida del dataset.
* **Adherencia a instrucciones:** cumplimiento binario de reglas duras (reset verificado incluido, nunca fuera del test namespace, se detiene tras N intentos, no lee fuente para generar tests, no auto-run, respeta workflow/cuota/timeout/aprobación).
* **Relevancia:** el hallazgo es información nueva y accionable, no ruido (conecta con la métrica de ruido del Segmento 10).

### Cómo se revisan los outputs
1. Evaluación offline contra el dataset antes de lanzar.
2. Muestreo continuo en producción contra el log crudo (% de muestreo `TBD`, alto al inicio, decreciente).
3. Bucle de regresión: cada falso positivo marcado por un usuario se agrega al dataset de evaluación.

### Red-teaming y escenarios adversariales

| # | Escenario | Mitigación |
|---|---|---|
| 1 | Inyección de prompt vía descripciones expuestas (OpenAPI, HTML, banners) | Contenido tratado siempre como dato; defensa en profundidad vía RBAC/NetworkPolicy del test ns. El agente **no** lee el source como instrucción ni como atajo. |
| 2 | Inyección de prompt vía logs/respuestas de la app en el sandbox | Post-mortem trata logs como texto a resumir, nunca como instrucción; no tiene capacidad de ejecutar acciones |
| 3 | Agotamiento de costo vía superficie combinatoria extrema | Límite duro de tiempo/tokens de planificación; corte y escalamiento a humano |
| 4 | Dockerfile / imagen maliciosa o breakout del warm namespace | NetworkPolicy namespace-only; no privileged; no montar el socket de Docker del host; RBAC que no agenda fuera del test ns; rebuild periódico desde imagen base |
| 5 | Intento de usar staging/prod del cliente como target | No hay credenciales de esos entornos; allowlist = solo el Service del entorno warm en el test ns |
| 6 | Fuga de secretos a través del reporte | Filtrado de patrones de secretos antes del LLM y antes de publicar |
| 7 | Boot loop de un artefacto roto | Fail-closed + tope de tiempo (`critica.md` R4) |

> ⚠️ Los escenarios 1, 4 y 5 confirman que los controles de infraestructura del principio #4 (Segmento 6) son la única defensa que sigue funcionando incluso si el modelo es engañado con éxito.

---

## 12. Riesgos y mitigaciones

| # | Riesgo | Categoría | Prob. | Impacto | Mitigación | Fuente |
|---|---|---|---|---|---|---|
| 1 | Contaminación entre corridas / escape del warm namespace | Técnico/Seguridad | Alta | Muy alto | Reset verificado obligatorio + cuarentena si falla + NetworkPolicy/RBAC del test ns + rebuild/teardown periódico | `critica.md` R1 |
| 2 | Alucinación de payloads / bucle 400 | Técnico | Alta | Medio | Ensayo obligatorio; escalamiento a humano (UC5) | `critica.md` R2 |
| 3 | Costo de inferencia descontrolado | Técnico/Financiero | Media | Alto | Arquitectura cerebro/músculo | `critica.md` R3 |
| 4 | Artefacto que no arranca / reintento infinito | Técnico | Alta | Medio | Fail-closed + handoff estructurado | `critica.md` R4 |
| 5 | Comoditización por GitHub + copiloto / incumbentes | Mercado/Producto | Alta `[VERIFICAR]` | Alto | Apostar al bucle medido (confirm, warm con reset verificado, post-mortem), no al agente aislado | `pvd.md` sec. 9 |
| 6 | Confianza ciega en output de IA equivocado | Producto/Seguridad | Media-Alta | Alto | Evidencia cruda junto al resumen; auditoría humana continua | Derivado de `critica.md` R2 |
| 7 | Datos sensibles/compliance (fintech) | Legal/Seguridad | Media | Alto | MVP con datos 100% sintéticos en el entorno warm, nunca PII real; reset verificado anti-fuga entre corridas | Gap identificado, Segmento 3 |
| 8 | Inyección indirecta de prompts vía superficie expuesta | Seguridad | Media | Alto si sin controles | Dato no instrucción + defensa en profundidad de infraestructura | Segmento 11 |
| 9 | Límite de autonomía deja de ser verificable por recorte de MVP | Producto | Media | Alto | M7/M8 y confirm-required en Must Have sin negociación | Advertencia propia, Segmentos 6 y 8 |
| 10 | Dependencia de un único proveedor de LLM | Técnico | Media | Medio | Capa de abstracción sobre el proveedor; degradación aceptable sin comprometer corridas en curso | Derivado de arquitectura, Segmento 9 |
| 11 | Willingness-to-pay no validada | Mercado/Negocio | Media-Alta | Alto | Validar con pilotos antes de contratos/precios cerrados | `pvd.md` sec. 7 `[VERIFICAR]` |

---

## 13. Plan de entrega alineado al curso

Kubernetes **sigue siendo** el orquestador: namespaces, Deployments/StatefulSets para el warm, Jobs para ensayo/runners/reset, HPA/KEDA o CronJob para scale-down en idle, NetworkPolicy, RBAC. Lo que cambia es el **modelo**: el SUT es el entorno warm reutilizable en el test namespace (no staging del cliente), con reset verificado entre corridas y rebuild/teardown periódico.

### Módulos 4-5 (Kubernetes y CKA)
**Se construye:** Namespace de prueba aislado con el **entorno warm** (Deployment de la app + `StatefulSet`/PVC para DB warm + Redis warm), `Jobs` para ensayo (M2) y reset verificado (M5), primeros `Deployments` del control plane (M1) en el entorno estable.
**Se valida:** ciclo de vida de `Jobs`, almacenamiento warm con reset, `kubectl logs`/`describe` como insumo del post-mortem (M6), que **nada** del SUT vive fuera del test ns, y que el warm escala hacia abajo en idle.

### Módulo 6 (CKAD): qué se despliega
`Deployments` completos de M1, M6, M7, M8 en el entorno estable; `ConfigMaps`/`Secrets` sintéticos para el warm; `Jobs` parametrizados para runners (M4) y reset (M5) con `resource limits`; CronJob de rebuild/higiene; `liveness`/`readiness probes` de la app bajo prueba + job de verificación de reset.

### Módulo 7 (CKS): qué se asegura
RBAC de mínimo privilegio (no agenda fuera del test ns); `NetworkPolicy` que no sale del test ns y bloquea runners hacia LLM; admisión de políticas (Kyverno/OPA) reforzando "solo test ns + workflow declarado"; gestión de secretos (ninguno de staging/prod del cliente); escaneo de imágenes del SUT; cuarentena automática si el reset no verifica.

### Módulo 8 (producción): qué queda corriendo con GitOps y qué se mide
El **producto** (UI estable) **y el entorno warm** desplegados y mantenidos vía GitOps (ArgoCD/Flux); el warm se reutiliza por corrida con reset verificado. Se empiezan a medir los KPIs del Segmento 10 (Time-to-first-isolated-run con warm start, Ensayo a la primera, Deploy sobre warm, Completitud de reset 100%, higiene de rebuild, incidentes de autonomía = 0, costo en idle).

### Sesión 16: qué se demuestra
1. Camino feliz en vivo (journey 7.1): notificación de GitHub → confirm → deploy sobre warm `ready` → flujo generado → reset verificado → post-mortem.
2. Bloqueo en vivo de un intento de salir del test namespace (principio #4) — **no** una allowlist de hosts de staging.
3. Fail-closed en vivo: artefacto que no arranca **o reset que no verifica** → cuarentena + handoff, sin siguiente corrida.
4. Métricas reales del dashboard (completitud de reset, ensayo a la primera, estado del warm, costo idle).
5. Reconocimiento explícito del alcance MoSCoW y de lo que queda pendiente frente al PVB completo.

```mermaid
flowchart LR
    A["Modulos 4-5 / CKA<br/>warm namespace, Jobs, storage warm<br/>app + DB + Redis reutilizables"] --> B["Modulo 6 / CKAD<br/>Deployments estables + warm<br/>reset verificado + workflows"]
    B --> C["Modulo 7 / CKS<br/>RBAC, NetworkPolicy<br/>cuarentena + solo test ns"]
    C --> D["Modulo 8 / Produccion<br/>GitOps del producto + warm<br/>reset 100% + idle + rebuild"]
    D --> E["Sesion 16<br/>GitHub notify → confirm → warm<br/>+ reset verificado + bloqueo de escape"]
```
