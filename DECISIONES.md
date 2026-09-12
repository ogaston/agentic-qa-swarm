# Decisiones

### 1. 2 Cosas que el marco me obligo a decidir


1. El uso especifico de varias herramientas como; el lenguaje para el control plane, la utilizacion de Flux para la implementacion de mecanismos en GitOps y el uso de un lenguaje distinto para los agentes (TS/Python).

2. El manejo de las sesiones, ya que era algo que no habia tomado en cuenta, pero estas pueden ser persistentes o tener un tiempo de gracia de 24h.

### 2. Lo que pedi cambiar

Justamente el punto 2 fue algo que solicite que cambiara, ya que esto no habia estado claro en el PRD. El PRD hablaba de realizar teardown pero sin ser especifico en cuando se realizaria. Entonces el cambio fue, otorgar mejor un tiempo de gracia. 


### 3. Grafo de dependencias

Pude observar que la U5 no depende de ninguna. Mientras que: 

U1->U4
U4->U5
U1/U4 -> U2 -> U3


### 4. El riesgo que cambio

El riesgo numero 1 giraba entorno a la seguridad del sandbox y sus leftover. Por eso en U5, se habla de RBAC/NetworkPolicy, en U4 de gates y politicas, haciendo esto que la confinacion de los procesos se mantenga mas controlada. 




1b. Metricas especificas de lo que debia perseguir para lograr un MVP funcional (tales como precisión >80%, ruido <20%)



- Preferi que se creara todo en un monorepo, ya que considero que es la forma mas facil de manejar un ecosistema de paquetes de software.
- Utilice go para el manejo de los componentes de kubernetes porque es muy robusto, ligero y ademas es un lenguaje que personalmente estoy aprendiendo, entonces quiero explorarlo un poco mas. 
-
