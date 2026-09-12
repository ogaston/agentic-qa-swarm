# Unit of Work Story Map — Agentic QA Swarm

> Todas las historias Must asignadas (13/13). Los Should (S1-S5) se historian después (Pregunta 5 = B).

| Historia | Unidad | Notas |
|---|---|---|
| US-M1 Notificación sin auto-run | U1 | Intake + inbox |
| US-M2 Pull + boot aislado (incl. confirm) | U1 (confirm) + U2 (boot) | Historia compartida: criterio de confirm en U1, criterios de boot en U2 |
| US-M3 Superficie externa | U3 | agent-planner |
| US-M4 Flujos inspectables (generación) | U3 | agent-planner; ejecución en U2 (US-M6) |
| US-M5 Ensayo bloqueante | U2 | Gate `ensayo_passed` |
| US-M6 Corrida + evidencia | U2 | Runners + MinIO (U5 provee MinIO) |
| US-M7.1 Teardown tras corrida | U2 | Job teardown + verificación |
| US-M7.2 Housekeeping abandonadas | U2 | CronJob + sesión persistida |
| US-M8.1 RBAC test-ns-only | U4 (+ U5 base) | Manifiestos revisables en U5, lógica de gates en U4 |
| US-M8.2 NetworkPolicy sin egress/LLM | U4 (+ U5 base) | Idem |
| US-M8.3 Confirm-required + cero staging/prod | U4 (+ U1 confirm) | Gate en U4, registro en U1 |
| US-M9 Post-mortem | U3 | agent-reporter |
| US-M10 GitOps del producto | U5 | Flux + CI + observabilidad |

- Cobertura: 13/13 Must asignadas; 0 sin asignar.
- Compartidas (frontera explícita): US-M2 (U1/U2), US-M4 (U3 genera, U2 ejecuta), US-M8.x (U4 lógica, U5 manifiestos base).