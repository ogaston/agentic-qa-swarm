import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import type { components } from '../../api/schema.gen';
import { renderApp } from '../../test/renderApp';
import { server } from '../../test/server';

type Repository = components['schemas']['Repository'];
type RepositoryActivity = components['schemas']['RepositoryActivity'];

const repo1: Repository = {
  repo: 'acme/shop',
  image_repo: 'ghcr.io/acme/shop',
  enabled: true,
  owner: 'ana',
  created_at: '2026-10-01T12:00:00Z',
  warm: {
    namespace: 'aqs-warm-acme-shop',
    warm_id: 'warm-shop-1',
    state: 'ready',
    reset_verified: true,
    baseline_version: 'b1',
  },
};

const repo2: Repository = {
  repo: 'other/billing',
  image_repo: 'ghcr.io/other/billing-service',
  enabled: false,
  owner: 'carlos',
  created_at: '2026-10-02T12:00:00Z',
  warm: {
    namespace: 'aqs-warm-other-billing',
    warm_id: 'warm-bill-1',
    state: 'provisioning',
  },
};

const activity1: RepositoryActivity = {
  repo: 'acme/shop',
  has_webhook: true,
  last_event: 'push',
  last_sha: '1234567890abcdef',
  last_seen_at: '2026-10-28T10:00:00Z',
};

const activity2: RepositoryActivity = {
  repo: 'other/billing',
  has_webhook: false,
};

function setupAuthAndRepos(userRole: 'user' | 'admin' = 'user', principalId = 'ana', repoList: Repository[] = [repo1, repo2]) {
  server.use(
    http.post('/api/auth/login', () =>
      HttpResponse.json({ token: 'tok', expires_at: '2099-01-01T00:00:00Z', session_id: 's1' }),
    ),
    http.get('/api/auth/session', () =>
      HttpResponse.json({ principal_id: principalId, role: userRole, session_id: 's1', expires_at: '2099-01-01T00:00:00Z' }),
    ),
    http.get('/api/repositories', () => HttpResponse.json(repoList)),
    http.get('/api/repositories/acme/shop/activity', () => HttpResponse.json(activity1)),
    http.get('/api/repositories/other/billing/activity', () => HttpResponse.json(activity2)),
  );
}

async function loginAndNavigate() {
  const user = userEvent.setup();
  renderApp('/login');
  await user.type(screen.getByLabelText('Usuario'), 'ana');
  await user.type(screen.getByLabelText('Contraseña'), 'clave-larga-1');
  await user.click(screen.getByRole('button', { name: 'Entrar' }));
  await waitFor(() => screen.getByRole('link', { name: 'Repositorios' }));
  await user.click(screen.getByRole('link', { name: 'Repositorios' }));
  await waitFor(() => screen.getByText('Configura tus repositorios de GitHub para pruebas continuas de caja negra.'));
  return user;
}

