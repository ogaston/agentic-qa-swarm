// U10-T05: compuerta de fidelidad visual, dashboard frente a mockups/swarm-mock.html.
// Uso (desde web/dashboard, tras `npm ci`): `npm run -s e2e:fidelidad [-- --out <dir>] [-- --control]`.
//   --out <dir>  directorio del informe y las capturas (por defecto bitacoras/U10-T05/ en la raíz).
//   --control    CA-5: compara el mockup contra sí mismo (mismas vistas, temas y anchos). Debe salir 0 y 0 %.
//                Escribe en <dir>/control/ para no pisar el informe real. Argumentos desconocidos: error.
// Para cada vista (inbox, run, runs, warm, login) x tema (claro, oscuro) x ancho (1280, 375):
//   1. estilos calculados de anclas (cabecera, títulos, tarjetas, insignias) con ±1 px y colores exactos;
//   2. estructura: secuencia de regiones y encabezados (tipo + etiqueta) de la vista activa;
//   3. captura de página completa de ambos y diferencia de píxeles (canal distinto > TOLERANCIA_CANAL);
//      umbral declarado: UMBRAL_PIXELES de la superficie.
// El dashboard se construye en un directorio temporal y se sirve en local. La API se simula con
// respuestas fijas (page.route) tomadas de contracts/**/examples/valid, igual que T03: MSW en el
// navegador exigiría service worker en public/ y tocar el build, y el build no cambia (T05 solo mide).
// Informe en bitacoras/U10-T05/ (informe.json, informe.html y capturas/). Sale 0 solo si todo pasa.
import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import http from 'node:http';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { chromium } from 'playwright';
import { INFORME_FIDELIDAD, NOTIFICACIONES, PLAN_FIDELIDAD, RECIBOS_FIDELIDAD, RUN_ID, WARM_FIDELIDAD } from './fidelidad-datos.mjs';

const DASHBOARD = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const RAIZ = path.resolve(DASHBOARD, '../..');
const MOCKUP = path.join(RAIZ, 'mockups/swarm-mock.html');
const DEFECTO_SALIDA = path.join(RAIZ, 'bitacoras/U10-T05');
const USO = 'uso: e2e-fidelidad.mjs [--control] [--out <directorio>]';

// Argumentos: solo --control y --out. Cualquier otro se rechaza (no se ignora en silencio).
function leerOpciones(argv) {
  const opciones = { control: false, out: DEFECTO_SALIDA };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === '--control') opciones.control = true;
    else if (a === '--out') {
      if (!argv[i + 1] || argv[i + 1].startsWith('--')) throw new Error(`--out necesita un directorio. ${USO}`);
      opciones.out = path.resolve(argv[++i]);
    } else throw new Error(`argumento desconocido: ${a}. ${USO}`);
  }
  return opciones;
}

let OPCIONES;
try {
  OPCIONES = leerOpciones(process.argv.slice(2));
} catch (e) {
  console.error(e.message);
  process.exit(2);
}
const SALIDA = OPCIONES.control ? path.join(OPCIONES.out, 'control') : OPCIONES.out;
const CAPTURAS = path.join(SALIDA, 'capturas');
const DIST = fs.mkdtempSync(path.join(os.tmpdir(), 'u10-t05-dist-'));

const ID = RUN_ID;
const UMBRAL_PIXELES = 0.05; // ≤ 5 % del área de la captura
const TOLERANCIA_CANAL = 16; // diferencia máxima por canal (0–255) para considerar «igual»
const TOLERANCIA_PX = 1; // ±1 px en propiedades de longitud

const TEMAS = ['light', 'dark'];
const ANCHOS = [1280, 375];

// Vistas: ruta de la app, vista del mockup, si requieren sesión y estado de datos (run).
const VISTAS = [
  { id: 'inbox', ruta: '/inbox', mock: 'inbox', sesion: true },
  { id: 'run', ruta: `/runs/${ID}`, mock: 'run', sesion: true, demoMock: 'bug' },
  { id: 'runs', ruta: '/runs', mock: 'runs', sesion: true },
  { id: 'warm', ruta: '/warm', mock: 'warm', sesion: true },
  { id: 'login', ruta: '/login', mock: 'login', sesion: false },
];

