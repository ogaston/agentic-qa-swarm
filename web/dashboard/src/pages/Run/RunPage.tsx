import { useRef } from 'react';
import { useParams } from 'react-router-dom';
import { ApiError, apiFetch } from '../../api/client';
import { ApiErrorNotice } from '../../api/errors';
import type { components } from '../../api/schema.gen';
import { usePolling } from '../../hooks/usePolling';

type Run = components['schemas']['Run'];

const ID_RUN = /^run-[0-9a-f]{32}$/;
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

const COLOR_FALLIDA = '#b00020';

function esTerminal(run: Run): boolean {
  return run.state === 'done' || run.state === 'failed';
}

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

  return (
    <main>
      <h1>Corrida</h1>
      <p>
        <code>{id}</code>
      </p>
      {data?.trace_id !== undefined && (
        <p>
          Trace: <code>{data.trace_id}</code>
        </p>
      )}

      {noEncontrada && <p role="alert">Corrida no encontrada</p>}

      {!noEncontrada && error !== null && <ApiErrorNotice error={error} />}
      {!noEncontrada && error !== null && data !== null && <p>Mostrando la última lectura correcta.</p>}
      {agotado && (
        <p>
          <button type="button" onClick={refrescar}>
            Reintentar
          </button>
        </p>
      )}

      {data !== null && <Linea run={data} ultimaFase={ultimaFase.current} />}
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
        <p role="status" data-estado="failed" style={{ color: COLOR_FALLIDA, fontWeight: 'bold' }}>
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
    <ol>
      {FASES.map((fase, i) => {
        const esActual = i === actual;
        const estado = esActual ? 'actual' : i < hechasHasta ? 'hecha' : 'pendiente';
        return (
          <li key={fase.estado} data-fase-estado={estado} aria-current={esActual ? 'step' : undefined}>
            <span>{fase.etiqueta}</span> <code>{fase.estado}</code> <small>{estado}</small>
          </li>
        );
      })}
    </ol>
  );
}
