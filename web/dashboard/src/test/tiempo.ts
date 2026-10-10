import { act } from '@testing-library/react';
import { vi } from 'vitest';

// Capturado al cargar el módulo, antes de que las pruebas instalen temporizadores falsos.
const macrotarea = globalThis.setTimeout;

/**
 * Avanza el reloj falso `ms` milisegundos y deja correr las promesas y
 * macrotareas pendientes (MSW resuelve respuestas fuera de la cola de microtareas).
 */
export async function avanzar(ms: number): Promise<void> {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
    for (let i = 0; i < 5; i++) {
      await new Promise<void>((resolver) => macrotarea(() => resolver(), 0));
    }
  });
}

/** Deja correr las respuestas pendientes de MSW sin avanzar el reloj. */
export async function vaciar(): Promise<void> {
  await avanzar(0);
}

export const TIMERS_FALSOS: { toFake: Array<'setTimeout' | 'clearTimeout' | 'setInterval' | 'clearInterval' | 'Date'> } = {
  toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval', 'Date'],
};
