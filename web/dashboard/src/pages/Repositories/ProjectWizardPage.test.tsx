import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { components } from '../../api/schema.gen';
import { currentLocation, renderApp } from '../../test/renderApp';
import { server } from '../../test/server';

type Repository = components['schemas']['Repository'];
type RepositoryCredentials = components['schemas']['RepositoryCredentials'];

const repoMock: Repository = {
  repo: 'acme/shop',
  name: 'Acme Tienda',
  description: 'Servicio de compras',
  resources: ['postgresql', 'redis'],
  webhook_secret_version: 1,
  image_repo: 'ghcr.io/acme/shop',
  enabled: true,
  owner: 'ana',
  created_at: '2026-11-01T12:00:00Z',
  warm: {
    namespace: 'aqs-warm-acme-shop',
    warm_id: 'warm-shop-1',
    state: 'provisioning',
    infrastructure_ready: false,
  },
};

const credentialsMock: RepositoryCredentials = {
  repository: repoMock,
  webhook_secret: 'whsec_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef',
};

function setupAuth() {
  server.use(
    http.post('/api/auth/login', () =>
      HttpResponse.json({ token: 'tok', expires_at: '2099-01-01T00:00:00Z', session_id: 's1' }),
    ),
    http.get('/api/auth/session', () =>
      HttpResponse.json({ principal_id: 'ana', role: 'user', session_id: 's1', expires_at: '2099-01-01T00:00:00Z' }),
    ),
  );
}

async function loginAndGoToWizard() {
  const user = userEvent.setup();
  renderApp('/login?next=' + encodeURIComponent('/new'));
  await user.type(screen.getByLabelText('Usuario'), 'ana');
  await user.type(screen.getByLabelText('Contraseña'), 'clave-larga-1');
  await user.click(screen.getByRole('button', { name: 'Entrar' }));
  await waitFor(() => expect(currentLocation()).toBe('/new'));
  await screen.findByRole('heading', { level: 2, name: /1\. Información del proyecto/ });
  return user;
}

