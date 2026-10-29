import { useRef } from 'react';
import { useParams } from 'react-router-dom';
import { ApiError, apiFetch } from '../../api/client';
import type { components } from '../../api/schema.gen';
import { usePolling } from '../../hooks/usePolling';
import { PlanFlujos, PostMortem, Veredicto, RunErrorNotice } from './Resultados';

type Run = components['schemas']['Run'];

const ID_RUN = /^run-[0-9a-f]{16}$/;
const INTERVALO_MS = 3000;
const FASES = [
  { estado: 'confirmed', etiqueta: 'Confirmada' },
  { estado: 'warm_ready', etiqueta: 'Entorno listo' },
  { estado: 'deploying', etiqueta: 'Preparando la versión' },
  { estado: 'inferring', etiqueta: 'Analizando tu aplicación' },
  { estado: 'rehearsing', etiqueta: 'Validando las pruebas' },
  { estado: 'running', etiqueta: 'Ejecutando pruebas' },
  { estado: 'resetting', etiqueta: 'Restaurando el entorno' },
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
      <main className="az-container run-page">
        <h1>Corrida</h1>
        <p role="alert" className="alert-box alert-danger">
          Identificador de corrida no válido.
        </p>
      </main>
    );
  }
  // key: cambiar de corrida remonta la vista y reinicia el sondeo.
  return <VistaCorrida key={id} id={id} />;
}

function VistaCorrida({ id }: { id: string }) {
  // El recibo de confirmación puede llegar antes de que el controlador consuma run.confirmed.
  // Hasta la primera lectura correcta, un 404 usa el presupuesto acotado de reintentos.
  const corridaVista = useRef(false);
  const { data, error, agotado, detenido, refrescar } = usePolling<Run>(
    async () => {
      const run = await apiFetch<Run>(`/api/runs/${id}`);
      corridaVista.current = true;
      return run;
    },
    INTERVALO_MS,
    {
      isTerminal: esTerminal,
      isFatal: (e) => corridaVista.current && e instanceof ApiError && e.status === 404,
    },
  );

  // Última fase no fallida vista en el sondeo: sirve de «última fase alcanzada» si falla.
  const ultimaFase = useRef<string | null>(null);
  if (data !== null && data.state !== 'failed') ultimaFase.current = data.state;

  const es404 = error instanceof ApiError && error.status === 404;
  const esperandoCorrida = es404 && data === null && !agotado;
  const noEncontrada = es404 && !esperandoCorrida;
  const estadoConsulta = noEncontrada
    ? 'No encontrada'
    : data !== null
      ? descripcionEstado(data)
      : esperandoCorrida
        ? 'Esperando corrida'
        : agotado ? 'Consulta detenida' : 'Cargando…';
  // El plan existe desde que la corrida llega a «rehearsing». En `failed` se pide siempre: si no hay
  // plan, la API responde plan_not_ready y la vista lo dice. No depende de la última fase vista.
  const alcanzoEnsayo =
    data !== null && (data.state === 'failed' || indiceFase(data.state) >= INDICE_ENSAYO);
  // El informe existe al terminar («done») o, si falla, cuando el controlador ya guardó su report_uri.
  const informeListo = data !== null && (data.state === 'done' || (data.state === 'failed' && data.report_uri !== undefined));
  const enCurso = data !== null && !esTerminal(data);
  const plan = <PlanFlujos key="plan" id={id} activo={alcanzoEnsayo} automatico={enCurso} enEjecucion={data?.state === 'running'} encabezado={encabezadoPlan(data?.state)} />;
  const informe = data !== null ? <PostMortem key="informe" id={id} activo={informeListo} verdict={data.verdict} /> : null;

  return (
    <main className="az-container run-page">
      <div className="az-page-header">
        <div className="az-page-title">
          <div className="run-titulo">
            <h1>
              Resumen de la corrida
            </h1>
            {data !== null && <EstadoCorrida run={data} ultimaFase={ultimaFase.current} />}
            {data?.verdict !== undefined && <Veredicto verdict={data.verdict} />}
          </div>
          <p>Resultados y progreso de las pruebas de tu aplicación.</p>
          <p className="run-reference">Referencia: <code>{id}</code></p>
        </div>

        <div className="az-header-meta">
          <div className="az-meta-item">
            <span className="az-meta-label">Estado de corrida</span>
            <span className="az-meta-value">{estadoConsulta}</span>
          </div>
        </div>
      </div>

      {noEncontrada && (
        <p role="alert" className="alert-box alert-danger">
          Corrida no encontrada
        </p>
      )}

      {esperandoCorrida && (
        <p role="status" className="notice">
          Esperando a que la corrida esté disponible. Reintentando automáticamente cada 3 s…
        </p>
      )}

      {!es404 && error !== null && <RunErrorNotice error={error} />}
      {!noEncontrada && error !== null && data !== null && <p className="notice">Mostrando la última lectura correcta.</p>}
      {agotado && (
        <p>
          <button type="button" className="btn btn-secondary" data-testid="run-retry-button" onClick={refrescar}>
            Reintentar
          </button>
        </p>
      )}

      {data !== null && <Aviso run={data} ultimaFase={ultimaFase.current} />}

      {data === null && !noEncontrada && error === null && <p>Cargando corrida…</p>}
      {detenido && data !== null && !esTerminal(data) && <p>Actualización automática pausada.</p>}

      <div className="grid-main-side">
        <div className="stack">
          {enCurso ? [plan, informe] : [informe, plan]}
        </div>

        <section className="card card-pad" aria-label="Línea de tiempo de fases">
          <div className="card-head">
            <div>
              <h2 className="card-title">Línea de tiempo de la corrida</h2>
              <div className="card-sub">
                {data !== null && esTerminal(data) ? 'Resumen de las etapas de tu prueba' : 'El progreso se actualiza automáticamente'}
              </div>
            </div>
            {data !== null && <ResumenFases run={data} ultimaFase={ultimaFase.current} />}
          </div>
          {data !== null ? <Linea run={data} ultimaFase={ultimaFase.current} /> : <p>Sin fases todavía.</p>}
        </section>
      </div>
    </main>
  );
}

