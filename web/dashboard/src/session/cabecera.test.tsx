import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { server } from '../test/server';
import { currentLocation, renderApp } from '../test/renderApp';

// Cabecera de ProtectedLayout (U10-T01, revisión F-03): iniciales del avatar, «Rol: …», botón
// «Salir», navegación con su nombre accesible y etiqueta dinámica del selector de tema.

beforeEach(() => {
  window.localStorage.clear();
  delete document.documentElement.dataset.theme;
});

async function entrarComo(principal: string) {
  server.use(
    http.post('/api/auth/login', () =>
      HttpResponse.json({ token: 'tok', expires_at: '2099-01-01T00:00:00Z', session_id: 's1' }),
    ),
    http.get('/api/auth/session', () =>
      HttpResponse.json({ principal_id: principal, role: 'user', session_id: 's1', expires_at: '2099-01-01T00:00:00Z' }),
    ),
  );
  const user = userEvent.setup();
  renderApp('/login');
  await user.type(screen.getByLabelText('Usuario'), principal);
  await user.type(screen.getByLabelText('Contraseña'), 'clave-larga-1');
  await user.click(screen.getByRole('button', { name: 'Entrar' }));
  await waitFor(() => expect(currentLocation()).toBe('/inbox'));
  return user;
}

describe('cabecera protegida', () => {
  it('chip de sesión: avatar con iniciales, principal y «Rol: …»', async () => {
    await entrarComo('ana');
    const chip = document.querySelector('.az-session-chip') as HTMLElement;
    expect(chip.querySelector('.az-avatar')?.textContent).toBe('AN');
    const filas = chip.querySelectorAll('.az-principal > span');
    expect(chip.querySelector('.az-principal > strong')?.textContent).toBe('ana');
    expect(filas[0]?.textContent).toBe('Rol: user');
  });

  it('iniciales de un principal con separador: operador-demo → OD', async () => {
    await entrarComo('operador-demo');
    const chip = document.querySelector('.az-session-chip') as HTMLElement;
    expect(chip.querySelector('.az-avatar')?.textContent).toBe('OD');
  });

  it('navegación con nombre accesible «Principal» y botón «Salir»', async () => {
    await entrarComo('ana');
    const nav = screen.getByRole('navigation', { name: 'Principal' });
    expect(within(nav).getByRole('link', { name: 'Entorno' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Salir' })).toBeTruthy();
  });

  it('selector de tema: aria-label y title cambian con el tema, como en el mockup', async () => {
    const user = await entrarComo('ana');
    const boton = screen.getByRole('button', { name: 'Cambiar a tema oscuro' });
    expect(boton.getAttribute('title')).toBe('Cambiar a tema oscuro');
    await user.click(boton);
    expect(boton.getAttribute('aria-label')).toBe('Cambiar a tema claro');
    expect(boton.getAttribute('title')).toBe('Cambiar a tema claro');
  });
});
