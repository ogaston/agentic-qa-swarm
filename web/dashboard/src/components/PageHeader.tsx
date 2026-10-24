import type { ReactNode } from 'react';

export interface ElementoMeta {
  etiqueta: string;
  valor: ReactNode;
}

interface PropsPageHeader {
  titulo: string;
  /** Texto o fragmento (p. ej. con `<code>`) bajo el título. */
  descripcion?: ReactNode;
  meta?: readonly ElementoMeta[];
  /** Acciones de la franja derecha (p. ej. un botón «Actualizar»), como un elemento más de .az-header-meta. */
  acciones?: ReactNode;
}

/** Encabezado de página del mockup: título, descripción y franja de metadatos (.az-page-header). */
export function PageHeader({ titulo, descripcion, meta = [], acciones }: PropsPageHeader) {
  const hayMeta = meta.length > 0 || acciones !== undefined;
  return (
    <div className="az-page-header">
      <div className="az-page-title">
        <h1>{titulo}</h1>
        {descripcion !== undefined && <p>{descripcion}</p>}
      </div>
      {hayMeta && (
        <div className="az-header-meta">
          {meta.map((m) => (
            <div className="az-meta-item" key={m.etiqueta}>
              <span className="az-meta-label">{m.etiqueta}</span>
              <span className="az-meta-value">{m.valor}</span>
            </div>
          ))}
          {acciones !== undefined && <div className="az-meta-item">{acciones}</div>}
        </div>
      )}
    </div>
  );
}