describe('ProjectWizardPage (Three-step onboarding wizard)', () => {
  afterEach(() => vi.unstubAllEnvs());

  it('muestra el stepper de cuatro hitos con estado actual, completado y pendiente', async () => {
    setupAuth();
    const user = await loginAndGoToWizard();

    expect(screen.getByTestId('wizard-step-1').getAttribute('aria-current')).toBe('step');
    expect(screen.getByTestId('wizard-step-1').textContent).toMatch(/Paso 1.*Detalles del proyecto.*En curso/i);
    expect(screen.getByTestId('wizard-step-2').textContent).toMatch(/Paso 2.*Recursos.*Pendiente/i);

    await user.type(screen.getByTestId('wizard-project-name'), 'Pasarela Pagos');
    await user.click(screen.getByTestId('wizard-contract-port'));
    await user.click(screen.getByTestId('wizard-contract-image'));
    await user.click(screen.getByTestId('wizard-contract-db'));
    await user.click(screen.getByTestId('wizard-btn-next-step1'));

    expect(screen.getByTestId('wizard-step-1').getAttribute('aria-current')).toBeNull();
    expect(screen.getByTestId('wizard-step-1').textContent).toMatch(/Completado/i);
    expect(screen.getByTestId('wizard-step-2').getAttribute('aria-current')).toBe('step');
    expect(screen.getByTestId('wizard-step-2').textContent).toMatch(/En curso/i);
  });

  it('valida campos obligatorios en el Paso 1 antes de avanzar', async () => {
    setupAuth();
    const user = await loginAndGoToWizard();

    // Intentar avanzar con formulario vacío
    const btnNext = screen.getByTestId('wizard-btn-next-step1');
    await user.click(btnNext);

    expect(screen.getByRole('alert')).toBeTruthy();
    expect(screen.getByText('El nombre del proyecto es obligatorio.')).toBeTruthy();
    expect(screen.getByText('Debes confirmar la exposición de HTTP en el puerto 8080.')).toBeTruthy();
    expect(screen.getByText('Debes confirmar la publicación de imágenes en GHCR.')).toBeTruthy();
    expect(screen.getByText('Debes confirmar el uso de DATABASE_URL y ejecución de migraciones.')).toBeTruthy();

    // Llenar campos requeridos
    await user.type(screen.getByTestId('wizard-project-name'), 'Pasarela Pagos');
    await user.click(screen.getByTestId('wizard-contract-port'));
    await user.click(screen.getByTestId('wizard-contract-image'));
    await user.click(screen.getByTestId('wizard-contract-db'));

    await user.click(btnNext);

    // Debe avanzar al paso 2
    await screen.findByRole('heading', { level: 2, name: /2\. Recursos de infraestructura/ });
  });

  it('permite seleccionar recursos en el Paso 2 con MySQL/MongoDB deshabilitados', async () => {
    setupAuth();
    const user = await loginAndGoToWizard();

    // Completar paso 1
    await user.type(screen.getByTestId('wizard-project-name'), 'Pasarela Pagos');
    await user.click(screen.getByTestId('wizard-contract-port'));
    await user.click(screen.getByTestId('wizard-contract-image'));
    await user.click(screen.getByTestId('wizard-contract-db'));
    await user.click(screen.getByTestId('wizard-btn-next-step1'));

    await screen.findByRole('heading', { level: 2, name: /2\. Recursos de infraestructura/ });

    const checkPg = screen.getByTestId('res-postgresql') as HTMLInputElement;
    const checkRedis = screen.getByTestId('res-redis') as HTMLInputElement;

    expect(checkPg.checked).toBe(true);
    expect(checkRedis.checked).toBe(true);

    // Recursos ilustrativos deshabilitados con texto
    expect(screen.getByText('MySQL 8.4')).toBeTruthy();
    expect(screen.getByText('MongoDB 7.0')).toBeTruthy();
    expect(screen.getAllByText('Próximamente').length).toBe(4);
    expect(screen.getByText('Memcached')).toBeTruthy();
    expect(screen.getByText('Valkey')).toBeTruthy();
    expect(screen.getByRole('heading', { level: 3, name: 'Bases de datos' })).toBeTruthy();
    expect(screen.getByRole('heading', { level: 3, name: 'Caché y mensajería' })).toBeTruthy();
    expect(screen.getByTestId('resource-logo-postgresql').getAttribute('src')).toBe('/logos/postgresql.svg');
    expect(screen.getByTestId('resource-logo-redis').getAttribute('src')).toBe('/logos/redis.svg');
    expect(screen.getByTestId('resource-logo-mysql').getAttribute('src')).toBe('/logos/mysql.svg');
    expect(screen.getByTestId('resource-logo-mongodb').getAttribute('src')).toBe('/logos/mongodb.svg');

    // Desmarcar Redis (dejar solo PostgreSQL)
    await user.click(checkRedis);
    expect(checkRedis.checked).toBe(false);

    // Avanzar al paso 3
    await user.click(screen.getByTestId('wizard-btn-next-step2'));
    await screen.findByRole('heading', { level: 2, name: /3\. Repositorio GitHub/ });
  });

  it('valida formato de repositorio en el Paso 3 y realiza envío único', async () => {
    setupAuth();
    let postBody: Record<string, unknown> | null = null;
    server.use(
      http.post('/api/repositories', async ({ request }) => {
        postBody = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json(credentialsMock, { status: 201 });
      }),
      http.get('/api/repositories/acme/shop', () => HttpResponse.json(repoMock)),
      http.get('/api/repositories/acme/shop/activity', () =>
        HttpResponse.json({ repo: 'acme/shop', has_webhook: false }),
      ),
    );

    const user = await loginAndGoToWizard();

    // Paso 1
    await user.type(screen.getByTestId('wizard-project-name'), 'Pasarela Pagos');
    await user.type(screen.getByTestId('wizard-project-desc'), 'Servicio core');
    await user.click(screen.getByTestId('wizard-contract-port'));
    await user.click(screen.getByTestId('wizard-contract-image'));
    await user.click(screen.getByTestId('wizard-contract-db'));
    await user.click(screen.getByTestId('wizard-btn-next-step1'));

    // Paso 2
    await screen.findByRole('heading', { level: 2, name: /2\. Recursos de infraestructura/ });
    await user.click(screen.getByTestId('wizard-btn-next-step2'));

    // Paso 3
    await screen.findByRole('heading', { level: 2, name: /3\. Repositorio GitHub/ });

    const inputRepo = screen.getByTestId('wizard-repo-name');
    const inputImage = screen.getByTestId('wizard-image-repo') as HTMLInputElement;
    const btnSubmit = screen.getByTestId('wizard-btn-submit') as HTMLButtonElement;

    // Inicialmente submit deshabilitado por falta de repo
    expect(btnSubmit.disabled).toBe(true);

    // Escribir repo
    await user.type(inputRepo, 'acme/shop');
    expect(inputImage.value).toBe('ghcr.io/acme/shop');
    expect(btnSubmit.disabled).toBe(false);

    // Enviar formulario
    await user.click(btnSubmit);

    await waitFor(() => {
      expect(postBody).toEqual({
        name: 'Pasarela Pagos',
        description: 'Servicio core',
        repo: 'acme/shop',
        image_repo: 'ghcr.io/acme/shop',
        resources: ['postgresql', 'redis'],
      });
    });

    // Debe mostrar paso 4
    await screen.findByRole('heading', { level: 2, name: /4\. Configuración del Webhook/ });
  });

  it('muestra un error accionable si un control plane antiguo no entrega credenciales', async () => {
    setupAuth();
    server.use(http.post('/api/repositories', () => HttpResponse.json(repoMock, { status: 201 })));
    const user = await loginAndGoToWizard();

    await user.type(screen.getByTestId('wizard-project-name'), 'Pasarela Pagos');
    await user.click(screen.getByTestId('wizard-contract-port'));
    await user.click(screen.getByTestId('wizard-contract-image'));
    await user.click(screen.getByTestId('wizard-contract-db'));
    await user.click(screen.getByTestId('wizard-btn-next-step1'));
    await user.click(screen.getByTestId('wizard-btn-next-step2'));
    await user.type(screen.getByTestId('wizard-repo-name'), 'acme/shop');
    await user.click(screen.getByTestId('wizard-btn-submit'));

    expect(await screen.findByText(/no se puede completar sin sus credenciales/i)).toBeTruthy();
    expect(screen.queryByRole('heading', { level: 2, name: /4\. Configuración del Webhook/ })).toBeNull();
  });

  it('explica el desajuste de contrato cuando el control plane rechaza los campos de Proyecto', async () => {
    setupAuth();
    server.use(
      http.post('/api/repositories', () =>
        HttpResponse.json({ code: 'bad_request', message: 'JSON inválido' }, { status: 400 }),
      ),
    );
    const user = await loginAndGoToWizard();

    await user.type(screen.getByTestId('wizard-project-name'), 'Pasarela Pagos');
    await user.click(screen.getByTestId('wizard-contract-port'));
    await user.click(screen.getByTestId('wizard-contract-image'));
    await user.click(screen.getByTestId('wizard-contract-db'));
    await user.click(screen.getByTestId('wizard-btn-next-step1'));
    await user.click(screen.getByTestId('wizard-btn-next-step2'));
    await user.type(screen.getByTestId('wizard-repo-name'), 'acme/shop');
    await user.click(screen.getByTestId('wizard-btn-submit'));

    expect(await screen.findByText(/no reconoce los campos de Proyecto/i)).toBeTruthy();
    expect(screen.getByText(/Referencia para soporte:/i)).toBeTruthy();
    expect(screen.queryByRole('heading', { level: 2, name: /4\. Configuración del Webhook/ })).toBeNull();
  });

  it('muestra credenciales en el Paso 4, permite copiar, rotar y completa con éxito al verificar', async () => {
    setupAuth();

    let secretVersion = 1;
    server.use(
      http.post('/api/repositories', () => HttpResponse.json(credentialsMock, { status: 201 })),
      http.post('/api/repositories/acme/shop/webhook-secret/rotate', () => {
        secretVersion += 1;
        return HttpResponse.json({
          repository: { ...repoMock, webhook_secret_version: secretVersion },
          webhook_secret: 'whsec_rotated_9999999999999999999999999999999999999999999999999999999999999999',
        });
      }),
      // Polling que inicia no listo y luego transiciona a listo
      http.get('/api/repositories/acme/shop', () =>
        HttpResponse.json({
          ...repoMock,
          warm: {
            ...repoMock.warm,
            state: 'ready',
            infrastructure_ready: true,
          },
        }),
      ),
      http.get('/api/repositories/acme/shop/activity', () =>
        HttpResponse.json({
          repo: 'acme/shop',
          has_webhook: true,
          last_event: 'ping',
          last_seen_at: new Date().toISOString(),
        }),
      ),
    );

    const user = await loginAndGoToWizard();

    // Completar pasos 1, 2, 3
    await user.type(screen.getByTestId('wizard-project-name'), 'Pasarela');
    await user.click(screen.getByTestId('wizard-contract-port'));
    await user.click(screen.getByTestId('wizard-contract-image'));
    await user.click(screen.getByTestId('wizard-contract-db'));
    await user.click(screen.getByTestId('wizard-btn-next-step1'));

    await screen.findByRole('heading', { level: 2, name: /2\. Recursos de infraestructura/ });
    await user.click(screen.getByTestId('wizard-btn-next-step2'));

    await screen.findByRole('heading', { level: 2, name: /3\. Repositorio GitHub/ });
    await user.type(screen.getByTestId('wizard-repo-name'), 'acme/shop');
    await user.click(screen.getByTestId('wizard-btn-submit'));

    // Paso 4
    await screen.findByRole('heading', { level: 2, name: /4\. Configuración del Webhook/ });

    // Copiar URL y secreto
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    });

    const btnCopyUrl = screen.getByTestId('wizard-btn-copy-url');
    await user.click(btnCopyUrl);
    expect(writeText).toHaveBeenCalled();

    const btnCopySecret = screen.getByTestId('wizard-btn-copy-secret');
    await user.click(btnCopySecret);
    expect(writeText).toHaveBeenCalledWith(credentialsMock.webhook_secret);

    // Rotar secreto
    const btnRotate = screen.getByTestId('wizard-btn-rotate-secret');
    await user.click(btnRotate);

    await waitFor(() => {
      const secretInput = screen.getByTestId('wizard-webhook-secret') as HTMLInputElement;
      expect(secretInput.value).toContain('whsec_rotated_');
      expect(screen.getByText(/Nuevo secreto generado con éxito/)).toBeTruthy();
    });

    // Verificación en vivo (polling termina en completado)
    await waitFor(() => {
      expect(screen.getByTestId('wizard-success-banner')).toBeTruthy();
      expect(screen.getByTestId('badge-infra-ready')).toBeTruthy();
      expect(screen.getByTestId('badge-webhook-ready')).toBeTruthy();
    });

    // Enlaces de navegación final
    const inboxLink = screen.getByTestId('wizard-link-inbox');
    expect(inboxLink.getAttribute('href')).toBe('/inbox?project=acme%2Fshop');

    const reposLink = screen.getByTestId('wizard-link-repos');
    expect(reposLink.getAttribute('href')).toBe('/inbox');
  });
});
