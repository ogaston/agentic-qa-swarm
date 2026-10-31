import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { server } from '../test/server';
import { currentLocation, renderApp } from '../test/renderApp';
import { SESSION_STORAGE_KEY } from './SessionContext';

const stored = { version: 1, token: 'persisted-token', expiresAt: '2099-01-01T00:00:00Z' };
function seed(value: unknown = stored) { localStorage.setItem(SESSION_STORAGE_KEY, JSON.stringify(value)); }
function sessionHandler() {
  return http.get('/api/auth/session', ({ request }) => {
    expect(request.headers.get('Authorization')).toBe('Bearer persisted-token');
    return HttpResponse.json({ principal_id: 'restored-user', role: 'user', session_id: 's1', expires_at: stored.expiresAt });
  });
}

describe('persistent session', () => {
  it('validates the server session before protected requests and restores across remounts', async () => {
    seed();
    let validated = false;
    server.use(http.get('/api/auth/session', async () => {
      await new Promise((resolve) => setTimeout(resolve, 20));
      validated = true;
      return HttpResponse.json({ principal_id: 'server-user', role: 'user', session_id: 's1', expires_at: stored.expiresAt });
    }), http.get('/api/notifications', () => {
      expect(validated).toBe(true);
      return HttpResponse.json([]);
    }));
    const first = renderApp('/inbox');
    expect(screen.getByRole('status').textContent).toBe('Restaurando sesión…');
    await screen.findByText('server-user');
    expect(currentLocation()).toBe('/inbox');
    first.unmount();
    validated = false;
    renderApp('/inbox');
    await screen.findByText('server-user');
    expect(currentLocation()).toBe('/inbox');
  });

  it.each([{}, { ...stored, version: 9 }, { ...stored, token: '' }, { ...stored, expiresAt: 'bad' }, { ...stored, expiresAt: '2000-01-01T00:00:00Z' }])('rejects invalid stored data: %j', async (value) => {
    seed(value);
    renderApp('/inbox');
    await screen.findByLabelText('Usuario');
    expect(localStorage.getItem(SESSION_STORAGE_KEY)).toBeNull();
    expect(currentLocation()).toBe('/login?next=%2Finbox');
  });

  it('removes malformed JSON', async () => {
    localStorage.setItem(SESSION_STORAGE_KEY, '{');
    renderApp('/inbox');
    await screen.findByLabelText('Usuario');
    expect(localStorage.getItem(SESSION_STORAGE_KEY)).toBeNull();
  });

  it.each([401, 503])('fails closed when restore returns %s', async (status) => {
    seed();
    server.use(http.get('/api/auth/session', () => HttpResponse.json({ code: 'unavailable', message: 'x' }, { status })));
    renderApp('/inbox');
    await screen.findByLabelText('Usuario');
    expect(localStorage.getItem(SESSION_STORAGE_KEY)).toBeNull();
  });

  it('clears persisted credentials even if logout fails', async () => {
    seed();
    server.use(sessionHandler(), http.post('/api/auth/logout', () => new HttpResponse(null, { status: 500 })));
    renderApp('/inbox');
    await screen.findByText('restored-user');
    await userEvent.click(screen.getByRole('button', { name: 'Salir' }));
    await screen.findByLabelText('Usuario');
    expect(localStorage.getItem(SESSION_STORAGE_KEY)).toBeNull();
  });

  it('logs in with storage blocked without crashing', async () => {
    const get = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('blocked'); });
    const set = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('blocked'); });
    const remove = vi.spyOn(Storage.prototype, 'removeItem').mockImplementation(() => { throw new Error('blocked'); });
    try {
      server.use(http.post('/api/auth/login', () => HttpResponse.json({ token: stored.token, expires_at: stored.expiresAt })), sessionHandler());
      renderApp('/login');
      const user = userEvent.setup();
      await user.type(await screen.findByLabelText('Usuario'), 'ana');
      await user.type(screen.getByLabelText('Contraseña'), 'password');
      await user.click(screen.getByRole('button', { name: 'Entrar' }));
      await waitFor(() => expect(currentLocation()).toBe('/inbox'));
    } finally { get.mockRestore(); set.mockRestore(); remove.mockRestore(); }
  });
});
