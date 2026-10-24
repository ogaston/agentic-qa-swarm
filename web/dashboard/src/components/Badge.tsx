import type { ReactNode } from 'react';

export type TonoBadge = 'ok' | 'warn' | 'danger' | 'accent' | 'info' | 'teal' | 'neutral';

interface PropsBadge {
  tono: TonoBadge;
  /** Punto de color delante del texto (el texto sigue siendo el significado). */
  punto?: boolean;
  children: ReactNode;
}

/** Insignia semántica del mockup (.badge-<tono>). El texto nunca se omite: no es solo color. */
export function Badge({ tono, punto = false, children }: PropsBadge) {
  return (
    <span className={`badge badge-${tono}`}>
      {punto && <span className="badge-dot" aria-hidden="true" />}
      {children}
    </span>
  );
}