describe('RepositoriesPage (U12 Self-Service)', () => {
  it('muestra cabecera, instrucciones de webhook y lista de repositorios', async () => {
    setupAuthAndRepos();
    await loginAndNavigate();

    expect(screen.getByRole('heading', { level: 1, name: 'Repositorios' })).toBeTruthy();
    expect(screen.getByText('Configuración de Webhooks en GitHub')).toBeTruthy();
    expect(screen.getByTestId('webhook-url').textContent).toContain('/webhooks/github');

    // Tabla con repositorios
    await waitFor(() => {
      expect(screen.getByText('acme/shop')).toBeTruthy();
      expect(screen.getByText('other/billing')).toBeTruthy();
    });

    // Estados warm
    expect(screen.getByText('Listo')).toBeTruthy();
    expect(screen.getByText('Aprovisionando')).toBeTruthy();

    // Actividades
    await waitFor(() => {
      expect(screen.getByText(/Último webhook:/)).toBeTruthy();
      expect(screen.getByTestId('no-webhook-text')).toBeTruthy();
    });
  });

  it('permite copiar la URL de webhook al portapapeles', async () => {
    setupAuthAndRepos();
    const user = await loginAndNavigate();

    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    });

    const btnCopiar = screen.getByTestId('btn-copiar-webhook-url');
    await user.click(btnCopiar);
    expect(writeText).toHaveBeenCalledWith(expect.stringContaining('/webhooks/github'));
    expect(screen.getByText('¡Copiado!')).toBeTruthy();
  });

  it('muestra mensaje amigable cuando no hay repositorios registrados', async () => {
    setupAuthAndRepos('user', 'ana', []);
    await loginAndNavigate();

    await waitFor(() => {
      expect(screen.getByText('Sin repositorios registrados aún. Añade el primero para comenzar.')).toBeTruthy();
    });
  });

  it('abre el modal de añadir repositorio, valida formato y App Contract, y crea repositorio', async () => {
    setupAuthAndRepos();

    let postLlamado = false;
    let postBody: Record<string, unknown> | null = null;
    server.use(
      http.post('/api/repositories', async ({ request }) => {
        postLlamado = true;
        postBody = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json(
          {
            repo: 'acme/new-app',
            image_repo: 'ghcr.io/acme/new-app',
            enabled: true,
            owner: 'ana',
            warm: {
              namespace: 'aqs-warm-acme-new-app',
              warm_id: 'warm-new-1',
              state: 'provisioning',
            },
          },
          { status: 201 },
        );
      }),
      http.get('/api/repositories/acme/new-app/activity', () =>
        HttpResponse.json({ repo: 'acme/new-app', has_webhook: false }),
      ),
    );

    const user = await loginAndNavigate();

    // Abrir modal
    await user.click(screen.getByTestId('btn-anadir-repo'));
    expect(screen.getByRole('dialog', { name: 'Añadir repositorio' })).toBeTruthy();

    const repoInput = screen.getByLabelText(/Repositorio/) as HTMLInputElement;
    const imageInput = screen.getByLabelText(/Prefijo de imagen/) as HTMLInputElement;
    const btnSubmit = screen.getByTestId('btn-submit-repo') as HTMLButtonElement;

    // Inicialmente deshabilitado
    expect(btnSubmit.disabled).toBe(true);

    // Escribir repo
    await user.type(repoInput, 'acme/new-app');
    expect(imageInput.value).toBe('ghcr.io/acme/new-app');

    // Marcar los 3 checkboxes del App Contract
    await user.click(screen.getByTestId('check-contract-db'));
    await user.click(screen.getByTestId('check-contract-port'));
    await user.click(screen.getByTestId('check-contract-image'));

    // Ahora habilitado
    expect(btnSubmit.disabled).toBe(false);

    // Enviar
    await user.click(btnSubmit);

    await waitFor(() => {
      expect(postLlamado).toBe(true);
      expect(postBody).toEqual({
        repo: 'acme/new-app',
        image_repo: 'ghcr.io/acme/new-app',
      });
      expect(screen.queryByRole('dialog')).toBeNull();
      expect(screen.getByText('acme/new-app')).toBeTruthy();
    });
  });

  it('permite conmutar enabled/disabled con PATCH cuando el usuario es owner o admin', async () => {
    setupAuthAndRepos('user', 'ana'); // ana es owner de acme/shop

    let patchLlamado = false;
    server.use(
      http.patch('/api/repositories/acme/shop', async ({ request }) => {
        patchLlamado = true;
        const body = (await request.json()) as { enabled: boolean };
        return HttpResponse.json({ ...repo1, enabled: body.enabled });
      }),
    );

    const user = await loginAndNavigate();

    const toggleBtn = screen.getByTestId('toggle-btn-acme-shop') as HTMLButtonElement;
    expect(toggleBtn.disabled).toBe(false);
    expect(toggleBtn.textContent).toBe('Desactivar');

    await user.click(toggleBtn);

    await waitFor(() => {
      expect(patchLlamado).toBe(true);
      expect(toggleBtn.textContent).toBe('Activar');
    });
  });

  it('deshabilita editar y eliminar para un repositorio del cual el usuario no es owner ni admin', async () => {
    setupAuthAndRepos('user', 'ana'); // ana no es owner de other/billing (owner es carlos)
    await loginAndNavigate();

    const toggleBtn = screen.getByTestId('toggle-btn-other-billing') as HTMLButtonElement;
    const deleteBtn = screen.getByTestId('delete-btn-other-billing') as HTMLButtonElement;

    expect(toggleBtn.disabled).toBe(true);
    expect(deleteBtn.disabled).toBe(true);
    expect(toggleBtn.getAttribute('title')).toContain('Solo el creador o un administrador');
    expect(deleteBtn.getAttribute('title')).toContain('Solo el creador o un administrador');
  });

  it('habilita editar y eliminar para cualquier repositorio si el usuario es admin', async () => {
    setupAuthAndRepos('admin', 'admin-user'); // admin puede editar ambos
    await loginAndNavigate();

    const toggle1 = screen.getByTestId('toggle-btn-acme-shop') as HTMLButtonElement;
    const delete1 = screen.getByTestId('delete-btn-acme-shop') as HTMLButtonElement;
    const toggle2 = screen.getByTestId('toggle-btn-other-billing') as HTMLButtonElement;
    const delete2 = screen.getByTestId('delete-btn-other-billing') as HTMLButtonElement;

    expect(toggle1.disabled).toBe(false);
    expect(delete1.disabled).toBe(false);
    expect(toggle2.disabled).toBe(false);
    expect(delete2.disabled).toBe(false);
  });

  it('abre modal de confirmación y elimina repositorio mediante DELETE', async () => {
    setupAuthAndRepos('user', 'ana');

    let deleteLlamado = false;
    server.use(
      http.delete('/api/repositories/acme/shop', () => {
        deleteLlamado = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );

    const user = await loginAndNavigate();

    // Abrir modal eliminar
    await user.click(screen.getByTestId('delete-btn-acme-shop'));
    expect(screen.getByRole('dialog', { name: 'Eliminar repositorio' })).toBeTruthy();
    expect(screen.getByText(/¿Estás seguro de que deseas eliminar el repositorio/)).toBeTruthy();

    // Confirmar
    await user.click(screen.getByTestId('btn-confirm-delete'));

    await waitFor(() => {
      expect(deleteLlamado).toBe(true);
      expect(screen.queryByRole('dialog')).toBeNull();
      expect(screen.queryByText('acme/shop')).toBeNull();
    });
  });
});
