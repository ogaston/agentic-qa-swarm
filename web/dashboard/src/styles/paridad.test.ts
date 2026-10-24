import { describe, expect, it } from 'vitest';

// Paridad de reglas con el mockup (U10-T01, CA-2). Lee sin navegador el bloque <style> de
// mockups/swarm-mock.html y la hoja del dashboard (src/styles.css + src/styles/*.css). Para cada
// selector de nivel superior del mockup que toca una clase del inventario de U10-T01 y no lleva
// pseudoclase, atributo ni estado, exige que el dashboard tenga el MISMO selector con las mismas
// propiedades height, padding, border, border-radius, font-size, font-weight y gap (mismo valor
// textual, por tanto mismos tokens). Las @-reglas (@media) no se comparan: son adaptación
// responsiva y quedan fuera de este criterio. El JS del mockup no se lee.

// Mismo patrón que tokens.test.ts: Vitest no expone node:fs en el tipado y la ruta sale del cwd
// (`npm test` corre en web/dashboard).
const fsModulo = 'node:fs';
const { readFileSync, readdirSync } = (await import(/* @vite-ignore */ fsModulo)) as {
  readFileSync: (ruta: string, codificacion: 'utf8') => string;
  readdirSync: (ruta: string) => string[];
};
const proceso = (globalThis as unknown as { process: { cwd: () => string } }).process;
const raiz = proceso.cwd();
const leer = (ruta: string) => readFileSync(`${raiz}/${ruta}`, 'utf8');

const PROPIEDADES = ['height', 'padding', 'border', 'border-radius', 'font-size', 'font-weight', 'gap'];

// Inventario de U10-T01 (Alcance, «Portar de mockups/swarm-mock.html»), por nombre de clase.
const INVENTARIO: readonly RegExp[] = [
  /^az-(header|brand|nav|subbar)(-|$)/,
  /^az-(logo|session-chip|avatar|principal|container|page-header|page-title|tablist|tab|action-link)$/,
  /^az-meta-/,
  /^(card|badge|btn)(-|$)/,
  /^alert-(box|danger|warn)$/,
  /^segmented$/,
  /^kpi-/,
  /^kv-list$/,
  /^stack$/,
  /^grid-main-side$/,
  /^form-field$/,
  /^modal-/,
  /^login-wrap$/,
];

const enInventario = (clase: string): boolean => INVENTARIO.some((re) => re.test(clase));

interface Regla {
  selector: string;
  decl: Map<string, string>;
}

const normalizar = (s: string): string => s.replace(/\s+/g, ' ').trim();

const sinComentarios = (css: string): string => css.replace(/\/\*[\s\S]*?\*\//g, '');

function declaraciones(cuerpo: string): Map<string, string> {
  const out = new Map<string, string>();
  for (const parte of cuerpo.split(';')) {
    const i = parte.indexOf(':');
    if (i < 0) continue;
    out.set(normalizar(parte.slice(0, i)).toLowerCase(), normalizar(parte.slice(i + 1)));
  }
  return out;
}

/** Reglas de nivel superior. Recorre llaves y omite las @-reglas con todo su contenido anidado. */
function reglasTop(css: string): Regla[] {
  const texto = sinComentarios(css);
  const out: Regla[] = [];
  let profundidad = 0;
  let inicioCabecera = 0;
  let inicioCuerpo = 0;
  let cabecera = '';
  let omitida = false;
  for (let i = 0; i < texto.length; i++) {
    const ch = texto[i];
    if (ch === ';' && profundidad === 0) {
      // Sentencia sin bloque de nivel superior (p. ej. @import): no pertenece a la cabecera siguiente.
      inicioCabecera = i + 1;
    } else if (ch === '{') {
      if (profundidad === 0) {
        cabecera = normalizar(texto.slice(inicioCabecera, i));
        inicioCuerpo = i + 1;
        omitida = cabecera.startsWith('@');
      }
      profundidad++;
    } else if (ch === '}') {
      profundidad--;
      if (profundidad === 0) {
        if (!omitida && cabecera !== '') {
          out.push({ selector: cabecera, decl: declaraciones(texto.slice(inicioCuerpo, i)) });
        }
        inicioCabecera = i + 1;
      }
    }
  }
  return out;
}

const items = (selector: string): string[] => selector.split(',').map(normalizar).filter((s) => s !== '');

const clases = (selector: string): string[] =>
  [...selector.matchAll(/\.([a-zA-Z][\w-]*)/g)].map((m) => m[1] ?? '');

// Mockup: bloque <style> y sus reglas de nivel superior.
const mockup = leer('../../mockups/swarm-mock.html');
const ini = mockup.indexOf('<style>');
const fin = mockup.indexOf('</style>');
if (ini < 0 || fin < 0) throw new Error('mockups/swarm-mock.html no tiene bloque <style>');
const reglasMock = reglasTop(mockup.slice(ini + '<style>'.length, fin));

const clasesInventario = new Set<string>();
const refs: Array<{ item: string; decl: Map<string, string> }> = [];
for (const r of reglasMock) {
  for (const item of items(r.selector)) {
    const tocaInventario = clases(item).filter(enInventario);
    tocaInventario.forEach((c) => clasesInventario.add(c));
    const sinEstado = !/[:[]/.test(item);
    const tieneProp = PROPIEDADES.some((p) => r.decl.has(p));
    if (tocaInventario.length > 0 && sinEstado && tieneProp) refs.push({ item, decl: r.decl });
  }
}

// Dashboard: todas las hojas de estilo, en orden. Las propiedades de un mismo selector se
// fusionan (la última declaración gana, como en cascada plana).
const hojasDashboard = [
  'src/styles.css',
  ...readdirSync(`${raiz}/src/styles`)
    .filter((f) => f.endsWith('.css'))
    .sort()
    .map((f) => `src/styles/${f}`),
];
const reglasDash = hojasDashboard.flatMap((h) => reglasTop(leer(h)));
const fusionadas = new Map<string, Map<string, string>>();
for (const r of reglasDash) {
  for (const item of items(r.selector)) {
    const m = fusionadas.get(item) ?? new Map<string, string>();
    for (const [k, v] of r.decl) m.set(k, v);
    fusionadas.set(item, m);
  }
}
const clasesDashboard = new Set(reglasDash.flatMap((r) => clases(r.selector)));

describe('paridad de reglas con mockups/swarm-mock.html (U10-T01 CA-2)', () => {
  it('el mockup aporta inventario y referencias (sin lectura vacía)', () => {
    expect(clasesInventario.size).toBeGreaterThan(40);
    expect(refs.length).toBeGreaterThan(40);
  });

  it('cada clase del inventario existe en el CSS del dashboard', () => {
    const faltan = [...clasesInventario].filter((c) => !clasesDashboard.has(c)).sort();
    expect(faltan).toEqual([]);
  });

  it('cada selector del mockup existe en el dashboard con las mismas propiedades', () => {
    const diferencias: string[] = [];
    for (const ref of refs) {
      const dash = fusionadas.get(ref.item);
      if (dash === undefined) {
        diferencias.push(`falta la regla «${ref.item}»`);
        continue;
      }
      for (const prop of PROPIEDADES) {
        const esperado = ref.decl.get(prop);
        if (esperado === undefined) continue;
        const real = dash.get(prop);
        if (real !== esperado) {
          diferencias.push(`${ref.item} · ${prop}: mockup «${esperado}», dashboard «${real ?? '(ausente)'}»`);
        }
      }
    }
    expect(diferencias).toEqual([]);
  });
});
