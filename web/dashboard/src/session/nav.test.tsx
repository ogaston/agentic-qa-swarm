import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';
import { server } from '../test/server';
import { currentLocation, renderApp } from '../test/renderApp';

describe('cabecera de navegación protegida', () => {
  it('enlaza a Inbox, Mis corridas y Warm con sus href', async () => {
    server.use(
      http.post('/api/auth/login', () =>
        HttpResponse.json({ token: 'tok', expires_at: '2099-01-01T00:00:00Z', session_id: 's1' }),
      ),
      http.get('/api/auth/session', () =>
        HttpResponse.json({ principal_id: 'ana', role: 'user', session_id: 's1', expires_at: '2099-01-01T00:00:00Z' }),
      ),
    );
    const user = userEvent.setup();
    renderApp('/login');
    await user.type(screen.getByLabelText('Usuario'), 'ana');
    await user.type(screen.getByLabelText('Contraseña'), 'clave-larga-1');
    await user.click(screen.getByRole('button', { name: 'Entrar' }));
    await waitFor(() => expect(currentLocation()).toBe('/inbox'));
    const nav = screen.getByRole('navigation', { name: 'Principal' });
    expect(nav.querySelectorAll('a').length).toBe(3);
    expect(screen.getByRole('link', { name: 'Inbox' }).getAttribute('href')).toBe('/inbox');
    expect(screen.getByRole('link', { name: 'Mis corridas' }).getAttribute('href')).toBe('/runs');
    expect(screen.getByRole('link', { name: 'Warm' }).getAttribute('href')).toBe('/warm');
  });
});
