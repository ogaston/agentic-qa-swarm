import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import type { components } from '../../api/schema.gen';
import { ApiError, apiFetch } from '../../api/client';
import { ApiErrorNotice } from '../../api/errors';
import { FLOW_FAMILIES, type FlowFamily } from '../../inbox/flows';

type Notification = components['schemas']['Notification'];
type ConfirmationReceipt = components['schemas']['ConfirmationReceipt'];
type Estado = Notification['state'];

const PESTANAS: { estado: Estado; etiqueta: string; vacio: string }[] = [
  { estado: 'pending', etiqueta: 'Pendientes', vacio: 'No hay notificaciones pendientes' },
  { estado: 'confirmed', etiqueta: 'Confirmadas', vacio: 'No hay notificaciones confirmadas' },
  { estado: 'rejected', etiqueta: 'Rechazadas', vacio: 'No hay notificaciones rechazadas' },
];

const MSG_YA_NO_PENDIENTE = 'Esta notificación ya no está pendiente';
const MSG_NO_EXISTE = 'La notificación ya no existe';

/** Inbox: lista filtrable, detalle y confirmación explícita. Sin polling ni confirmaciones automáticas. */
export function InboxPage() {
  const navigate = useNavigate();
  const [estado, setEstado] = useState<Estado>('pending');
  const [lista, setLista] = useState<Notification[] | null>(null);
  const [errorLista, setErrorLista] = useState<unknown>(null);
  const [seleccionadaId, setSeleccionadaId] = useState<string | null>(null);
  const [panelAbierto, setPanelAbierto] = useState(false);
  const [familias, setFamilias] = useState<FlowFamily[]>([...FLOW_FAMILIES]);
  const [enVuelo, setEnVuelo] = useState(false);
  const [mensajeAccion, setMensajeAccion] = useState<string | null>(null);
  const [errorAccion, setErrorAccion] = useState<unknown>(null);
  const enCurso = useRef(false);
  const peticion = useRef(0);
  const tituloPanel = useRef<HTMLHeadingElement | null>(null);

  // Carga la pestaña indicada. Una respuesta vieja (otra pestaña o recarga posterior) se descarta.
  const cargar = useCallback(async (porEstado: Estado) => {
    const id = ++peticion.current;
    setErrorLista(null);
    try {
      const datos = await apiFetch<Notification[]>(`/api/notifications?state=${porEstado}`);
      if (id !== peticion.current) return;
      setLista(datos);
    } catch (error) {
      if (id !== peticion.current) return;
      setLista([]);
      setErrorLista(error);
    }
  }, []);

  useEffect(() => {
    setLista(null);
    setSeleccionadaId(null);
    setPanelAbierto(false);
    void cargar(estado);
  }, [estado, cargar]);

  const seleccionada = lista?.find((n) => n.id === seleccionadaId) ?? null;

  function elegirPestana(nueva: Estado) {
    setMensajeAccion(null);
    setErrorAccion(null);
    setEstado(nueva);
  }

  function abrirPanel() {
    setFamilias([...FLOW_FAMILIES]);
    setErrorAccion(null);
    setMensajeAccion(null);
    setPanelAbierto(true);
  }

  // Foco inicial en el título del panel.
  useEffect(() => {
    if (panelAbierto) tituloPanel.current?.focus();
  }, [panelAbierto]);

  function cerrarPanel() {
    if (enCurso.current) return;
    setPanelAbierto(false);
  }

  function alPulsarTeclaPanel(evento: KeyboardEvent<HTMLDivElement>) {
    if (evento.key === 'Escape') cerrarPanel();
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
      navigate(`/runs/${encodeURIComponent(recibo.run_id)}`);
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) return; // lo gestiona la sesión (T03)
      if (error instanceof ApiError && error.status === 409) {
        setPanelAbierto(false);
        setMensajeAccion(MSG_YA_NO_PENDIENTE);
        void cargar(estado);
      } else if (error instanceof ApiError && error.status === 404) {
        setPanelAbierto(false);
        setMensajeAccion(MSG_NO_EXISTE);
        void cargar(estado);
      } else {
        setErrorAccion(error);
      }
    } finally {
      enCurso.current = false;
      setEnVuelo(false);
    }
  }

  const textoVacio = PESTANAS.find((p) => p.estado === estado)?.vacio ?? '';

  return (
    <main>
      <h1>Inbox</h1>

      <div role="tablist" aria-label="Estado de las notificaciones">
        {PESTANAS.map((p) => (
          <button
            key={p.estado}
            type="button"
            role="tab"
            id={`tab-${p.estado}`}
            aria-selected={p.estado === estado}
            aria-controls="inbox-lista"
            onClick={() => elegirPestana(p.estado)}
          >
            {p.etiqueta}
          </button>
        ))}
      </div>

      <div>
        <button
          type="button"
          onClick={() => {
            void cargar(estado);
          }}
        >
          Actualizar
        </button>
      </div>

      <div id="inbox-lista" role="region" aria-labelledby={`tab-${estado}`}>
        {lista === null && errorLista === null ? <p>Cargando notificaciones…</p> : null}
        {errorLista !== null ? <ApiErrorNotice error={errorLista} /> : null}
        {lista !== null && errorLista === null && lista.length === 0 ? (
          <p>{textoVacio}</p>
        ) : null}
        {lista !== null && errorLista === null && lista.length > 0 ? (
          <table>
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
                <tr key={n.id} aria-selected={n.id === seleccionadaId}>
                  <td>{n.repo}</td>
                  <td>{n.github_event}</td>
                  <td>
                    <code>{n.sha.slice(0, 7)}</code>
                  </td>
                  <td>{n.artifact ? `${n.artifact.kind} · ${n.artifact.ref}` : '—'}</td>
                  <td>{n.state}</td>
                  <td>
                    <button
                      type="button"
                      onClick={() => {
                        setSeleccionadaId(n.id);
                        setPanelAbierto(false);
                        setErrorAccion(null);
                        setMensajeAccion(null);
                      }}
                    >
                      Ver detalle
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : null}
      </div>

      {mensajeAccion !== null ? <p role="alert">{mensajeAccion}</p> : null}

      {seleccionada !== null ? (
        <section aria-label="Detalle de la notificación">
          <h2>Detalle</h2>
          <dl>
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
                ? `${seleccionada.artifact.kind} · ${seleccionada.artifact.ref}`
                : 'Sin artefacto'}
            </dd>
            <dt>Estado</dt>
            <dd>{seleccionada.state}</dd>
          </dl>
          {seleccionada.state === 'pending' && !panelAbierto ? (
            <button type="button" onClick={abrirPanel}>
              Revisar y confirmar
            </button>
          ) : null}
        </section>
      ) : null}

      {panelAbierto && seleccionada !== null ? (
        <div
          role="dialog"
          aria-modal="true"
          aria-labelledby="confirm-titulo"
          onKeyDown={alPulsarTeclaPanel}
        >
          <h2 id="confirm-titulo" tabIndex={-1} ref={tituloPanel}>
            Confirmar corrida
          </h2>
          <dl>
            <dt>Repositorio</dt>
            <dd>{seleccionada.repo}</dd>
            <dt>SHA</dt>
            <dd>
              <code>{seleccionada.sha}</code>
            </dd>
            <dt>Artefacto</dt>
            <dd>
              {seleccionada.artifact
                ? `${seleccionada.artifact.kind} · ${seleccionada.artifact.ref}`
                : 'Sin artefacto'}
            </dd>
          </dl>
          <fieldset>
            <legend>Familias de flujos</legend>
            {FLOW_FAMILIES.map((f) => (
              <label key={f}>
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
          <button type="button" onClick={cerrarPanel} disabled={enVuelo}>
            Cancelar
          </button>{' '}
          <button
            type="button"
            onClick={() => void confirmar()}
            disabled={familias.length === 0 || enVuelo}
          >
            Confirmar corrida
          </button>
        </div>
      ) : null}
    </main>
  );
}
