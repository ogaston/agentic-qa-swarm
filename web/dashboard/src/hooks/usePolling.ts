import { useCallback, useEffect, useRef, useState } from 'react';
import { ApiError } from '../api/client';

export type PollingOptions<T> = {
  /** Estado terminal: al recibirlo el sondeo se detiene. */
  isTerminal: (data: T) => boolean;
  /** Error que detiene el sondeo al primer fallo (p. ej. 404). */
  isFatal?: (error: unknown) => boolean;
  /** Fallos seguidos antes de dejar de sondear. Por defecto 5. */
  maxFailures?: number;
  /** Si es `false`, no hay sondeo. */
  enabled?: boolean;
};

export type PollingResult<T> = {
  data: T | null;
  error: unknown;
  fallosSeguidos: number;
  /** Se alcanzó `maxFailures`: el sondeo está parado hasta `refrescar()`. */
  agotado: boolean;
  /** Parado por estado terminal o por error fatal. */
  detenido: boolean;
  /** Petición inmediata; reanuda el sondeo si estaba agotado. */
  refrescar: () => void;
};

type Estado<T> = {
  data: T | null;
  error: unknown;
  fallosSeguidos: number;
  agotado: boolean;
  detenido: boolean;
};

/** Tope de espera que impone un `Retry-After`, para no quedarse parado horas. */
const ESPERA_MAXIMA_MS = 300_000;

/**
 * Sondeo de una petición cada `intervalMs`. Reglas:
 * - Nunca hay dos peticiones en vuelo; la siguiente se programa al terminar la anterior.
 * - Con la pestaña oculta no se pide nada (`visibilitychange`); al volver se pide de inmediato.
 * - Estado terminal o error fatal → se detiene.
 * - `maxFailures` fallos seguidos → se detiene y queda `agotado` hasta `refrescar()`.
 * - Un `Retry-After` en el error espera `min(300 s, max(intervalMs, retryAfter))` antes de la siguiente
 *   petición; volver a la pestaña respeta esa espera pendiente.
 * - Desmontar no pide más. La petición en curso termina, pero su resultado se descarta.
 *
 * `fn` puede cambiar en cada render; el sondeo usa siempre la última. Para cambiar de
 * recurso, la vista debe remontarse (`key`), porque el efecto solo depende de
 * `intervalMs` y `enabled`.
 */
export function usePolling<T>(
  fn: () => Promise<T>,
  intervalMs: number,
  opciones: PollingOptions<T>,
): PollingResult<T> {
  const fnRef = useRef(fn);
  fnRef.current = fn;
  const opcionesRef = useRef(opciones);
  opcionesRef.current = opciones;
  const ejecutarRef = useRef<(() => void) | null>(null);

  const [estado, setEstado] = useState<Estado<T>>({
    data: null,
    error: null,
    fallosSeguidos: 0,
    agotado: false,
    detenido: false,
  });

  const enabled = opciones.enabled !== false;

  useEffect(() => {
    if (!enabled) return;

    let vivo = true;
    let timer: ReturnType<typeof setTimeout> | null = null;
    let enVuelo = false;
    let fallos = 0;
    let agotado = false;
    let detenido = false;
    // Instante (epoch ms) antes del que no se debe pedir, por un Retry-After.
    let esperaHasta = 0;

    const parar = () => {
      if (timer !== null) clearTimeout(timer);
      timer = null;
    };

    const oculta = () => document.visibilityState === 'hidden';

    const programar = (esperaMs: number = intervalMs) => {
      parar();
      if (!vivo || detenido || agotado || oculta()) return;
      timer = setTimeout(() => void ejecutar(), esperaMs);
    };

    const publicar = (parcial: Partial<Estado<T>>) => {
      if (!vivo) return;
      setEstado((previo) => ({
        ...previo,
        ...parcial,
        fallosSeguidos: fallos,
        agotado,
        detenido,
      }));
    };

    const ejecutar = async () => {
      parar();
      if (!vivo || enVuelo || oculta()) return;
      enVuelo = true;
      try {
        const data = await fnRef.current();
        if (!vivo) return;
        fallos = 0;
        agotado = false;
        detenido = opcionesRef.current.isTerminal(data);
        publicar({ data, error: null });
        programar();
      } catch (error) {
        if (!vivo) return;
        fallos += 1;
        if (opcionesRef.current.isFatal?.(error) === true) {
          detenido = true;
        } else if (fallos >= (opcionesRef.current.maxFailures ?? 5)) {
          agotado = true;
        }
        publicar({ error });
        const retryAfter = error instanceof ApiError ? error.retryAfter : undefined;
        const esperaMs =
          retryAfter === undefined
            ? intervalMs
            : Math.min(ESPERA_MAXIMA_MS, Math.max(intervalMs, retryAfter * 1000));
        esperaHasta = Date.now() + esperaMs;
        programar(esperaMs);
      } finally {
        enVuelo = false;
      }
    };

    const alCambiarVisibilidad = () => {
      if (oculta()) {
        parar();
        return;
      }
      if (detenido || agotado) return;
      const restaMs = esperaHasta - Date.now();
      if (restaMs > 0) programar(restaMs);
      else void ejecutar();
    };

    ejecutarRef.current = () => {
      if (!vivo) return;
      agotado = false;
      detenido = false;
      fallos = 0;
      void ejecutar();
    };

    document.addEventListener('visibilitychange', alCambiarVisibilidad);
    void ejecutar();

    return () => {
      vivo = false;
      parar();
      document.removeEventListener('visibilitychange', alCambiarVisibilidad);
      ejecutarRef.current = null;
    };
  }, [intervalMs, enabled]);

  const refrescar = useCallback(() => {
    ejecutarRef.current?.();
  }, []);

  return {
    data: estado.data,
    error: estado.error,
    fallosSeguidos: estado.fallosSeguidos,
    agotado: estado.agotado,
    detenido: estado.detenido,
    refrescar,
  };
}
