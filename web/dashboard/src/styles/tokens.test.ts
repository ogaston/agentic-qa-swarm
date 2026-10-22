import { describe, expect, it } from 'vitest';

// Contraste AA (U9-T04 CA-3): lee tokens.css y styles.css, sin navegador, y comprueba ≥ 4.5:1 en
// tema claro y oscuro para: (a) los pares declarados en DESIGN.md §2.2, (b) los pares que la propia
// hoja usa, derivados de sus reglas (color y fondo de una misma regla, o de un componente y su
// :hover), y (c) todo texto de la hoja sobre las superficies de la app y todo fondo con el texto por
// defecto. Así un cambio en styles.css que rompa AA falla aquí sin que alguien lo tenga que listar.

// Vitest vacía los CSS importados (?raw devuelve ''), así que los archivos se leen con el módulo
// de Node. El especificador va en variable para no exigir @types/node al typecheck, y la ruta
// se resuelve desde el directorio de trabajo (`npm test` corre en web/dashboard).
const fsModulo = 'node:fs';
const { readFileSync } = (await import(/* @vite-ignore */ fsModulo)) as {
  readFileSync: (ruta: string, codificacion: 'utf8') => string;
};
const proceso = (globalThis as unknown as { process: { cwd: () => string } }).process;
const leer = (ruta: string) => readFileSync(`${proceso.cwd()}/${ruta}`, 'utf8');
const tokensCss = leer('src/styles/tokens.css');
const hojaCss = leer('src/styles.css');

const DARK_MARKER = '@media (prefers-color-scheme: dark)';

/** Separa el bloque del tema claro (`:root {…}`) del bloque oscuro (`@media … { :root {…} }`). */
function splitThemes(css: string): { light: string; dark: string } {
  const at = css.indexOf(DARK_MARKER);
  if (at < 0) throw new Error('tokens.css no declara el tema oscuro con prefers-color-scheme');
  return { light: css.slice(0, at), dark: css.slice(at) };
}

/** Extrae `--nombre: valor;` de un bloque. Los valores se devuelven sin espacios sobrantes. */
function parseTokens(block: string): Map<string, string> {
  const out = new Map<string, string>();
  for (const m of block.matchAll(/--([a-z0-9-]+)\s*:\s*([^;]+);/g)) {
    out.set(m[1] ?? '', (m[2] ?? '').trim());
  }
  return out;
}

function hexToRgb(hex: string): [number, number, number] {
  const h = hex.replace('#', '');
  if (!/^[0-9a-fA-F]{6}$/.test(h)) throw new Error(`color no hexadecimal de 6 dígitos: ${hex}`);
  return [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16)) as [number, number, number];
}

function luminance(hex: string): number {
  const [r, g, b] = hexToRgb(hex).map((c) => {
    const s = c / 255;
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
  }) as [number, number, number];
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function contrast(a: string, b: string): number {
  const la = luminance(a);
  const lb = luminance(b);
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05);
}

/** Resuelve un color (`var(--x)` o hex de 6 dígitos) con los tokens de un tema. null si no es un color. */
function resolver(valor: string, tokens: Map<string, string>): string | null {
  const v = valor.trim();
  const ref = /^var\(--([a-z0-9-]+)\)$/.exec(v);
  const crudo = ref ? tokens.get(ref[1] ?? '') : v;
  if (crudo === undefined) throw new Error(`falta el token ${v}`);
  return /^#[0-9a-fA-F]{6}$/.test(crudo) ? crudo : null;
}

// Pares declarados en DESIGN.md §2.2 (texto/fondo e insignia/fondo). Los nombres son tokens.
const PARES_DESIGN: ReadonlyArray<readonly [string, string]> = [
  ['text', 'surface'],
  ['text', 'bg'],
  ['text', 'surface-2'],
  ['text-muted', 'surface'],
  ['text-muted', 'surface-2'],
  ['text-faint', 'surface'],
  ['text-faint', 'surface-2'],
  ['accent-text', 'accent-soft'],
  ['accent-text', 'surface'],
  ['ok', 'ok-soft'],
  ['ok', 'surface'],
  ['warn', 'warn-soft'],
  ['warn', 'surface'],
  ['danger', 'danger-soft'],
  ['danger', 'surface'],
  ['info', 'info-soft'],
  ['teal', 'teal-soft'],
];

interface Regla {
  base: string;
  raw: string;
  color: string | null;
  fondo: string | null;
}

/** Reglas planas de la hoja: cada selector de una lista, con su base (sin pseudoclases) y sus declaraciones. */
function reglas(css: string): Regla[] {
  const out: Regla[] = [];
  for (const m of css.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    const selector = (m[1] ?? '').trim();
    const cuerpo = m[2] ?? '';
    if (selector === '' || selector.startsWith('@')) continue;
    const color = /(?:^|[;\s])color\s*:\s*([^;]+)/.exec(cuerpo)?.[1]?.trim() ?? null;
    const fondo = /background(?:-color)?\s*:\s*([^;]+)/.exec(cuerpo)?.[1]?.trim() ?? null;
    for (const sel of selector.split(',')) {
      const base = sel.trim().replace(/::?[a-z-]+(\([^)]*\))?/g, '').trim();
      out.push({ base, raw: sel.trim(), color, fondo });
    }
  }
  return out;
}

