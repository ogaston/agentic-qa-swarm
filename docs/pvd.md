# Producto

| | |
|---|---|
| **Nombre** | Agentic QA Swarm |
| **Descripción** | Plataforma conectada a GitHub que, tras confirmar un commit, PR o tag/release, trae el artefacto, arranca la app en Docker dentro de un namespace de prueba dedicado, genera flujos de QA de caja negra a partir de la superficie externa, los ejecuta en aislamiento y entrega un post-mortem de lógica de negocio. |
| **Tagline interno** | *GitHub notifica, el humano confirma, el sandbox ejecuta, el humano lee el reporte.* |

---

## 1. Problema

Probar una app transaccional "en serio" no es un problema de herramientas sueltas: es un problema de integración que nadie cierra. GitHub Actions corre lo que ya está en el repo. k6, Gatling y JMeter ejecutan carga si alguien escribió el script. Chaos Mesh y Gremlin rompen infraestructura. Los escáneres SAST/DAST no ven estado. Nadie, en cada commit/PR/release, **levanta una copia aislada de la app, entiende lo que expone hacia afuera, genera el trabajo de un QA y explica qué invariante de negocio falló** —sin tocar staging.

### El patrón que se repite

- En cada evento de GitHub, CI prueba código o pega contra staging compartido. No hay una copia desechable de **la app en ejecución** contra la que un QA de caja negra pueda trabajar.
- Escribir y mantener flujos de QA (funcionales, de estado, de concurrencia) toma **semanas, no horas**. El 80% es plomería de autenticación, setup y teardown, no diseño de prueba. `[VERIFICAR]`
- Esos scripts se rompen con cada cambio mínimo de la superficie externa. La deuda de mantenimiento hace que el equipo los abandone y asuma el riesgo en producción.
- Las fallas caras —condiciones de carrera, invariantes de negocio, mass assignment / lógica de negocio de OWASP API Security— no las detecta un test unitario ni un escáner estático. Solo flujos reales contra la app corriendo, con intención de negocio.
- Ya existen copilotos que generan aserciones para un endpoint. El hueco no es generar un request: es **arrancar el artefacto en aislamiento, generar los flujos, no escapar del namespace de prueba y explicar el hallazgo**.

### ¿Sobrevive al próximo salto de modelos?

- [x] **Sí** — es un problema de **workflow / integración**, no de output.

Un modelo más inteligente infiere mejor los flujos, pero no resuelve quién levanta el artefacto, quién confirma el run, cómo se ensaya antes de la corrida completa, ni cómo se garantiza que staging y producción del cliente nunca son el blanco. Eso es gobernanza del sandbox, no inteligencia. Cuanto mejores sean los modelos, más urgente se vuelve ponerles límites verificables: notify-no-run, ensayo obligatorio, teardown del namespace, IA fuera del bucle de ejecución.

> **Durability Score: 4/5**

---

## 2. Segmento target

**Beachhead:** equipos de plataforma / backend de 3 a 15 ingenieros, en scale-ups de 50 a 500 empleados que ya usan GitHub, pueden correr su app en Docker y operan (o pueden operar) Kubernetes. Sector: fintech, e-commerce, logística y SaaS B2B transaccional. Un bug de lógica de negocio les cuesta dinero, no solo un ticket. OpenAPI/Swagger es una **ventaja para descubrir la superficie**, no un requisito de entrada.

### Tres señales de beachhead

1. Tienen GitHub (commit, PR, tag/release) y un artefacto containerizable (Dockerfile, compose, o imagen publicada).
2. Ya tienen CI, pero no genera QA de caja negra contra una copia aislada de la app; o alguien escribió k6/Gatling y lo abandonó.
3. No tienen un equipo de SRE/QA dedicado a esto; el VP de Engineering no justifica contratarlo. No quieren que una herramienta toque su staging.

> **No es para:** equipos que no pueden correr la app en Docker; equipos sin GitHub; empresas con QA de caja negra aislado y staffed. OpenAPI ausente no descalifica si la app expone HTTP/UI al arrancar.

### ¿Quién controla el veto de confianza?

El **VP of Engineering / Head of Platform**. Mata la adopción con una pregunta:

> ¿Esto toca nuestro staging o producción?

Si la respuesta no es *no: solo el namespace de prueba de la plataforma; la app vive y muere con la corrida; teardown obligatorio*, la conversación se acaba. Por eso el aislamiento del SUT no es un detalle de implementación — es el argumento de venta. El único entorno estable es el de **la plataforma** (el dashboard que usa el equipo), no el de la app del cliente.

---

## 3. Ventaja competitiva primaria

- [x] **Workflow**

### Tres capas acumulables

| Capa | Qué es | Qué pasa si falta |
|---|---|---|
| **Cerebro fuera, músculo dentro** | El LLM observa la superficie externa del sandbox y genera flujos deterministas. Los runners ejecutan sin llamar al modelo. | Se quiebra el margen y la latencia de la corrida. |
| **Ensayo antes de corrida completa** | Un flujo unitario debe pasar (p. ej. `2xx` / invariante mínima) antes de autorizar la corrida completa en el namespace. | El producto es un generador caro de HTTP 400. |
| **Sandbox en namespace dedicado + teardown** | Cada corrida arranca la app en Docker dentro de un namespace de prueba y lo destruye al terminar. Staging y producción del cliente no son el blanco. | El primer cliente serio no vuelve: o no confía, o el cluster queda sucio. |

