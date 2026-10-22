// Medición U10-T04 (CA-3 y CA-4): sirve dist/ con la CSP estricta de vite.config.ts,
// simula la API, mide scroll horizontal a 375 px y captura pantallas. Sin dependencias del repo.
import { createServer } from 'node:http';
import { readFileSync, existsSync, statSync } from 'node:fs';
import { extname, join, normalize } from 'node:path';
import pwCore from '/home/omarjayg/.npm/_npx/e41f203b7505f1fb/node_modules/playwright-core/index.js';
const { chromium } = pwCore;

const DIST = process.argv[2];
const MOCKUP = process.argv[3];
const SALIDA = process.argv[4];
const RUTA_CHROME = '/home/omarjayg/.cache/ms-playwright/chromium-1234/chrome-linux64/chrome';
const CSP =
  "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
  "connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'";
const MIME = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml', '.woff2': 'font/woff2' };

const servidor = createServer((req, res) => {
  const limpia = normalize(decodeURIComponent(req.url.split('?')[0]));
  let archivo = join(DIST, limpia);
  if (!existsSync(archivo) || statSync(archivo).isDirectory()) archivo = join(DIST, 'index.html');
  res.writeHead(200, { 'Content-Type': MIME[extname(archivo)] ?? 'application/octet-stream', 'Content-Security-Policy': CSP });
  res.end(readFileSync(archivo));
});
await new Promise((r) => servidor.listen(4399, '127.0.0.1', r));
const BASE = 'http://127.0.0.1:4399';

const RECIBOS = [
  { run_id: 'run-84a1d9b4c02e11ef', notification_id: 'notif-7f4790e-01', confirmed_by: 'operador-demo', confirmed_at: '2026-10-21T20:41:12Z' },
  { run_id: 'run-3c09e12a77b40019', notification_id: 'notif-a1c9e02-02', confirmed_by: 'operador-demo', confirmed_at: '2026-10-21T18:15:04Z' },
];
const WARM = { warm_id: 'env-aqs-test-01', state: 'ready', reset_verified: false, baseline_version: 'v1.4.2-baseline' };
const SESION = { principal_id: 'operador-demo', role: 'operador', session_id: 's1', expires_at: '2099-01-01T00:00:00Z' };

const browser = await chromium.launch({ executablePath: RUTA_CHROME, args: ['--no-sandbox'] });
const resultados = {};
const errores = [];

async function contexto(ancho, alto) {
  const ctx = await browser.newContext({ viewport: { width: ancho, height: alto }, colorScheme: 'light' });
  await ctx.route('**/api/**', (ruta) => {
    const url = new URL(ruta.request().url());
    const p = url.pathname;
    const json = (body, status = 200) => ruta.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });
    if (p === '/api/auth/login') return json({ token: 'tok', expires_at: SESION.expires_at, session_id: 's1' });
    if (p === '/api/auth/session') return json(SESION);
    if (p === '/api/confirmations') return json(RECIBOS);
    if (p === '/api/warm') return json(WARM);
    return json([]);
  });
  return ctx;
}

/** Estilos computados del enlace run_id (mismo elemento en el dashboard y en el mockup). */
function estiloEnlace(el) {
  const s = getComputedStyle(el);
  return { color: s.color, fontWeight: s.fontWeight, textDecorationLine: s.textDecorationLine };
}

async function medirScroll(page) {
  return page.evaluate(() => ({
    anchoVentana: window.innerWidth,
    anchoDocumento: document.documentElement.scrollWidth,
    scrollHorizontal: document.documentElement.scrollWidth > window.innerWidth,
  }));
}

async function entrar(page) {
  await page.goto(`${BASE}/login`);
  await page.fill('#login-usuario', 'operador-demo');
  await page.fill('#login-clave', 'clave-larga-1');
  await page.click('button[type="submit"]');
  await page.waitForURL(/\/inbox/);
}

// CA-3: 375 px sin scroll horizontal en las tres vistas.
{
  const ctx = await contexto(375, 812);
  const page = await ctx.newPage();
  page.on('console', (m) => { if (m.type() === 'error' || /Content Security/i.test(m.text())) errores.push(`[375 ${m.type()}] ${m.text()}`); });
  page.on('pageerror', (e) => errores.push(`[375 pageerror] ${e.message}`));
  await page.goto(`${BASE}/login`);
  await page.waitForSelector('.login-wrap');
  resultados['375 login'] = await medirScroll(page);
  await page.screenshot({ path: `${SALIDA}/375-login.png`, fullPage: true });
  await entrar(page);
  await page.click('a.az-nav-link[href="/runs"]');
  await page.waitForSelector('table');
  resultados['375 runs'] = await medirScroll(page);
  await page.screenshot({ path: `${SALIDA}/375-runs.png`, fullPage: true });
  await page.click('a.az-nav-link[href="/warm"]');
  await page.waitForSelector('dl.kv-list');
  resultados['375 warm'] = await medirScroll(page);
  await page.screenshot({ path: `${SALIDA}/375-warm.png`, fullPage: true });
  await ctx.close();
}

// CA-4: capturas a 1280 px para comparar con el mockup.
{
  const ctx = await contexto(1280, 900);
  const page = await ctx.newPage();
  page.on('console', (m) => { if (m.type() === 'error' || /Content Security/i.test(m.text())) errores.push(`[1280 ${m.type()}] ${m.text()}`); });
  page.on('pageerror', (e) => errores.push(`[1280 pageerror] ${e.message}`));
  await page.goto(`${BASE}/login`);
  await page.waitForSelector('.login-wrap');
  await page.screenshot({ path: `${SALIDA}/1280-login.png`, fullPage: true });
  await entrar(page);
  await page.click('a.az-nav-link[href="/runs"]');
  await page.waitForSelector('table');
  resultados['1280 runs enlace run_id'] = await page.$eval('td[data-label="Corrida"] a', estiloEnlace);
  await page.screenshot({ path: `${SALIDA}/1280-runs.png`, fullPage: true });
  await page.click('a.az-nav-link[href="/warm"]');
  await page.waitForSelector('dl.kv-list');
  await page.screenshot({ path: `${SALIDA}/1280-warm.png`, fullPage: true });
  await ctx.close();
}

// Mockup: una vista por captura (las secciones están ocultas por defecto).
{
  const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const page = await ctx.newPage();
  await page.goto(`file://${MOCKUP}`);
  for (const vista of ['runs', 'warm', 'login']) {
    await page.evaluate((v) => {
      document.querySelectorAll('section.view').forEach((s) => {
        s.style.display = s.dataset.view === v ? 'block' : 'none';
      });
    }, vista);
    if (vista === 'runs') {
      resultados['mockup runs enlace run_id'] = await page.$eval('section.view[data-view="runs"] a[data-goto-run="bug"]', estiloEnlace);
    }
    await page.locator(`section.view[data-view="${vista}"]`).screenshot({ path: `${SALIDA}/mockup-1280-${vista}.png` });
  }
  await ctx.close();
}

await browser.close();
servidor.close();
console.log(JSON.stringify({ resultados, errores }, null, 2));
