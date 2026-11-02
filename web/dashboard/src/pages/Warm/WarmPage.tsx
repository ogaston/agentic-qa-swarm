import type { ReactNode } from 'react';
import { ApiError, apiFetch } from '../../api/client';
import { ApiErrorNotice } from '../../api/errors';
import type { components } from '../../api/schema.gen';
import { Kpi } from '../../components/Kpi';
import { PageHeader } from '../../components/PageHeader';
import { usePolling } from '../../hooks/usePolling';

type Warm = components['schemas']['WarmState'];

const INTERVALO_MS = 10_000;

/** Color de la insignia por estado. El texto de la etiqueta lleva la información; el color, solo apoyo. */
const COLOR_ESTADO: Record<Warm['state'], 'ok' | 'warn' | 'neutral'> = {
  ready: 'ok',
  dirty: 'warn',
  cuarentena: 'warn',
  'idle-escalado': 'neutral',
  provisioning: 'neutral',
  deleting: 'warn',
};

const ETIQUETAS: Record<Warm['state'], string> = {
  ready: 'Listo',
  dirty: 'Requiere limpieza',
  cuarentena: 'En cuarentena',
  'idle-escalado': 'En pausa',
  provisioning: 'Aprovisionando',
  deleting: 'Eliminando',
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
  provisioning: <path d="M12 6v6l4 2" />,
  deleting: <path d="M6 18L18 6M6 6l12 12" />,
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
    <main className="az-container warm-page">
      <PageHeader
        titulo="Entorno"
        descripcion="Consulta si tu entorno de pruebas está disponible para la próxima corrida."
        acciones={
          <button type="button" className="btn btn-primary" data-testid="warm-refresh-button" onClick={refrescar}>
            Actualizar
          </button>
        }
      />

      {warmNoDisponible && (
        <div role="alert" className="alert-box alert-danger">
          No pudimos consultar el estado del entorno. Intenta actualizar en unos momentos.
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

      {data === null && error === null && <p role="status">Consultando tu entorno…</p>}

      {data !== null && data.state === 'ready' && !data.reset_verified && (
        <div role="alert" className="alert-box alert-warn">
          La limpieza del entorno aún no está verificada. Espera su verificación antes de confiar en los resultados.
        </div>
      )}

      {data !== null && (
        <section aria-label="Tarjeta del entorno" className="card card-pad">
          <div className="card-head">
            <div>
              <h2 className="card-title">Tu entorno de pruebas</h2>
              <div className="card-sub">Un espacio separado de producción para probar los cambios de tu equipo.</div>
            </div>
            <span className={`badge badge-${COLOR_ESTADO[data.state]}`} data-testid="warm-estado" data-state={data.state}>
              <IconoEstado estado={data.state} />
              {ETIQUETAS[data.state]}
            </span>
          </div>
          <div className="kpi-strip kpi-sin-margen dashboard-environment-grid">
            <Kpi
              etiqueta="Disponibilidad"
              valor={ETIQUETAS[data.state]}
              tono={data.state === 'ready' && !data.reset_verified ? 'warn' : COLOR_ESTADO[data.state]}
              claseValor="kpi-valor-20"
              descripcion={data.state === 'ready' ? 'El entorno está preparado para recibir pruebas.' : data.state === 'dirty' ? 'Necesita una limpieza antes de volver a usarse.' : data.state === 'cuarentena' ? 'Está apartado temporalmente y requiere revisión.' : 'Está en pausa para ahorrar recursos.'}
            />
            <Kpi
              etiqueta="Limpieza verificada"
              valor={data.reset_verified ? 'Verificada' : 'Pendiente'}
              tono={data.reset_verified ? 'ok' : 'warn'}
              claseValor="kpi-valor-20"
              descripcion={data.reset_verified ? 'Se verificó que no quedan datos de las pruebas anteriores.' : 'Aún no se ha confirmado la limpieza de las pruebas anteriores.'}
            />
          </div>
        </section>
      )}
    </main>
  );
}