/** Encabezado del plan según el estado de la corrida (mockup: inferidos, en ejecución, ejecutados). */
function encabezadoPlan(estado: string | undefined): string {
  if (estado === 'done' || estado === 'failed') return 'Flujos ejecutados';
  if (estado === 'running') return 'Flujos en ejecución';
  return 'Flujos de prueba';
}

/** Texto del estado para la franja de metadatos: etiqueta legible y código, sin repetir la fase. */
function descripcionEstado(run: Run): string {
  if (run.state === 'failed') return 'Corrida fallida';
  if (run.state === 'done') return 'Completada';
  const indice = indiceFase(run.state);
  return FASES[indice]?.etiqueta ?? 'En curso';
}

/** Insignia de estado del encabezado: texto, no solo color. El veredicto va aparte (Veredicto). */
function EstadoCorrida({ run }: { run: Run; ultimaFase: string | null }) {
  if (run.state === 'failed') {
    return <span className="badge badge-danger">Corrida fallida</span>;
  }
  if (run.state === 'done') return null;
  const indice = indiceFase(run.state);
  if (indice < 0) {
    return <span className="badge badge-neutral">En curso</span>;
  }
  return (
    <span className="badge badge-accent">
      Fase {indice + 1} de 9: {FASES[indice]?.etiqueta}
    </span>
  );
}

/** Banner de fallida (`alert-box alert-danger`, `data-estado="failed"`). El color va en clase. */
function Aviso({ run, ultimaFase }: { run: Run; ultimaFase: string | null }) {
  if (run.state !== 'failed') return null;
  const conocida = FASES.some((f) => f.estado === ultimaFase);
  const texto = conocida ? `Fallida · última fase alcanzada: ${FASES.find((f) => f.estado === ultimaFase)?.etiqueta}` : 'Fallida';
  return (
    <div role="status" data-estado="failed" className="alert-box alert-danger status-failed">
      {texto}
    </div>
  );
}

/** Contador de fases de la cabecera de la línea de tiempo (texto, no solo color). */
function ResumenFases({ run, ultimaFase }: { run: Run; ultimaFase: string | null }) {
  if (run.state === 'done') return <span className="badge badge-ok">9/9 completadas</span>;
  if (run.state === 'failed') {
    const indiceUltima = FASES.findIndex((f) => f.estado === ultimaFase);
    return (
      <span className="badge badge-danger">
        {indiceUltima >= 0 ? `Detenida tras la fase ${indiceUltima + 1} de 9` : 'Detenida'}
      </span>
    );
  }
  const indice = indiceFase(run.state);
  return <span className="badge badge-accent">{indice >= 0 ? `Fase ${indice + 1} de 9 en curso` : 'En curso'}</span>;
}

function Linea({ run, ultimaFase }: { run: Run; ultimaFase: string | null }) {
  const indiceUltima = FASES.findIndex((f) => f.estado === ultimaFase);
  if (run.state === 'failed') {
    const conocida = indiceUltima >= 0;
    return <Fases actual={-1} hechasHasta={conocida ? indiceUltima + 1 : 0} />;
  }
  if (run.state === 'done') {
    return <Fases actual={-1} hechasHasta={FASES.length} />;
  }
  const indice = FASES.findIndex((f) => f.estado === run.state);
  return <Fases actual={indice} hechasHasta={indice} />;
}

const ETIQUETA_ESTADO = { hecha: 'hecha', actual: 'actual', pendiente: 'pendiente' } as const;
const CLASE_ESTADO = { hecha: 'badge-ok', actual: 'badge-accent', pendiente: 'badge-neutral' } as const;

/**
 * Línea de tiempo. Las fases con índice menor que `hechasHasta` están hechas; la de
 * índice `actual` es la fase en curso (`aria-current`); el resto, pendientes. Cada fila
 * lleva icono y el estado escrito: el color nunca es lo único que la distingue.
 */
function Fases({ actual, hechasHasta }: { actual: number; hechasHasta: number }) {
  return (
    <ol aria-label="Fases de la corrida" className="phase-timeline">
      {FASES.map((fase, i) => {
        const esActual = i === actual;
        const estado = esActual ? 'actual' : i < hechasHasta ? 'hecha' : 'pendiente';
        const icono = estado === 'hecha' ? '✓' : String(i + 1);
        return (
          <li
            key={fase.estado}
            data-fase={fase.estado}
            data-fase-estado={estado}
            aria-current={esActual ? 'step' : undefined}
          >
            <div className="phase-left">
              <span className="phase-idx" aria-hidden="true">
                {icono}
              </span>
              <div>
                <div className="phase-name">{fase.etiqueta}</div>
              </div>
            </div>
            <span className={`badge ${CLASE_ESTADO[estado]}`}>
              {esActual ? <>
                <span className="phase-spinner" aria-hidden="true" />
                <span className="sr-only">En curso</span>
              </> : ETIQUETA_ESTADO[estado]}
            </span>
          </li>
        );
      })}
    </ol>
  );
}
