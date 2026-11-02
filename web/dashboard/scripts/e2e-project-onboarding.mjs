// e2e-project-onboarding.mjs
// Verification of the Project Onboarding scenario (Step 8 of AI-DLC plan)
// Viewports: 1440x900 (desktop), 390x844 (mobile standard), 375x667 (mobile compact)
// Themes: light, dark
import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import http from 'node:http';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright';

const DASHBOARD = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const DIST = fs.mkdtempSync(path.join(os.tmpdir(), 'aqs-onboarding-dist-'));

const VIEWPORTS = [
  { name: 'desktop-1440', width: 1440, height: 900 },
  { name: 'mobile-390', width: 390, height: 844 },
  { name: 'mobile-375', width: 375, height: 667 },
];

const THEMES = ['light', 'dark'];

function construir() {
  console.log('== Construyendo producción en', DIST, '==');
  const r = spawnSync('npx', ['vite', 'build', '--outDir', DIST, '--emptyOutDir'], {
    cwd: DASHBOARD,
    stdio: 'inherit',
  });
  if (r.status !== 0) throw new Error('vite build falló');
}

function servir() {
  const tipos = {
    '.html': 'text/html',
    '.js': 'text/javascript',
    '.css': 'text/css',
    '.svg': 'image/svg+xml',
    '.png': 'image/png',
  };
  return new Promise((resolver) => {
    const srv = http.createServer((req, res) => {
      const url = decodeURIComponent((req.url || '/').split('?')[0]);
      let archivo = path.join(DIST, url);
      if (!fs.existsSync(archivo) || fs.statSync(archivo).isDirectory()) {
        archivo = path.join(DIST, 'index.html');
      }
      res.writeHead(200, {
        'Content-Type': tipos[path.extname(archivo)] || 'application/octet-stream',
      });
      fs.createReadStream(archivo).pipe(res);
    });
    srv.listen(0, '127.0.0.1', () => resolver(srv));
  });
}

