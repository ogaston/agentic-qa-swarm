import { act, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError, apiFetch } from '../api/client';
import { server } from '../test/server';
import { currentLocation, renderApp } from '../test/renderApp';

const TOKEN = 'tok-MARCADOR-unico-7f3a9c';

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
    server.use(...loginHandlers(expiraEn));
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderApp('/login');
    await loginDesdeFormulario(user);
    await act(async () => {
      vi.advanceTimersByTime(61_000);
    });
    await waitFor(() => expect(currentLocation().startsWith('/login')).toBe(true));
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