// Anclas (estilo calculado). `todos` compara todas las coincidencias por orden; sin él, la primera.
// `global` busca en el documento completo (cabecera fuera de la vista).
const ANCLAS = [
  { id: 'cabecera', sel: '.az-header', global: true, props: ['height', 'background-color', 'border-bottom-width', 'border-bottom-color'] },
  { id: 'titulo', sel: '.az-page-title', props: ['font-size', 'font-weight', 'color'] },
  { id: 'tarjeta', sel: '.card', todos: true, props: ['height', 'border-top-width', 'border-top-left-radius', 'border-top-color', 'background-color'] },
  { id: 'rejilla', sel: '.kpi-row, .kpi-strip, .grid-main-side', todos: true, props: ['display', 'row-gap', 'column-gap'] },
];

const ESTRUCTURA_SEL =
  'header, nav[aria-label], [role=tablist], h1, h2, h3, section[aria-label]:not(.view), details.card > summary';

// Respuestas fijas de la API (la misma forma que los ejemplos del contrato).
const FIJOS = {
  notificaciones: NOTIFICACIONES,
  recibos: RECIBOS_FIDELIDAD,
  warm: WARM_FIDELIDAD,
  corrida: { id: ID, trace_id: 'tr-99201a4f-8c1e-44d0-a712', state: 'done', verdict: 'bug' },
  plan: PLAN_FIDELIDAD,
  informe: INFORME_FIDELIDAD,
};

function construir() {
  const r = spawnSync('npx', ['vite', 'build', '--outDir', DIST, '--emptyOutDir'], { cwd: DASHBOARD, stdio: 'inherit' });
  if (r.status !== 0) throw new Error('vite build falló');
}

function servir() {
  const tipos = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml', '.png': 'image/png' };
  return new Promise((resolver) => {
    const srv = http.createServer((req, res) => {
      const url = decodeURIComponent((req.url || '/').split('?')[0]);
      let archivo = path.join(DIST, url);
      if (!fs.existsSync(archivo) || fs.statSync(archivo).isDirectory()) archivo = path.join(DIST, 'index.html');
      res.writeHead(200, { 'Content-Type': tipos[path.extname(archivo)] || 'application/octet-stream' });
      fs.createReadStream(archivo).pipe(res);
    });
    srv.listen(0, '127.0.0.1', () => resolver(srv));
  });
}

async function rutaApi(route) {
  const url = new URL(route.request().url());
  const json = (cuerpo, status = 200) =>
    route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(cuerpo) });
  const p = url.pathname;
  if (p === '/api/auth/login') return json({ token: 'token-fidelidad', expires_at: new Date(Date.now() + 3600e3).toISOString() });
  if (p === '/api/auth/session') return json({ principal_id: 'operador-demo', role: 'user' });
  if (p === '/api/notifications') {
    const estado = url.searchParams.get('state');
    return json(FIJOS.notificaciones.filter((n) => estado === null || n.state === estado));
  }
  if (p === '/api/confirmations') return json(FIJOS.recibos);
  if (p === '/api/warm') return json(FIJOS.warm);
  if (p === `/api/runs/${ID}`) return json(FIJOS.corrida);
  if (p === `/api/runs/${ID}/plan`) return json(FIJOS.plan);
  if (p === `/api/reports/${ID}`) return json(FIJOS.informe);
  return json({ code: 'not_found', message: p }, 404);
}

