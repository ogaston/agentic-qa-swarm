import { act, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError, apiFetch } from '../api/client';
import { server } from '../test/server';
import { currentLocation, renderApp } from '../test/renderApp';
import { SessionProvider, useSession } from './SessionContext';

const TOKEN = 'tok-MARCADOR-unico-7f3a9c';

// Cabecera Authorization que llegó al último probe. `undefined` = ningún probe todavía.
let ultimaAutorizacion: string | null | undefined;

function probeHandler() {
  return http.get('/api/probe', ({ request }) => {
    ultimaAutorizacion = request.headers.get('authorization');
    return HttpResponse.json({ ok: true });
  });
}

/** Tras un login real, comprueba que el probe SÍ lleva el Bearer (así la prueba no es vacía). */
async function confirmarBearerVivo() {
  await apiFetch('/api/probe');
  expect(ultimaAutorizacion).toBe(`Bearer ${TOKEN}`);
}

function loginHandlers(expiresAt = '2099-01-01T00:00:00Z') {
  return [
    http.post('/api/auth/login', () =>
      HttpResponse.json({ token: TOKEN, expires_at: expiresAt, session_id: 's1' }),
    ),
    http.get('/api/auth/session', () =>
      HttpResponse.json({ principal_id: 'ana', role: 'user', session_id: 's1', expires_at: expiresAt }),
    ),
  ];
}

async function loginDesdeFormulario(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText('Usuario'), 'ana');
  await user.type(screen.getByLabelText('Contraseña'), 'clave-larga-1');
  await user.click(screen.getByRole('button', { name: 'Entrar' }));
  await waitFor(() => expect(currentLocation()).toBe('/inbox'));
}

afterEach(() => {
  vi.useRealTimers();
});

describe('sesión', () => {
  it('logout borra la sesión aunque el servidor responda 500', async () => {
    let logoutLlamado = false;
    server.use(
      ...loginHandlers(),
      http.post('/api/auth/logout', () => {
        logoutLlamado = true;
        return HttpResponse.json({ code: 'internal', message: 'boom' }, { status: 500 });
      }),
    );
    const user = userEvent.setup();
    renderApp('/login');
    await loginDesdeFormulario(user);
    await user.click(screen.getByRole('button', { name: 'Salir' }));
    await waitFor(() => expect(currentLocation()).toBe('/login'));
    expect(logoutLlamado).toBe(true);
    expect(screen.queryByText('ana')).toBeNull();
  });

  it('expira: al llegar expires_at la sesión se cierra y vuelve a /login', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const ahora = Date.now();
    const expiraEn = new Date(ahora + 60_000).toISOString();
    server.use(...loginHandlers(expiraEn), probeHandler());
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderApp('/login');
    await loginDesdeFormulario(user);
    await act(async () => {
      vi.advanceTimersByTime(61_000);
    });
    await waitFor(() => expect(currentLocation()).toBe('/login?next=%2Finbox'));
    await expect(apiFetch('/api/probe')).resolves.toBeDefined();
    expect(ultimaAutorizacion).toBeNull();
  });

  it('401 de una llamada protegida borra la sesión y redirige con next', async () => {
    server.use(
      ...loginHandlers(),
      http.get('/api/notifications', () =>
        HttpResponse.json({ code: 'unauthorized', message: 'x' }, { status: 401 }),
      ),
    );
    const user = userEvent.setup();
    renderApp('/login');
    await loginDesdeFormulario(user);
    await expect(apiFetch('/api/notifications')).rejects.toBeInstanceOf(ApiError);
    await waitFor(() => expect(currentLocation()).toBe('/login?next=%2Finbox'));
    expect(screen.queryByText('ana')).toBeNull();
  });

  it('401: ruta protegida sin sesión redirige a /login sin petición a la API', async () => {
    const peticiones: string[] = [];
    const registrar = ({ request }: { request: Request }) => {
      peticiones.push(request.url);
    };
    server.events.on('request:start', registrar);
    try {
      renderApp('/warm');
      await waitFor(() => expect(currentLocation()).toBe('/login?next=%2Fwarm'));
      expect(peticiones).toEqual([]);
    } finally {
      server.events.removeListener('request:start', registrar);
    }
  });

  it('el token no persiste: storage vacío, sin cookie y sin token en el DOM', async () => {
    server.use(...loginHandlers());
    const user = userEvent.setup();
    renderApp('/login');
    await loginDesdeFormulario(user);
    expect(localStorage.length).toBe(0);
    expect(sessionStorage.length).toBe(0);
    expect(document.cookie).toBe('');
    expect(document.body.innerHTML.includes(TOKEN)).toBe(false);
  });
});

