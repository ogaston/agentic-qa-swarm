import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import type { components } from '../../api/schema.gen';
import { ApiError, apiFetch } from '../../api/client';
import { ApiErrorNotice } from '../../api/errors';
import { FLOW_FAMILIES, type FlowFamily } from '../../inbox/flows';
import { Badge, type TonoBadge } from '../../components/Badge';
import { Kpi } from '../../components/Kpi';
import { PageHeader, type ElementoMeta } from '../../components/PageHeader';

type Notification = components['schemas']['Notification'];
type ConfirmationReceipt = components['schemas']['ConfirmationReceipt'];
type WarmState = components['schemas']['WarmState'];
type Estado = Notification['state'];

const PESTANAS: { estado: Estado; etiqueta: string; vacio: string }[] = [
  { estado: 'pending', etiqueta: 'Pendientes', vacio: 'No hay notificaciones pendientes' },
  { estado: 'confirmed', etiqueta: 'Confirmadas', vacio: 'No hay notificaciones confirmadas' },
  { estado: 'rejected', etiqueta: 'Rechazadas', vacio: 'No hay notificaciones rechazadas' },
];

/** Color de la insignia por estado de la notificación; la etiqueta de texto es la que informa. */
const COLOR_ESTADO: Record<Estado, TonoBadge> = {
  pending: 'warn',
  confirmed: 'teal',
  rejected: 'danger',
};

/** Etiquetas del estado del warm (mismas que la vista del entorno). */
const ETIQUETA_ENTORNO: Record<WarmState['state'], string> = {
  ready: 'Listo',
  dirty: 'Sucio (requiere reset)',
  cuarentena: 'En cuarentena',
  'idle-escalado': 'Inactivo (escalado)',
};

const MSG_YA_NO_PENDIENTE = 'Esta notificación ya no está pendiente';
const MSG_NO_EXISTE = 'La notificación ya no existe';
const RUN_ID = /^run-[0-9a-f]{16}$/;

/** Inbox: pestañas por estado, KPI, listado en tarjeta y confirmación explícita. Sin polling ni confirmaciones automáticas. */
/** Etiqueta del tipo de artefacto como en el mockup (image, build_from_repo); el dato real es el contrato. */
const ETIQUETA_ARTEFACTO: Record<string, string> = { 'published-image': 'image', 'build-from-repo': 'build_from_repo' };
function textoArtefacto(a: { kind: string; ref: string }): string {
  return `${ETIQUETA_ARTEFACTO[a.kind] ?? a.kind} · ${a.ref}`;
}

