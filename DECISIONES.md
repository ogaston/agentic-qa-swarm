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

### 5. Pivote a entorno tibio (warm) reutilizable — 2026-09-17

Se abandona el modelo "reconstruir todo por corrida" por un **entorno warm propio de la plataforma** (namespace de prueba de vida larga, aislado de staging/prod): se reutiliza entre corridas para recortar startup y costo, escala hacia abajo en idle y se reconstruye o desmonta periódicamente por higiene. Entre corridas aplica **reset verificado** (restart + clean DB + flush cache + verificación con `reset_verified=true`; cuarentena si falla). Se mantiene: confirmación humana, aislamiento, resource limits, NetworkPolicy, runners deterministas sin LLM, evidencia y fail-closed. La complejidad se configura por **workflows de negocio** (cuotas, timeouts, aprobaciones). Archivos tocados: `specs/prd.md` + `entradas/prd.md` (sincronizados), `unidades-y-tareas.md` (U1/U2/U4/U5), `aidlc-docs/aidlc-state.md`. Riesgo #1 pasa a ser **contaminación entre corridas**; KPI "teardown 100%" pasa a **"reset verificado 100% + higiene de rebuild"**.




1b. Metricas especificas de lo que debia perseguir para lograr un MVP funcional (tales como precisión >80%, ruido <20%)



- Preferi que se creara todo en un monorepo, ya que considero que es la forma mas facil de manejar un ecosistema de paquetes de software.
- Utilice go para el manejo de los componentes de kubernetes porque es muy robusto, ligero y ademas es un lenguaje que personalmente estoy aprendiendo, entonces quiero explorarlo un poco mas. 

### 6. Configuración de modelos para el Loop de Agentes (Módulo 6) — 2026-10-01

Para despachar las 37 tareas de `unidades-y-tareas.md` mediante el loop de tres agentes, se adopta el escalonamiento de modelos con ajuste de esfuerzo de razonamiento:
- **Orquestador**: `Opus 5.5` (reemplaza `fable`) — máxima visión arquitectónica para arbitraje, control de dependencias entre olas y apertura de PRs limpios.
- **Codificador**: `Sonnet 5.5 (effort: low)` — generación de código ágil, enfocada y costo-eficiente por tarea en su propio worktree.
- **Revisor**: `Sonnet 5.5 (effort: high)` — análisis exhaustivo con razonamiento profundo para ejecutar comandos de verificación, auditar contratos y detectar fallos sutiles.
- **Humano**: Revisa y fusiona (el techo del loop).
