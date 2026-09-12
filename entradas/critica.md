# Deep Research: Crítica Adversarial y Riesgos Técnicos
**Producto:** Agentic QA Swarm (QA-as-a-Service)

## 1. Riesgo Crítico 1: Sandbox leftover y escape de namespace
El riesgo más letal para la adopción ya no es ensuciar el Postgres de staging del cliente: **staging no es el blanco**. El riesgo es dejar basura en el **namespace de prueba** (Deployments, Pods, PVCs, compose) o una NetworkPolicy floja que alcance fuera de él (staging, producción, la red del cluster).
* **El ataque adversarial:** "Tu herramienta llenó nuestro cluster de workloads huérfanos" / "Dijeron que aislaban y la corrida salió del namespace de prueba".
* **Límite arquitectónico necesario:** teardown forzado de todos los workloads de la corrida (app Docker + deps + runners); RBAC que no agenda fuera del test namespace; NetworkPolicy que no sale de él; cero credenciales de staging/producción del cliente en el sandbox.

## 2. Riesgo Crítico 2: Alucinación de restricciones (El bucle del Error 400)
Los LLMs son propensos a ignorar restricciones sutiles de la superficie externa. Si un campo requiere un formato específico (ej. un RUT o un UUID válido) y el agente alucina datos genéricos, la app en el sandbox rechazará todas las peticiones con `400 Bad Request`.
* **El ataque adversarial:** "El agente gasta recursos disparando tráfico inútil que nunca llega a la lógica de negocio porque es bloqueado en la validación inicial".
* **Límite arquitectónico necesario:** fase de ensayo unitario **dentro del sandbox**. El agente debe probar el flujo con un solo usuario y verificar éxito (p. ej. HTTP `2xx`) antes de autorizar la corrida completa.

## 3. Riesgo de Viabilidad Financiera: Costo de Inferencia vs. Rendimiento
Una corrida de QA o de estrés lógico genera miles de peticiones.
* **El ataque adversarial:** "Si el agente llama a la API del LLM por cada paso del flujo, la latencia será ridícula y el costo por tokens quebrará a la empresa operadora".
* **Límite arquitectónico necesario:** Diseño estricto de bifurcación cerebro/músculo. La IA (agente planificador) solo actúa *off-cluster* para inferir la superficie y generar flujos deterministas. Los runners en el namespace de prueba ejecutan ese código sin llamadas adicionales al LLM. El LLM vuelve a intervenir únicamente en el análisis *post-mortem* de los logs consolidados.

## 4. Riesgo Crítico 4: El artefacto no arranca
Un commit o un PR puede no tener imagen publicable, un Dockerfile roto, o secretos de entorno que el sandbox no debe copiar de staging.
* **El ataque adversarial:** "Confirmé la corrida y el producto se quedó reintentando un compose que nunca levantó; quemaron compute y no me dijeron qué faltaba".
* **Límite arquitectónico necesario:** fail-closed. Si el artefacto no bootea en un tope de tiempo, la corrida se detiene, no se generan flujos contra el vacío, y el usuario recibe un handoff estructurado (logs de boot, hipótesis, acción sugerida). Cero reintento infinito.