El moat real no es el agente (replicable). Es el bucle completo:

```
evento GitHub → notify → confirm → artifact → Docker en namespace
  → inferir superficie externa → flujos QA → ensayo → run → teardown → post-mortem
```

…más la biblioteca de flujos que acertaron en APIs reales. Las clases de ataque (carrera, flujo roto, mass assignment) son una familia dentro de esos flujos, no el catálogo entero del producto.

---

## 4. Arena competitiva

- [x] **Disruptor (AI-Disrupted)** — El workflow (probar la app en un evento de GitHub) existe hace una década. Lo estamos reimaginando, no inventando.

### Cómo sobrevivimos a incumbentes y open source

| Actor | Qué resuelve | Relación con nosotros |
|---|---|---|
| **GitHub Actions / CI genérico** | Corre la suite que ya vive en el repo, a menudo white-box o contra staging. | Alternativa real. No genera QA de caja negra contra un sandbox desechable ni entrega post-mortem de lógica de negocio. |
| **k6 / Gatling / JMeter** | Motores de carga (k6 es abierto). | No competimos: podemos orquestarlos como **un** ejecutor. El valor no está en el motor; está en levantar el artefacto, inferir, generar, ensayar y explicar. |
| **Gremlin / Chaos Mesh** | Caos de infraestructura (nodos, latencia). | Adyacentes: no atacan lógica de negocio en Capa 7. `[VERIFICAR: roadmaps L7]` |
| **Postman AI** | Aserciones de un endpoint. | No arranca el artefacto, no cubre el trabajo de un QA, no aisla en un namespace. `[VERIFICAR]` |

