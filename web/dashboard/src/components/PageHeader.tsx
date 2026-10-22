import type { ReactNode } from 'react';

export interface ElementoMeta {
  etiqueta: string;
  valor: ReactNode;
}

interface PropsPageHeader {
  titulo: string;
  descripcion?: string;
  meta?: readonly ElementoMeta[];
}

/** Encabezado de página del mockup: título, descripción y franja de metadatos (.az-page-header). */
export function PageHeader({ titulo, descripcion, meta = [] }: PropsPageHeader) {
  return (
    <div className="az-page-header">
      <div className="az-page-title">
        <h1>{titulo}</h1>
        {descripcion !== undefined && <p>{descripcion}</p>}
      </div>
      {meta.length > 0 && (
        <div className="az-header-meta">
          {meta.map((m) => (
            <div className="az-meta-item" key={m.etiqueta}>
              <span className="az-meta-label">{m.etiqueta}</span>
              <span className="az-meta-value">{m.valor}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
