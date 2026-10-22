import { describe, expect, it } from 'vitest';

// Contraste AA (U9-T04 CA-3): lee los tokens del archivo CSS, sin navegador, y comprueba
// ≥ 4.5:1 para cada par texto/fondo e insignia/fondo declarado, en tema claro y oscuro.

// Vitest vacía los CSS importados (?raw devuelve ''), así que el archivo se lee con el módulo
// de Node. El especificador va en variable para no exigir @types/node al typecheck, y la ruta
// se resuelve desde el directorio de trabajo (`npm test` corre en web/dashboard).
const fsModulo = 'node:fs';
const { readFileSync } = (await import(/* @vite-ignore */ fsModulo)) as {
  readFileSync: (ruta: string, codificacion: 'utf8') => string;
};
const proceso = (globalThis as unknown as { process: { cwd: () => string } }).process;
const tokensCss = readFileSync(`${proceso.cwd()}/src/styles/tokens.css`, 'utf8');

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

// Pares declarados en DESIGN.md §2.2 (texto/fondo e insignia/fondo). El color de texto del
// botón primario es literal (#ffffff) sobre --accent.
const PARES: ReadonlyArray<readonly [string, string]> = [
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

function valor(tokens: Map<string, string>, nombre: string): string {
  const v = tokens.get(nombre);
  if (v === undefined) throw new Error(`falta el token --${nombre}`);
  return v;
}

const temas = splitThemes(tokensCss);

describe.each([
  ['claro', temas.light],
  ['oscuro', temas.dark],
])('contraste AA en tema %s', (_nombre, bloque) => {
  const tokens = parseTokens(bloque);

  it.each(PARES)('%s sobre %s ≥ 4.5:1', (fg, bg) => {
    const ratio = contrast(valor(tokens, fg), valor(tokens, bg));
    expect(ratio, `--${fg} sobre --${bg}`).toBeGreaterThanOrEqual(4.5);
  });

  it('botón primario: #ffffff sobre --accent ≥ 4.5:1', () => {
    expect(contrast('#ffffff', valor(tokens, 'accent'))).toBeGreaterThanOrEqual(4.5);
  });
});