describe('el token del módulo se borra en todo cierre', () => {
  it('tras logout con 500 una petición apiFetch no lleva Authorization', async () => {
    server.use(
      ...loginHandlers(),
      probeHandler(),
      http.post('/api/auth/logout', () => HttpResponse.json({ code: 'internal', message: 'boom' }, { status: 500 })),
    );
    const user = userEvent.setup();
    renderApp('/login');
    await loginDesdeFormulario(user);
    await confirmarBearerVivo();
    await user.click(screen.getByRole('button', { name: 'Salir' }));
    await waitFor(() => expect(currentLocation()).toBe('/login'));
    await apiFetch('/api/probe');
    expect(ultimaAutorizacion).toBeNull();
  });

  it('tras logout con 204 una petición apiFetch no lleva Authorization', async () => {
    server.use(
      ...loginHandlers(),
      probeHandler(),
      http.post('/api/auth/logout', () => new HttpResponse(null, { status: 204 })),
    );
    const user = userEvent.setup();
    renderApp('/login');
    await loginDesdeFormulario(user);
    await confirmarBearerVivo();
    await user.click(screen.getByRole('button', { name: 'Salir' }));
    await waitFor(() => expect(currentLocation()).toBe('/login'));
    await apiFetch('/api/probe');
    expect(ultimaAutorizacion).toBeNull();
  });

  it('tras expirar una petición apiFetch no lleva Authorization', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const expiraEn = new Date(Date.now() + 60_000).toISOString();
    server.use(...loginHandlers(expiraEn), probeHandler());
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderApp('/login');
    await loginDesdeFormulario(user);
    await confirmarBearerVivo();
    await act(async () => {
      vi.advanceTimersByTime(61_000);
    });
    await waitFor(() => expect(currentLocation()).toBe('/login?next=%2Finbox'));
    await apiFetch('/api/probe');
    expect(ultimaAutorizacion).toBeNull();
  });

  it('tras un 401 una petición apiFetch no lleva Authorization', async () => {
    server.use(
      ...loginHandlers(),
      probeHandler(),
      http.get('/api/notifications', () => HttpResponse.json({ code: 'unauthorized', message: 'x' }, { status: 401 })),
    );
    const user = userEvent.setup();
    renderApp('/login');
    await loginDesdeFormulario(user);
    await confirmarBearerVivo();
    await expect(apiFetch('/api/notifications')).rejects.toBeInstanceOf(ApiError);
    await waitFor(() => expect(currentLocation()).toBe('/login?next=%2Finbox'));
    await apiFetch('/api/probe');
    expect(ultimaAutorizacion).toBeNull();
  });
});

describe('expires_at inválido', () => {
  it('expires_at no parseable rechaza el login: sin sesión, sin token y sin temporizador', async () => {
    server.use(...loginHandlers('no-es-una-fecha'), probeHandler());
    const user = userEvent.setup();
    renderApp('/login');
    await user.type(screen.getByLabelText('Usuario'), 'ana');
    await user.type(screen.getByLabelText('Contraseña'), 'clave-larga-1');
    await user.click(screen.getByRole('button', { name: 'Entrar' }));
    expect(await screen.findByText(/expires_at no es una fecha válida/)).toBeTruthy();
    expect(currentLocation()).toBe('/login');
    expect(screen.queryByText('ana')).toBeNull();
    await apiFetch('/api/probe');
    expect(ultimaAutorizacion).toBeNull();
  });
});

describe('login descarta la sesión previa (A-2 / F-03)', () => {
  type Api = ReturnType<typeof useSession>;
  let api: Api | null = null;

  function Capturar() {
    api = useSession();
    return null;
  }

  function montar() {
    render(
      <MemoryRouter initialEntries={['/login']}>
        <SessionProvider>
          <Capturar />
        </SessionProvider>
      </MemoryRouter>,
    );
    if (api === null) throw new Error('sin api');
    return api;
  }

  it('dos logins seguidos: el segundo POST no lleva el Bearer del primero', async () => {
    const cabeceras: (string | null)[] = [];
    let emitidos = 0;
    server.use(
      http.post('/api/auth/login', ({ request }) => {
        cabeceras.push(request.headers.get('authorization'));
        emitidos += 1;
        return HttpResponse.json({ token: `T${emitidos}`, expires_at: '2099-01-01T00:00:00Z', session_id: 's' });
      }),
      http.get('/api/auth/session', () =>
        HttpResponse.json({ principal_id: 'ana', role: 'user', session_id: 's', expires_at: '2099-01-01T00:00:00Z' }),
      ),
    );
    const sesion = montar();
    await act(async () => {
      await sesion.login({ username: 'ana', password: 'x' });
    });
    await act(async () => {
      await sesion.login({ username: 'ana', password: 'x' });
    });
    expect(cabeceras).toEqual([null, null]);
  });

  it('un login fallido con sesión previa deja sin sesión', async () => {
    let intento = 0;
    server.use(
      http.post('/api/auth/login', () => {
        intento += 1;
        if (intento === 1) {
          return HttpResponse.json({ token: 'T1', expires_at: '2099-01-01T00:00:00Z', session_id: 's' });
        }
        return HttpResponse.json({ code: 'unauthorized', message: 'x' }, { status: 401 });
      }),
      http.get('/api/auth/session', () =>
        HttpResponse.json({ principal_id: 'ana', role: 'user', session_id: 's', expires_at: '2099-01-01T00:00:00Z' }),
      ),
    );
    const sesion = montar();
    await act(async () => {
      await sesion.login({ username: 'ana', password: 'x' });
    });
    expect(api?.session?.principal.principal_id).toBe('ana');
    await act(async () => {
      await expect(sesion.login({ username: 'ana', password: 'malo' })).rejects.toBeInstanceOf(ApiError);
    });
    expect(api?.session).toBeNull();
  });
});