export function InboxPage() {
  const navigate = useNavigate();
  const [estado, setEstado] = useState<Estado>('pending');
  const [lista, setLista] = useState<Notification[] | null>(null);
  const [errorLista, setErrorLista] = useState<unknown>(null);
  // Contadores de las pestañas: solo de las pestañas ya cargadas (no se piden listas extra).
  const [conteos, setConteos] = useState<Partial<Record<Estado, number>>>({});
  // Estado del warm para la cabecera y el KPI. null = no disponible o aún no leído.
  const [warm, setWarm] = useState<WarmState | null>(null);
  const [seleccionadaId, setSeleccionadaId] = useState<string | null>(null);
  const [panelAbierto, setPanelAbierto] = useState(false);
  const [familias, setFamilias] = useState<FlowFamily[]>([...FLOW_FAMILIES]);
  const [enVuelo, setEnVuelo] = useState(false);
  const [mensajeAccion, setMensajeAccion] = useState<string | null>(null);
  const [errorAccion, setErrorAccion] = useState<unknown>(null);
  const enCurso = useRef(false);
  const peticion = useRef(0);
  // Pestaña vigente, para que una recarga tras una acción use siempre la pestaña actual.
  const estadoRef = useRef<Estado>('pending');
  const tituloPanel = useRef<HTMLHeadingElement | null>(null);
  const dialogoRef = useRef<HTMLDivElement | null>(null);
  const disparadorRef = useRef<HTMLButtonElement | null>(null);
  const panelAntes = useRef(false);

  // Carga la pestaña indicada. Una respuesta vieja (otra pestaña o recarga posterior) se descarta.
  const cargar = useCallback(async (porEstado: Estado) => {
    const id = ++peticion.current;
    setErrorLista(null);
    // Sin contadores de la carga anterior mientras esta llega.
    setConteos({});
    try {
      const datos = await apiFetch<Notification[]>(`/api/notifications?state=${porEstado}`);
      if (id !== peticion.current) return;
      setLista(datos);
      // Solo el contador de la carga vigente: los demás quedarían obsoletos sin refrescar su lista.
      setConteos({ [porEstado]: datos.length });
    } catch (error) {
      if (id !== peticion.current) return;
      setConteos({});
      setLista([]);
      setErrorLista(error);
    }
  }, []);

  const cargarWarm = useCallback(async () => {
    try {
      setWarm(await apiFetch<WarmState>('/api/warm'));
    } catch {
      setWarm(null);
    }
  }, []);

  useEffect(() => {
    void cargarWarm();
  }, [cargarWarm]);

  useEffect(() => {
    setLista(null);
    setSeleccionadaId(null);
    setPanelAbierto(false);
    void cargar(estado);
  }, [estado, cargar]);

  const seleccionada = lista?.find((n) => n.id === seleccionadaId) ?? null;

  function elegirPestana(nueva: Estado) {
    if (enCurso.current) return; // sin cambiar de pestaña con un POST en vuelo
    setMensajeAccion(null);
    setErrorAccion(null);
    estadoRef.current = nueva;
    setEstado(nueva);
  }

  /** Flechas, Inicio y Fin mueven la selección y el foco entre pestañas. */
  function alPulsarTeclaPestana(evento: KeyboardEvent<HTMLDivElement>) {
    const orden = PESTANAS.map((p) => p.estado);
    const i = orden.indexOf(estado);
    let j: number | null = null;
    if (evento.key === 'ArrowRight') j = (i + 1) % orden.length;
    else if (evento.key === 'ArrowLeft') j = (i - 1 + orden.length) % orden.length;
    else if (evento.key === 'Home') j = 0;
    else if (evento.key === 'End') j = orden.length - 1;
    if (j === null) return;
    evento.preventDefault();
    const nueva = orden[j] as Estado;
    if (nueva !== estado) elegirPestana(nueva);
    document.getElementById(`tab-${nueva}`)?.focus();
  }

  function abrirPanel() {
    setFamilias([...FLOW_FAMILIES]);
    setErrorAccion(null);
    setMensajeAccion(null);
    setPanelAbierto(true);
  }

  // Foco inicial en el título del panel; al cerrarlo, el foco vuelve al disparador.
  useEffect(() => {
    if (panelAbierto) tituloPanel.current?.focus();
    else if (panelAntes.current) disparadorRef.current?.focus();
    panelAntes.current = panelAbierto;
  }, [panelAbierto]);

  function cerrarPanel() {
    if (enCurso.current) return;
    setPanelAbierto(false);
  }

  /** Esc cierra; Tab y Mayús+Tab cicla el foco dentro del diálogo (foco atrapado). */
  function alPulsarTeclaPanel(evento: KeyboardEvent<HTMLDivElement>) {
    if (evento.key === 'Escape') {
      cerrarPanel();
      return;
    }
    if (evento.key !== 'Tab') return;
    const dialogo = dialogoRef.current;
    if (dialogo === null) return;
    const enfocables = Array.from(
      dialogo.querySelectorAll<HTMLElement>('button:not([disabled]), input:not([disabled])'),
    );
    const primero = enfocables[0];
    const ultimo = enfocables[enfocables.length - 1];
    if (primero === undefined || ultimo === undefined) {
      evento.preventDefault();
      return;
    }
    const activo = document.activeElement;
    if (evento.shiftKey && (activo === primero || activo === tituloPanel.current)) {
      evento.preventDefault();
      ultimo.focus();
    } else if (!evento.shiftKey && activo === ultimo) {
      evento.preventDefault();
      primero.focus();
    }
  }

  function alternarFamilia(familia: FlowFamily) {
    setFamilias((actuales) =>
      actuales.includes(familia) ? actuales.filter((f) => f !== familia) : [...actuales, familia],
    );
  }

  async function confirmar() {
    if (seleccionada === null || enCurso.current || familias.length === 0) return;
    enCurso.current = true;
    setEnVuelo(true);
    setMensajeAccion(null);
    setErrorAccion(null);
    try {
      const recibo = await apiFetch<ConfirmationReceipt>(
        `/api/notifications/${encodeURIComponent(seleccionada.id)}/confirm`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ flows: familias }),
        },
      );
      if (!RUN_ID.test(recibo.run_id)) {
        // La confirmación ya se registró: no navegamos a una ruta sin sentido.
        setPanelAbierto(false);
        setMensajeAccion(
          `La confirmación se registró, pero no se pudo abrir la corrida (respuesta sin run_id válido). Notificación ${seleccionada.id}.`,
        );
        void cargar(estadoRef.current);
        return;
      }
      navigate(`/runs/${encodeURIComponent(recibo.run_id)}`);
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) return; // lo gestiona la sesión (T03)
      if (error instanceof ApiError && error.status === 409) {
        setPanelAbierto(false);
        setMensajeAccion(MSG_YA_NO_PENDIENTE);
        void cargar(estadoRef.current);
      } else if (error instanceof ApiError && error.status === 404) {
        setPanelAbierto(false);
        setMensajeAccion(MSG_NO_EXISTE);
        void cargar(estadoRef.current);
      } else {
        setErrorAccion(error);
      }
    } finally {
      enCurso.current = false;
      setEnVuelo(false);
    }
  }

  function actualizar() {
    void cargar(estado);
    void cargarWarm();
  }

  const textoVacio = PESTANAS.find((p) => p.estado === estado)?.vacio ?? '';

  // Cabecera: solo celdas con dato real de la API. «Familias activas» no existe en la API: se omite.
  const meta: ElementoMeta[] = [];
  if (warm !== null) {
    // U10-T06: el mockup muestra el identificador del entorno (warm_id, dato real) en verde, como su estado.
    meta.push({
      etiqueta: 'Entorno',
      valor: <span className="meta-ok">{`${ETIQUETA_ENTORNO[warm.state]} (${warm.warm_id})`}</span>,
    });
    meta.push({
      etiqueta: 'Reset verificado',
      valor: <span className="mono">{warm.reset_verified ? 'sí' : 'no'}</span>,
    });
  }

  const textoContador = (estadoPestana: Estado) => {
    const n = conteos[estadoPestana];
    return n === undefined ? '—' : String(n);
  };

  return (
    <main className="az-container">
      <PageHeader
        titulo="Inbox de notificaciones"
        descripcion="Eventos recibidos desde GitHub. Cero auto-run: ninguna corrida arranca sin confirmación explícita."
        meta={meta}
      />

      <div className="az-subbar">
        <div
          role="tablist"
          aria-label="Estado de las notificaciones"
          className="az-tablist"
          onKeyDown={alPulsarTeclaPestana}
        >
          {PESTANAS.map((p) => {
            const seleccionadaPestana = p.estado === estado;
            const contador = conteos[p.estado];
            return (
              <button
                key={p.estado}
                type="button"
                role="tab"
                id={`tab-${p.estado}`}
                aria-selected={seleccionadaPestana}
                aria-controls="inbox-lista"
                tabIndex={seleccionadaPestana ? 0 : -1}
                disabled={enVuelo}
                className="az-tab"
                onClick={() => elegirPestana(p.estado)}
              >
                {contador === undefined ? p.etiqueta : `${p.etiqueta} (${contador})`}
              </button>
            );
          })}
        </div>
        <div className="az-subbar-actions">
          <button type="button" className="az-action-link" onClick={actualizar}>
            <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <path
                d="M20 11a8.1 8.1 0 00-15.5-2m-.5-4v4h4M4 13a8.1 8.1 0 0015.5 2m.5 4v-4h-4"
                stroke="currentColor"
                strokeWidth="1.8"
                strokeLinecap="round"
                strokeLinejoin="round"
              />
            </svg>
            Actualizar
          </button>
        </div>
      </div>

      <div className="kpi-strip">
        <Kpi
          etiqueta="Notificaciones pendientes"
          valor={textoContador('pending')}
          insignia={{ clase: 'badge-accent', texto: 'esperando confirm' }}
          barra={60}
          colorBarra="var(--accent)"
        />
        <Kpi
          etiqueta="Corridas confirmadas"
          valor={textoContador('confirmed')}
          insignia={{ clase: 'badge-info', texto: 'auditable' }}
          barra={40}
          colorBarra="var(--info)"
        />
        <Kpi
          etiqueta="Estado del entorno"
          valor={warm !== null ? ETIQUETA_ENTORNO[warm.state] : 'Sin dato'}
          claseValor="kpi-valor-20"
          insignia={warm !== null ? { clase: 'badge-ok', texto: warm.state, punto: true } : undefined}
          barra={100}
          colorBarra="var(--ok)"
        />
        <Kpi etiqueta="Rechazadas" valor={textoContador('rejected')} barra={100} colorBarra="var(--teal)" />
      </div>

      <div id="inbox-lista" role="tabpanel" aria-labelledby={`tab-${estado}`}>
        {lista === null && errorLista === null ? <p>Cargando notificaciones…</p> : null}
        {errorLista !== null ? <ApiErrorNotice error={errorLista} /> : null}
        {lista !== null && errorLista === null && lista.length === 0 ? (
          <p className="empty">{textoVacio}</p>
        ) : null}
        {lista !== null && errorLista === null && lista.length > 0 ? (
          <div className="card">
            <div className="card-pad listado-cabecera">
              <div className="card-title">Listado de notificaciones</div>
              <div className="card-sub">
                Eventos recibidos listos para revisión y confirmación previa a la inferencia de flujos sobre el entorno.
              </div>
            </div>
            <div className="table-wrap">
              <table className="data-table">
                <thead>
                  <tr>
                    <th scope="col">Repositorio</th>
                    <th scope="col">Evento</th>
                    <th scope="col">SHA</th>
                    <th scope="col">Artefacto</th>
                    <th scope="col">Estado</th>
                    <th scope="col">
                      <span className="sr-only">Acciones</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {lista.map((n) => (
                    <tr key={n.id}>
                      <td data-label="Repositorio">
                        <strong>{n.repo}</strong>
                      </td>
                      <td data-label="Evento">
                        <span className="badge badge-neutral mono">{n.github_event}</span>
                      </td>
                      <td data-label="SHA">
                        <code>{n.sha.slice(0, 7)}</code>
                      </td>
                      <td data-label="Artefacto">
                        <span className="mono">
                          {n.artifact ? textoArtefacto(n.artifact) : '—'}
                        </span>
                      </td>
                      <td data-label="Estado">
                        <Badge tono={COLOR_ESTADO[n.state]} punto>
                          {n.state}
                        </Badge>
                      </td>
                      <td data-label="Acciones">
                        <button
                          type="button"
                          className="btn btn-primary btn-sm"
                          disabled={enVuelo}
                          onClick={() => {
                            setSeleccionadaId(n.id);
                            setPanelAbierto(false);
                            setErrorAccion(null);
                            setMensajeAccion(null);
                          }}
                        >
                          Revisar y confirmar →
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        ) : null}
      </div>

      {mensajeAccion !== null ? <p role="alert" className="notice notice-danger">{mensajeAccion}</p> : null}

      {seleccionada !== null ? (
        <section aria-label="Detalle de la notificación" className="card card-pad detail">
          <h2>Detalle</h2>
          <div className="stack">
          <dl className="kv-list">
            <dt>Repositorio</dt>
            <dd>{seleccionada.repo}</dd>
            <dt>Evento</dt>
            <dd>{seleccionada.github_event}</dd>
            <dt>SHA</dt>
            <dd>
              <code>{seleccionada.sha}</code>
            </dd>
            <dt>Artefacto</dt>
            <dd>
              {seleccionada.artifact
                ? textoArtefacto(seleccionada.artifact)
                : 'Sin artefacto'}
            </dd>
            <dt>Estado</dt>
            <dd>{seleccionada.state}</dd>
          </dl>
          {seleccionada.state === 'pending' ? (
            <div>
              <button ref={disparadorRef} type="button" className="btn btn-primary" onClick={abrirPanel}>
                Revisar y confirmar
              </button>
            </div>
          ) : null}
          </div>
        </section>
      ) : null}

      {panelAbierto && seleccionada !== null ? (
        <div className="modal-backdrop open">
          <div
            ref={dialogoRef}
            role="dialog"
            aria-modal="true"
            aria-labelledby="confirm-titulo"
            className="modal-dialog"
            onKeyDown={alPulsarTeclaPanel}
          >
            <h2 id="confirm-titulo" tabIndex={-1} ref={tituloPanel}>
              Confirmar corrida
            </h2>
            <dl className="kv-list">
              <dt>Repositorio</dt>
              <dd>{seleccionada.repo}</dd>
              <dt>SHA</dt>
              <dd>
                <code>{seleccionada.sha}</code>
              </dd>
              <dt>Artefacto</dt>
              <dd>
                {seleccionada.artifact
                  ? textoArtefacto(seleccionada.artifact)
                  : 'Sin artefacto'}
              </dd>
            </dl>
            <fieldset className="fieldset">
              <legend>Familias de flujos</legend>
              {FLOW_FAMILIES.map((f) => (
                <label key={f} className="check">
                  <input
                    type="checkbox"
                    name="familias"
                    value={f}
                    checked={familias.includes(f)}
                    onChange={() => alternarFamilia(f)}
                  />{' '}
                  {f}
                </label>
              ))}
            </fieldset>
            {errorAccion !== null ? <ApiErrorNotice error={errorAccion} /> : null}
            <button type="button" className="btn btn-secondary" onClick={cerrarPanel} disabled={enVuelo}>
              Cancelar
            </button>{' '}
            <button
              type="button"
              className="btn btn-primary"
              onClick={() => void confirmar()}
              disabled={familias.length === 0 || enVuelo}
            >
              Confirmar corrida
            </button>
          </div>
        </div>
      ) : null}
    </main>
  );
}
