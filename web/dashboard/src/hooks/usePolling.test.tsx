import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../api/client';
import { TIMERS_FALSOS, avanzar, vaciar } from '../test/tiempo';
import { usePolling } from './usePolling';

function setVisibilidad(valor: 'visible' | 'hidden') {
  Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => valor });
  document.dispatchEvent(new Event('visibilitychange'));
}

function errorApi(status: number): ApiError {
  return new ApiError({ status, code: 'x', message: 'x', requestId: 'r' });
}

beforeEach(() => {
  vi.useFakeTimers(TIMERS_FALSOS);
});

afterEach(() => {
  setVisibilidad('visible');
  vi.useRealTimers();
});

describe('usePolling', () => {
  it('pide al montar y luego cada intervalo, sin solaparse', async () => {
    const fn = vi.fn(async () => ({ v: 1 }));
    renderHook(() => usePolling(fn, 1000, { isTerminal: () => false }));
    await vaciar();
    expect(fn).toHaveBeenCalledTimes(1);
    await avanzar(1000);
    expect(fn).toHaveBeenCalledTimes(2);
    await avanzar(1000);
    expect(fn).toHaveBeenCalledTimes(3);
  });

  it('se detiene al llegar a un estado terminal', async () => {
    const fn = vi.fn(async () => ({ estado: 'done' }));
    renderHook(() => usePolling(fn, 1000, { isTerminal: (d) => d.estado === 'done' }));
    await vaciar();
    await avanzar(10_000);
    expect(fn).toHaveBeenCalledTimes(1);
  });

  it('pausa con la pestaña oculta y reanuda al volver', async () => {
    const fn = vi.fn(async () => ({ v: 1 }));
    renderHook(() => usePolling(fn, 1000, { isTerminal: () => false }));
    await vaciar();
    setVisibilidad('hidden');
    await avanzar(9000);
    expect(fn).toHaveBeenCalledTimes(1);
    setVisibilidad('visible');
    await vaciar();
    expect(fn).toHaveBeenCalledTimes(2);
  });

  it('desmontar detiene el sondeo', async () => {
    const fn = vi.fn(async () => ({ v: 1 }));
    const { unmount } = renderHook(() => usePolling(fn, 1000, { isTerminal: () => false }));
    await vaciar();
    unmount();
    await avanzar(9000);
    expect(fn).toHaveBeenCalledTimes(1);
  });

  it('tras maxFailures fallos seguidos deja de sondear y marca agotado', async () => {
    const fn = vi.fn(async () => {
      throw errorApi(503);
    });
    const { result } = renderHook(() => usePolling(fn, 1000, { isTerminal: () => false }));
    await avanzar(60_000);
    expect(fn).toHaveBeenCalledTimes(5);
    expect(result.current.agotado).toBe(true);
    expect(result.current.fallosSeguidos).toBe(5);
  });

  it('un error isFatal detiene el sondeo al primer fallo', async () => {
    const fn = vi.fn(async () => {
      throw errorApi(404);
    });
    const { result } = renderHook(() =>
      usePolling(fn, 1000, { isTerminal: () => false, isFatal: (e) => e instanceof ApiError && e.status === 404 }),
    );
    await avanzar(10_000);
    expect(fn).toHaveBeenCalledTimes(1);
    expect(result.current.detenido).toBe(true);
  });

  it('respeta Retry-After en 429: la siguiente petición espera max(intervalo, retryAfter)', async () => {
    const fn = vi.fn(async () => {
      throw new ApiError({ status: 429, code: 'rate_limited', message: 'x', requestId: 'r', retryAfter: 30 });
    });
    renderHook(() => usePolling(fn, 3000, { isTerminal: () => false }));
    await vaciar();
    expect(fn).toHaveBeenCalledTimes(1);
    await avanzar(12_000);
    expect(fn).toHaveBeenCalledTimes(1);
    await avanzar(18_000);
    expect(fn).toHaveBeenCalledTimes(2);
  });

  it('Retry-After sin tope: la espera se limita a 300 s', async () => {
    const fn = vi.fn(async () => {
      throw new ApiError({ status: 429, code: 'rate_limited', message: 'x', requestId: 'r', retryAfter: 3600 });
    });
    renderHook(() => usePolling(fn, 3000, { isTerminal: () => false }));
    await vaciar();
    await avanzar(299_000);
    expect(fn).toHaveBeenCalledTimes(1);
    await avanzar(2000);
    expect(fn).toHaveBeenCalledTimes(2);
  });

  it('volver a la pestaña no salta una espera de Retry-After pendiente', async () => {
    const fn = vi.fn(async () => {
      throw new ApiError({ status: 429, code: 'rate_limited', message: 'x', requestId: 'r', retryAfter: 30 });
    });
    renderHook(() => usePolling(fn, 3000, { isTerminal: () => false }));
    await vaciar();
    setVisibilidad('hidden');
    await avanzar(5000);
    setVisibilidad('visible');
    await avanzar(5000);
    expect(fn).toHaveBeenCalledTimes(1);
    await avanzar(20_000);
    expect(fn).toHaveBeenCalledTimes(2);
  });

  it('refrescar tras agotarse reanuda el sondeo', async () => {
    let fallar = true;
    const fn = vi.fn(async () => {
      if (fallar) throw errorApi(503);
      return { v: 1 };
    });
    const { result } = renderHook(() => usePolling(fn, 1000, { isTerminal: () => false }));
    await avanzar(10_000);
    expect(result.current.agotado).toBe(true);
    fallar = false;
    act(() => result.current.refrescar());
    await vaciar();
    expect(result.current.agotado).toBe(false);
    expect(result.current.data).toEqual({ v: 1 });
    await avanzar(1000);
    expect(fn.mock.calls.length).toBeGreaterThan(6);
  });
});
