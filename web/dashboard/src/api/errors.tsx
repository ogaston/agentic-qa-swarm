import { useEffect, useMemo, useState } from 'react';
import { ApiError } from './client';

/**
 * Segundos que faltan hasta `endAt` (epoch ms), recalculados cada segundo.
 * Devuelve 0 si no hay espera o ya terminó.
 */
export function useCountdownUntil(endAt: number | null): number {
  const [now, setNow] = useState<number>(() => Date.now());

  useEffect(() => {
    if (endAt === null || endAt <= Date.now()) return;
    setNow(Date.now());
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [endAt]);

  if (endAt === null) return 0;
  return Math.max(0, Math.ceil((endAt - now) / 1000));
}

/**
 * Aviso común de error de la API para las vistas. Nunca muestra el token: solo
 * usa `status`, `code`, `message`, `retryAfter` y `requestId` del `ApiError`.
 */
export function ApiErrorNotice({ error }: { error: unknown }) {
  const retryAfter = error instanceof ApiError ? error.retryAfter : undefined;
  const endAt = useMemo(
    () => (retryAfter === undefined ? null : Date.now() + retryAfter * 1000),
    [retryAfter, error],
  );
  const restante = useCountdownUntil(endAt);

  if (!(error instanceof ApiError)) {
    return (
      <div role="alert">
        <p>Ha ocurrido un error inesperado.</p>
      </div>
    );
  }

  if (error.status === 429) {
    const texto =
      restante > 0
        ? `Demasiados intentos, espera ${restante} s`
        : 'Demasiados intentos. Vuelve a intentarlo en unos instantes.';
    return (
      <div role="alert">
        <p>{texto}</p>
        <p>Referencia para soporte: {error.requestId}</p>
      </div>
    );
  }

  if (error.status === 503 || error.status === 0) {
    const texto =
      restante > 0 ? `Servicio no disponible. Reintenta en ${restante} s` : 'Servicio no disponible';
    return (
      <div role="alert">
        <p>{texto}</p>
        <p>Referencia para soporte: {error.requestId}</p>
      </div>
    );
  }

  return (
    <div role="alert">
      <p>
        <code>{error.code}</code> {error.message}
      </p>
      <p>Referencia para soporte: {error.requestId}</p>
    </div>
  );
}
