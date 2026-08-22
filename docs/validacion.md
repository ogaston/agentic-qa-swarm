# Deep Research: Validación del Problema y Mercado
**Producto:** Agentic QA Swarm (QA-as-a-Service)

## 1. Naturaleza del Problema
Diseñar y mantener QA de caja negra contra la app **en ejecución**, en cada commit/PR/release, es un proceso manual, frágil y costoso —o se omite y se asume el riesgo en producción.
* **El dolor técnico:** CI corre la suite que ya vive en el repo (white-box) o apunta a staging compartido. No levanta una copia aislada del artefacto para que un QA observe la superficie externa y ejercite flujos de negocio (autenticar → crear recurso → modificar en concurrencia → transiciones inválidas).
* **Fragilidad del código:** los scripts escritos a mano (k6, Playwright, colecciones Postman) generan deuda técnica. Un cambio mínimo en lo que la app expone rompe el script y exige intervención manual.

## 2. Evidencia de Mercado
* **Vulnerabilidades de lógica de negocio:** Según el marco OWASP (mass assignment y problemas de lógica de negocio), los errores críticos en producción rara vez provienen de fallas sintácticas, sino de *race conditions* y manipulaciones de estado que los escáneres estáticos (SAST/DAST) no pueden detectar. Eso justifica un **post-mortem de lógica de negocio**, no un dump de JUnit.
* **Adopción de DevOps:** Los equipos de plataforma (3-15 ingenieros) suelen abandonar el mantenimiento de flujos complejos de QA/Chaos porque el tiempo de ingeniería no escala, y rechazan herramientas que usen su staging como blanco.

## 3. Análisis del Ecosistema CNCF y Herramientas Actuales
El ecosistema actual carece de este bucle:
* **GitHub Actions:** excelente disparador. Corre lo que el repo ya trae. No genera QA de caja negra contra un sandbox desechable.
* **k6 / Locust:** excelentes motores de ejecución, integrables en Kubernetes, pero "tontos". Dependen de que un ingeniero escriba la secuencia. Pueden ser **un** ejecutor, no el producto.
* **Postman (funciones de IA):** aserciones simples para un endpoint; no arranca el artefacto ni cubre el trabajo de un QA.
* **Gremlin / Chaos Mesh:** fallos de infraestructura (nodos, latencia), no flujos lógicos desde la capa de aplicación.

## 4. Conclusión de Viabilidad
Existe una oportunidad clara para un middleware agentual que actúe como un "ingeniero de QA de caja negra". Al delegar la observación de la superficie externa y la redacción de flujos a un LLM, y la ejecución al namespace de prueba (app Docker + runners efímeros), se reduce el *Time-to-first-isolated-run* —desde la **notificación de GitHub confirmada** hasta el reporte— de semanas a minutos, sin usar staging del cliente.
