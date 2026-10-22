import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';
import { server } from './test/server';
import { currentLocation, renderApp } from './test/renderApp';

const SESION = { principal_id: 'ana', role: 'user', session_id: 's1', expires_at: '2099-01-01T00:00:00Z' };

function conSesionValida() {
  server.use(
    http.post('/api/auth/login', () =>
      HttpResponse.json({ token: 'tok', expires_at: '2099-01-01T00:00:00Z', session_id: 's1' }),
    ),
    http.get('/api/auth/session', () => HttpResponse.json(SESION)),
  )
}

describe('rutas de la app', () => {
  it('/ sin sesión termina en /login?next=%2Finbox', async () => {
    renderApp('/');
    await waitFor(() => expect(currentLocation()).toBe('/login?next=%2Finbox'));
  });

  it('/ con sesión termina en /inbox', async () => {
    conSesionValida();
    const user = userEvent.setup();
    renderApp('/login?next=%2F');
    await user.type(screen.getByLabelText('Usuario'), 'ana');
    await user.type(screen.getByLabelText('Contraseña'), 'clave-larga-1');
    await user.click(screen.getByRole('button', { name: 'Entrar' }));
    await waitFor(() => expect(currentLocation()).toBe('/inbox'));
  });

  it('una ruta desconocida sigue mostrando «No encontrado»', async () => {
    renderApp('/xyz');
    expect(screen.getByRole('heading', { name: 'No encontrado' })).toBeTruthy();
    expect(currentLocation()).toBe('/xyz');
  });
});
