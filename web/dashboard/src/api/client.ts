import type { components } from './schema.gen';

type ErrorBody = components['schemas']['Error'];

/**
 * Error de la API. Se lanza para cualquier respuesta no 2xx y para fallos de
 * red. `code` sale del cuerpo `Error` del contrato; si el cuerpo no es un
 * `Error` JSON válido, vale `unexpected_response` (o `network_error` si la
 * petición nunca llegó al servidor).
 */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly retryAfter?: number;
  readonly requestId: string;

  constructor(init: {
    status: number;
    code: string;
    message: string;
    requestId: string;
    retryAfter?: number;
  }) {
    super(init.message);
    this.name = 'ApiError';
    this.status = init.status;
    this.code = init.code;
    this.requestId = init.requestId;
    if (init.retryAfter !== undefined) this.retryAfter = init.retryAfter;
  }
}

// Token en memoria del módulo. Se inyecta desde fuera (U6-T03 lo guarda en
// memoria de la pestaña); este módulo nunca lo persiste.
let authToken: string | null = null;

export function setAuthToken(token: string | null): void {
  authToken = token;
}

// Manejador global de 401: la sesión lo registra para borrar el estado y redirigir.
// Solo se invoca si la petición llevaba token (un 401 de login sin sesión es un
// fallo de credenciales, no una sesión caducada). El error se sigue lanzando.
let unauthorizedHandler: (() => void) | null = null;

export function setUnauthorizedHandler(handler: (() => void) | null): void {
  unauthorizedHandler = handler;
}

function newRequestId(): string {
  // UUID v4 (36 caracteres): cumple ^[A-Za-z0-9._-]{8,64}$.
  return globalThis.crypto.randomUUID();
}

function parseRetryAfter(value: string | null): number | undefined {
  if (value === null) return undefined;
  const trimmed = value.trim();
  if (/^\d+$/.test(trimmed)) return Number(trimmed);
  const date = Date.parse(trimmed);
  if (Number.isNaN(date)) return undefined;
  return Math.max(0, Math.ceil((date - Date.now()) / 1000));
}

function isErrorBody(value: unknown): value is ErrorBody {
  if (typeof value !== 'object' || value === null) return false;
  const v = value as Record<string, unknown>;
  return typeof v['code'] === 'string' && typeof v['message'] === 'string';
}

/**
 * Llama a la API con una sola petición (sin reintentos). `path` es relativo y
 * empieza por `/api/`; el proxy de Vite quita ese prefijo.
 */
export async function apiFetch<T>(path: string, init: RequestInit = {}): Promise<T> {
  const requestId = newRequestId();
  const sentWithToken = authToken !== null;
  const headers = new Headers(init.headers);
  headers.set('Accept', 'application/json');
  headers.set('X-Request-Id', requestId);
  if (authToken !== null) headers.set('Authorization', `Bearer ${authToken}`);

  // fetch de Node no resuelve URLs relativas; en el navegador esto equivale a la ruta relativa.
  const base = typeof location !== 'undefined' ? location.href : 'http://localhost/';
  const url = new URL(path, base).toString();

  let response: Response;
  try {
    response = await fetch(url, { ...init, headers });
  } catch (cause) {
    throw new ApiError({
      status: 0,
      code: 'network_error',
      message: cause instanceof Error ? cause.message : 'fallo de red',
      requestId,
    });
  }

  const responseRequestId = response.headers.get('X-Request-Id') ?? requestId;
  const text = await response.text();
  let body: unknown = undefined;
  let jsonOk = true;
  if (text.length > 0) {
    try {
      body = JSON.parse(text);
    } catch {
      jsonOk = false;
    }
  }

  if (response.ok) {
    if (!jsonOk) {
      throw new ApiError({
        status: response.status,
        code: 'unexpected_response',
        message: 'la respuesta no es JSON',
        requestId: responseRequestId,
      });
    }
    return body as T;
  }

  const retryAfter = parseRetryAfter(response.headers.get('Retry-After'));
  const withRetry = retryAfter === undefined ? {} : { retryAfter };

  if (response.status === 401 && sentWithToken && unauthorizedHandler !== null) {
    unauthorizedHandler();
  }

  if (jsonOk && isErrorBody(body)) {
    throw new ApiError({
      status: response.status,
      code: body.code,
      message: body.message,
      requestId: responseRequestId,
      ...withRetry,
    });
  }

  throw new ApiError({
    status: response.status,
    code: 'unexpected_response',
    message: `HTTP ${response.status} sin cuerpo Error válido`,
    requestId: responseRequestId,
    ...withRetry,
  });
}
