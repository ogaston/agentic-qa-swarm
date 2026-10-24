import type { ReactNode } from 'react';
import { ApiError, apiFetch } from '../../api/client';
import { ApiErrorNotice } from '../../api/errors';
import type { components } from '../../api/schema.gen';
import { useUnaVez } from '../../hooks/useUnaVez';

type Report = components['schemas']['Report'];
type FlowPlan = components['schemas']['FlowPlan'];
type Verdict = NonNullable<components['schemas']['Run']['verdict']>;

const VEREDICTOS: Record<Verdict, { etiqueta: string; descripcion: string; clase: string }> = {
  bug: { etiqueta: 'Veredicto: bug', descripcion: 'Se encontraron fallos de lógica de negocio.', clase: 'badge-danger' },
  'sin-hallazgos': {
    etiqueta: 'Veredicto: sin-hallazgos',
    descripcion: 'Ningún flujo incumplió una invariante.',
    clase: 'badge-ok',
  },
  inconcluso: {
    etiqueta: 'Veredicto: inconcluso',
    descripcion: 'La corrida no permite afirmar ni descartar fallos.',
    clase: 'badge-warn',
  },
};

/** Insignia del veredicto como en el mockup («Veredicto: bug»): el texto dice cuál es; el color solo refuerza. */
export function Veredicto({ verdict }: { verdict: Verdict }) {
  const v = VEREDICTOS[verdict];
  return (
    <span data-veredicto={verdict} className={`badge ${v.clase}`}>
      {v.etiqueta}
    </span>
  );
}

/** Clase de la insignia de método: un color por verbo, con el verbo escrito. */
function claseMetodo(metodo: string): string {
  switch (metodo.toUpperCase()) {
    case 'GET':
      return 'badge-teal';
    case 'POST':
      return 'badge-info';
    case 'PUT':
    case 'PATCH':
      return 'badge-warn';
    case 'DELETE':
      return 'badge-danger';
    default:
      return 'badge-neutral';
  }
}

