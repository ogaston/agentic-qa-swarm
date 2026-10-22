import { useRef } from 'react';
import { useParams } from 'react-router-dom';
import { ApiError, apiFetch } from '../../api/client';
import { ApiErrorNotice } from '../../api/errors';
import type { components } from '../../api/schema.gen';
import { usePolling } from '../../hooks/usePolling';
import { PlanFlujos, PostMortem, Veredicto } from './Resultados';

type Run = components['schemas']['Run'];

const ID_RUN = /^run-[0-9a-f]{16}$/;
const INTERVALO_MS = 3000;
const FASES = [
  { estado: 'confirmed', etiqueta: 'Confirmada' },
  { estado: 'warm_ready', etiqueta: 'Warm listo' },
  { estado: 'deploying', etiqueta: 'Desplegando' },
  { estado: 'inferring', etiqueta: 'Infiriendo superficie' },
  { estado: 'rehearsing', etiqueta: 'Ensayando' },
  { estado: 'running', etiqueta: 'Ejecutando' },
  { estado: 'resetting', etiqueta: 'Reseteando' },
  { estado: 'reporting', etiqueta: 'Generando informe' },
  { estado: 'done', etiqueta: 'Completada' },
] as const;

function esTerminal(run: Run): boolean {
  return run.state === 'done' || run.state === 'failed';
}

function indiceFase(estado: string | null): number {
  return FASES.findIndex((f) => f.estado === estado);
}

const INDICE_ENSAYO = indiceFase('rehearsing');

/** Ruta de la corrida (`/runs/:id`). Valida el id antes de pedir nada. */
export function RunPage() {
  const { id = '' } = useParams();
  if (!ID_RUN.test(id)) {
    return (
      <main>
        <h1>Corrida</h1>
        <p role="alert">Identificador de corrida no válido.</p>
      </main>
    );
  }
  // key: cambiar de corrida remonta la vista y reinicia el sondeo.
  return <VistaCorrida key={id} id={id} />;
}

function VistaCorrida({ id }: { id: string }) {
  const { data, error, agotado, detenido, refrescar } = usePolling<Run>(
    () => apiFetch<Run>(`/api/runs/${id}`),
    INTERVALO_MS,
    {
      isTerminal: esTerminal,
      isFatal: (e) => e instanceof ApiError && e.status === 404,
    },
  );

  // Última fase no fallida vista en el sondeo: sirve de «última fase alcanzada» si falla.
  const ultimaFase = useRef<string | null>(null);
  if (data !== null && data.state !== 'failed') ultimaFase.current = data.state;

  const noEncontrada = error instanceof ApiError && error.status === 404;
  // El plan existe desde que la corrida llega a «rehearsing». En `failed` se pide siempre: si no hay
  // plan, la API responde plan_not_ready y la vista lo dice. No depende de la última fase vista.
  const alcanzoEnsayo =
    data !== null && (data.state === 'failed' || indiceFase(data.state) >= INDICE_ENSAYO);
  // El informe existe al terminar («done») o, si falla, cuando el controlador ya guardó su report_uri.
  const informeListo = data !== null && (data.state === 'done' || (data.state === 'failed' && data.report_uri !== undefined));

  return (
    <main className="page">
      <div className="page-header">
        <div className="page-title">
          <h1>Corrida</h1>
          <p className="page-desc">
            <code className="mono">{id}</code>
            {data?.trace_id !== undefined && (
              <>
                {' · '}Trace: <code className="mono">{data.trace_id}</code>
              </>
            )}
          </p>
        </div>
      </div>

      {noEncontrada && <p role="alert" className="notice notice-danger">Corrida no encontrada</p>}

      {!noEncontrada && error !== null && <ApiErrorNotice error={error} />}
      {!noEncontrada && error !== null && data !== null && <p className="notice">Mostrando la última lectura correcta.</p>}
      {agotado && (
        <p>
          <button type="button" className="btn btn-secondary" onClick={refrescar}>
            Reintentar
          </button>
        </p>
      )}

      {data !== null && <Linea run={data} ultimaFase={ultimaFase.current} />}
      {data?.verdict !== undefined && <Veredicto verdict={data.verdict} />}
      <PostMortem id={id} activo={informeListo} />
      <PlanFlujos id={id} activo={alcanzoEnsayo} />
      {data === null && !noEncontrada && error === null && <p>Cargando corrida…</p>}
      {detenido && data !== null && !esTerminal(data) && <p>Sondeo detenido.</p>}
    </main>
  );
}

function Linea({ run, ultimaFase }: { run: Run; ultimaFase: string | null }) {
  const indiceUltima = FASES.findIndex((f) => f.estado === ultimaFase);
  if (run.state === 'failed') {
    const conocida = indiceUltima >= 0;
    const texto = conocida ? `Fallida · última fase alcanzada: ${ultimaFase}` : 'Fallida';
    return (
      <>
        <p role="status" data-estado="failed" className="notice status-failed">
          {texto}
        </p>
        <Fases actual={-1} hechasHasta={conocida ? indiceUltima + 1 : 0} />
      </>
    );
  }
  if (run.state === 'done') {
    return <Fases actual={-1} hechasHasta={FASES.length} />;
  }
  const indice = FASES.findIndex((f) => f.estado === run.state);
  return <Fases actual={indice} hechasHasta={indice} />;
}

/**
 * Línea de tiempo. Las fases con índice menor que `hechasHasta` están hechas; la de
 * índice `actual` es la fase en curso (`aria-current`); el resto, pendientes.
 */
function Fases({ actual, hechasHasta }: { actual: number; hechasHasta: number }) {
  return (
    <ol aria-label="Fases de la corrida" className="phase-timeline">
      {FASES.map((fase, i) => {
        const esActual = i === actual;
        const estado = esActual ? 'actual' : i < hechasHasta ? 'hecha' : 'pendiente';
        return (
          <li
            key={fase.estado}
            className={`phase phase-${estado}`}
            data-fase={fase.estado}
            data-fase-estado={estado}
            aria-current={esActual ? 'step' : undefined}
          >
            <span className="phase-label">{fase.etiqueta}</span>{' '}
            <code className="mono">{fase.estado}</code>{' '}
            <small className="phase-tag">{estado}</small>
          </li>
        );
      })}
    </ol>
  );
}
