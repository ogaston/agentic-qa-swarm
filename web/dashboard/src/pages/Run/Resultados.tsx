import { useEffect, useRef, useState, type ReactNode } from 'react';
import { ApiError, apiFetch, downloadEvidence } from '../../api/client';
import type { components } from '../../api/schema.gen';
import { useUnaVez } from '../../hooks/useUnaVez';

type Report = components['schemas']['Report'];
type FlowPlan = components['schemas']['FlowPlan'];
type Verdict = NonNullable<components['schemas']['Run']['verdict']>;

const VEREDICTOS: Record<Verdict, { etiqueta: string; descripcion: string; clase: string }> = {
  bug: { etiqueta: 'Fallos encontrados', descripcion: 'Se encontraron fallos de lógica de negocio.', clase: 'badge-danger' },
  'sin-hallazgos': {
    etiqueta: 'Sin hallazgos',
    descripcion: 'No se detectaron fallos en los flujos probados.',
    clase: 'badge-ok',
  },
  inconcluso: {
    etiqueta: 'Resultado inconcluso',
    descripcion: 'La corrida no permite afirmar ni descartar fallos.',
    clase: 'badge-warn',
  },
};

/** Traduce referencias operativas conocidas en los textos del informe, sin modificar las rutas probadas. */
function textoCliente(texto: string): string {
  return texto
    .replace(/\b(?:go-run-controller|agent-reporter|agent-planner|agent-runner|agent-inferer|agent-rehearsal)\b/gi, 'servicio de pruebas')
    .replace(/\b(?:MinIO|Kubernetes|Flux|Rego)\b/gi, 'plataforma de pruebas')
    .replace(/(?:s3|http|https):\/\/[^\s]*\.svc(?:\.cluster\.local)?[^\s]*|s3:\/\/[^\s]+/gi, 'referencia de evidencia');
}

