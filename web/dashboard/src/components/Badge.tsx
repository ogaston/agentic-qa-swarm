import type { ReactNode } from 'react';

export type TonoBadge = 'ok' | 'warn' | 'danger' | 'accent' | 'info' | 'teal' | 'neutral';

interface PropsBadge {
  tono: TonoBadge;
  /** Punto de color delante del texto (el texto sigue siendo el significado). */
  punto?: boolean;
  /** Sin fondo ni borde, solo texto coloreado y punto opcional. */
  plano?: boolean;
  className?: string;
  children: ReactNode;
}

/** Insignia semántica del mockup (.badge-<tono>). El texto nunca se omite: no es solo color. */
export function Badge({ tono, punto = false, plano = false, className, children }: PropsBadge) {
  const clases = ['badge', `badge-${tono}`];
  if (plano) clases.push('badge-plano');
  if (className) clases.push(className);
  return (
    <span className={clases.join(' ')}>
      {punto && <span className="badge-dot" aria-hidden="true" />}
      {children}
    </span>
  );
}