interface Par {
  nombre: string;
  fg: string;
  bg: string;
}

/** Pares (texto, fondo) que la hoja usa: misma regla, o color de un componente con fondo de su :hover/estado. */
function paresDeLaHoja(tokens: Map<string, string>): Par[] {
  const rs = reglas(hojaCss);
  const pares: Par[] = [];
  const añadir = (nombre: string, colorV: string | null, fondoV: string | null) => {
    if (colorV === null || fondoV === null) return;
    const fg = resolver(colorV, tokens);
    const bg = resolver(fondoV, tokens);
    if (fg !== null && bg !== null) pares.push({ nombre, fg, bg });
  };
  for (const r of rs) añadir(`${r.base} (misma regla)`, r.color, r.fondo);
  for (const r of rs) {
    for (const f of rs) {
      if (f.base === r.base && f !== r) añadir(`${r.base} (color + fondo de otra regla)`, r.color, f.fondo);
    }
  }
  return pares;
}

const temas = splitThemes(tokensCss);

describe.each([
  ['claro', temas.light],
  ['oscuro', temas.dark],
])('contraste AA en tema %s', (_nombre, bloque) => {
  const tokens = parseTokens(bloque);
  const valor = (nombre: string): string => {
    const v = tokens.get(nombre);
    if (v === undefined) throw new Error(`falta el token --${nombre}`);
    return v;
  };
  const rs = reglas(hojaCss);
  // Texto que hereda de la superficie de la app: reglas con color y sin fondo propio. Los textos con
  // fondo propio ya se prueban contra ese fondo (pares derivados). Y al revés: fondos sin color propio
  // reciben el texto por defecto (--text) o secundario (--text-muted).
  // Se excluyen los pseudoelementos (adornos sin texto: el punto del wordmark, el punto pulsante) y los
  // estados :hover, que cambian el fondo de un control cuyo texto ya se prueba en su regla base.
  const decorativa = (r: Regla) => r.raw.includes('::');
  const textos = [...new Set(rs.filter((r) => r.color !== null && r.fondo === null && !decorativa(r)).map((r) => r.color ?? ''))];
  const fondos = [...new Set(rs.filter((r) => r.fondo !== null && r.color === null && !decorativa(r) && !r.raw.includes(':hover')).map((r) => r.fondo ?? ''))];

  it.each(PARES_DESIGN)('DESIGN §2.2: --%s sobre --%s ≥ 4.5:1', (fg, bg) => {
    expect(contrast(valor(fg), valor(bg)), `--${fg} sobre --${bg}`).toBeGreaterThanOrEqual(4.5);
  });

  it('botón primario: --on-accent sobre --accent y sobre --accent-hover ≥ 4.5:1', () => {
    expect(contrast(valor('on-accent'), valor('accent'))).toBeGreaterThanOrEqual(4.5);
    expect(contrast(valor('on-accent'), valor('accent-hover'))).toBeGreaterThanOrEqual(4.5);
  });

  it('pares derivados de styles.css ≥ 4.5:1', () => {
    const pares = paresDeLaHoja(tokens);
    expect(pares.length).toBeGreaterThan(0);
    const malos = pares
      .map((p) => ({ ...p, ratio: contrast(p.fg, p.bg) }))
      .filter((p) => p.ratio < 4.5)
      .map((p) => `${p.nombre}: ${p.fg} sobre ${p.bg} = ${p.ratio.toFixed(2)}`);
    expect(malos).toEqual([]);
  });

  it('todo texto de la hoja sobre las superficies de la app (--bg, --surface, --surface-2) ≥ 4.5:1', () => {
    const malos: string[] = [];
    for (const t of textos) {
      const fg = resolver(t, tokens);
      if (fg === null) continue;
      for (const sup of ['bg', 'surface', 'surface-2']) {
        const ratio = contrast(fg, valor(sup));
        if (ratio < 4.5) malos.push(`${t} sobre --${sup} = ${ratio.toFixed(2)}`);
      }
    }
    expect(malos).toEqual([]);
  });

  it('todo fondo de la hoja con --text y --text-muted ≥ 4.5:1', () => {
    const malos: string[] = [];
    for (const f of fondos) {
      const bg = resolver(f, tokens);
      if (bg === null) continue;
      for (const txt of ['text', 'text-muted']) {
        const ratio = contrast(valor(txt), bg);
        if (ratio < 4.5) malos.push(`--${txt} sobre ${f} = ${ratio.toFixed(2)}`);
      }
    }
    expect(malos).toEqual([]);
  });
});
