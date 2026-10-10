import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { server } from '../test/server';
import { ApiError, apiFetch, setAuthToken, setUnauthorizedHandler } from './client';

const REQUEST_ID_PATTERN = /^[A-Za-z0-9._-]{8,64}$/;

afterEach(() => setAuthToken(null));

describe('apiFetch — cabeceras', () => {
  it('no envía Authorization sin token', async () => {
    let auth: string | null | undefined;
    server.use(
      http.get('*/api/notifications', ({ request }) => {
        auth = request.headers.get('authorization');
        return HttpResponse.json([]);
      }),
    );
    await apiFetch('/api/notifications');
    expect(auth).toBeNull();
  });

  it('envía Authorization: Bearer solo cuando hay token inyectado', async () => {
    let auth: string | null | undefined;
    server.use(
      http.get('*/api/notifications', ({ request }) => {
        auth = request.headers.get('authorization');
        return HttpResponse.json([]);
      }),
    );
    setAuthToken('tok-123');
    await apiFetch('/api/notifications');
    expect(auth).toBe('Bearer tok-123');
  });

  it('envía Accept: application/json y un X-Request-Id con el patrón del contrato', async () => {
    let accept: string | null = null;
    let requestId: string | null = null;
    server.use(
      http.get('*/api/notifications', ({ request }) => {
        accept = request.headers.get('accept');
        requestId = request.headers.get('x-request-id');
        return HttpResponse.json([]);
      }),
    );
    await apiFetch('/api/notifications');
    expect(accept).toBe('application/json');
    expect(requestId).not.toBeNull();
    expect(requestId).toMatch(REQUEST_ID_PATTERN);
  });

  it('genera un X-Request-Id distinto en cada llamada', async () => {
    const ids: string[] = [];
    server.use(
      http.get('*/api/notifications', ({ request }) => {
        ids.push(request.headers.get('x-request-id') ?? '');
        return HttpResponse.json([]);
      }),
    );
    await apiFetch('/api/notifications');
    await apiFetch('/api/notifications');
    expect(ids[0]).not.toBe(ids[1]);
  });
});

describe('apiFetch — respuestas y errores', () => {
  it('devuelve el cuerpo JSON en 2xx', async () => {
    server.use(http.get('*/api/notifications', () => HttpResponse.json([{ id: 'n1' }])));
    await expect(apiFetch('/api/notifications')).resolves.toEqual([{ id: 'n1' }]);
  });

  it('un 4xx con cuerpo Error se convierte en ApiError con code, message y status', async () => {
    server.use(
      http.get('*/api/runs/r1', () =>
        HttpResponse.json({ code: 'run_not_found', message: 'no existe' }, { status: 404 }),
      ),
    );
    const err = await apiFetch('/api/runs/r1').catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 404, code: 'run_not_found', message: 'no existe' });
  });

  it('un 429 con Retry-After produce retryAfter numérico', async () => {
    server.use(
      http.get('*/api/audit', () =>
        HttpResponse.json(
          { code: 'rate_limited', message: 'despacio' },
          { status: 429, headers: { 'Retry-After': '7' } },
        ),
      ),
    );
    const err = await apiFetch('/api/audit').catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 429, code: 'rate_limited', retryAfter: 7 });
  });

  it('un cuerpo no JSON produce code unexpected_response', async () => {
    server.use(
      http.get('*/api/warm', () => new HttpResponse('<html>502</html>', { status: 502 })),
    );
    const err = await apiFetch('/api/warm').catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 502, code: 'unexpected_response' });
  });

  it('hace exactamente una petición por llamada, también si falla (sin reintentos)', async () => {
    let hits = 0;
    server.use(
      http.get('*/api/warm', () => {
        hits += 1;
        return HttpResponse.json({ code: 'boom', message: 'x' }, { status: 503 });
      }),
    );
    await apiFetch('/api/warm').catch(() => undefined);
    expect(hits).toBe(1);
  });
});

describe('apiFetch — fallo de red', () => {
  it('si fetch rechaza, lanza ApiError con code network_error y status 0', async () => {
    server.use(http.get('*/api/notifications', () => HttpResponse.error()));
    const err = await apiFetch('/api/notifications').catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 0, code: 'network_error' });
  });
});

describe('apiFetch — manejador de 401 global', () => {
  it('llama al manejador solo si la petición llevaba token, y sigue lanzando ApiError', async () => {
    const manejador = vi.fn();
    setUnauthorizedHandler(manejador);
    try {
      server.use(
        http.get('/api/x', () => HttpResponse.json({ code: 'unauthorized', message: 'm' }, { status: 401 })),
      );
      await expect(apiFetch('/api/x')).rejects.toMatchObject({ status: 401 });
      expect(manejador).not.toHaveBeenCalled();
      setAuthToken('t');
      await expect(apiFetch('/api/x')).rejects.toMatchObject({ status: 401 });
      expect(manejador).toHaveBeenCalledTimes(1);
    } finally {
      setAuthToken(null);
      setUnauthorizedHandler(null);
    }
  });
});