// Mide en el navegador: anclas (estilos), estructura visible. Solo cuenta elementos visibles.
function medirEnPagina({ anclas, estructuraSel }) {
  const visible = (el) => el.getClientRects().length > 0;
  const todos = (sel) => Array.from(document.querySelectorAll(sel)).filter(visible);
  const estilos = {};
  for (const a of anclas) {
    const els = Array.from(document.querySelectorAll(a.sel)).filter(visible);
    const lista = a.todos ? els : els.slice(0, 1);
    estilos[a.id] = lista.map((el) => {
      const cs = getComputedStyle(el);
      const valores = {};
      for (const p of a.props) valores[p] = cs.getPropertyValue(p).trim();
      return valores;
    });
  }
  // Normalización declarada: el mockup añade «(GET …)» a sus encabezados como nota de API y cita un
  // run_id de demo; ambos son datos, no estructura. La cabecera no tiene etiqueta de texto.
  const normalizar = (t) =>
    t
      .replace(/\(GET [^)]*\)/g, '')
      .replace(/run-[0-9a-f]+/g, 'run-<id>')
      .replace(/Estado operativo de \S+/g, 'Estado operativo de <warm>')
      .replace(/\s+/g, ' ')
      .trim()
      .slice(0, 60);
  // Insignias: estilo por variante (clases badge-*). El texto depende de los datos (método HTTP, código,
  // nombre de flujo) y el mockup trae los suyos: se informa, no se compara.
  const insignias = todos('.badge').map((el) => {
    const cs = getComputedStyle(el);
    const variante = [...el.classList].filter((c) => c.startsWith('badge-')).sort().join(' ') || 'badge';
    return {
      variante,
      texto: normalizar(el.textContent || ''),
      color: cs.color,
      'background-color': cs.backgroundColor,
      'border-top-left-radius': cs.borderTopLeftRadius,
    };
  });
  const estructura = todos(estructuraSel).map((el) => {
    if (el.tagName === 'HEADER') return 'cabecera';
    if (el.tagName === 'NAV') return `navegacion: ${normalizar(el.getAttribute('aria-label') || '')}`;
    if (el.getAttribute('role') === 'tablist') return `pestanas: ${normalizar(el.getAttribute('aria-label') || '')}`;
    if (/^H[1-3]$/.test(el.tagName)) return `encabezado: ${normalizar(el.textContent || '')}`;
    // Región: section con aria-label, o el título (strong) del resumen de un bloque plegable de tarjeta.
    const titulo = el.tagName === 'SUMMARY' ? el.querySelector('strong') : null;
    const texto = el.getAttribute('aria-label') || (titulo ? titulo.textContent : el.textContent) || '';
    return `region: ${normalizar(texto)}`;
  });
  const alto = document.documentElement.scrollHeight;
  const desbordamiento = document.documentElement.scrollWidth - document.documentElement.clientWidth;
  return { estilos, estructura, insignias, alto, desbordamiento };
}

function distintoValor(a, b) {
  if (a === b) return false;
  const na = parseFloat(a);
  const nb = parseFloat(b);
  const px = a.endsWith('px') && b.endsWith('px');
  if (px && !Number.isNaN(na) && !Number.isNaN(nb)) return Math.abs(na - nb) > TOLERANCIA_PX;
  return true;
}

function compararEstilos(app, mock) {
  const diferencias = [];
  for (const a of ANCLAS) {
    const la = app.estilos[a.id] || [];
    const lm = mock.estilos[a.id] || [];
    if (la.length !== lm.length) {
      diferencias.push({ ancla: a.id, propiedad: '(cantidad)', app: String(la.length), mock: String(lm.length) });
      continue;
    }
    la.forEach((va, i) => {
      for (const p of a.props) {
        if (distintoValor(va[p], lm[i][p])) {
          diferencias.push({ ancla: `${a.id}[${i}]`, propiedad: p, app: va[p], mock: lm[i][p] });
        }
      }
    });
  }
  // Insignias: cada variante presente en la app debe existir en el mockup con el mismo color y fondo.
  const porVariante = new Map();
  for (const b of mock.insignias) if (!porVariante.has(b.variante)) porVariante.set(b.variante, b);
  const vistas = new Set();
  for (const a of app.insignias) {
    if (vistas.has(a.variante)) continue;
    vistas.add(a.variante);
    const m = porVariante.get(a.variante);
    if (!m) {
      diferencias.push({ ancla: 'insignia', propiedad: `(variante ${a.variante} ausente en mockup)`, app: a.texto, mock: '-' });
      continue;
    }
    for (const p of ['color', 'background-color', 'border-top-left-radius']) {
      if (distintoValor(a[p], m[p])) {
        diferencias.push({ ancla: `insignia ${a.variante}`, propiedad: p, app: a[p], mock: m[p] });
      }
    }
  }
  return diferencias;
}