> **Riesgo honesto:** si GitHub + un copiloto cierran el bucle compose up → observar API → generar tests → reportar, parte de la diferenciación se evapora. El wedge que queda es: confirmación humana, namespace dedicado, teardown 100%, nunca staging/prod del cliente, post-mortem de lógica de negocio. Está contemplado en la [sección 9](#9-riesgos-críticos).

---

## 5. UX paradigm

- [x] **Agent** — La IA ejecuta autónomamente dentro de límites duros.

**Por qué no Autonomous:** un webhook que arranca la app y dispara QA sin confirmar es exactamente lo que el comprador no va a firmar. El límite es el producto: **auto-notify sí, auto-run no**.

| | |
|---|---|
| **Qué hace sin preguntar** | Recibir el evento de GitHub y crear la notificación. Tras confirmar: traer el artefacto, arrancar Docker en el namespace de prueba, inferir la superficie **externa**, redactar flujos, ensayar, correr, recoger logs, redactar el post-mortem, destruir el sandbox. |
| **Qué no hace nunca** | Auto-ejecutar en el webhook; leer el código fuente de la app para inventar tests; apuntar a staging o producción del cliente; llamar al LLM por cada request de la corrida; omitir teardown. |

Ese reparto también es la línea de defensa económica: si el modelo alucina un payload, el peor resultado del ensayo es un 400 barato dentro del sandbox, no un cluster lleno de basura ni un staging ajeno tocado.

---

## 6. AI decision triangle

- [x] **Capability**

### Trade-offs que acepto

| Sacrificio | Decisión |
|---|---|
| **Velocidad de planificación** | Un plan correcto en 3 minutos vale más que un flujo alucinado en 20 segundos. La corrida completa no sale hasta que el ensayo pasa. |
| **Costo de planificación** | Modelos grandes para inferir la superficie y redactar el post-mortem. Cero inferencia en los runners. |
| **Cobertura al inicio** | Acertar en flujos de QA de lógica de negocio (incluye carrera, doble-gasto, broken flow, mass assignment, idempotencia, saturación de estado) antes de opinar mal sobre 60 tipos de prueba. Visual, a11y y scanners de seguridad quedan fuera del MVP. |

> **Razón de fondo:** el costo de una corrida equivocada no es el token — es un namespace sucio, un artefacto que no arranca y se reintenta forever, o un reporte que no encontró nada. Esa confianza se pierde una sola vez.

---

## 7. Modelo económico

**Modelo de pricing:** Hybrid Tiered — suscripción por repositorio GitHub conectado, con límites de corridas aisladas por mes. `[INTERNO]` `[VERIFICAR: willingness to pay]`

| Tier | Precio | Incluye |
|---|---|---|
| **Team** | $500/mes | 1 repo GitHub, 20 corridas/mes en el namespace de prueba |
| **Business** | $1.800/mes | 3 repos, 80 corridas, SSO, cuotas de namespace custom |
| **Enterprise** | A convenir | El sandbox corre en el clúster del cliente (namespace de prueba dedicado; requisito frecuente en fintech) |

**¿Escala a 10x usuarios?**

- [x] **Sí**, con un ajuste. El costo marginal principal es inferencia de planificación + compute efímero del sandbox (app Docker + runners), que escala con corridas, no con asientos. Por eso el límite del tier se define en corridas y no en seats.

### Economía por cuenta (tier Business) `[INTERNO]`

| Concepto | Mensual |
|---|---|
| Inferencia (plan + post-mortem, ~80 corridas) | $160 |
| Compute efímero del sandbox (app + deps + runners en K8s) | $90 |
| Almacenamiento de reportes y evidencia | $25 |
| Soporte prorrateado | $120 |
| **Costo total** | **$395** |
| **Revenue** | **$1.800** |
| **Gross margin** | **78%** |

El margen se sostiene si el ensayo filtra los planes rotos y si el artefacto arranca. Si se escala una corrida completa con payloads 400, o si un compose roto se reintenta sin tope, el compute se dispara — por eso el ensayo unitario y el fail-closed al boot son decisiones económicas, no solo técnicas.

---

## 8. Métricas de éxito

### Métricas de usuario

| Métrica | Definición | Target |
|---|---|---|
| **Time-to-first-isolated-run** | Desde confirmación de un evento GitHub hasta primera corrida en sandbox que pasa ensayo | < 5 minutos |
| **Tasa de hallazgo lógico** | Corridas que revelan un defecto de lógica de negocio no cubierto por los tests del cliente | >30% de las APIs del beachhead en 90 días `[INTERNO]` |

### Métricas específicas de IA

| Métrica | Definición | Target |
|---|---|---|
| **Ensayo a la primera** | Planes que pasan el ensayo a la primera | >70% al mes 6. Por debajo de 40%, el producto es un generador de 400s. `[INTERNO]` |
| **Boot del artefacto** | % de corridas confirmadas en las que la app arranca en el namespace de prueba | Alto; fallos de boot son handoff estructurado, no reintento infinito `[INTERNO]` |
| **Completitud de teardown** | % de workloads del sandbox (app, deps, runners) destruidos al terminar o abandonar | **100%**. Cualquier filtración de namespace es incidente de confianza. `[INTERNO]` |
| **Precisión del post-mortem** | vs. diagnóstico humano | >80% en los flujos cubiertos `[INTERNO]` |

**Métrica de salud secundaria:** costo de infra + inferencia por corrida. Sostiene el margen bruto.

---

## 9. Riesgos críticos

### 1. ¿Qué pasa si el problema desaparece en 12 meses por comoditización?

**Riesgo alto.** Generar tests desde una API expuesta ya lo intentan copilotos y Postman. Si GitHub Actions + un copiloto cierran compose up → generar QA → reportar, el diferenciador central se evapora. `[VERIFICAR: roadmaps actuales]`

**Mitigación:** no apostar a generar el script. Apostar al bucle medido (notify/confirm, namespace dedicado, ensayo, teardown 100%, hallazgos en artefactos reales) y construir sobre ejecutores estándar (k6 u otros) en vez de contra ellos.

### 2. ¿Puede un competidor replicarlo en menos de 6 semanas?

El software sí; el loop de confianza no. Un equipo competente conecta un GitHub App, levanta compose y pide a un LLM que genere tests en un mes. Lo que **no** se replica en seis semanas:

- La biblioteca de flujos que sobrevivieron ensayo y encontraron bugs en apps production-like.
- El teardown que un Head of Platform ya dejó correr en *su* cluster (namespace de prueba, sin tocar staging).
- El historial de hallazgos por tipo de app (pagos, carritos, reservas), que se construye operando.

### 3. Si tienes éxito a escala, ¿cuál es la primera forma en que se rompe la confianza?

Tres vectores, en orden de probabilidad:

| # | Vector | Por qué duele | Mitigación |
|---|---|---|---|
| 1 | **Sandbox leftover / escape de namespace** | La app o sus deps quedan corriendo, o una NetworkPolicy floja alcanza fuera del namespace de prueba. El más letal: el producto convence a alguien de que "aislamos" y no es verdad. | Teardown obligatorio del namespace; NetworkPolicy que no sale del test ns; RBAC que no agenda fuera de él. |
| 2 | **Corrida de 400s / artefacto que no arranca** | El agente alucina la superficie, o el Dockerfile/imagen de un PR no bootea, y se quema compute sin tocar lógica de negocio. | Ensayo antes de corrida completa; fail-closed si el artefacto no levanta; "no sé generar este flujo" y "no pude arrancar la app" son respuestas válidas. |
| 3 | **LLM en el bucle de request** | Un diseño que llame al modelo por cada paso de la corrida quiebra el margen. | Invariante de arquitectura: runners sin credenciales de modelo; el plano de control es el único que infiere. |

> **Verdad incómoda:** este producto vive o muere por la confianza de una sola persona por cuenta —el Head of Platform— y esa confianza se pierde con un namespace sucio o con una corrida que escapó a staging. Por eso el confirm, el ensayo y el teardown no se relajan nunca, ni siquiera cuando el cliente pida "salta el ensayo, que tengo prisa".
