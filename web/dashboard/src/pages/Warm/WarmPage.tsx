import { ApiError, apiFetch } from '../../api/client';
import { ApiErrorNotice } from '../../api/errors';
import type { components } from '../../api/schema.gen';
import { usePolling } from '../../hooks/usePolling';

type Warm = components['schemas']['WarmState'];

const INTERVALO_MS = 10_000;

/** Color de la insignia por estado. El texto de la etiqueta lleva la información; el color, solo apoyo. */
const COLOR_ESTADO: Record<Warm['state'], 'ok' | 'warn' | 'danger' | 'neutral'> = {
  ready: 'ok',
  dirty: 'warn',
  cuarentena: 'warn',
  'idle-escalado': 'neutral',
};

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
    <main className="page">
      <div className="page-header">
        <div className="page-title">
          <h1>Entorno warm</h1>
          <p className="page-desc">Estado del entorno aislado donde corren las pruebas.</p>
        </div>
      </div>

      {warmNoDisponible && <p role="alert" className="notice notice-danger">Estado del warm no disponible</p>}
      {error !== null && !warmNoDisponible && <ApiErrorNotice error={error} />}
      {error !== null && data !== null && <p className="notice">Mostrando la última lectura correcta.</p>}
      {agotado && (
        <p>
          <button type="button" className="btn btn-secondary" onClick={refrescar}>
            Reintentar
          </button>
        </p>
      )}

      {data !== null && (
        <section aria-label="Tarjeta del warm" className="card">
          <p>
            Estado:{' '}
            <span
              className={`badge badge-${COLOR_ESTADO[data.state]}`}
              data-testid="warm-estado"
              data-state={data.state}
            >
              {ETIQUETAS[data.state]}
            </span>
          </p>
          <p>
            Reset verificado:{' '}
            <span className={`badge badge-${data.reset_verified ? 'ok' : 'warn'}`}>
              {data.reset_verified ? 'sí' : 'no'}
            </span>
          </p>
          {data.state === 'ready' && !data.reset_verified && (
            <p role="alert" className="notice notice-danger">
              El warm está listo, pero el reset no está verificado. No confíes en sus resultados hasta verificarlo.
            </p>
          )}
          <dl>
            <dt>Versión base</dt>
            <dd>
              <code>{data.baseline_version}</code>
            </dd>
            <dt>Warm</dt>
            <dd>
              <code>{data.warm_id}</code>
            </dd>
          </dl>
        </section>
      )}

      <p>
        <button type="button" className="btn btn-secondary" onClick={refrescar}>
          Actualizar
        </button>
      </p>
    </main>
  );
}