/** Clase de la insignia de estado esperado por familia (2xx, 4xx, 5xx). */
function claseEsperado(estado: number): string {
  if (estado >= 200 && estado < 300) return 'badge-ok';
  if (estado >= 400 && estado < 500) return 'badge-warn';
  if (estado >= 500) return 'badge-danger';
  return 'badge-neutral';
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

/** Tarjeta del post-mortem: cabecera del mockup y contenido (texto, nunca HTML). */
function TarjetaPostMortem({ verdict, children }: { verdict: Verdict | undefined; children: ReactNode }) {
  return (
    <section aria-label="Informe post-mortem" className="card card-pad">
      <div className="card-head">
        <div>
          <h2 className="card-title">Informe post-mortem</h2>
          <div className="card-sub">
            {verdict !== undefined ? VEREDICTOS[verdict].descripcion : 'Generado por agent-reporter al terminar la corrida.'}
          </div>
        </div>
      </div>
      {children}
    </section>
  );
}

/** Post-mortem: `GET /api/reports/{run}` una sola vez; «Reintentar» es la única petición extra. */
export function PostMortem({ id, activo, verdict }: { id: string; activo: boolean; verdict: Verdict | undefined }) {
  const { data, error, cargando, reintentar } = useUnaVez<Report>(
    () => apiFetch<Report>(`/api/reports/${id}`),
    activo,
  );

  if (!activo) {
    return (
      <TarjetaPostMortem verdict={verdict}>
        <p className="empty">Sin reporte aún.</p>
      </TarjetaPostMortem>
    );
  }
  if (cargando) {
    return (
      <TarjetaPostMortem verdict={verdict}>
        <p>Cargando informe…</p>
      </TarjetaPostMortem>
    );
  }

  if (error !== null) {
    if (error instanceof ApiError && error.status === 404 && error.code === 'report_not_ready') {
      return (
        <TarjetaPostMortem verdict={verdict}>
          <p>
            El informe aún no está disponible.{' '}
            <button type="button" className="btn btn-secondary" onClick={reintentar}>
              Reintentar
            </button>
          </p>
        </TarjetaPostMortem>
      );
    }
    if (error instanceof ApiError && (error.status === 403 || error.status === 404)) {
      return (
        <TarjetaPostMortem verdict={verdict}>
          <p role="alert" className="notice notice-danger">Sin acceso al informe.</p>
        </TarjetaPostMortem>
      );
    }
    if (error instanceof ApiError && (error.status >= 500 || error.status === 0)) {
      return (
        <TarjetaPostMortem verdict={verdict}>
          <AvisoServicio onReintentar={reintentar} />
        </TarjetaPostMortem>
      );
    }
    return (
      <TarjetaPostMortem verdict={verdict}>
        <ApiErrorNotice error={error} />
      </TarjetaPostMortem>
    );
  }

  if (data === null) return null;

  return (
    <TarjetaPostMortem verdict={data.verdict}>
      <p className="report-summary">{data.summary}</p>
      {data.findings.length === 0 ? (
        <p className="empty">No se encontraron hallazgos en esta corrida.</p>
      ) : (
        <div className="findings-list">
          {data.findings.map((f) => (
            <div key={f.finding_id} className="finding-item">
              <div className="finding-head">
                <div className="finding-route">
                  <span className={`badge mono ${claseMetodo(f.method)}`}>{f.method}</span>
                  <code className="finding-path">{f.path}</code>
                </div>
                <span className="mono finding-id">ID: {f.finding_id}</span>
              </div>
              <dl className="kv-list">
                <dt>Invariante violado</dt>
                <dd>{f.invariant}</dd>
                <dt>Causa raíz</dt>
                <dd>{f.root_cause}</dd>
                <dt>Evidencia (copiable)</dt>
                <dd>
                  <ul className="evidence-list">
                    {f.evidence_uris.map((uri, i) => (
                      <li key={`${i}-${uri}`}>
                        <code className="evidence-uri">{uri}</code>
                      </li>
                    ))}
                  </ul>
                </dd>
              </dl>
            </div>
          ))}
        </div>
      )}
    </TarjetaPostMortem>
  );
}

/** Plan de flujos guardado (`GET /api/runs/{id}/plan`), una sola vez. La tarjeta se pliega; cada flujo, también. */
export function PlanFlujos({ id, activo }: { id: string; activo: boolean }) {
  const { data, error, cargando, reintentar } = useUnaVez<FlowPlan>(
    () => apiFetch<FlowPlan>(`/api/runs/${id}/plan`),
    activo,
  );

  let contenido: ReactNode;
  if (!activo) {
    contenido = <p className="empty">El plan se guarda al llegar a «Ensayando».</p>;
  } else if (cargando) {
    contenido = <p>Cargando plan…</p>;
  } else if (error !== null) {
    if (error instanceof ApiError && error.status === 404 && error.code === 'plan_not_ready') {
      contenido = <p>Plan aún no disponible.</p>;
    } else {
      contenido = (
        <>
          <ApiErrorNotice error={error} />
          <p>
            <button type="button" className="btn btn-secondary" onClick={reintentar}>
              Reintentar
            </button>
          </p>
        </>
      );
    }
  } else if (data !== null) {
    contenido = (
      <div className="flow-list">
        {data.flows.map((flow) => (
          <details key={flow.flow_id} className="flow-block flow-details">
            <summary>
              <span className="flow-title">
                <svg className="chevron-icon" viewBox="0 0 24 24" fill="none" aria-hidden="true">
                  <path d="M9 18l6-6-6-6" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
                </svg>
                <strong>{flow.name}</strong>
              </span>
              <span className={`badge ${flow.invariant_probe ? 'badge-accent' : 'badge-ok'}`}>
                {flow.invariant_probe ? 'sondeo de invariante' : 'happy-path'}
              </span>
            </summary>
            <div className="flow-content">
              <p>
                <strong>Invariante:</strong> {flow.invariant}
              </p>
              <p className="flow-id">
                Identificador: <code className="mono">{flow.flow_id}</code>
              </p>
              <div className="table-wrap">
                <table className="data-table">
                  <thead>
                    <tr>
                      <th scope="col">Paso</th>
                      <th scope="col">Método</th>
                      <th scope="col">Ruta</th>
                      <th scope="col">Estado esperado</th>
                      <th scope="col">Solicitud</th>
                    </tr>
                  </thead>
                  <tbody>
                    {flow.steps.map((paso, i) => (
                      <tr key={i}>
                        <td data-label="Paso" className="mono">
                          {i + 1}
                        </td>
                        <td data-label="Método">
                          <span className={`badge mono ${claseMetodo(paso.method)}`}>{paso.method}</span>
                        </td>
                        <td data-label="Ruta">
                          <code>{paso.path}</code>
                        </td>
                        <td data-label="Estado esperado">
                          <span className={`badge mono ${claseEsperado(paso.expect_status)}`}>{paso.expect_status}</span>
                        </td>
                        <td data-label="Solicitud">
                          {paso.body !== undefined ? (
                            <pre className="json-body">{JSON.stringify(paso.body, null, 2)}</pre>
                          ) : (
                            <span className="text-muted">Sin cuerpo</span>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          </details>
        ))}
      </div>
    );
  } else {
    contenido = null;
  }

  return (
    <details className="card card-pad plan-card" open>
      <summary className="plan-summary">
        <span>
          <strong>Plan de flujos</strong>
          <span className="card-sub">Flujos de prueba y sondeo de invariantes de negocio</span>
        </span>
        {data !== null && (
          <span className="badge badge-neutral mono">
            workflow: {data.workflow} · {data.flows.length} flujos
          </span>
        )}
      </summary>
      <div className="plan-body">{contenido}</div>
    </details>
  );
}
