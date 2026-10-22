import type { ReactNode } from 'react';

interface PropsCard {
  /** Título de la cabecera de la tarjeta (.card-head), opcional. */
  titulo?: string;
  /** Subtítulo bajo el título (.card-sub). */
  sub?: string;
  /** Relleno del mockup (.card-pad). Por defecto activo. */
  pad?: boolean;
  children?: ReactNode;
}

/** Tarjeta del mockup: superficie con borde plano, cabecera opcional y relleno (.card-pad). */
export function Card({ titulo, sub, pad = true, children }: PropsCard) {
  return (
    <div className={pad ? 'card card-pad' : 'card'}>
      {titulo !== undefined && (
        <div className="card-head">
          <div>
            <div className="card-title">{titulo}</div>
            {sub !== undefined && <div className="card-sub">{sub}</div>}
          </div>
        </div>
      )}
      {children}
    </div>
  );
}