/** Insignia del veredicto como en el mockup («Veredicto: bug»): el texto dice cuál es; el color solo refuerza. */
export function Veredicto({ verdict }: { verdict: Verdict }) {
  const v = VEREDICTOS[verdict];
  return (
    <span data-veredicto={verdict} className={`badge ${v.clase}`}>
      <span className="badge-dot" aria-hidden="true" />
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
function TarjetaPostMortem({
  verdict,
  hallazgos,
  children,
}: {
  id: string;
  verdict: Verdict | undefined;
  /** Número de hallazgos del informe cargado: si está, la cabecera muestra «veredicto (N hallazgos)» como el mockup. */
  hallazgos?: number;
  children: ReactNode;
}) {
  return (
    <section aria-label="Resultados de la prueba" className="card card-pad">
      <div className="card-head">
        <div>
          <h2 className="card-title">
            Resultados de la prueba
          </h2>
          <div className="card-sub">
            {verdict !== undefined ? VEREDICTOS[verdict].descripcion : 'El informe estará disponible cuando termine la prueba.'}
          </div>
        </div>
        {verdict !== undefined && hallazgos !== undefined && (
          <span className={`badge ${VEREDICTOS[verdict].clase}`}>
            <span className="badge-dot" aria-hidden="true" />
            {`${hallazgos} ${hallazgos === 1 ? 'hallazgo' : 'hallazgos'}`}
          </span>
        )}
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
      <TarjetaPostMortem id={id} verdict={verdict}>
        <p className="empty">Los resultados estarán disponibles al finalizar la prueba.</p>
      </TarjetaPostMortem>
    );
  }
  if (cargando) {
    return (
      <TarjetaPostMortem id={id} verdict={verdict}>
        <p>Cargando informe…</p>
      </TarjetaPostMortem>
    );
  }

  if (error !== null) {
    if (error instanceof ApiError && error.status === 404 && error.code === 'report_not_ready') {
      return (
        <TarjetaPostMortem id={id} verdict={verdict}>
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
        <TarjetaPostMortem id={id} verdict={verdict}>
          <p role="alert" className="notice notice-danger">Sin acceso al informe.</p>
        </TarjetaPostMortem>
      );
    }
    if (error instanceof ApiError && (error.status >= 500 || error.status === 0)) {
      return (
        <TarjetaPostMortem id={id} verdict={verdict}>
          <AvisoServicio onReintentar={reintentar} />
        </TarjetaPostMortem>
      );
    }
    return (
      <TarjetaPostMortem id={id} verdict={verdict}>
        <RunErrorNotice error={error} />
      </TarjetaPostMortem>
    );
  }

  if (data === null) return null;

  return (
    <TarjetaPostMortem id={id} verdict={data.verdict} hallazgos={data.findings.length}>
      <p className="report-summary">{textoCliente(data.summary)}</p>
      {data.findings.length > 0 && (
        <div className="findings-list">
          {data.findings.map((f, findingIndex) => (
            <div key={f.finding_id} className="finding-item">
              <div className="finding-head">
                <div className="finding-route">
                  <span className={`badge mono ${claseMetodo(f.method)}`}>{f.method}</span>
                  <code className="finding-path">{f.path}</code>
                </div>
                <span className="mono finding-id">ID: {f.finding_id}</span>
              </div>
              <dl className="kv-list">
                <dt>Regla de negocio incumplida</dt>
                <dd>{textoCliente(f.invariant)}</dd>
                <dt>Causa del fallo</dt>
                <dd>{textoCliente(f.root_cause)}</dd>
                <dt>Evidencias</dt>
                <dd>
                  <ul className="evidence-list">
                    {f.evidence_uris.map((uri, i) => (
                      <li key={`${i}-${uri}`}>
                        <ReferenciaEvidencia uri={uri} indice={i} runId={id} findingIndex={findingIndex} />
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

/**
 * Plan de flujos guardado (`GET /api/runs/{id}/plan`), una sola vez. Región «Plan de flujos» con su encabezado
 * de estado (mockup: «Flujos inferidos», «Flujos en ejecución», «Flujos ejecutados»). La tarjeta se pliega; cada flujo, también.
 * Mientras la corrida está activa, plan_not_ready se reintenta con un presupuesto acotado.
 */
export function PlanFlujos({ id, activo, encabezado, automatico = false, enEjecucion = false }: {
  id: string; activo: boolean; encabezado: string; automatico?: boolean; enEjecucion?: boolean;
}) {
  const intentos = useRef(0);
  const { data, error, cargando, reintentar } = useUnaVez<FlowPlan>(
    () => {
      intentos.current += 1;
      return apiFetch<FlowPlan>(`/api/runs/${id}/plan`);
    },
    activo,
  );
  const planPendiente = error instanceof ApiError && error.status === 404 && error.code === 'plan_not_ready';
  const [resultados, setResultados] = useState<Record<string, 'passed' | 'failed' | 'timeout'>>({});
  useEffect(() => {
    if (!activo || data === null) return;
    let vigente = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const consultar = async () => {
      const respuestas = await Promise.all(data.flows.map(async (flow) => {
        try {
          const resultado = await apiFetch<{ status?: string }>(`/api/run-evidence/${encodeURIComponent(id)}/${encodeURIComponent(flow.flow_id)}/result.json`);
          return [flow.flow_id, resultado.status] as const;
        } catch { return [flow.flow_id, undefined] as const; }
      }));
      if (!vigente) return;
      const disponibles: Record<string, 'passed' | 'failed' | 'timeout'> = {};
      for (const [flow, status] of respuestas) if (status === 'passed' || status === 'failed' || status === 'timeout') disponibles[flow] = status;
      setResultados(disponibles);
      if (automatico) timer = setTimeout(consultar, 3000);
    };
    void consultar();
    return () => { vigente = false; if (timer !== undefined) clearTimeout(timer); };
  }, [activo, automatico, data, id]);
  const estabaEjecutando = useRef(false);
  useEffect(() => {
    const iniciaEjecucion = enEjecucion && !estabaEjecutando.current;
    estabaEjecutando.current = enEjecucion;
    // Un plan pendiente durante el ensayo se consulta de inmediato al comenzar la ejecución.
    if (iniciaEjecucion && activo && planPendiente && !cargando) {
      intentos.current = 0;
      reintentar();
    }
  }, [enEjecucion, activo, planPendiente, cargando, reintentar]);
  useEffect(() => {
    if (!activo || !automatico || cargando || !planPendiente || intentos.current >= 5) return;
    const timer = setTimeout(reintentar, 3000);
    return () => clearTimeout(timer);
  }, [activo, automatico, cargando, error, planPendiente, reintentar]);
  const reintentarPlan = () => {
    intentos.current = 0;
    reintentar();
  };

  let contenido: ReactNode;
  if (!activo) {
    contenido = <p className="empty">Los flujos aparecerán cuando se preparen las pruebas.</p>;
  } else if (cargando) {
    contenido = <p>Cargando plan…</p>;
  } else if (error !== null) {
    if (error instanceof ApiError && error.status === 404 && error.code === 'plan_not_ready') {
      contenido = <p>
        <span role="status">Plan aún no disponible.</span>{' '}
        {automatico && intentos.current < 5
          ? 'Las pruebas aparecerán automáticamente cuando estén listas.'
          : <button type="button" className="btn btn-secondary" data-testid="run-plan-retry-button" onClick={reintentarPlan}>Reintentar</button>}
      </p>;
    } else {
      contenido = (
        <>
          <RunErrorNotice error={error} />
          <p>
            <button type="button" className="btn btn-secondary" data-testid="run-plan-retry-button" onClick={reintentarPlan}>
              Reintentar
            </button>
          </p>
        </>
      );
    }
  } else if (data !== null) {
    contenido = (
      <div className="flow-list">
        {[
          { title: 'Flujos principales', flows: data.flows.filter((flow) => !flow.invariant_probe) },
          { title: 'Reglas de negocio', flows: data.flows.filter((flow) => flow.invariant_probe) },
        ].filter((group) => group.flows.length > 0).map((group) => (
          <section className="flow-group" key={group.title} aria-label={group.title}>
            <h3 className="flow-group-title">{group.title}</h3>
            <div className="flow-group-list">
              {group.flows.map((flow) => (
          <div className="flow-item" key={flow.flow_id}>
          <details className="flow-block flow-details">
            <summary>
              <span className="flow-title">
                <svg className="chevron-icon" viewBox="0 0 24 24" fill="none" aria-hidden="true">
                  <path d="M9 18l6-6-6-6" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
                </svg>
                <strong>
                  {textoCliente(flow.name)}
                </strong>
              </span>
              <span className={`badge flow-status ${resultados[flow.flow_id] === 'passed' ? 'badge-ok' : resultados[flow.flow_id] === 'failed' ? 'badge-danger' : resultados[flow.flow_id] === 'timeout' ? 'badge-warn' : 'badge-neutral'}`}>
                {resultados[flow.flow_id] === 'passed' && <span aria-hidden="true">✓</span>}
                {resultados[flow.flow_id] === 'failed' && <span aria-hidden="true">✕</span>}
                {resultados[flow.flow_id] === 'passed' ? 'Aprobada' : resultados[flow.flow_id] === 'failed' ? 'Fallida' : resultados[flow.flow_id] === 'timeout' ? 'Tiempo agotado' : 'Sin resultado'}
              </span>
            </summary>
            <div className="flow-content">
              <p>
                <strong>Regla de negocio:</strong> {textoCliente(flow.invariant)}
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
                          <code>{paso.expect_status}</code>
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
          {resultados[flow.flow_id] && <div className="flow-evidence">
            <ReferenciaEvidencia uri={`s3://evidence/runs/${id}/${flow.flow_id}/result.json`} indice={0} runId={id} />
            <ReferenciaEvidencia uri={`s3://evidence/runs/${id}/${flow.flow_id}/logs.txt`} indice={1} runId={id} />
          </div>}
          </details>
          </div>
              ))}
            </div>
          </section>
        ))}
      </div>
    );
  } else {
    contenido = null;
  }

  return (
    <section className="card plan-card" aria-label="Plan de flujos">
      <details open>
        <summary className="plan-summary card-pad">
          <span>
            <h2 className="card-title">
              {encabezado}
            </h2>
            <span className="card-sub">Recorridos y reglas de negocio de tu aplicación</span>
          </span>
          {data !== null && (
            <span className="badge badge-neutral mono">
              {data.flows.length} flujos
            </span>
          )}
        </summary>
        <div className="plan-body card-pad">{contenido}</div>
      </details>
    </section>
  );
}

/** Los errores técnicos se conservan en la API; la vista ofrece una explicación accionable. */
export function RunErrorNotice({ error }: { error: unknown }) {
  const status = error instanceof ApiError ? error.status : 0;
  return <p role="alert" className="notice notice-danger">{
    status === 401 ? 'Tu sesión ha caducado. Vuelve a iniciar sesión.' :
    status === 403 ? 'No tienes acceso a esta información.' :
    status === 429 ? 'Demasiados intentos. Vuelve a intentarlo en unos instantes.' :
    'No se pudo actualizar la información. Vuelve a intentarlo en unos instantes.'
  }</p>;
}

/** Mantiene copiable la referencia original sin exponer la ubicación de almacenamiento en pantalla. */
function ReferenciaEvidencia({ uri, indice, runId, findingIndex }: { uri: string; indice: number; runId: string; findingIndex?: number }) {
  const [estado, setEstado] = useState('');
  if (findingIndex !== undefined && uri.startsWith('s3://')) {
    const path = `/api/report-evidence/${encodeURIComponent(runId)}/${findingIndex}/${indice}`;
    return <><a href={path} onClick={async (event) => {
      event.preventDefault();
      try { await downloadEvidence(path, uri.split('/').pop() || 'evidencia'); setEstado(''); }
      catch { setEstado('No se pudo descargar la evidencia.'); }
    }}>Descargar evidencia {indice + 1}</a><span role="status">{estado}</span></>;
  }
  const match = uri.match(/^s3:\/\/[^/]+\/runs\/([^/]+)\/([^/]+)\/(result\.json|logs\.txt)$/);
  if (match && match[1] === runId) {
    const name = match[3] ?? '';
    return <><button type="button" className="btn btn-secondary" onClick={async () => {
      try { await downloadEvidence(`/api/run-evidence/${encodeURIComponent(runId)}/${encodeURIComponent(match[2] ?? '')}/${name}`, name); setEstado(''); }
      catch { setEstado('No se pudo descargar la evidencia.'); }
    }}>{name === 'result.json' ? 'Descargar resultados' : 'Descargar registros'}</button><span role="status">{estado}</span></>;
  }
  if (/^https?:\/\//i.test(uri)) return <a href={uri} target="_blank" rel="noreferrer">Abrir evidencia {indice + 1}</a>;
  return <>
    <button type="button" className="btn btn-secondary" data-testid="run-evidence-copy-button" onClick={async () => {
      try {
        await navigator.clipboard.writeText(uri);
        setEstado('Referencia copiada');
      } catch {
        setEstado('No se pudo copiar la referencia. Comprueba el permiso del navegador.');
      }
    }}>Copiar referencia de evidencia {indice + 1}</button>
    <span role="status">{estado}</span>
  </>;
}