function compararEstructura(app, mock) {
  const n = Math.max(app.length, mock.length);
  const diferencias = [];
  for (let i = 0; i < n; i++) {
    if (app[i] !== mock[i]) diferencias.push({ indice: i, app: app[i] ?? '(falta)', mock: mock[i] ?? '(falta)' });
  }
  return diferencias;
}

async function compararPixeles(navegador, pngApp, pngMock) {
  const contexto = await navegador.newContext();
  const pagina = await contexto.newPage();
  await pagina.setContent('<html><body></body></html>');
  const res = await pagina.evaluate(
    async ({ a, b, tol }) => {
      const cargar = (src) =>
        new Promise((ok, err) => {
          const img = new Image();
          img.onload = () => ok(img);
          img.onerror = () => err(new Error('imagen no carga'));
          img.src = src;
        });
      const [ia, ib] = await Promise.all([cargar(a), cargar(b)]);
      const W = Math.max(ia.width, ib.width);
      const H = Math.max(ia.height, ib.height);
      const datos = (img) => {
        const c = document.createElement('canvas');
        c.width = W;
        c.height = H;
        const x = c.getContext('2d');
        x.fillStyle = 'rgb(255,0,255)';
        x.fillRect(0, 0, W, H);
        x.drawImage(img, 0, 0);
        return x.getImageData(0, 0, W, H).data;
      };
      const da = datos(ia);
      const db = datos(ib);
      let distintos = 0;
      for (let i = 0; i < da.length; i += 4) {
        if (
          Math.abs(da[i] - db[i]) > tol ||
          Math.abs(da[i + 1] - db[i + 1]) > tol ||
          Math.abs(da[i + 2] - db[i + 2]) > tol
        ) {
          distintos++;
        }
      }
      return { W, H, distintos, total: W * H, ancho: [ia.width, ib.width], alto: [ia.height, ib.height] };
    },
    { a: pngApp, b: pngMock, tol: TOLERANCIA_CANAL },
  );
  await contexto.close();
  return { ...res, porcentaje: res.distintos / res.total };
}

async function vistaApp(navegador, base, vista, tema, ancho) {
  const contexto = await navegador.newContext({ viewport: { width: ancho, height: 800 }, deviceScaleFactor: 1 });
  await contexto.addInitScript((t) => window.localStorage.setItem('aqs-tema', t), tema);
  await contexto.route('**/api/**', (route) => rutaApi(route));
  const pagina = await contexto.newPage();
  await pagina.goto(`${base}/login`);
  await pagina.waitForSelector('#login-usuario');
  if (vista.sesion) {
    await pagina.fill('#login-usuario', 'operador-demo');
    await pagina.fill('#login-clave', 'clave-demo');
    await pagina.getByRole('button', { name: 'Entrar' }).click();
    await pagina.waitForFunction(() => !window.location.pathname.startsWith('/login'));
    // Navegación del cliente: conserva la sesión en memoria (el token no va a almacenamiento).
    await pagina.evaluate((ruta) => {
      window.history.pushState({}, '', ruta);
      window.dispatchEvent(new PopStateEvent('popstate'));
    }, vista.ruta);
  }
  await pagina.waitForLoadState('networkidle');
  await pagina.waitForTimeout(300);
  const medida = await pagina.evaluate(medirEnPagina, { anclas: ANCLAS, estructuraSel: ESTRUCTURA_SEL });
  // Se compara el ancho del viewport (lo que ve el usuario sin desplazarse en horizontal). El desbordamiento
  // se informa aparte: el mockup a 375 px sobrepasa su ancho y, sin recorte, la captura sería de otro tamaño.
  const png = await pagina.screenshot({
    fullPage: true,
    animations: 'disabled',
    clip: { x: 0, y: 0, width: ancho, height: medida.alto },
  });
  await contexto.close();
  return { ...medida, png };
}

