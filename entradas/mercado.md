# Análisis de Mercado
**Producto:** Agentic QA Swarm

## 1. Tamaño y Tendencia del Mercado
El producto opera en la intersección de tres mercados en hipercrecimiento:
* **Continuous Testing / QA Automation:** Valorado en ~$24B USD, con un enfoque creciente en herramientas nativas de la nube.
* **API Security Testing:** Impulsado por el aumento de arquitecturas de microservicios. Gartner estima que las vulnerabilidades en APIs son el vector de ataque más frecuente en aplicaciones web empresariales.
* **CI sobre GitHub:** cada commit, PR y release ya dispara pipelines; el hueco es QA de caja negra contra el **artefacto aislado**, no otra suite white-box en Actions.

## 2. Segmentación (TAM, SAM, SOM)
* **TAM (Total Addressable Market):** Toda empresa global de software B2B o B2C que publique artefactos (repo o imagen) con una superficie externa transaccional (E-commerce, Fintech, SaaS).
* **SAM (Serviceable Available Market):** Empresas medianas (50-500 empleados) que usan GitHub, pueden correr la app en Docker, y operan (o pueden operar) Kubernetes, donde un error lógico genera pérdidas financieras directas.
* **SOM (Serviceable Obtainable Market):** Startups y scale-ups tecnológicas en fase de crecimiento (Series A-C) que tienen equipos de plataforma o DevOps, pero que carecen del presupuesto para contratar un equipo de SRE/QA dedicado, y que no aceptan herramientas que usen su staging como blanco.

## 3. Análisis Competitivo
| Competidor | Enfoque Principal | Debilidad Frente a Nuestra Solución |
| :--- | :--- | :--- |
| **GitHub Actions / CI genérico** | Corre la suite que ya vive en el repo. | White-box o contra staging. No genera QA de caja negra ni post-mortem de lógica de negocio. |
| **k6 (Grafana Labs)** | Pruebas de carga como código (JavaScript). | Determinista. Requiere semanas de programación manual. No arranca el artefacto. |
| **Gremlin** | Chaos Engineering puro. | Se enfoca en infraestructura (apagar nodos), no en lógica de negocio (Capa 7). |
| **Postman (AI)** | Interfaz de desarrollo de APIs. | No levanta el artefacto en un namespace desechable ni cubre el trabajo de un QA. |

**Ventaja Competitiva Injusta:** Confirmación humana sobre el evento de GitHub + artefacto Docker en un namespace de prueba dedicado (nunca staging/prod del cliente) + QA de caja negra generado desde la superficie externa + post-mortem de lógica de negocio.
