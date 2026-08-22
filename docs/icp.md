# Perfil del Cliente Ideal (ICP - Ideal Customer Profile)

## 1. Demografía de la Empresa
* **Tamaño:** Scale-ups y medianas empresas tecnológicas (50 a 500 empleados).
* **Verticales:** E-commerce, Fintech, Logística, y SaaS B2B transaccional.
* **Stack Tecnológico Requerido:** 
  * Usan GitHub (commits, PRs, tags/releases) como fuente de cambio.
  * Su app puede correr en Docker (Dockerfile, compose, o imagen publicada).
  * Operan —o pueden operar— microservicios sobre Kubernetes (EKS, GKE, AKS), porque el sandbox vive en un namespace de prueba dedicado.
  * Superficie externa observable al arrancar: HTTP y, si existe, OpenAPI 3.0 / Swagger (ventaja, no requisito de entrada).

## 2. El "Buyer Persona" (Quién firma el cheque)
* **Rol:** VP of Engineering / Director of Platform Engineering.
* **Dolor Principal:** "Cada vez que lanzamos una feature transaccional, descubrimos *bugs* de lógica de negocio en producción que cuestan dinero. No puedo justificar 3 ingenieros de QA, y no voy a dejar que una herramienta pegue contra nuestro staging".
* **Métrica que le importa:** Reducción del *Change Failure Rate* (Tasa de fallos por cambio) y no aumentar el presupuesto de SRE.
* **Veto de confianza:** "¿Esto toca nuestro staging o producción?" — la única respuesta aceptable es no: solo el namespace de prueba de la plataforma.

## 3. El "User Persona" (Quién usa el producto a diario)
* **Rol:** Platform Engineer, DevOps Engineer, o Backend Lead.
* **Contexto de Uso:** Equipos de 3 a 15 ingenieros.
* **Dolor Principal:** CI ya corre en cada PR, pero solo ejecuta lo que el repo ya trae (o apunta a staging). Mantener flujos de QA de caja negra a mano es tedioso y se rompe cuando cambia la superficie externa.
* **Beneficio Esperado:** Conectar el repo de GitHub. En cada commit, PR o release aparece una notificación; confirma; la app arranca aislada; el agente genera los flujos, corre el QA y entrega en Slack/dashboard un post-mortem de lógica de negocio —sin escribir tests y sin tocar staging.
