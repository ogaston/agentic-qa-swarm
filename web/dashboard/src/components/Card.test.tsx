import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { Card } from './Card';

// U10-T01 (revisión F-01): `.card` debe ser el del mockup, sin relleno ni margen propios. Se lee la
// regla de la hoja, sin navegador. `Card pad={false}` tiene que quedar sin relleno de verdad.

// Mismo patrón que tokens.test.ts: Vitest no expone node:fs en el tipado; la ruta sale del cwd.
const fsModulo = 'node:fs';
const { readFileSync } = (await import(/* @vite-ignore */ fsModulo)) as {
  readFileSync: (ruta: string, codificacion: 'utf8') => string;
};
const proceso = (globalThis as unknown as { process: { cwd: () => string } }).process;
const hoja = readFileSync(`${proceso.cwd()}/src/styles.css`, 'utf8');

/** Declaraciones de la regla cuyo selector es exactamente `sel` (no `.card-pad`, `.card-vista`…). */
function declaracionesDe(sel: string): string {
  const escapado = sel.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const m = new RegExp(`(?:^|\\n)${escapado}\\s*\\{([^}]*)\\}`).exec(hoja);
  if (m === null) throw new Error(`no hay regla ${sel} en styles.css`);
  return m[1] ?? '';
}

describe('Card y la regla .card del mockup', () => {
  it('la regla .card no declara relleno ni margen propios', () => {
    expect(declaracionesDe('.card')).not.toMatch(/padding|margin/);
  });

  it('Card pad={false} renderiza solo la clase .card (sin .card-pad)', () => {
    const { container } = render(<Card pad={false}>x</Card>);
    expect(container.firstElementChild?.className).toBe('card');
    expect(declaracionesDe('.card')).not.toMatch(/padding/);
  });

  it('Card por defecto añade .card-pad, que sí declara relleno', () => {
    const { container } = render(<Card>x</Card>);
    expect(container.firstElementChild?.className).toBe('card card-pad');
    expect(declaracionesDe('.card-pad')).toMatch(/padding:\s*20px 22px/);
  });

  it('el relleno heredado de las vistas vive en .card-vista', () => {
    expect(declaracionesDe('.card-vista')).toMatch(/padding:\s*16px/);
    expect(declaracionesDe('.card-vista')).toMatch(/margin-bottom:\s*16px/);
  });
});