async function runScenario(browser, baseUrl, viewport, theme) {
  const label = `[${viewport.name}][${theme}]`;
  console.log(`${label} Iniciando verificación de escenario...`);

  const consoleErrors = [];
  const pageErrors = [];

  const context = await browser.newContext({
    viewport: { width: viewport.width, height: viewport.height },
    deviceScaleFactor: 1,
    reducedMotion: 'reduce',
  });

  await context.addInitScript((t) => {
    window.localStorage.setItem('aqs-tema', t);
  }, theme);

  // Mock API states
  let repoCreated = false;
  let secretRotated = false;
  let pollCount = 0;

  const mockRepos = [
    {
      repo: 'acme/portal-pagos',
      name: 'Portal de Pagos',
      description: 'Pasarela de pagos transaccional',
      resources: ['postgresql', 'redis'],
      webhook_secret_version: 1,
      image_repo: 'ghcr.io/acme/portal-pagos',
      enabled: true,
      owner: 'operador-demo',
      created_at: '2026-10-25T10:00:00Z',
      warm: {
        namespace: 'aqs-w-acme-portal-pagos',
        warm_id: 'w-acme-portal-pagos',
        state: 'ready',
        infrastructure_ready: true,
      },
    },
    {
      repo: 'acme/auth-service',
      name: 'Servicio de Autenticación',
      description: 'Microservicio stateless',
      resources: [],
      webhook_secret_version: 0,
      image_repo: 'ghcr.io/acme/auth-service',
      enabled: true,
      owner: 'operador-demo',
      created_at: '2026-10-26T12:00:00Z',
      warm: {
        namespace: 'aqs-w-acme-auth-service',
        warm_id: 'w-acme-auth-service',
        state: 'ready',
        infrastructure_ready: true,
      },
    },
  ];

  const mockNotifications = [
    {
      id: 'notif-1',
      repo: 'acme/portal-pagos',
      github_event: 'commit',
      sha: 'abc1234',
      state: 'pending',
      created_at: new Date().toISOString(),
      artifact: { kind: 'published-image', ref: 'ghcr.io/acme/portal-pagos:abc1234' },
      risk_level: 'medium',
      impact_score: 50,
      reason: 'Cambio en pasarela',
    },
    {
      id: 'notif-2',
      repo: 'acme/auth-service',
      github_event: 'commit',
      sha: 'def5678',
      state: 'confirmed',
      created_at: new Date().toISOString(),
      artifact: { kind: 'published-image', ref: 'ghcr.io/acme/auth-service:def5678' },
      risk_level: 'low',
      impact_score: 20,
      reason: 'Ajuste de tokens',
    },
  ];

  await context.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    const p = url.pathname;
    const method = route.request().method();
    const json = (data, status = 200) =>
      route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(data) });

    if (p === '/api/auth/login') {
      return json({
        token: 'token-onboarding-test',
        expires_at: new Date(Date.now() + 3600e3).toISOString(),
      });
    }
    if (p === '/api/auth/session') {
      return json({
        principal_id: 'operador-demo',
        role: 'admin',
        expires_at: new Date(Date.now() + 3600e3).toISOString(),
      });
    }
    if (p === '/api/warm') {
      const proj = url.searchParams.get('project');
      return json({
        state: 'ready',
        phase: 'idle',
        healthy: true,
        infrastructure_ready: true,
        project: proj || 'acme/portal-pagos',
        namespace: proj ? `aqs-w-${proj.replace('/', '-')}` : 'aqs-w-acme-portal-pagos',
      });
    }
    if (p === '/api/notifications') {
      const state = url.searchParams.get('state');
      let items = mockNotifications;
      if (state) items = items.filter((n) => n.state === state);
      return json(items);
    }
    if (p === '/api/confirmations') {
      return json([]);
    }
    if (p === '/api/runs') {
      return json([]);
    }
    if (p === '/api/repositories' && method === 'GET') {
      const list = [...mockRepos];
      if (repoCreated) {
        list.push({
          repo: 'acme/catalogo-productos',
          name: 'Catálogo de Productos',
          description: 'Microservicio de catálogo con Postgres y Redis',
          resources: ['postgresql', 'redis'],
          webhook_secret_version: secretRotated ? 2 : 1,
          image_repo: 'ghcr.io/acme/catalogo-productos',
          enabled: true,
          owner: 'operador-demo',
          created_at: new Date().toISOString(),
          warm: {
            namespace: 'aqs-w-acme-catalogo-productos',
            warm_id: 'w-acme-catalogo-productos',
            state: pollCount >= 2 ? 'ready' : 'provisioning',
            infrastructure_ready: pollCount >= 2,
          },
        });
      }
      return json(list);
    }
    if (p === '/api/repositories' && method === 'POST') {
      repoCreated = true;
      const body = JSON.parse(route.request().postData() || '{}');
      return json(
        {
          repository: {
            repo: body.repo || `${body.owner}/${body.repo}`,
            name: body.name,
            description: body.description,
            resources: body.resources,
            webhook_secret_version: 1,
            image_repo: body.image_repo,
            enabled: true,
            owner: 'operador-demo',
            created_at: new Date().toISOString(),
            warm: {
              namespace: `aqs-w-acme-catalogo-productos`,
              warm_id: `w-acme-catalogo-productos`,
              state: 'provisioning',
              infrastructure_ready: false,
            },
          },
          webhook_secret:
            'whsec_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef',
        },
        201,
      );
    }
    if (p === '/api/repositories/acme/catalogo-productos' && method === 'GET') {
      pollCount++;
      return json({
        repo: 'acme/catalogo-productos',
        name: 'Catálogo de Productos',
        resources: ['postgresql', 'redis'],
        webhook_secret_version: secretRotated ? 2 : 1,
        image_repo: 'ghcr.io/acme/catalogo-productos',
        enabled: true,
        owner: 'operador-demo',
        created_at: new Date().toISOString(),
        warm: {
          namespace: 'aqs-w-acme-catalogo-productos',
          warm_id: 'w-acme-catalogo-productos',
          state: pollCount >= 2 ? 'ready' : 'provisioning',
          infrastructure_ready: pollCount >= 2,
        },
      });
    }
    if (p === '/api/repositories/acme/catalogo-productos/activity' && method === 'GET') {
      return json({
        repo: 'acme/catalogo-productos',
        has_webhook: pollCount >= 2,
        last_event: pollCount >= 2 ? 'ping' : undefined,
        last_seen_at: pollCount >= 2 ? new Date().toISOString() : undefined,
      });
    }
    if (
      p === '/api/repositories/acme/catalogo-productos/webhook-secret/rotate' &&
      method === 'POST'
    ) {
      secretRotated = true;
      return json({
        repository: {
          repo: 'acme/catalogo-productos',
          name: 'Catálogo de Productos',
          resources: ['postgresql', 'redis'],
          webhook_secret_version: 2,
          image_repo: 'ghcr.io/acme/catalogo-productos',
          enabled: true,
          owner: 'operador-demo',
          created_at: new Date().toISOString(),
          warm: {
            namespace: 'aqs-w-acme-catalogo-productos',
            warm_id: 'w-acme-catalogo-productos',
            state: pollCount >= 2 ? 'ready' : 'provisioning',
            infrastructure_ready: pollCount >= 2,
          },
        },
        webhook_secret:
          'whsec_fedcba0123456789fedcba0123456789fedcba0123456789fedcba0123456789',
      });
    }
    return json({ code: 'not_found', message: p }, 404);
  });

  const page = await context.newPage();

  page.on('console', (msg) => {
    if (msg.type() === 'error') {
      consoleErrors.push(msg.text());
    }
  });
  page.on('pageerror', (err) => {
    pageErrors.push(err.message);
  });

  // Login
  await page.goto(`${baseUrl}/login`);
  await page.waitForSelector('#login-usuario');
  await page.fill('#login-usuario', 'operador-demo');
  await page.fill('#login-clave', 'clave-demo');
  await page.getByRole('button', { name: 'Entrar' }).click();
  await page.waitForURL(`${baseUrl}/inbox`);

  // Verify Inbox & Selector
  console.log(`${label} Verificando Inbox y Selector de proyectos...`);
  await page.waitForSelector('#inbox-project-selector');
  const selector = page.locator('#inbox-project-selector');
  await selector.waitFor({ state: 'visible' });

  // Verify label
  const labelText = await page.locator('label[for="inbox-project-selector"]').textContent();
  if (!labelText || !labelText.includes('Proyecto:')) {
    throw new Error(`${label} Etiqueta de selector no encontrada o incorrecta: ${labelText}`);
  }

  // Verify options
  const options = await selector.locator('option').allTextContents();
  if (!options.some((o) => o.includes('Todos los proyectos'))) {
    throw new Error(`${label} Falta opción 'Todos los proyectos'`);
  }
  if (!options.some((o) => o.includes('Portal de Pagos'))) {
    throw new Error(`${label} Falta opción 'Portal de Pagos'`);
  }
  if (!options.some((o) => o.includes('Crear nuevo proyecto'))) {
    throw new Error(`${label} Falta opción '+ Crear nuevo proyecto'`);
  }

  // Filter by project
  await selector.selectOption('acme/portal-pagos');
  await page.waitForURL(/project=acme%2Fportal-pagos/);

  // Check overflow
  const overflow1 = await page.evaluate(
    () => document.documentElement.scrollWidth - window.innerWidth,
  );
  if (overflow1 > 1) {
    const overflowing = await page.evaluate(() => {
      const docW = window.innerWidth;
      const elements = Array.from(document.querySelectorAll('*'));
      return elements
        .filter((el) => el.getBoundingClientRect().right > docW + 1)
        .map((el) => ({
          tag: el.tagName,
          className: el.className,
          id: el.id,
          right: el.getBoundingClientRect().right,
          width: el.getBoundingClientRect().width,
        }));
    });
    console.log(`${label} Elementos desbordados:`, overflowing);
    throw new Error(`${label} Desbordamiento horizontal detectado en inbox: ${overflow1}px`);
  }

  // Select add new project option -> should navigate to /new
  await selector.selectOption('__new__');
  await page.waitForURL(`${baseUrl}/new`);

  // Check Wizard Step 1
  console.log(`${label} Verificando Wizard Step 1 (Detalles y Contrato)...`);
  await page.waitForSelector('#step-1-title');

  // Attempt to proceed without name -> should trigger validation
  const nextBtnStep1 = page.locator('[data-testid="wizard-btn-next-step1"]');
  await nextBtnStep1.click();
  const alertStep1 = page.locator('div[role="alert"].alert-danger');
  await alertStep1.waitFor({ state: 'visible' });
  const step1Error = await alertStep1.textContent();
  if (!step1Error || !step1Error.includes('El nombre del proyecto es obligatorio')) {
    throw new Error(`${label} Error esperado de validación en Paso 1 no visible: ${step1Error}`);
  }

  // Fill project details
  await page.fill('[data-testid="wizard-project-name"]', 'Catálogo de Productos');
  await page.fill('[data-testid="wizard-project-desc"]', 'Microservicio de catálogo con Postgres y Redis');

  // Check contract checkboxes
  await page.locator('[data-testid="wizard-contract-port"]').click();
  await page.locator('[data-testid="wizard-contract-image"]').click();
  await page.locator('[data-testid="wizard-contract-db"]').click();

  // Advance to Step 2
  await nextBtnStep1.click();
  await page.waitForSelector('#step-2-title');

  // Verify Step 2 Resources
  console.log(`${label} Verificando Wizard Step 2 (Recursos)...`);
  const pgCheckbox = page.locator('[data-testid="res-postgresql"]');
  const redisCheckbox = page.locator('[data-testid="res-redis"]');

  // PG and Redis should be enabled and checked by default
  if (!(await pgCheckbox.isChecked())) throw new Error(`${label} Postgres debería estar marcado por defecto`);
  if (!(await redisCheckbox.isChecked())) throw new Error(`${label} Redis debería estar marcado por defecto`);

  // MySQL and MongoDB disabled check
  const disabledCards = page.locator('.resource-card.disabled');
  const disabledCount = await disabledCards.count();
  if (disabledCount < 2) throw new Error(`${label} Debería haber 2 tarjetas deshabilitadas`);

  // Verify "Próximamente" badge
  const proxBadges = await page.locator('.badge-neutral').allTextContents();
  if (!proxBadges.some((t) => t.includes('Próximamente'))) {
    throw new Error(`${label} Badge Próximamente no encontrado en recursos`);
  }

  // Test toggling Redis
  await redisCheckbox.click();
  if (await redisCheckbox.isChecked()) throw new Error(`${label} Redis no se desmarcó correctamente`);
  await redisCheckbox.click();

  // Check overflow on step 2
  const overflow2 = await page.evaluate(
    () => document.documentElement.scrollWidth - window.innerWidth,
  );
  if (overflow2 > 1) {
    throw new Error(`${label} Desbordamiento horizontal detectado en Step 2: ${overflow2}px`);
  }

  // Advance to Step 3
  const nextBtnStep2 = page.locator('[data-testid="wizard-btn-next-step2"]');
  await nextBtnStep2.click();
  await page.waitForSelector('#step-3-title');

  // Verify Step 3 Repository form
  console.log(`${label} Verificando Wizard Step 3 (Repositorio y Envío)...`);
  const repoInput = page.locator('[data-testid="wizard-repo-name"]');
  const submitBtn = page.locator('[data-testid="wizard-btn-submit"]');

  // Button should be disabled initially
  if (!(await submitBtn.isDisabled())) {
    throw new Error(`${label} Botón de envío debería estar deshabilitado con campo vacío`);
  }

  // Valid repository
  await repoInput.fill('acme/catalogo-productos');
  const imagePrefix = await page.locator('[data-testid="wizard-image-repo"]').inputValue();
  if (!imagePrefix.includes('ghcr.io/acme/catalogo-productos')) {
    throw new Error(`${label} Prefijo de imagen no prellenado correctamente: ${imagePrefix}`);
  }

  // Button should now be enabled
  if (await submitBtn.isDisabled()) {
    throw new Error(`${label} Botón de envío debería estar habilitado tras ingresar repo válido`);
  }

  // Submit project creation
  await submitBtn.click();
  await page.waitForSelector('#step-4-title');

  // Verify Step 4 Credentials and Live Status
  console.log(`${label} Verificando Wizard Step 4 (Credenciales y Verificación en Vivo)...`);
  const secretInput = page.locator('[data-testid="wizard-webhook-secret"]');
  const secretValue = await secretInput.inputValue();
  if (!secretValue.startsWith('whsec_')) {
    throw new Error(`${label} Secreto derivado no visible en el campo: ${secretValue}`);
  }

  // Test Copy buttons
  await page.locator('[data-testid="wizard-btn-copy-url"]').click();
  const copyUrlNotice = await page.locator('[data-testid="wizard-btn-copy-url"]').textContent();
  if (!copyUrlNotice.includes('Copiado')) {
    throw new Error(`${label} Feedback de copia de URL no mostrado`);
  }

  await page.locator('[data-testid="wizard-btn-copy-secret"]').click();
  const copySecNotice = await page.locator('[data-testid="wizard-btn-copy-secret"]').textContent();
  if (!copySecNotice.includes('Copiado')) {
    throw new Error(`${label} Feedback de copia de Secreto no mostrado`);
  }

  // Test Rotate Secret
  const rotateBtn = page.locator('[data-testid="wizard-btn-rotate-secret"]');
  await rotateBtn.click();
  await page.waitForFunction(
    (initialSecret) => {
      const el = document.querySelector('[data-testid="wizard-webhook-secret"]');
      return el && el.value !== initialSecret;
    },
    secretValue,
  );
  const rotatedSecretValue = await secretInput.inputValue();
  if (!rotatedSecretValue.startsWith('whsec_fedcba')) {
    throw new Error(`${label} Secreto rotado no actualizado: ${rotatedSecretValue}`);
  }

  // Wait for polling to observe readiness
  console.log(`${label} Esperando finalización del sondeo en vivo...`);
  await page.waitForSelector('[data-testid="wizard-success-banner"]', { timeout: 10000 });
  const successTitle = await page.locator('[data-testid="wizard-success-banner"] h3').textContent();
  if (!successTitle || !successTitle.includes('verificado exitosamente')) {
    throw new Error(`${label} Banner de éxito no mostrado: ${successTitle}`);
  }

  // Check links in success banner
  const linkInbox = page.locator('[data-testid="wizard-link-inbox"]');
  if ((await linkInbox.count()) === 0) {
    throw new Error(`${label} Enlace a inbox filtrado no encontrado en banner de éxito`);
  }

  // Check final overflow on step 4
  const overflow4 = await page.evaluate(
    () => document.documentElement.scrollWidth - window.innerWidth,
  );
  if (overflow4 > 1) {
    throw new Error(`${label} Desbordamiento horizontal detectado en Step 4: ${overflow4}px`);
  }

  // Check errors
  if (consoleErrors.length > 0) {
    console.warn(`${label} Advertencia - errores de consola:`, consoleErrors);
  }
  if (pageErrors.length > 0) {
    throw new Error(`${label} Errores de página no capturados: ${pageErrors.join(', ')}`);
  }

  await context.close();
  console.log(`${label} ✅ Todas las verificaciones pasaron sin errores ni desbordamientos.`);
}

async function main() {
  construir();
  const servidor = await servir();
  const puerto = servidor.address().port;
  const baseUrl = `http://127.0.0.1:${puerto}`;
  console.log('Servidor de prueba escuchando en', baseUrl);

  const browser = await chromium.launch({ headless: true });

  try {
    for (const viewport of VIEWPORTS) {
      for (const theme of THEMES) {
        await runScenario(browser, baseUrl, viewport, theme);
      }
    }
    console.log('\n======================================================');
    console.log('🎉 TODAS LAS PRUEBAS DE NAVEGADOR (6 combinaciones) EXITOSAS');
    console.log('======================================================\n');
  } finally {
    await browser.close();
    servidor.close();
    try {
      fs.rmSync(DIST, { recursive: true, force: true });
    } catch {
      // ignore
    }
  }
}

main().catch((err) => {
  console.error('\n❌ Error en verificación de navegador:', err);
  process.exit(1);
});
