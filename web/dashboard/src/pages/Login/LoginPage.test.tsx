import { act, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { server } from '../../test/server';
import { currentLocation, renderApp } from '../../test/renderApp';

const LOGIN = '/api/auth/login';
const SESSION = '/api/auth/session';

function okLogin(principal = 'ana', role = 'user') {
  return [
    http.post(LOGIN, () =>
      HttpResponse.json({ token: 'tok-login-1', expires_at: '2099-01-01T00:00:00Z', session_id: 's1' }),
    ),
    http.get(SESSION, () =>
      HttpResponse.json({ principal_id: principal, role, session_id: 's1', expires_at: '2099-01-01T00:00:00Z' }),
    ),
  ] as const;
}

async function fillAndSubmit(user: ReturnType<typeof userEvent.setup>, usuario = 'ana', clave = 'clave-larga-1') {
  await user.type(screen.getByLabelText('Usuario'), usuario);
  await user.type(screen.getByLabelText('Contraseña'), clave);
  await user.click(screen.getByRole('button', { name: 'Entrar' }));
}

describe('LoginPage', () => {
  it('login correcto llama a /auth/session, navega a /inbox y muestra principal_id', async () => {
    server.use(...okLogin('ana', 'user'));
    const user = userEvent.setup();
    renderApp('/login');
    await fillAndSubmit(user);
    await waitFor(() => expect(currentLocation()).toBe('/inbox'));
    expect(screen.getByText('ana')).toBeTruthy();
    expect(screen.getByText('user')).toBeTruthy();
  });

  it('401 muestra el mensaje genérico y no navega', async () => {
    server.use(
      http.post(LOGIN, () =>
        HttpResponse.json({ code: 'unauthorized', message: 'x' }, { status: 401 }),
      ),
    );
    const user = userEvent.setup();
    renderApp('/login');
    await fillAndSubmit(user);
    expect(await screen.findByText('Usuario, contraseña o código incorrectos')).toBeTruthy();
    expect(currentLocation()).toBe('/login');
  });

  it('401 con usuario admin muestra el campo de código de verificación', async () => {
    server.use(
      http.post(LOGIN, () =>
        HttpResponse.json({ code: 'unauthorized', message: 'x' }, { status: 401 }),
      ),
    );
    const user = userEvent.setup();
    renderApp('/login');
    expect(screen.queryByLabelText('Código de verificación')).toBeNull();
    await fillAndSubmit(user, 'admin');
    expect(await screen.findByLabelText('Código de verificación')).toBeTruthy();
  });

  it('429 con Retry-After: 30 muestra cuenta atrás desde 30 y deshabilita el botón', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      server.use(
        http.post(LOGIN, () =>
          HttpResponse.json(
            { code: 'rate_limited', message: 'x' },
            { status: 429, headers: { 'Retry-After': '30' } },
          ),
        ),
      );
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderApp('/login');
      await fillAndSubmit(user);
      expect(await screen.findByText(/espera 30 s/)).toBeTruthy();
      const boton = screen.getByRole('button', { name: /Entrar/ }) as HTMLButtonElement;
      expect(boton.disabled).toBe(true);
      await act(async () => {
        vi.advanceTimersByTime(1000);
      });
      expect(screen.getByText(/espera 29 s/)).toBeTruthy();
    } finally {
      vi.useRealTimers();
    }
  });

  it('503 muestra que el servicio de identidad no está disponible', async () => {
    server.use(
      http.post(LOGIN, () => HttpResponse.json({ code: 'unavailable', message: 'x' }, { status: 503 })),
    );
    const user = userEvent.setup();
    renderApp('/login');
    await fillAndSubmit(user);
    expect(await screen.findByText('Servicio de identidad no disponible')).toBeTruthy();
  });

  it('un fallo de red muestra que el servicio de identidad no está disponible', async () => {
    server.use(http.post(LOGIN, () => HttpResponse.error()));
    const user = userEvent.setup();
    renderApp('/login');
    await fillAndSubmit(user);
    expect(await screen.findByText('Servicio de identidad no disponible')).toBeTruthy();
  });

  it('dos pulsaciones seguidas en Entrar envían un solo POST', async () => {
    let posts = 0;
    server.use(
      http.post(LOGIN, async () => {
        posts += 1;
        await new Promise((r) => setTimeout(r, 20));
        return HttpResponse.json({ code: 'unauthorized', message: 'x' }, { status: 401 });
      }),
    );
    const user = userEvent.setup();
    renderApp('/login');
    await user.type(screen.getByLabelText('Usuario'), 'ana');
    await user.type(screen.getByLabelText('Contraseña'), 'clave-larga-1');
    const boton = screen.getByRole('button', { name: 'Entrar' });
    await user.dblClick(boton);
    await screen.findByText('Usuario, contraseña o código incorrectos');
    expect(posts).toBe(1);
  });

  it('el OTP se envía solo si se rellenó', async () => {
    const cuerpos: unknown[] = [];
    server.use(
      http.post(LOGIN, async ({ request }) => {
        cuerpos.push(await request.json());
        return HttpResponse.json({ code: 'unauthorized', message: 'x' }, { status: 401 });
      }),
    );
    const user = userEvent.setup();
    renderApp('/login');
    await fillAndSubmit(user, 'admin');
    await screen.findByText('Usuario, contraseña o código incorrectos');
    await user.type(screen.getByLabelText('Código de verificación'), '123456');
    await user.click(screen.getByRole('button', { name: 'Entrar' }));
    await waitFor(() => expect(cuerpos.length).toBe(2));
    expect(cuerpos[0]).not.toHaveProperty('otp');
    expect(cuerpos[1]).toMatchObject({ otp: '123456' });
  });

  it('next=//evil.example se ignora y va a /inbox', async () => {
    server.use(...okLogin());
    const user = userEvent.setup();
    renderApp('/login?next=//evil.example');
    await fillAndSubmit(user);
    await waitFor(() => expect(currentLocation()).toBe('/inbox'));
  });

  it('next con tabulador («/\\t/evil.example») se ignora y va a /inbox', async () => {
    server.use(...okLogin());
    const user = userEvent.setup();
    renderApp('/login?next=' + encodeURIComponent('/\t/evil.example'));
    await fillAndSubmit(user);
    await waitFor(() => expect(currentLocation()).toBe('/inbox'));
  });

  it('next=/warm se respeta tras el login', async () => {
    server.use(...okLogin());
    const user = userEvent.setup();
    renderApp('/login?next=/warm');
    await fillAndSubmit(user);
    await waitFor(() => expect(currentLocation()).toBe('/warm'));
  });
});

