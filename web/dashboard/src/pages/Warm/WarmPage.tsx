import type { ReactNode } from 'react';
import { ApiError, apiFetch } from '../../api/client';
import { ApiErrorNotice } from '../../api/errors';
import type { components } from '../../api/schema.gen';
import { PageHeader } from '../../components/PageHeader';
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

/** Trazo del icono por estado (decorativo: el texto de la etiqueta es el significado). */
const ICONO_ESTADO: Record<Warm['state'], ReactNode> = {
  ready: <path d="M5 12.5l4.2 4.2L19 7" />,
  dirty: (
    <>
      <path d="M12 4l9 16H3z" />
      <path d="M12 10v4" />
      <path d="M12 17h.01" />
    </>
  ),
  cuarentena: <path d="M12 3l8 3v6c0 4.5-3.4 8-8 9-4.6-1-8-4.5-8-9V6z" />,
  'idle-escalado': <path d="M20 14.5A8 8 0 0 1 9.5 4 8 8 0 1 0 20 14.5z" />,
};

function IconoEstado({ estado }: { estado: Warm['state'] }) {
  return (
    <svg
      viewBox="0 0 24 24"
      width="14"
      height="14"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      {ICONO_ESTADO[estado]}
    </svg>
  );
}

/** Estado del warm (`/warm`). Solo lectura: sondeo cada 10 s, pausado con la pestaña oculta. */
export function WarmPage() {
  const { data, error, agotado, refrescar } = usePolling<Warm>(
    () => apiFetch<Warm>('/api/warm'),
    INTERVALO_MS,
    { isTerminal: () => false },
  );

  const warmNoDisponible = error instanceof ApiError && error.status === 503 && error.code === 'warm_unavailable';

  return (
    <main className="az-container">
      <PageHeader
        titulo="Entorno"
        descripcion={
          <>
            Estado en vivo del namespace aislado <span className="mono">aqs-test</span> (<code>GET /api/warm</code>,
            sondeo cada 10 s en solo lectura).
          </>
        }
        acciones={
          <button type="button" className="btn btn-primary" onClick={refrescar}>
            Actualizar
          </button>
        }
      />

      {warmNoDisponible && (
        <div role="alert" className="alert-box alert-danger">
          Estado del warm no disponible
        </div>
      )}
      {error !== null && !warmNoDisponible && <ApiErrorNotice error={error} />}
      {error !== null && data !== null && <p className="notice">Mostrando la última lectura correcta.</p>}
      {agotado && (
        <p>
          <button type="button" className="btn btn-secondary" onClick={refrescar}>
            Reintentar
          </button>
        </p>
      )}

      {data !== null && data.state === 'ready' && !data.reset_verified && (
        <div role="alert" className="alert-box alert-warn">
          El entorno está listo, pero el reset no está verificado. No confíes en sus resultados hasta verificarlo.
        </div>
      )}

      {data !== null && (
        <section aria-label="Tarjeta del entorno" className="card card-pad">
          <div className="card-head">
            <div>
              <h2 className="card-title">
                Estado operativo de <code>{data.warm_id}</code>
              </h2>
              <div className="card-sub">
                Pre-desplegado (App + Postgres + Redis) y reutilizado entre corridas con gate obligatorio{' '}
                <code>reset_verified=true</code>.
              </div>
            </div>
            <span className={`badge badge-${COLOR_ESTADO[data.state]}`} data-testid="warm-estado" data-state={data.state}>
              <IconoEstado estado={data.state} />
              {ETIQUETAS[data.state]}
            </span>
          </div>
          <div className="kpi-strip kpi-sin-margen">
            <div className="card kpi-box kpi-fondo">
              <div className="kpi-label">
                Estado (<code>state</code>)
              </div>
              <div className="kpi-row">
                <span className="kpi-val kpi-valor-20">{ETIQUETAS[data.state]}</span>
              </div>
            </div>
            <div className="card kpi-box kpi-fondo">
              <div className="kpi-label">
                Reset verificado (<code>reset_verified</code>)
              </div>
              <div className="kpi-row">
                <span className="kpi-val kpi-valor-20 mono">{data.reset_verified ? 'sí' : 'no'}</span>
              </div>
            </div>
            <div className="card kpi-box kpi-fondo">
              <div className="kpi-label">
                Versión base (<code>baseline_version</code>)
              </div>
              <div className="kpi-row">
                <span className="kpi-val kpi-valor-18 mono">{data.baseline_version}</span>
              </div>
            </div>
            <div className="card kpi-box kpi-fondo">
              <div className="kpi-label">Identificador del entorno</div>
              <div className="kpi-row">
                <span className="kpi-val kpi-valor-18 mono">{data.warm_id}</span>
              </div>
            </div>
          </div>
        </section>
      )}
    </main>
  );
}
