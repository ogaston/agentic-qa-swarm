import { act, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse, type HttpHandler } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import type { components } from '../../api/schema.gen';
import { server } from '../../test/server';
import { currentLocation, renderApp } from '../../test/renderApp';

type Notification = components['schemas']['Notification'];
type ConfirmationReceipt = components['schemas']['ConfirmationReceipt'];
type Estado = Notification['state'];

const LOGIN = '/api/auth/login';
const SESSION = '/api/auth/session';
const LIST = '/api/notifications';
const CONFIRM = '/api/notifications/:id/confirm';

const pendienteA: Notification = {
  id: 'n-1',
  github_event: 'commit',
  repo: 'acme/api',
  sha: 'abcdef1234567890',
  state: 'pending',
};
const pendienteB: Notification = {
  id: 'n-2',
  github_event: 'pull_request',
  repo: 'acme/web',
  sha: '1111111222222233',
  state: 'pending',
  artifact: { kind: 'build-from-repo', ref: 'pr-42' },
};
const pendienteC: Notification = {
  id: 'n-3',
  github_event: 'tag',
  repo: 'acme/lib',
  sha: '999999aaaaaaabbb',
  state: 'pending',
  artifact: { kind: 'published-image', ref: 'ghcr.io/acme/lib:1.2.0' },
};
const confirmada: Notification = {
  id: 'n-4',
  github_event: 'commit',
  repo: 'acme/confirmado',
  sha: 'cccccccdddddddd',
  state: 'confirmed',
};
const rechazada: Notification = {
  id: 'n-5',
  github_event: 'commit',
  repo: 'acme/rechazado',
  sha: 'eeeeeeeffffffff',
  state: 'rejected',
};

const recibo: ConfirmationReceipt = {
  run_id: 'run-0123456789abcdef0123456789abcdef',
  notification_id: 'n-1',
  confirmed_by: 'ana',
  confirmed_at: '2026-10-10T12:00:00Z',
};

type Registro = {
  listas: string[];
  posts: { id: string; body: unknown }[];
};

function nuevoRegistro(): Registro {
  return { listas: [], posts: [] };
}

function sesionOk(): HttpHandler[] {
  return [
    http.post(LOGIN, () =>
      HttpResponse.json({ token: 'tok-1', expires_at: '2099-01-01T00:00:00Z', session_id: 's1' }),
    ),
    http.get(SESSION, () =>
      HttpResponse.json({
        principal_id: 'ana',
        role: 'user',
        session_id: 's1',
        expires_at: '2099-01-01T00:00:00Z',
      }),
    ),
  ];
}

function listaPorEstado(porEstado: Partial<Record<Estado, Notification[]>>, reg: Registro) {
  return http.get(LIST, ({ request }) => {
    const estado = (new URL(request.url).searchParams.get('state') ?? 'pending') as Estado;
    reg.listas.push(estado);
    return HttpResponse.json(porEstado[estado] ?? []);
  });
}

/** Entra por la pantalla de login real y espera a estar en /inbox. */
async function abrirInbox(handlers: HttpHandler[]) {
  server.use(...sesionOk(), ...handlers);
  const user = userEvent.setup();
  renderApp('/login');
  await user.type(screen.getByLabelText('Usuario'), 'ana');
  await user.type(screen.getByLabelText('Contraseña'), 'clave-larga-1');
  await user.click(screen.getByRole('button', { name: 'Entrar' }));
  await waitFor(() => expect(currentLocation()).toBe('/inbox'));
  return user;
}

function filaDe(repo: string): HTMLElement {
  const fila = screen.getByText(repo).closest('tr');
  if (fila === null) throw new Error(`sin fila para ${repo}`);
  return fila;
}

/** Selecciona la notificación pendiente del repo indicado y abre el panel de confirmación. */
async function abrirPanel(user: ReturnType<typeof userEvent.setup>, repo: string) {
  await screen.findByText(repo);
  await user.click(within(filaDe(repo)).getByRole('button', { name: 'Ver detalle' }));
  await user.click(screen.getByRole('button', { name: 'Revisar y confirmar' }));
  return screen.getByRole('dialog');
}

