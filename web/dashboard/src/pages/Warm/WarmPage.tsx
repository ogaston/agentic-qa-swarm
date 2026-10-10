import { ApiError, apiFetch } from '../../api/client';
import { ApiErrorNotice } from '../../api/errors';
import type { components } from '../../api/schema.gen';
import { usePolling } from '../../hooks/usePolling';

type Warm = components['schemas']['WarmState'];

const INTERVALO_MS = 10_000;

const ETIQUETAS: Record<Warm['state'], string> = {
  ready: 'Listo',
  dirty: 'Sucio (requiere reset)',
  cuarentena: 'En cuarentena',
  'idle-escalado': 'Inactivo (escalado)',
};

/** Estado del warm (`/warm`). Solo lectura: sondeo cada 10 s, pausado con la pestaña oculta. */
export function WarmPage() {
  const { data, error, agotado, refrescar } = usePolling<Warm>(
    () => apiFetch<Warm>('/api/warm'),
    INTERVALO_MS,
    { isTerminal: () => false },
  );

  const warmNoDisponible = error instanceof ApiError && error.status === 503 && error.code === 'warm_unavailable';

  return (
    <main>
      <h1>Entorno warm</h1>

      {warmNoDisponible && <p role="alert">Estado del warm no disponible</p>}
      {error !== null && !warmNoDisponible && <ApiErrorNotice error={error} />}
      {agotado && (
        <p>
          <button type="button" onClick={refrescar}>
            Reintentar
          </button>
        </p>
      )}

      {data !== null && (
        <section aria-label="Tarjeta del warm">
          <p>
            Estado: <span data-testid="warm-estado" data-state={data.state}>{ETIQUETAS[data.state]}</span>
          </p>
          <p>Reset verificado: {data.reset_verified ? 'sí' : 'no'}</p>
          {data.state === 'ready' && !data.reset_verified && (
            <p role="alert">
              El warm está listo, pero el reset no está verificado. No confíes en sus resultados hasta verificarlo.
            </p>
          )}
          <p>
            Versión base: <code>{data.baseline_version}</code>
          </p>
          <p>
            Warm: <code>{data.warm_id}</code>
          </p>
        </section>
      )}

      <p>
        <button type="button" onClick={refrescar}>
          Actualizar
        </button>
      </p>
    </main>
  );
}