// Neutralizaciones de la PREPARACIÓN del mockup (no de la app). Cada una queda en el informe (campo
// `neutralizaciones` de cada combinación). Demo: el conmutador «Previsualizar estado (Demo)» de la corrida y
// los paneles «Previsualizar estados de error (U6-T03)» del login son previsualizaciones que U10 excluye de la
// app (DESIGN.md §1.3; unidades-y-tareas.md, límites de U10). Se ocultan con display:none al medir el mockup.
// El conmutador «Simular estado del entorno» de Entorno no está en esta lista: no se oculta.
const OCULTAR_MOCK = [
  { id: 'demo-conmutador-corrida', vista: 'run', sel: '.az-meta-item:has(> #runDemoSwitcher)' },
  { id: 'demo-paneles-error-login', vista: 'login', sel: 'div:has(> .segmented > [data-login-demo])' },
];
// Desbordamiento horizontal del mockup a 375 px (DESIGN.md §4.4 prohíbe copiarlo). Dos reglas mínimas, solo a
// ese ancho: (1) la columna de 1fr del mockup (≤860 px) crece con el contenido ancho de la tabla; (2) el
// .sr-only de la cabecera de tabla escapa a la página porque .table-wrap no es ancestro posicionado.
const ANCHO_DESBORDE = 375;
const CSS_DESBORDE_MOCK = [
  { id: 'desbordamiento-grid-main-side', css: '.grid-main-side { grid-template-columns: minmax(0, 1fr); }' },
  { id: 'desbordamiento-table-wrap', css: '.table-wrap { position: relative; }' },
];

async function vistaMockup(navegador, vista, tema, ancho) {
  const contexto = await navegador.newContext({ viewport: { width: ancho, height: 800 }, deviceScaleFactor: 1 });
  const pagina = await contexto.newPage();
  await pagina.goto(pathToFileURL(MOCKUP).href);
  const ocultar = OCULTAR_MOCK.filter((o) => o.vista === vista.id);
  const css = ancho === ANCHO_DESBORDE ? CSS_DESBORDE_MOCK : [];
  for (const c of css) await pagina.addStyleTag({ content: c.css });
  // Un selector que no aparece en el mockup es un error: la neutralización no puede quedar en silencio.
  await pagina.evaluate(
    ({ mock, tema, demo, ocultar }) => {
      document.documentElement.setAttribute('data-theme', tema);
      showView(mock);
      if (demo) applyRunDemo(demo);
      for (const o of ocultar) {
        const el = document.querySelector(o.sel);
        if (!el) throw new Error(`neutralización ${o.id}: no hay «${o.sel}» en el mockup`);
        el.style.setProperty('display', 'none', 'important');
      }
    },
    { mock: vista.mock, tema, demo: vista.demoMock, ocultar },
  );
  const neutralizaciones = [
    ...ocultar.map((o) => ({ id: o.id, tipo: 'ocultar', sel: o.sel })),
    ...css.map((c) => ({ id: c.id, tipo: 'css', regla: c.css })),
  ];
  await pagina.waitForTimeout(300);
  const medida = await pagina.evaluate(medirEnPagina, { anclas: ANCLAS, estructuraSel: ESTRUCTURA_SEL });
  // Se compara el ancho del viewport (lo que ve el usuario sin desplazarse en horizontal). El desbordamiento
  // se informa aparte: el mockup a 375 px sobrepasa su ancho y, sin recorte, la captura sería de otro tamaño.
  const png = await pagina.screenshot({
    fullPage: true,
    animations: 'disabled',
    clip: { x: 0, y: 0, width: ancho, height: medida.alto },
  });
  await contexto.close();
  return { ...medida, png, neutralizaciones };
}

