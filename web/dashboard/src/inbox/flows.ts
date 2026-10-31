/**
 * Familias de flujos que se pueden elegir al confirmar una corrida. S2 (editar
 * flujos) está fuera de alcance: la lista es fija y solo las familias
 * disponibles se pueden marcar; `happy-path` va marcada por defecto.
 */
export const FLOW_FAMILIES = [
  { id: 'happy-path', etiqueta: 'happy-path', disponible: true },
  { id: 'load', etiqueta: 'Pruebas de carga', disponible: false },
  { id: 'security', etiqueta: 'Pruebas de seguridad', disponible: false },
] as const;

type FamiliaDeFlujos = (typeof FLOW_FAMILIES)[number];
type FamiliaDisponible = Extract<FamiliaDeFlujos, { disponible: true }>;

/** Ids de las familias que el servidor ya sabe ejecutar (hoy solo `happy-path`). */
export type FlowFamily = FamiliaDisponible['id'];

/** Distingue las familias elegibles de las que solo se muestran como «Próximamente». */
export function esDisponible(familia: FamiliaDeFlujos): familia is FamiliaDisponible {
  return familia.disponible;
}

/** Familias disponibles, las únicas que se marcan por defecto y se envían al confirmar. */
export function familiasDisponibles(): FlowFamily[] {
  return FLOW_FAMILIES.filter(esDisponible).map((familia) => familia.id);
}
