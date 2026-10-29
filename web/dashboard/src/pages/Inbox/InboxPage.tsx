import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import type { components } from '../../api/schema.gen';
import { ApiError, apiFetch } from '../../api/client';
import { ApiErrorNotice } from '../../api/errors';
import { FLOW_FAMILIES, type FlowFamily } from '../../inbox/flows';
import { Badge, type TonoBadge } from '../../components/Badge';
import { Kpi } from '../../components/Kpi';
import { PageHeader } from '../../components/PageHeader';

type Notification = components['schemas']['Notification'];
type ConfirmationReceipt = components['schemas']['ConfirmationReceipt'];
type WarmState = components['schemas']['WarmState'];
type Estado = Notification['state'];

const PESTANAS: { estado: Estado; etiqueta: string; vacio: string; vacioDescripcion: string }[] = [
  {
    estado: 'pending',
    etiqueta: 'Pendientes',
    vacio: 'No hay notificaciones pendientes',
    vacioDescripcion:
      'Cuando tu equipo publique un cambio, aparecerá aquí para que decidas si quieres probarlo. Nada se ejecuta sin tu confirmación.',
  },
  {
    estado: 'confirmed',
    etiqueta: 'Confirmadas',
    vacio: 'No hay notificaciones confirmadas',
    vacioDescripcion:
      'Las notificaciones que confirmes aparecerán aquí con su corrida asociada, para que vuelvas a ellas cuando lo necesites.',
  },
  {
    estado: 'rejected',
    etiqueta: 'Rechazadas',
    vacio: 'No hay notificaciones rechazadas',
    vacioDescripcion:
      'Las notificaciones que descartes quedarán aquí como historial de los cambios que decidiste no probar.',
  },
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
  dirty: 'Requiere limpieza',
  cuarentena: 'En cuarentena',
  'idle-escalado': 'En pausa',
};

type TonoEntorno = 'ok' | 'warn' | 'neutral';

