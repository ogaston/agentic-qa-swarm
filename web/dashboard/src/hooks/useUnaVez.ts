import { useCallback, useEffect, useRef, useState } from 'react';

export type UnaVez<T> = {
  data: T | null;
  error: unknown;
  cargando: boolean;
  /** Petición inmediata y única: solo la dispara el usuario (p. ej. «Reintentar»). */
  reintentar: () => void;
};

type Estado<T> = { data: T | null; error: unknown; cargando: boolean };

/**
 * Una petición que se hace una sola vez, cuando `activo` pasa a `true`, y no se repite
 * sola (no entra en ningún sondeo). Solo `reintentar()` pide de nuevo.
 * Las respuestas que llegan tras desmontar se descartan.
 */
export function useUnaVez<T>(fn: () => Promise<T>, activo: boolean): UnaVez<T> {
  const fnRef = useRef(fn);
  fnRef.current = fn;
  const pidioRef = useRef(false);
  const vivoRef = useRef(false);
  const [estado, setEstado] = useState<Estado<T>>({ data: null, error: null, cargando: false });

  const pedir = useCallback(() => {
    pidioRef.current = true;
    setEstado((previo) => ({ ...previo, cargando: true }));
    fnRef.current().then(
      (data) => {
        if (vivoRef.current) setEstado({ data, error: null, cargando: false });
      },
      (error: unknown) => {
        if (vivoRef.current) setEstado({ data: null, error, cargando: false });
      },
    );
  }, []);

  useEffect(() => {
    vivoRef.current = true;
    return () => {
      vivoRef.current = false;
    };
  }, []);

  useEffect(() => {
    if (activo && !pidioRef.current) pedir();
  }, [activo, pedir]);

  return { data: estado.data, error: estado.error, cargando: estado.cargando, reintentar: pedir };
}
