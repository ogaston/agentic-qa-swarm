import { act, fireEvent, render, screen } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { server } from '../../test/server';
import { TIMERS_FALSOS, avanzar, vaciar } from '../../test/tiempo';
import { WarmPage } from './WarmPage';

const WARM = { warm_id: 'warm-1', state: 'ready', reset_verified: true, baseline_version: 'v1.2.0' };

function setVisibilidad(valor: 'visible' | 'hidden') {
  Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => valor });
  document.dispatchEvent(new Event('visibilitychange'));
}

function renderWarm() {
  return render(
    <MemoryRouter initialEntries={['/warm']}>
      <WarmPage />
    </MemoryRouter>,
  );
}

function contarPeticiones(respuesta: () => Response | ReturnType<typeof HttpResponse.json>) {
  const contador = { peticiones: 0 };
  server.use(
    http.get('/api/warm', () => {
      contador.peticiones++;
      return respuesta() as ReturnType<typeof HttpResponse.json>;
    }),
  );
  return contador;
}

beforeEach(() => {
  vi.useFakeTimers(TIMERS_FALSOS);
});

afterEach(() => {
  setVisibilidad('visible');
  vi.useRealTimers();
});

describe('WarmPage', () => {
  it('los 4 estados se muestran con texto distinto', async () => {
    const textos = new Set<string>();
    for (const state of ['ready', 'dirty', 'cuarentena', 'idle-escalado']) {
      server.resetHandlers();
      contarPeticiones(() => HttpResponse.json({ ...WARM, state }));
      const { unmount } = renderWarm();
      await vaciar();
      const insignia = document.querySelector('[data-testid="warm-estado"]');
      expect(insignia?.textContent?.trim().length).toBeGreaterThan(0);
      textos.add(insignia?.textContent?.trim() ?? '');
      unmount();
    }
    expect(textos.size).toBe(4);
  });

  it('ready con reset_verified=false muestra aviso explícito', async () => {
    contarPeticiones(() => HttpResponse.json({ ...WARM, reset_verified: false }));
    renderWarm();
    await vaciar();
    expect(screen.getByRole('alert').textContent).toMatch(/reset no está verificado/i);
  });

  it('503 warm_unavailable muestra «Estado del warm no disponible»', async () => {
    contarPeticiones(() =>
      HttpResponse.json({ code: 'warm_unavailable', message: 'x' }, { status: 503, headers: { 'Retry-After': '5' } }),
    );
    renderWarm();
    await vaciar();
    expect(screen.getByText('Estado del warm no disponible')).toBeTruthy();
  });

  it('sondeo cada 10 s: 3 peticiones en 25 s', async () => {
    const contador = contarPeticiones(() => HttpResponse.json(WARM));
    renderWarm();
    await vaciar();
    await avanzar(10_000);
    await avanzar(10_000);
    expect(contador.peticiones).toBe(3);
    await avanzar(5000);
    expect(contador.peticiones).toBe(3);
  });

  it('pestaña oculta: pausa el sondeo', async () => {
    const contador = contarPeticiones(() => HttpResponse.json(WARM));
    renderWarm();
    await vaciar();
    setVisibilidad('hidden');
    await avanzar(25_000);
    expect(contador.peticiones).toBe(1);
  });

  it('5 fallos seguidos: deja de sondear y aparece «Reintentar»', async () => {
    const contador = contarPeticiones(() => HttpResponse.json({ code: 'x', message: 'x' }, { status: 500 }));
    renderWarm();
    await vaciar();
    await avanzar(60_000);
    expect(contador.peticiones).toBe(5);
    expect(screen.getByRole('button', { name: 'Reintentar' })).toBeTruthy();
  });

  it('el botón «Actualizar» pide el estado de nuevo', async () => {
    const contador = contarPeticiones(() => HttpResponse.json(WARM));
    renderWarm();
    await vaciar();
    act(() => {
      fireEvent.click(screen.getByRole('button', { name: 'Actualizar' }));
    });
    await vaciar();
    expect(contador.peticiones).toBe(2);
  });
});
