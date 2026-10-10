/**
 * Familias de flujos que se pueden elegir al confirmar una corrida. S2 (editar
 * flujos) está fuera de alcance: la lista es fija y `happy-path` va marcada
 * por defecto.
 */
export const FLOW_FAMILIES = ['happy-path'] as const;

export type FlowFamily = (typeof FLOW_FAMILIES)[number];
