# Personas — Agentic QA Swarm

> Fuente: PRD §3 (buyer personas y veto de confianza), §7 (journeys 7.1-7.4). Sin roles inventados (Pregunta 4 = A).

## P1 — Marta, Backend Lead (User diario)

- **Rol**: Backend Lead / Platform Engineer en scale-up transaccional (fintech, e-commerce, logística, SaaS B2B).
- **Contexto**: equipo de 3-15 ingenieros, sin SRE/QA dedicado; repo conectado (`payments-api`); CI existente que no genera QA de caja negra.
- **Motivación**: encontrar bugs de lógica de negocio antes del merge/release sin escribir ni mantener flujos de QA y sin que staging se entere.
- **Mide**: tiempo no perdido en mantenimiento de flujos; cero contacto con staging.
- **Historias**: US-M1, US-M2, US-M3, US-M4, US-M5, US-M6, US-M9.

## P2 — Julián, Head of Platform (Admin)

- **Rol**: Head of Platform / Director of Platform Engineering; administra la plataforma para su equipo.
- **Contexto**: habilita el producto por primera vez; define guardrails; audita.
- **Motivación**: operar dentro de límites de autonomía auditables; justificar el gasto con métricas agregadas.
- **Mide**: guardrails configurados antes de la primera corrida; **reset verificado 100% + higiene de rebuild**; incidentes de autonomía = 0.
- **Permisos exclusivos**: escritura en M7-gobernanza (panel de políticas, eventos, cuotas, confirm-required); lectura del resto.
- **Historias**: US-M7.1, US-M7.2, US-M8.1, US-M8.2, US-M8.3, US-M10.

## P3 — VP of Engineering (Buyer, veto de confianza)

- **Rol**: VP of Engineering; firma el cheque; no opera la plataforma a diario.
- **Contexto**: necesita bajar el *Change Failure Rate* sin contratar SRE/QA.
- **Motivación**: evidencia de reducción de incidentes con aislamiento verificablemente separado de staging/producción.
- **Veto**: *"¿Esto toca nuestro staging o producción?"* — si la respuesta no es demostrablemente "no", no hay compra.
- **Mide**: Change Failure Rate (lagging, cualitativo); % de cuentas con política configurada antes de la primera corrida.
- **Historias**: US-M8.3 (evidencia de aislamiento), US-M9 (evidencia de hallazgos), US-M10 (producto operable vía GitOps).