// U10-T03 (CA-4): capturas lado a lado de la vista de corrida, app frente a mockup.
// Uso (desde la raíz del worktree, tras `npm ci` en web/dashboard):
//   PLAYWRIGHT_PATH=<ruta-al-paquete-playwright> node bitacoras/U10-T03-capturas/capturar.cjs
// Sin PLAYWRIGHT_PATH se usa el paquete «playwright» resoluble desde este script.
// Construye el dashboard en un directorio temporal, lo sirve en local y simula la API con page.route.
const fs = require('fs');
const os = require('os');
const path = require('path');
const http = require('http');
const { spawnSync } = require('child_process');

const { chromium } = require(process.env.PLAYWRIGHT_PATH || 'playwright');

const RAIZ = path.resolve(__dirname, '../..');
const DASHBOARD = path.join(RAIZ, 'web/dashboard');
const SALIDA = __dirname;
const DIST = path.join(os.tmpdir(), 'u10-t03-dist');
const MOCKUP = path.join(RAIZ, 'mockups/swarm-mock.html');
const CONTRATOS = path.join(RAIZ, 'contracts/plans/examples/valid');
const ID = 'run-0123456789abcdef';

const leerJson = (f) => JSON.parse(fs.readFileSync(path.join(CONTRATOS, f), 'utf8'));
const INFORME_BUG = { ...leerJson('report.dos-hallazgos.json'), run_id: ID };
const INFORME_LIMPIO = { ...leerJson('report.sin-hallazgos.json'), run_id: ID };
const PLAN = { ...leerJson('flow-plan.body-y-sondeo.json'), run_id: ID };

// Estados de la app (API simulada) y su equivalente en el selector «Previsualizar estado» del mockup.
const ESTADOS = {
  bug: { mock: 'bug', run: { state: 'done', verdict: 'bug' }, informe: INFORME_BUG, plan: true },
  'sin-hallazgos': { mock: 'clean', run: { state: 'done', verdict: 'sin-hallazgos' }, informe: INFORME_LIMPIO, plan: true },
  'en-curso': { mock: 'running', run: { state: 'running' }, informe: null, plan: true },
  fallida: { mock: 'failed', run: { state: 'failed' }, informe: null, plan: true },
};
const TEMAS = ['light', 'dark'];
const ANCHOS = [1280, 375];

function construir() {
  const r = spawnSync('npx', ['vite', 'build', '--outDir', DIST, '--emptyOutDir'], {
    cwd: DASHBOARD,
    stdio: 'inherit',
  });
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

async function rutaApi(route, estado) {
  const req = route.request();
  const url = new URL(req.url());
  const json = (cuerpo, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(cuerpo) });
  if (url.pathname === '/api/auth/login') {
    return json({ token: 'token-captura', expires_at: new Date(Date.now() + 3600e3).toISOString() });
  }
  if (url.pathname === '/api/auth/session') return json({ principal_id: 'operador-demo', role: 'user' });
  if (url.pathname === `/api/runs/${ID}`) return json({ id: ID, trace_id: 'tr-99201a4f-8c1e-44d0-a712', ...estado.run });
  if (url.pathname === `/api/runs/${ID}/plan`) return estado.plan ? json(PLAN) : json({ code: 'plan_not_ready', message: 'x' }, 404);
  if (url.pathname === `/api/reports/${ID}`) {
    return estado.informe ? json(estado.informe) : json({ code: 'report_not_ready', message: 'x' }, 404);
  }
  return json({ code: 'not_found', message: url.pathname }, 404);
}

