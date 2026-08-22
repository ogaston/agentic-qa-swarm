# Panorama del Dominio (Domain Overview)
**Dominio:** Quality Assurance (QA) Automatizado contra el artefacto, en aislamiento, disparado por GitHub.

## 1. Estado Actual del Dominio
El desarrollo de software moderno ha consolidado el paradigma de "Shift-Left": unitarias, integración y SAST corren temprano en CI/CD, casi siempre disparadas por GitHub (commit, PR, tag/release). Ese pipeline, sin embargo, prueba **lo que el repositorio ya contiene** (tests white-box) o apunta a un **staging compartido**.

Lo que sigue siendo de final de ciclo y manual es el QA de caja negra sobre **una copia aislada de la app en ejecución**: entender lo que el sistema expone hacia afuera, generar los flujos que un QA escribiría, y reportar fallas de lógica de negocio —sin tocar el staging que usan las personas.

## 2. El Vacío en la Automatización
El ecosistema se parte en herramientas que no cierran ese ciclo:

* **CI genérico (GitHub Actions):** corre la suite que ya vive en el repo. No levanta el artefacto en un sandbox desechable para **generar** QA de caja negra; a menudo el job apunta a staging.
* **Infraestructura de inyección de fallos:** Chaos Mesh o Gremlin dominan la resiliencia en Kubernetes (Capa 4): apagar nodos, simular latencia. No entienden flujos de negocio.
* **Motores de carga estáticos:** k6, Gatling o JMeter simulan tráfico en Capa 7, pero un humano tiene que definir cada paso. Suelen ejecutarse contra un entorno ya levantado (casi siempre staging).
* **Copilotos de API:** generan aserciones para un endpoint aislado. No arrancan la app, no cubren el trabajo de un QA, no entregan un post-mortem de lógica de negocio.

El vacío no es "otro motor de k6". Es: **nadie recibe el evento de GitHub, levanta el artefacto en un namespace desechable, observa solo la superficie externa, genera los flujos de QA y explica qué invariante de negocio se rompió** —sin usar staging ni leer el código fuente para inventar las pruebas.

Las clases de ataque (condición de carrera, flujo multi-estado, mass assignment) son **una familia** de esos flujos, no el producto entero.

## 3. El Cambio de Paradigma (La oportunidad agentual)
Los LLMs con razonamiento profundo pueden, por primera vez, **observar una app en ejecución** y comportarse como un QA: entender lo que es visible desde fuera (OpenAPI si existe, HTTP, UI expuesta) e inferir los flujos a ejercitar.

El ingeniero de plataforma no pega un contrato ni escribe el test. Conecta GitHub. Cada commit, PR o tag/release **notifica**; tras confirmar, el sistema:

1. Trae el artefacto (repo o imagen).
2. Arranca la app en Docker, dentro de un **namespace de prueba dedicado**.
3. Infiere solo la superficie externa.
4. Genera los flujos de QA, los ensaya, los corre y destruye el sandbox.
5. Entrega un post-mortem de hallazgos de lógica de negocio.

Staging y producción del cliente no son el blanco. El único entorno estable es el de la plataforma (el dashboard que usa el equipo). La app bajo prueba vive y muere con la corrida.