describe('Inbox lista', () => {
  it('cada pestaña pide su ?state= y muestra solo lo devuelto', async () => {
    const reg = nuevoRegistro();
    const user = await abrirInbox([
      listaPorEstado({ pending: [pendienteA], confirmed: [confirmada], rejected: [rechazada] }, reg),
    ]);
    await screen.findByText('acme/api');
    expect(screen.queryByText('acme/confirmado')).toBeNull();

    await user.click(screen.getByRole('tab', { name: 'Confirmadas' }));
    await screen.findByText('acme/confirmado');
    expect(screen.queryByText('acme/api')).toBeNull();

    await user.click(screen.getByRole('tab', { name: 'Rechazadas' }));
    await screen.findByText('acme/rechazado');
    expect(screen.queryByText('acme/confirmado')).toBeNull();

    expect(reg.listas).toEqual(['pending', 'confirmed', 'rejected']);
  });

  it('una lista vacía muestra el mensaje de vacío', async () => {
    await abrirInbox([listaPorEstado({ pending: [] }, nuevoRegistro())]);
    expect(await screen.findByText('No hay notificaciones pendientes')).toBeTruthy();
  });

  it('un 503 muestra el aviso de servicio no disponible', async () => {
    await abrirInbox([
      http.get(LIST, () =>
        HttpResponse.json({ code: 'unavailable', message: 'x' }, { status: 503 }),
      ),
    ]);
    expect(await screen.findByText(/Servicio no disponible/)).toBeTruthy();
  });

  it('el sha se muestra truncado a 7 caracteres', async () => {
    await abrirInbox([listaPorEstado({ pending: [pendienteA] }, nuevoRegistro())]);
    expect(await screen.findByText('abcdef1')).toBeTruthy();
    expect(screen.queryByText('abcdef1234567890')).toBeNull();
  });

  it('Actualizar hace exactamente una petición nueva y no hay polling (2 s con temporizador falso)', async () => {
    const reg = nuevoRegistro();
    const user = await abrirInbox([listaPorEstado({ pending: [pendienteA] }, reg)]);
    await screen.findByText('acme/api');
    expect(reg.listas).toEqual(['pending']);

    await user.click(screen.getByRole('button', { name: 'Actualizar' }));
    await waitFor(() => expect(reg.listas).toHaveLength(2));
    expect(reg.listas).toEqual(['pending', 'pending']);
  });

  it('sin polling: con temporizadores falsos instalados antes de montar, 6 s no generan peticiones', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const reg = nuevoRegistro();
      await abrirInbox([listaPorEstado({ pending: [pendienteA] }, reg)]);
      await screen.findByText('acme/api');
      expect(reg.listas).toEqual(['pending']);

      await act(async () => {
        vi.advanceTimersByTime(6000);
      });
      expect(reg.listas).toEqual(['pending']);
    } finally {
      vi.useRealTimers();
    }
  });

  it('descarta la respuesta vieja de una pestaña anterior', async () => {
    const reg = nuevoRegistro();
    const user = await abrirInbox([
      http.get(LIST, async ({ request }) => {
        const estado = new URL(request.url).searchParams.get('state');
        reg.listas.push(estado ?? '');
        if (estado === 'pending') await new Promise((r) => setTimeout(r, 400));
        return HttpResponse.json(estado === 'confirmed' ? [confirmada] : [pendienteA]);
      }),
    ]);
    await user.click(screen.getByRole('tab', { name: 'Confirmadas' }));
    await screen.findByText('acme/confirmado');
    await new Promise((r) => setTimeout(r, 600));
    expect(screen.queryByText('acme/api')).toBeNull();
    expect(screen.queryByText('acme/confirmado')).not.toBeNull();
    expect(screen.getByRole('tab', { name: 'Confirmadas' }).getAttribute('aria-selected')).toBe('true');
  });
});