/** Tono semántico por estado del warm: el mismo mapeo que la vista del entorno (WarmPage). */
const TONO_ENTORNO: Record<WarmState['state'], TonoEntorno> = {
  ready: 'ok',
  dirty: 'warn',
  cuarentena: 'warn',
  'idle-escalado': 'neutral',
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
  // Estado del warm para el KPI. null = no disponible o aún no leído.
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
  // Modal de dos pasos: abierto mientras hay selección; panelAbierto = paso 2 (confirmación).
  const tituloModal = useRef<HTMLHeadingElement | null>(null);
  const dialogoRef = useRef<HTMLDivElement | null>(null);
  const disparadorModalRef = useRef<HTMLButtonElement | null>(null);
  const modalAntes = useRef(false);

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
  const modalAbierto = seleccionada !== null;

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

  function abrirDetalle(id: string, disparador: HTMLButtonElement) {
    disparadorModalRef.current = disparador;
    setSeleccionadaId(id);
    setPanelAbierto(false);
    setErrorAccion(null);
    setMensajeAccion(null);
  }

  function cerrarModal() {
    if (enCurso.current) return; // sin cerrar con un POST en vuelo
    setPanelAbierto(false);
    setSeleccionadaId(null);
  }

  // Foco inicial en el título del modal y al cambiar de paso; al cerrarlo, vuelve al botón de la fila.
  useEffect(() => {
    if (modalAbierto) tituloModal.current?.focus();
    else if (modalAntes.current) disparadorModalRef.current?.focus();
    modalAntes.current = modalAbierto;
  }, [modalAbierto, panelAbierto]);

  /** Esc cierra el modal; Tab y Mayús+Tab ciclan el foco dentro del diálogo (foco atrapado). */
  function alPulsarTeclaModal(evento: KeyboardEvent<HTMLDivElement>) {
    if (evento.key === 'Escape') {
      cerrarModal();
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
    if (evento.shiftKey && (activo === primero || activo === tituloModal.current)) {
      evento.preventDefault();
      ultimo.focus();
    } else if (!evento.shiftKey && activo === ultimo) {
      evento.preventDefault();
      primero.focus();
    }
  }

  function abrirPanel() {
    setFamilias([...FLOW_FAMILIES]);
    setErrorAccion(null);
    setMensajeAccion(null);
    setPanelAbierto(true);
  }

  /** Paso 2 → paso 1: conserva la selección y las familias marcadas. */
  function volverAlDetalle() {
    if (enCurso.current) return;
    setPanelAbierto(false);
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

  const pestanaVacia = PESTANAS.find((p) => p.estado === estado);
  const textoVacio = pestanaVacia?.vacio ?? '';

  const textoContador = (estadoPestana: Estado) => {
    const n = conteos[estadoPestana];
    return n === undefined ? '—' : String(n);
  };

  return (
    <main className="az-container">
      <PageHeader
        titulo="Inbox de notificaciones"
        descripcion="Revisa los cambios de tu equipo y elige cuáles probar. Cada corrida empieza con tu confirmación."
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
          tono="warn"
          descripcion="Cambios por revisar antes de iniciar las pruebas."
          icono={<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round"><path d="M4 4h16v16H4zM4 13h5l2 3h2l2-3h5M9 8h6" /></svg>}
        />
        <Kpi
          etiqueta="Corridas confirmadas"
          valor={textoContador('confirmed')}
          tono="info"
          descripcion={conteos.confirmed === undefined ? 'Abre Confirmadas para consultar el total.' : 'Pruebas autorizadas por tu equipo.'}
          icono={<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="8" /><path d="m8 12 3 3 5-6" /></svg>}
        />
        <Kpi
          etiqueta="Estado del entorno"
          valor={warm !== null ? ETIQUETA_ENTORNO[warm.state] : 'Sin dato'}
          claseValor="kpi-valor-20"
          tono={warm === null ? 'neutral' : warm.state === 'ready' && !warm.reset_verified ? 'warn' : TONO_ENTORNO[warm.state]}
          descripcion={warm === null ? 'No pudimos consultar su disponibilidad.' : warm.state === 'ready' && warm.reset_verified ? 'Disponible y con limpieza verificada.' : 'Consulta Entorno antes de iniciar otra corrida.'}
          icono={<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round"><rect x="3" y="4" width="18" height="13" rx="2" /><path d="M8 21h8M12 17v4M7 10h3l2-3 2 7 2-4h2" /></svg>}
        />
        <Kpi
          etiqueta="Rechazadas"
          valor={textoContador('rejected')}
          tono="danger"
          descripcion={conteos.rejected === undefined ? 'Abre Rechazadas para consultar el total.' : 'Cambios descartados por tu equipo.'}
          icono={<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round"><circle cx="12" cy="12" r="8" /><path d="m9 9 6 6m0-6-6 6" /></svg>}
        />
      </div>

      <div id="inbox-lista" role="tabpanel" aria-labelledby={`tab-${estado}`}>
        {lista === null && errorLista === null ? <p>Cargando notificaciones…</p> : null}
        {errorLista !== null ? <ApiErrorNotice error={errorLista} /> : null}
        {lista !== null && errorLista === null && lista.length === 0 ? (
          <div className="card">
            <section className="dashboard-empty" aria-labelledby="inbox-empty-title">
              <div className="dashboard-empty-art" aria-hidden="true">
                <svg viewBox="0 0 64 64" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M12 18h40l-4 30a4 4 0 0 1-4 3H20a4 4 0 0 1-4-3l-4-30z" />
                  <path d="M12 18l9-10h22l9 10" />
                  <path d="M12 18h13l4 6h6l4-6h13" />
                  <path d="M24 38h16" />
                </svg>
              </div>
              <h2 id="inbox-empty-title">{textoVacio}</h2>
              <p>{pestanaVacia?.vacioDescripcion ?? ''}</p>
              {estado === 'pending' ? (
                <Link to="/runs" className="btn btn-primary" data-testid="inbox-empty-runs-link">
                  Ver mis corridas <span aria-hidden="true">→</span>
                </Link>
              ) : null}
            </section>
          </div>
        ) : null}
        {lista !== null && errorLista === null && lista.length > 0 ? (
          <div className="card">
            <div className="card-pad listado-cabecera">
              <div className="card-title">Listado de notificaciones</div>
              <div className="card-sub">
                Revisa cada cambio y confirma las pruebas que quieres ejecutar.
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
                        <Badge tono={COLOR_ESTADO[n.state]} punto plano>
                          {n.state}
                        </Badge>
                      </td>
                      <td data-label="Acciones">
                        <button
                          type="button"
                          className="btn btn-primary btn-sm"
                          disabled={enVuelo}
                          onClick={(evento) => abrirDetalle(n.id, evento.currentTarget)}
                        >
                          Revisar
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
        <div className="modal-backdrop open">
          <div
            ref={dialogoRef}
            role="dialog"
            aria-modal="true"
            aria-labelledby="modal-titulo"
            aria-describedby="modal-paso"
            className="modal-dialog modal-lg"
            onKeyDown={alPulsarTeclaModal}
          >
            <header className="modal-header">
              <div className="modal-heading">
                <p className="modal-eyebrow">Cambio recibido</p>
                <h2 id="modal-titulo" tabIndex={-1} ref={tituloModal}>
                  Revisar notificación
                </h2>
                <p className="modal-paso" id="modal-paso">
                  {panelAbierto ? 'Paso 2 de 2 · Confirmar corrida' : 'Paso 1 de 2 · Detalle'}
                </p>
              </div>
              <Badge tono={COLOR_ESTADO[seleccionada.state]} punto>
                {seleccionada.state}
              </Badge>
            </header>
            <div className="modal-progress" aria-hidden="true">
              <span className="modal-progress-step active" />
              <span className={`modal-progress-step${panelAbierto ? ' active' : ''}`} />
            </div>
            {!panelAbierto ? (
              <>
                <div className="modal-body">
                  <section className="modal-section" aria-labelledby="modal-detalle-titulo">
                    <div className="modal-section-heading">
                      <h3 id="modal-detalle-titulo">Detalles del cambio</h3>
                      <p>Comprueba el origen y el artefacto antes de continuar.</p>
                    </div>
                    <dl className="kv-list modal-details">
                      <div className="modal-detail">
                        <dt>Repositorio</dt>
                        <dd>{seleccionada.repo}</dd>
                      </div>
                      <div className="modal-detail">
                        <dt>Evento</dt>
                        <dd><span className="mono">{seleccionada.github_event}</span></dd>
                      </div>
                      <div className="modal-detail">
                        <dt>SHA</dt>
                        <dd><code>{seleccionada.sha}</code></dd>
                      </div>
                      <div className="modal-detail">
                        <dt>Artefacto</dt>
                        <dd>
                          {seleccionada.artifact
                            ? textoArtefacto(seleccionada.artifact)
                            : 'Sin artefacto'}
                        </dd>
                      </div>
                    </dl>
                  </section>
                </div>
                <div className="modal-footer">
                  <p className="modal-footer-note">Nada se ejecuta sin tu confirmación.</p>
                  <div className="modal-footer-actions">
                    <button type="button" className="btn btn-secondary" onClick={cerrarModal} disabled={enVuelo}>
                      Cerrar
                    </button>
                    {seleccionada.state === 'pending' ? (
                      <button type="button" className="btn btn-primary" onClick={abrirPanel}>
                        Ver detalle →
                      </button>
                    ) : null}
                  </div>
                </div>
              </>
            ) : (
              <>
                <div className="modal-body">
                  <section className="modal-section" aria-labelledby="modal-resumen-titulo">
                    <div className="modal-section-heading">
                      <h3 id="modal-resumen-titulo">Resumen del cambio</h3>
                      <p>Esta será la versión que se probará en el entorno.</p>
                    </div>
                    <dl className="kv-list modal-details">
                      <div className="modal-detail">
                        <dt>Repositorio</dt>
                        <dd>{seleccionada.repo}</dd>
                      </div>
                      <div className="modal-detail">
                        <dt>SHA</dt>
                        <dd><code>{seleccionada.sha}</code></dd>
                      </div>
                      <div className="modal-detail">
                        <dt>Artefacto</dt>
                        <dd>
                          {seleccionada.artifact
                            ? textoArtefacto(seleccionada.artifact)
                            : 'Sin artefacto'}
                        </dd>
                      </div>
                    </dl>
                  </section>
                  <fieldset className="fieldset modal-flows" aria-describedby="modal-flows-hint">
                    <legend>Familias de flujos</legend>
                    <p className="modal-flows-hint" id="modal-flows-hint">
                      Elige qué pruebas quieres incluir en esta corrida.
                    </p>
                    <div className="modal-flow-options">
                      {FLOW_FAMILIES.map((f) => (
                        <label
                          key={f}
                          className={`check modal-flow-option${familias.includes(f) ? ' selected' : ''}`}
                        >
                          <input
                            type="checkbox"
                            name="familias"
                            value={f}
                            checked={familias.includes(f)}
                            onChange={() => alternarFamilia(f)}
                          />
                          <span className="mono">{f}</span>
                        </label>
                      ))}
                    </div>
                  </fieldset>
                  {errorAccion !== null ? <ApiErrorNotice error={errorAccion} /> : null}
                </div>
                <div className="modal-footer">
                  <p className="modal-footer-note">La corrida empezará solo después de confirmar.</p>
                  <div className="modal-footer-actions">
                    <button type="button" className="btn btn-secondary" onClick={volverAlDetalle} disabled={enVuelo}>
                      Atrás
                    </button>
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
              </>
            )}
          </div>
        </div>
      ) : null}
    </main>
  );
}