async function capturaApp(navegador, base, nombre, estado, tema, ancho) {
  const contexto = await navegador.newContext({ viewport: { width: ancho, height: 800 }, deviceScaleFactor: 1 });
  await contexto.addInitScript((t) => window.localStorage.setItem('aqs-tema', t), tema);
  await contexto.route('**/api/**', (route) => rutaApi(route, estado));
  const pagina = await contexto.newPage();
  await pagina.goto(`${base}/login`);
  await pagina.fill('#login-usuario', 'operador-demo');
  await pagina.fill('#login-clave', 'clave-demo');
  await pagina.getByRole('button', { name: 'Entrar' }).click();
  await pagina.waitForFunction(() => !window.location.pathname.startsWith('/login'));
  // Navegación del cliente: conserva la sesión en memoria.
  await pagina.evaluate((id) => {
    window.history.pushState({}, '', `/runs/${id}`);
    window.dispatchEvent(new PopStateEvent('popstate'));
  }, ID);
  await pagina.getByText('Línea de tiempo de la corrida').waitFor();
  await pagina.waitForTimeout(400);
  const desbordamiento = await pagina.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  const ruta = path.join(SALIDA, `app-${nombre}-${tema}-${ancho}.png`);
  await pagina.screenshot({ path: ruta, fullPage: true });
  await contexto.close();
  return { ruta, desbordamiento };
}

async function capturaMockup(navegador, nombre, estado, tema, ancho) {
  const contexto = await navegador.newContext({ viewport: { width: ancho, height: 800 }, deviceScaleFactor: 1 });
  const pagina = await contexto.newPage();
  await pagina.goto(`file://${MOCKUP}`);
  await pagina.evaluate(
    ({ modo, tema }) => {
      document.documentElement.setAttribute('data-theme', tema);
      showView('run');
      applyRunDemo(modo);
    },
    { modo: estado.mock, tema },
  );
  await pagina.waitForTimeout(400);
  const desbordamiento = await pagina.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  const seccion = pagina.locator('section[data-view="run"]');
  const ruta = path.join(SALIDA, `mock-${nombre}-${tema}-${ancho}.png`);
  await seccion.screenshot({ path: ruta });
  await contexto.close();
  return { ruta, desbordamiento };
}

async function paresLadoALado(navegador, nombre, tema, ancho, rutaApp, rutaMock) {
  const b64 = (f) => fs.readFileSync(f).toString('base64');
  const html = `<html><body style="margin:0;display:flex;gap:16px;align-items:flex-start;background:#888;font:14px sans-serif">
    <figure style="margin:0;padding:8px;background:#fff"><figcaption>app</figcaption><img src="data:image/png;base64,${b64(rutaApp)}"></figure>
    <figure style="margin:0;padding:8px;background:#fff"><figcaption>mockup</figcaption><img src="data:image/png;base64,${b64(rutaMock)}"></figure>
  </body></html>`;
  const contexto = await navegador.newContext({ viewport: { width: ancho * 2 + 80, height: 800 } });
  const pagina = await contexto.newPage();
  await pagina.setContent(html);
  await pagina.waitForTimeout(200);
  await pagina.screenshot({ path: path.join(SALIDA, `par-${nombre}-${tema}-${ancho}.png`), fullPage: true });
  await contexto.close();
}

(async () => {
  construir();
  const srv = await servir();
  const base = `http://127.0.0.1:${srv.address().port}`;
  const navegador = await chromium.launch();
  const resultados = [];
  try {
    for (const [nombre, estado] of Object.entries(ESTADOS)) {
      for (const tema of TEMAS) {
        for (const ancho of ANCHOS) {
          const app = await capturaApp(navegador, base, nombre, estado, tema, ancho);
          const mock = await capturaMockup(navegador, nombre, estado, tema, ancho);
          await paresLadoALado(navegador, nombre, tema, ancho, app.ruta, mock.ruta);
          resultados.push({ nombre, tema, ancho, desbordamientoApp: app.desbordamiento, desbordamientoMock: mock.desbordamiento });
          console.log(JSON.stringify(resultados[resultados.length - 1]));
        }
      }
    }
  } finally {
    await navegador.close();
    srv.close();
  }
  fs.writeFileSync(path.join(SALIDA, 'desbordamiento.json'), JSON.stringify(resultados, null, 2) + '\n');
})().catch((e) => {
  console.error(e);
  process.exit(1);
});