describe('Inbox confirma', () => {
  it('abrir el panel no envía ninguna petición POST', async () => {
    const reg = nuevoRegistro();
    const user = await abrirInbox([
      listaPorEstado({ pending: [pendienteA] }, reg),
      http.post(CONFIRM, async ({ params, request }) => {
        reg.posts.push({ id: String(params['id']), body: await request.json() });
        return HttpResponse.json(recibo, { status: 201 });
      }),
    ]);
    await abrirPanel(user, 'acme/api');
    expect(reg.posts).toHaveLength(0);
  });

  it('el cuerpo enviado es {"flows":["happy-path"]}', async () => {
    const reg = nuevoRegistro();
    const user = await abrirInbox([
      listaPorEstado({ pending: [pendienteA] }, reg),
      http.post(CONFIRM, async ({ params, request }) => {
        reg.posts.push({ id: String(params['id']), body: await request.json() });
        return HttpResponse.json(recibo, { status: 201 });
      }),
    ]);
    const dialogo = await abrirPanel(user, 'acme/api');
    await user.click(within(dialogo).getByRole('button', { name: 'Confirmar corrida' }));
    await waitFor(() => expect(reg.posts).toHaveLength(1));
    expect(reg.posts[0]).toEqual({ id: 'n-1', body: { flows: ['happy-path'] } });
  });

  it('sin familias seleccionadas el botón Confirmar corrida está deshabilitado', async () => {
    const user = await abrirInbox([listaPorEstado({ pending: [pendienteA] }, nuevoRegistro())]);
    const dialogo = await abrirPanel(user, 'acme/api');
    await user.click(within(dialogo).getByRole('checkbox', { name: 'happy-path' }));
    expect(
      (within(dialogo).getByRole('button', { name: 'Confirmar corrida' }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
  });

  it('doble clic envía un solo POST', async () => {
    const reg = nuevoRegistro();
    const user = await abrirInbox([
      listaPorEstado({ pending: [pendienteA] }, reg),
      http.post(CONFIRM, async ({ params, request }) => {
        reg.posts.push({ id: String(params['id']), body: await request.json() });
        await new Promise((resolver) => setTimeout(resolver, 150));
        return HttpResponse.json(recibo, { status: 201 });
      }),
    ]);
    const dialogo = await abrirPanel(user, 'acme/api');
    await user.dblClick(within(dialogo).getByRole('button', { name: 'Confirmar corrida' }));
    await waitFor(() => expect(currentLocation()).toBe(`/runs/${recibo.run_id}`));
    expect(reg.posts).toHaveLength(1);
  });

  it('201 navega a /runs/<run_id>', async () => {
    const user = await abrirInbox([
      listaPorEstado({ pending: [pendienteA] }, nuevoRegistro()),
      http.post(CONFIRM, () => HttpResponse.json(recibo, { status: 201 })),
    ]);
    const dialogo = await abrirPanel(user, 'acme/api');
    await user.click(within(dialogo).getByRole('button', { name: 'Confirmar corrida' }));
    await waitFor(() => expect(currentLocation()).toBe(`/runs/${recibo.run_id}`));
  });

  it('409 muestra su mensaje y recarga la lista', async () => {
    const reg = nuevoRegistro();
    const user = await abrirInbox([
      listaPorEstado({ pending: [pendienteA] }, reg),
      http.post(CONFIRM, () =>
        HttpResponse.json({ code: 'conflict', message: 'x' }, { status: 409 }),
      ),
    ]);
    const dialogo = await abrirPanel(user, 'acme/api');
    await user.click(within(dialogo).getByRole('button', { name: 'Confirmar corrida' }));
    expect(await screen.findByText('Esta notificación ya no está pendiente')).toBeTruthy();
    await waitFor(() => expect(reg.listas).toHaveLength(2));
  });

  it('404 muestra su mensaje y recarga la lista', async () => {
    const reg = nuevoRegistro();
    const user = await abrirInbox([
      listaPorEstado({ pending: [pendienteA] }, reg),
      http.post(CONFIRM, () =>
        HttpResponse.json({ code: 'not_found', message: 'x' }, { status: 404 }),
      ),
    ]);
    const dialogo = await abrirPanel(user, 'acme/api');
    await user.click(within(dialogo).getByRole('button', { name: 'Confirmar corrida' }));
    expect(await screen.findByText('La notificación ya no existe')).toBeTruthy();
    await waitFor(() => expect(reg.listas).toHaveLength(2));
  });

  it('un 400 muestra el aviso de error de la API con su referencia', async () => {
    const user = await abrirInbox([
      listaPorEstado({ pending: [pendienteA] }, nuevoRegistro()),
      http.post(CONFIRM, () =>
        HttpResponse.json({ code: 'bad_request', message: 'cuerpo inválido' }, { status: 400 }),
      ),
    ]);
    const dialogo = await abrirPanel(user, 'acme/api');
    await user.click(within(dialogo).getByRole('button', { name: 'Confirmar corrida' }));
    expect(await screen.findByText(/cuerpo inválido/)).toBeTruthy();
    expect(screen.getByText(/Referencia para soporte/)).toBeTruthy();
  });

  it('409 con un POST en vuelo: la pestaña no cambia y la recarga muestra pendientes', async () => {
    const reg = nuevoRegistro();
    const user = await abrirInbox([
      listaPorEstado({ pending: [pendienteA], confirmed: [confirmada] }, reg),
      http.post(CONFIRM, async () => {
        await new Promise((r) => setTimeout(r, 300));
        return HttpResponse.json({ code: 'conflict', message: 'x' }, { status: 409 });
      }),
    ]);
    const dialogo = await abrirPanel(user, 'acme/api');
    await user.click(within(dialogo).getByRole('button', { name: 'Confirmar corrida' }));
    await user.click(screen.getByRole('tab', { name: 'Confirmadas' }));
    expect(screen.getByRole('tab', { name: 'Pendientes' }).getAttribute('aria-selected')).toBe('true');

    expect(await screen.findByText('Esta notificación ya no está pendiente')).toBeTruthy();
    await waitFor(() => expect(reg.listas).toEqual(['pending', 'pending']));
    expect(within(screen.getByRole('table')).getByText('acme/api')).toBeTruthy();
    expect(screen.queryByText('acme/confirmado')).toBeNull();
  });

  it('un 400 con un POST en vuelo: la pestaña no cambia y el aviso sigue visible', async () => {
    const user = await abrirInbox([
      listaPorEstado({ pending: [pendienteA], confirmed: [confirmada] }, nuevoRegistro()),
      http.post(CONFIRM, async () => {
        await new Promise((r) => setTimeout(r, 300));
        return HttpResponse.json({ code: 'bad_request', message: 'cuerpo inválido' }, { status: 400 });
      }),
    ]);
    const dialogo = await abrirPanel(user, 'acme/api');
    await user.click(within(dialogo).getByRole('button', { name: 'Confirmar corrida' }));
    await user.click(screen.getByRole('tab', { name: 'Confirmadas' }));
    expect(screen.getByRole('tab', { name: 'Pendientes' }).getAttribute('aria-selected')).toBe('true');
    expect(await screen.findByText(/cuerpo inválido/)).toBeTruthy();
    expect(screen.getByRole('dialog')).toBeTruthy();
  });

  it('201 con run_id no válido no navega y avisa', async () => {
    const user = await abrirInbox([
      listaPorEstado({ pending: [pendienteA] }, nuevoRegistro()),
      http.post(CONFIRM, () => HttpResponse.json({ ...recibo, run_id: '..' }, { status: 201 })),
    ]);
    const dialogo = await abrirPanel(user, 'acme/api');
    await user.click(within(dialogo).getByRole('button', { name: 'Confirmar corrida' }));
    expect(await screen.findByText(/no se pudo abrir la corrida/)).toBeTruthy();
    expect(currentLocation()).toBe('/inbox');
  });

  it('notificaciones confirmed y rejected no ofrecen confirmar', async () => {
    const user = await abrirInbox([
      listaPorEstado({ pending: [pendienteA], confirmed: [confirmada], rejected: [rechazada] }, nuevoRegistro()),
    ]);
    await user.click(screen.getByRole('tab', { name: 'Confirmadas' }));
    await screen.findByText('acme/confirmado');
    await user.click(within(filaDe('acme/confirmado')).getByRole('button', { name: 'Ver detalle' }));
    expect(screen.queryByRole('button', { name: 'Revisar y confirmar' })).toBeNull();

    await user.click(screen.getByRole('tab', { name: 'Rechazadas' }));
    await screen.findByText('acme/rechazado');
    await user.click(within(filaDe('acme/rechazado')).getByRole('button', { name: 'Ver detalle' }));
    expect(screen.queryByRole('button', { name: 'Revisar y confirmar' })).toBeNull();
  });

  it('Escape cierra el panel sin enviar', async () => {
    const reg = nuevoRegistro();
    const user = await abrirInbox([
      listaPorEstado({ pending: [pendienteA] }, reg),
      http.post(CONFIRM, () => HttpResponse.json(recibo, { status: 201 })),
    ]);
    await abrirPanel(user, 'acme/api');
    await user.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(reg.posts).toHaveLength(0);
  });
});

describe('Inbox sin auto', () => {
  it('carga con 3 pendientes, recorre pestañas, abre y cierra un panel, y no hay ningún POST', async () => {
    const reg = nuevoRegistro();
    const user = await abrirInbox([
      listaPorEstado(
        { pending: [pendienteA, pendienteB, pendienteC], confirmed: [confirmada], rejected: [rechazada] },
        reg,
      ),
      http.post(CONFIRM, async ({ params, request }) => {
        reg.posts.push({ id: String(params['id']), body: await request.json() });
        return HttpResponse.json(recibo, { status: 201 });
      }),
    ]);
    await screen.findByText('acme/api');
    expect(screen.getAllByRole('row').length).toBeGreaterThanOrEqual(4);

    await user.click(screen.getByRole('tab', { name: 'Confirmadas' }));
    await screen.findByText('acme/confirmado');
    await user.click(screen.getByRole('tab', { name: 'Rechazadas' }));
    await screen.findByText('acme/rechazado');
    await user.click(screen.getByRole('tab', { name: 'Pendientes' }));
    await screen.findByText('acme/api');

    const dialogo = await abrirPanel(user, 'acme/api');
    expect(dialogo).toBeTruthy();
    await user.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).toBeNull();

    expect(reg.posts).toHaveLength(0);
  });
});