// U10-T04 CA-2: estructura de «Iniciar sesión» según el mockup (login-wrap, tarjeta con marca, alert-box).
describe('LoginPage (estructura del mockup, U10-T04)', () => {
  it('h1 «Iniciar sesión» dentro de login-wrap y tarjeta con la marca aqs', () => {
    renderApp('/login');
    const titulo = screen.getByRole('heading', { level: 1, name: 'Iniciar sesión' });
    const tarjeta = titulo.closest('.card');
    expect(document.querySelector('.login-wrap')).not.toBeNull();
    expect(tarjeta).not.toBeNull();
    expect(tarjeta?.querySelector('.az-logo')?.textContent).toBe('aqs');
  });

  it('sin cabecera de sesión en /login', () => {
    renderApp('/login');
    expect(document.querySelector('.az-header')).toBeNull();
  });

  it('las etiquetas de usuario, contraseña y TOTP son <label> visibles', async () => {
    const user = userEvent.setup();
    renderApp('/login');
    expect(screen.getByText('Usuario', { selector: 'label' })).toBeTruthy();
    expect(screen.getByText('Contraseña', { selector: 'label' })).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Tengo un código de verificación' }));
    expect(screen.getByText('Código de verificación', { selector: 'label' })).toBeTruthy();
  });

  it('401 es un alert-box alert-danger con role=alert', async () => {
    server.use(http.post(LOGIN, () => HttpResponse.json({ code: 'unauthorized', message: 'x' }, { status: 401 })));
    const user = userEvent.setup();
    renderApp('/login');
    await fillAndSubmit(user);
    const aviso = await screen.findByRole('alert');
    expect(aviso.classList.contains('alert-box')).toBe(true);
    expect(aviso.classList.contains('alert-danger')).toBe(true);
  });

  it('503 es un alert-box con role=alert', async () => {
    server.use(http.post(LOGIN, () => HttpResponse.json({ code: 'unavailable', message: 'x' }, { status: 503 })));
    const user = userEvent.setup();
    renderApp('/login');
    await fillAndSubmit(user);
    const aviso = await screen.findByRole('alert');
    expect(aviso.classList.contains('alert-box')).toBe(true);
  });

  it('429 con cuenta atrás es un alert-box alert-warn con role=alert', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      server.use(
        http.post(LOGIN, () =>
          HttpResponse.json({ code: 'rate_limited', message: 'x' }, { status: 429, headers: { 'Retry-After': '30' } }),
        ),
      );
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderApp('/login');
      await fillAndSubmit(user);
      const aviso = await screen.findByRole('alert');
      expect(aviso.classList.contains('alert-box')).toBe(true);
      expect(aviso.classList.contains('alert-warn')).toBe(true);
      expect(aviso.textContent).toMatch(/espera 30 s/);
    } finally {
      vi.useRealTimers();
    }
  });
});