async function main() {
  const control = OPCIONES.control;
  // En control no hay dashboard: el «lado app» es el mockup otra vez (CA-5). Así no se construye ni se sirve nada.
  let srv = null;
  let base = null;
  if (!control) {
    construir();
    srv = await servir();
    base = `http://127.0.0.1:${srv.address().port}`;
  }
  const navegador = await chromium.launch();
  const informe = [];
  fs.mkdirSync(CAPTURAS, { recursive: true });
  try {
    for (const vista of VISTAS) {
      for (const tema of TEMAS) {
        for (const ancho of ANCHOS) {
          const etiqueta = `${vista.id}-${tema}-${ancho}`;
          const app = control
            ? await vistaMockup(navegador, vista, tema, ancho)
            : await vistaApp(navegador, base, vista, tema, ancho);
          const mock = await vistaMockup(navegador, vista, tema, ancho);
          fs.writeFileSync(path.join(CAPTURAS, `app-${etiqueta}.png`), app.png);
          fs.writeFileSync(path.join(CAPTURAS, `mock-${etiqueta}.png`), mock.png);
          const estilos = compararEstilos(app, mock);
          const estructura = compararEstructura(app.estructura, mock.estructura);
          const pixeles = await compararPixeles(
            navegador,
            `data:image/png;base64,${app.png.toString('base64')}`,
            `data:image/png;base64,${mock.png.toString('base64')}`,
          );
          const pixelesOk = pixeles.porcentaje <= UMBRAL_PIXELES;
          const ok = estilos.length === 0 && estructura.length === 0 && pixelesOk;
          informe.push({
            vista: vista.id,
            tema,
            ancho,
            ok,
            desbordamientoHorizontal: { app: app.desbordamiento, mock: mock.desbordamiento },
            neutralizaciones: mock.neutralizaciones,
            estilos,
            estructura,
            pixeles: { distintos: pixeles.distintos, total: pixeles.total, porcentaje: pixeles.porcentaje, umbral: UMBRAL_PIXELES, ok: pixelesOk, tamanoApp: [pixeles.ancho[0], pixeles.alto[0]], tamanoMock: [pixeles.ancho[1], pixeles.alto[1]] },
            capturas: { app: `capturas/app-${etiqueta}.png`, mock: `capturas/mock-${etiqueta}.png` },
          });
          console.log(`${ok ? 'OK' : 'FALLA'} ${etiqueta} estilos=${estilos.length} estructura=${estructura.length} pixeles=${(pixeles.porcentaje * 100).toFixed(2)}%`);
        }
      }
    }
  } finally {
    await navegador.close();
    if (srv) srv.close();
    fs.rmSync(DIST, { recursive: true, force: true });
  }
  fs.writeFileSync(path.join(SALIDA, 'informe.json'), JSON.stringify(informe, null, 2) + '\n');
  fs.writeFileSync(path.join(SALIDA, 'informe.html'), generarHtml(informe));
  const fallos = informe.filter((x) => !x.ok).length;
  const pixelesMaximo = Math.max(...informe.map((x) => x.pixeles.porcentaje));
  console.log(`e2e:fidelidad${control ? ' --control' : ''}: ${informe.length - fallos} de ${informe.length} combinaciones OK (píxeles distintos máx. ${(pixelesMaximo * 100).toFixed(2)} %)`);
  console.log(`informe: ${path.join(SALIDA, 'informe.json')}`);
  return fallos === 0 ? 0 : 1;
}

function escapar(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]);
}

function generarHtml(informe) {
  const filas = informe
    .map((x) => {
      const lineas = [
        ...x.estilos.map((d) => `<li>estilo ${escapar(d.ancla)} · ${escapar(d.propiedad)}: app=${escapar(d.app)} mock=${escapar(d.mock)}</li>`),
        ...x.estructura.map((d) => `<li>estructura #${d.indice}: app=${escapar(d.app)} mock=${escapar(d.mock)}</li>`),
      ];
      return `<section><h2>${escapar(x.vista)} · ${escapar(x.tema)} · ${x.ancho} px · ${x.ok ? 'OK' : 'FALLA'}</h2>
<p>píxeles distintos: ${(x.pixeles.porcentaje * 100).toFixed(2)} % (umbral ${UMBRAL_PIXELES * 100} %)</p>
<ul>${lineas.join('') || '<li>sin diferencias de estilo ni de estructura</li>'}</ul>
<div style="display:flex;gap:12px;align-items:flex-start"><figure><figcaption>app</figcaption><img src="${x.capturas.app}" style="max-width:600px"></figure><figure><figcaption>mockup</figcaption><img src="${x.capturas.mock}" style="max-width:600px"></figure></div></section>`;
    })
    .join('\n');
  return `<!doctype html><meta charset="utf-8"><title>e2e:fidelidad</title><body style="font:14px sans-serif">${filas}</body>`;
}

main()
  .then((codigo) => process.exit(codigo))
  .catch((e) => {
    console.error(e);
    process.exit(1);
  });
