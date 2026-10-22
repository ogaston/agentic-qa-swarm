import type { ReactNode } from 'react';
import { ApiError, apiFetch } from '../../api/client';
import { ApiErrorNotice } from '../../api/errors';
import type { components } from '../../api/schema.gen';
import { useUnaVez } from '../../hooks/useUnaVez';

type Report = components['schemas']['Report'];
type FlowPlan = components['schemas']['FlowPlan'];
type Verdict = NonNullable<components['schemas']['Run']['verdict']>;

const VEREDICTOS: Record<Verdict, { etiqueta: string; descripcion: string }> = {
  bug: { etiqueta: 'Bug', descripcion: 'Se encontraron fallos de lógica de negocio.' },
  'sin-hallazgos': { etiqueta: 'Sin hallazgos', descripcion: 'Ningún flujo incumplió una invariante.' },
  inconcluso: { etiqueta: 'Inconcluso', descripcion: 'La corrida no permite afirmar ni descartar fallos.' },
};

const COLOR_VEREDICTO: Record<Verdict, 'danger' | 'ok' | 'warn'> = {
  bug: 'danger',
  'sin-hallazgos': 'ok',
  inconcluso: 'warn',
};

/** Veredicto con texto e insignia; el color no es lo único que lo distingue. */
export function Veredicto({ verdict }: { verdict: Verdict }) {
  const v = VEREDICTOS[verdict];
  return (
    <section aria-label="Veredicto" className="card verdict">
      <p>
        Veredicto:{' '}
        <strong data-veredicto={verdict} className={`badge badge-${COLOR_VEREDICTO[verdict]}`}>
          {v.etiqueta}
        </strong>
      </p>
      <p className="text-muted">{v.descripcion}</p>
    </section>
  );
}

/** Errores que se muestran igual en el informe y en el plan: aviso sin reintento automático. */
function AvisoServicio({ onReintentar }: { onReintentar: () => void }) {
  return (
    <p>
      <span role="alert" className="notice notice-danger">No se pudo cargar el informe. El servicio no está disponible.</span>{' '}
      <button type="button" className="btn btn-secondary" onClick={onReintentar}>
        Reintentar
      </button>
    </p>
  );
}

/** Post-mortem: `GET /api/reports/{run}` una sola vez; «Reintentar» es la única petición extra. */
export function PostMortem({ id, activo }: { id: string; activo: boolean }) {
  const { data, error, cargando, reintentar } = useUnaVez<Report>(
    () => apiFetch<Report>(`/api/reports/${id}`),
    activo,
  );

  if (!activo) return null;
  if (cargando) return <p>Cargando informe…</p>;

  if (error !== null) {
    if (error instanceof ApiError && error.status === 404 && error.code === 'report_not_ready') {
      return (
        <p>
          El informe aún no está disponible.{' '}
          <button type="button" className="btn btn-secondary" onClick={reintentar}>
            Reintentar
          </button>
        </p>
      );
    }
    if (error instanceof ApiError && (error.status === 403 || error.status === 404)) {
      return <p role="alert" className="notice notice-danger">Sin acceso al informe.</p>;
    }
    if (error instanceof ApiError && (error.status >= 500 || error.status === 0)) {
      return <AvisoServicio onReintentar={reintentar} />;
    }
    return <ApiErrorNotice error={error} />;
  }

  if (data === null) return null;

  return (
    <section aria-label="Post-mortem" className="card">
      <h2>Post-mortem</h2>
      <p>{data.summary}</p>
      {data.findings.length === 0 ? (
        <p className="empty">No se encontraron hallazgos en esta corrida.</p>
      ) : (
        <ol className="findings-list">
          {data.findings.map((f) => (
            <li key={f.finding_id} className="finding-card">
              <p>
                <code className="mono">
                  {f.method} {f.path}
                </code>
              </p>
              <p>Invariante: {f.invariant}</p>
              <p>Causa raíz: {f.root_cause}</p>
              <p>Evidencias:</p>
              <ul>
                {f.evidence_uris.map((uri, i) => (
                  <li key={`${i}-${uri}`}>
                    <code className="mono">{uri}</code>
                  </li>
                ))}
              </ul>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}

/** Plan de flujos guardado (`GET /api/runs/{id}/plan`), una sola vez, plegable. */
export function PlanFlujos({ id, activo }: { id: string; activo: boolean }) {
  const { data, error, cargando, reintentar } = useUnaVez<FlowPlan>(
    () => apiFetch<FlowPlan>(`/api/runs/${id}/plan`),
    activo,
  );

  if (!activo) return null;

  let contenido: ReactNode;
  if (cargando) {
    contenido = <p>Cargando plan…</p>;
  } else if (error !== null) {
    if (error instanceof ApiError && error.status === 404 && error.code === 'plan_not_ready') {
      contenido = <p>Plan aún no disponible.</p>;
    } else {
      contenido = (
        <>
          <ApiErrorNotice error={error} />
          <p>
            <button type="button" onClick={reintentar}>
              Reintentar
            </button>
          </p>
        </>
      );
    }
  } else if (data !== null) {
    contenido = (
      <ol className="flow-list">
        {data.flows.map((flow) => (
          <li key={flow.flow_id}>
            <p>
              <strong>{flow.name}</strong> <code className="mono">{flow.flow_id}</code>
              {flow.invariant_probe ? ' (sondeo)' : ''}
            </p>
            <p>Invariante: {flow.invariant}</p>
            <ol>
              {flow.steps.map((paso, i) => (
                <li key={i}>
                  <code className="mono">
                    {paso.method} {paso.path}
                  </code>{' '}
                  (espera {paso.expect_status})
                  {paso.body !== undefined && <pre className="mono code-block">{JSON.stringify(paso.body, null, 2)}</pre>}
                </li>
              ))}
            </ol>
          </li>
        ))}
      </ol>
    );
  } else {
    contenido = null;
  }

  return (
    <details className="card details">
      <summary>Flujos ejecutados</summary>
      {contenido}
    </details>
  );
}
