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
  it('muestra el identificador real sin inventar composición ni namespace', async () => {
    contarPeticiones(() => HttpResponse.json({ ...WARM, warm_id: 'otro-entorno' }));
    renderWarm();
    await vaciar();
    expect(screen.getAllByText('otro-entorno').length).toBeGreaterThan(0);
    expect(screen.queryAllByText(/aqs-test|Postgres|Redis/)).toHaveLength(0);
  });

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

  it('sin aviso si state=dirty aunque reset_verified=false', async () => {
    contarPeticiones(() => HttpResponse.json({ ...WARM, state: 'dirty', reset_verified: false }));
    renderWarm();
    await vaciar();
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it('sin aviso si state=ready y reset_verified=true', async () => {
    contarPeticiones(() => HttpResponse.json({ ...WARM, state: 'ready', reset_verified: true }));
    renderWarm();
    await vaciar();
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it('tras una lectura buena, un fallo marca el dato como desactualizado', async () => {
    let i = 0;
    contarPeticiones(() =>
      i++ === 0 ? HttpResponse.json(WARM) : HttpResponse.json({ code: 'warm_unavailable', message: 'x' }, { status: 503 }),
    );
    renderWarm();
    await vaciar();
    await avanzar(10_000);
    expect(screen.getByText('Estado del warm no disponible')).toBeTruthy();
    expect(screen.getByText(/Mostrando la última lectura correcta/)).toBeTruthy();
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

// U10-T04 CA-2: estructura de «Entorno» según el mockup (encabezado, tarjeta con kv-list, icono + texto).
describe('WarmPage (estructura del mockup, U10-T04)', () => {
  it('el título «Entorno» es el h1 y la tarjeta se titula con el warm_id', async () => {
    contarPeticiones(() => HttpResponse.json(WARM));
    renderWarm();
    await vaciar();
    expect(screen.getByRole('heading', { level: 1, name: 'Entorno' })).toBeTruthy();
    expect(screen.getByRole('heading', { level: 2, name: 'Estado operativo de warm-1' })).toBeTruthy();
  });

  it('la tarjeta expone estado, reset_verified, baseline_version y warm_id en un kpi-strip de 4 cuadros', async () => {
    // U10-T06 (ENTORNO, instrucción del orquestador): el mockup muestra kpi-strip en vez de dl.kv-list (prueba T04 CA-2).
    contarPeticiones(() => HttpResponse.json(WARM));
    const { container } = renderWarm();
    await vaciar();
    const rejilla = container.querySelector('.kpi-strip');
    expect(rejilla).not.toBeNull();
    expect(container.querySelector('dl.kv-list')).toBeNull();
    const cuadros = Array.from(rejilla?.querySelectorAll('.kpi-box') ?? []);
    expect(cuadros.length).toBe(4);
    const etiquetas = cuadros.map((c) => c.querySelector('.kpi-label')?.textContent ?? '');
    expect(etiquetas[0]?.startsWith('Estado')).toBe(true);
    expect(etiquetas[1]?.startsWith('Reset verificado')).toBe(true);
    expect(etiquetas[2]?.startsWith('Versión base')).toBe(true);
    expect(etiquetas[3]?.startsWith('Identificador del entorno')).toBe(true);
    expect(rejilla?.textContent).toContain('v1.2.0');
    expect(rejilla?.textContent).toContain('warm-1');
  });

  it('los 4 estados se leen como texto y llevan un icono decorativo', async () => {
    const esperado: Record<string, string> = {
      ready: 'Listo',
      dirty: 'Sucio (requiere reset)',
      cuarentena: 'En cuarentena',
      'idle-escalado': 'Inactivo (escalado)',
    };
    for (const state of Object.keys(esperado)) {
      server.resetHandlers();
      contarPeticiones(() => HttpResponse.json({ ...WARM, state }));
      const { unmount } = renderWarm();
      await vaciar();
      const insignia = document.querySelector('[data-testid="warm-estado"]');
      expect(insignia?.textContent?.trim()).toBe(esperado[state]);
      const icono = insignia?.querySelector('svg');
      expect(icono).not.toBeNull();
      expect(icono?.getAttribute('aria-hidden')).toBe('true');
      unmount();
    }
  });

  it('ready sin reset verificado: el aviso es alert-box alert-warn con role=alert', async () => {
    contarPeticiones(() => HttpResponse.json({ ...WARM, reset_verified: false }));
    renderWarm();
    await vaciar();
    const aviso = screen.getByRole('alert');
    expect(aviso.classList.contains('alert-box')).toBe(true);
    expect(aviso.classList.contains('alert-warn')).toBe(true);
  });

  it('un fallo 503 se muestra como alert-box alert-danger', async () => {
    contarPeticiones(() =>
      HttpResponse.json({ code: 'warm_unavailable', message: 'x' }, { status: 503, headers: { 'Retry-After': '5' } }),
    );
    renderWarm();
    await vaciar();
    const aviso = screen.getByRole('alert');
    expect(aviso.classList.contains('alert-box')).toBe(true);
    expect(aviso.classList.contains('alert-danger')).toBe(true);
  });

  it('«Actualizar» es un único botón en la cabecera', async () => {
    contarPeticiones(() => HttpResponse.json(WARM));
    renderWarm();
    await vaciar();
    expect(screen.getAllByRole('button', { name: 'Actualizar' }).length).toBe(1);
  });
});
