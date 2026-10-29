import { useEffect, useRef, useState, type FormEvent } from 'react';
import { apiFetch } from '../../api/client';
import { ApiErrorNotice } from '../../api/errors';
import type { components } from '../../api/schema.gen';
import { Badge, type TonoBadge } from '../../components/Badge';
import { Card } from '../../components/Card';
import { PageHeader } from '../../components/PageHeader';
import { useSession } from '../../session/SessionContext';

type Repository = components['schemas']['Repository'];
type RepositoryActivity = components['schemas']['RepositoryActivity'];
type WarmState = Repository['warm']['state'];

const REPO_REGEX = /^[a-z0-9-]+\/[a-z0-9._-]+$/;

const COLOR_WARM: Record<WarmState, TonoBadge> = {
  ready: 'ok',
  dirty: 'warn',
  cuarentena: 'danger',
  'idle-escalado': 'neutral',
  provisioning: 'info',
  deleting: 'warn',
};

const TEXTO_WARM: Record<WarmState, string> = {
  ready: 'Listo',
  dirty: 'Requiere limpieza',
  cuarentena: 'En cuarentena',
  'idle-escalado': 'En pausa',
  provisioning: 'Aprovisionando',
  deleting: 'Eliminando',
};

function formatFecha(fechaStr?: string): string {
  if (!fechaStr) return '';
  const d = new Date(fechaStr);
  if (Number.isNaN(d.getTime())) return fechaStr;
  return d.toLocaleString('es-ES', { hour12: false, timeZone: 'UTC' });
}

export function RepositoriesPage() {
  const { session } = useSession();
  const [repos, setRepos] = useState<Repository[] | null>(null);
  const [activities, setActivities] = useState<Record<string, RepositoryActivity>>({});
  const [error, setError] = useState<unknown>(null);
  const [enVuelo, setEnVuelo] = useState(false);

  // Webhook panel copy state
  const [copiado, setCopiado] = useState(false);

  // Modal Añadir repositorio
  const [modalCrearAbierto, setModalCrearAbierto] = useState(false);
  const [repoInput, setRepoInput] = useState('');
  const [imageInput, setImageInput] = useState('');
  const [imageTouched, setImageTouched] = useState(false);
  const [contractDb, setContractDb] = useState(false);
  const [contractPort, setContractPort] = useState(false);
  const [contractImage, setContractImage] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [submitError, setSubmitError] = useState<unknown>(null);

  // Modal Eliminar repositorio
  const [repoAEliminar, setRepoAEliminar] = useState<Repository | null>(null);
  const [deleteError, setDeleteError] = useState<unknown>(null);

  // Focus refs
  const btnAnadirRef = useRef<HTMLButtonElement>(null);
  const inputRepoRef = useRef<HTMLInputElement>(null);
  const tituloModalCrearRef = useRef<HTMLHeadingElement>(null);
  const tituloModalEliminarRef = useRef<HTMLHeadingElement>(null);

  const webhookUrl =
    typeof window !== 'undefined'
      ? `${window.location.protocol}//${window.location.host}/webhooks/github`
      : 'http://localhost/webhooks/github';

  const cargarRepositorios = async () => {
    try {
      const data = await apiFetch<Repository[]>('/api/repositories');
      setRepos(data);
      setError(null);
      // Cargar actividades para cada repo
      for (const r of data) {
        cargarActividad(r.repo);
      }
    } catch (e) {
      setError(e);
    }
  };

  const cargarActividad = async (fullName: string) => {
    try {
      const act = await apiFetch<RepositoryActivity>(`/api/repositories/${fullName}/activity`);
      setActivities((prev) => ({ ...prev, [fullName]: act }));
    } catch {
      // Actividad no disponible de momento: se ignora silenciosamente
    }
  };

  useEffect(() => {
    void cargarRepositorios();
  }, []);

  // Foco en modal crear
  useEffect(() => {
    if (modalCrearAbierto) {
      inputRepoRef.current?.focus();
    }
  }, [modalCrearAbierto]);

  // Foco en modal eliminar
  useEffect(() => {
    if (repoAEliminar !== null) {
      tituloModalEliminarRef.current?.focus();
    }
  }, [repoAEliminar]);

  const canEdit = (repo: Repository) => {
    if (!session) return false;
    return session.principal.role === 'admin' || session.principal.principal_id === repo.owner;
  };

  const copiarUrl = () => {
    if (typeof navigator !== 'undefined' && navigator.clipboard) {
      navigator.clipboard.writeText(webhookUrl).catch(() => {});
      setCopiado(true);
      setTimeout(() => setCopiado(false), 2000);
    }
  };

  const abrirModalCrear = () => {
    setRepoInput('');
    setImageInput('');
    setImageTouched(false);
    setContractDb(false);
    setContractPort(false);
    setContractImage(false);
    setFormError(null);
    setSubmitError(null);
    setModalCrearAbierto(true);
  };

  const cerrarModalCrear = () => {
    if (enVuelo) return;
    setModalCrearAbierto(false);
    btnAnadirRef.current?.focus();
  };

  const handleRepoInputChange = (val: string) => {
    const limpio = val.trim().toLowerCase();
    setRepoInput(limpio);
    if (!imageTouched) {
      setImageInput(limpio ? `ghcr.io/${limpio}` : '');
    }
    if (formError && REPO_REGEX.test(limpio)) {
      setFormError(null);
    }
  };

  const handleImageInputChange = (val: string) => {
    setImageTouched(true);
    setImageInput(val.trim());
  };

  const handleSubmitCrear = async (e: FormEvent) => {
    e.preventDefault();
    if (!REPO_REGEX.test(repoInput)) {
      setFormError('El formato debe ser owner/repo en minúsculas (ej. org/app)');
      return;
    }
    if (!contractDb || !contractPort || !contractImage) {
      setFormError('Debes confirmar todos los requisitos del App Contract.');
      return;
    }

    setEnVuelo(true);
    setSubmitError(null);
    try {
      const nuevo = await apiFetch<Repository>('/api/repositories', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          repo: repoInput,
          image_repo: imageInput || undefined,
        }),
      });
      setRepos((prev) => (prev ? [nuevo, ...prev] : [nuevo]));
      setModalCrearAbierto(false);
      btnAnadirRef.current?.focus();
      cargarActividad(nuevo.repo);
    } catch (err) {
      setSubmitError(err);
    } finally {
      setEnVuelo(false);
    }
  };

  const toggleEnabled = async (repo: Repository) => {
    if (!canEdit(repo) || enVuelo) return;
    setEnVuelo(true);
    try {
      const actualizado = await apiFetch<Repository>(`/api/repositories/${repo.repo}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled: !repo.enabled }),
      });
      setRepos((prev) => prev?.map((r) => (r.repo === repo.repo ? actualizado : r)) ?? null);
    } catch (err) {
      setError(err);
    } finally {
      setEnVuelo(false);
    }
  };

  const abrirModalEliminar = (repo: Repository) => {
    setDeleteError(null);
    setRepoAEliminar(repo);
  };

  const cerrarModalEliminar = () => {
    if (enVuelo) return;
    setRepoAEliminar(null);
    btnAnadirRef.current?.focus();
  };

  const confirmarEliminar = async () => {
    if (!repoAEliminar || enVuelo) return;
    setEnVuelo(true);
    setDeleteError(null);
    try {
      await apiFetch<void>(`/api/repositories/${repoAEliminar.repo}`, {
        method: 'DELETE',
      });
      setRepos((prev) => prev?.filter((r) => r.repo !== repoAEliminar.repo) ?? null);
      setRepoAEliminar(null);
      btnAnadirRef.current?.focus();
    } catch (err) {
      setDeleteError(err);
    } finally {
      setEnVuelo(false);
    }
  };

  const formValido =
    REPO_REGEX.test(repoInput) && contractDb && contractPort && contractImage;

  return (
    <main className="az-container repositories-page">
      <PageHeader
        titulo="Repositorios"
        descripcion="Configura tus repositorios de GitHub para pruebas continuas de caja negra."
        acciones={
          <button
            ref={btnAnadirRef}
            type="button"
            className="btn btn-primary"
            data-testid="btn-anadir-repo"
            onClick={abrirModalCrear}
          >
            Añadir repositorio
          </button>
        }
      />

      {error !== null && <ApiErrorNotice error={error} />}

      {/* Panel de instrucciones de Webhook */}
      <Card
        titulo="Configuración de Webhooks en GitHub"
        sub="Configura tu repositorio en GitHub para enviar eventos de push a este clúster."
      >
        <dl className="kv-list" style={{ marginTop: '12px' }}>
          <dt>Payload URL</dt>
          <dd style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
            <code data-testid="webhook-url">{webhookUrl}</code>
            <button
              type="button"
              className="btn btn-sm"
              data-testid="btn-copiar-webhook-url"
              onClick={copiarUrl}
            >
              {copiado ? '¡Copiado!' : 'Copiar URL'}
            </button>
          </dd>
          <dt>Content type</dt>
          <dd>
            <code>application/json</code>
          </dd>
          <dt>Events</dt>
          <dd>
            Selecciona el evento <code>push</code>.
          </dd>
          <dt>Secret</dt>
          <dd>
            <em>Solicita el secreto de webhook a tu operador.</em>
          </dd>
        </dl>
      </Card>

      {/* Lista de repositorios */}
      {repos === null && error === null && <p>Cargando repositorios…</p>}

      {repos !== null && (
        <Card titulo="Repositorios registrados">
          {repos.length === 0 ? (
            <div className="dashboard-empty" style={{ padding: '24px 0', textAlign: 'center' }}>
              <p>Sin repositorios registrados aún. Añade el primero para comenzar.</p>
              <button
                type="button"
                className="btn btn-primary"
                onClick={abrirModalCrear}
                style={{ marginTop: '12px' }}
              >
                Añadir repositorio
              </button>
            </div>
          ) : (
            <div className="table-wrap">
              <table className="data-table" data-testid="repos-table">
                <thead>
                  <tr>
                    <th scope="col">Repositorio</th>
                    <th scope="col">Prefijo de imagen</th>
                    <th scope="col">Entorno Warm</th>
                    <th scope="col">Actividad de Webhooks</th>
                    <th scope="col">Estado</th>
                    <th scope="col">
                      <span className="sr-only">Acciones</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {repos.map((repo) => {
                    const act = activities[repo.repo];
                    const editable = canEdit(repo);
                    const warmState = repo.warm?.state ?? 'provisioning';

                    return (
                      <tr key={repo.repo} data-testid={`repo-row-${repo.repo.replace('/', '-')}`}>
                        <td data-label="Repositorio" className="mono">
                          <strong>{repo.repo}</strong>
                        </td>
                        <td data-label="Prefijo de imagen" className="mono">
                          <code>{repo.image_repo}</code>
                        </td>
                        <td data-label="Entorno Warm">
                          <Badge tono={COLOR_WARM[warmState] ?? 'neutral'} punto>
                            {TEXTO_WARM[warmState] ?? warmState}
                          </Badge>
                        </td>
                        <td data-label="Actividad de Webhooks">
                          {act ? (
                            act.has_webhook ? (
                              <Badge tono="ok">
                                Último webhook: {formatFecha(act.last_seen_at)}
                              </Badge>
                            ) : (
                              <span className="muted" data-testid="no-webhook-text">
                                Sin webhooks recibidos aún
                              </span>
                            )
                          ) : (
                            <span className="muted">Consultando…</span>
                          )}
                        </td>
                        <td data-label="Estado">
                          <Badge tono={repo.enabled ? 'ok' : 'neutral'}>
                            {repo.enabled ? 'Activo' : 'Desactivado'}
                          </Badge>
                        </td>
                        <td data-label="Acciones">
                          <div style={{ display: 'flex', gap: '8px' }}>
                            <button
                              type="button"
                              className="btn btn-sm"
                              data-testid={`toggle-btn-${repo.repo.replace('/', '-')}`}
                              onClick={() => void toggleEnabled(repo)}
                              disabled={!editable || enVuelo}
                              title={
                                !editable
                                  ? 'Solo el creador o un administrador pueden modificar este repositorio'
                                  : undefined
                              }
                            >
                              {repo.enabled ? 'Desactivar' : 'Activar'}
                            </button>
                            <button
                              type="button"
                              className="btn btn-sm"
                              data-testid={`delete-btn-${repo.repo.replace('/', '-')}`}
                              onClick={() => abrirModalEliminar(repo)}
                              disabled={!editable || enVuelo}
                              title={
                                !editable
                                  ? 'Solo el creador o un administrador pueden eliminar este repositorio'
                                  : undefined
                              }
                            >
                              Eliminar
                            </button>
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </Card>
      )}

      {/* Modal Añadir Repositorio */}
      {modalCrearAbierto && (
        <div className="modal-backdrop open">
          <div
            role="dialog"
            aria-modal="true"
            aria-labelledby="modal-crear-titulo"
            className="modal-dialog"
            onKeyDown={(e) => {
              if (e.key === 'Escape') cerrarModalCrear();
            }}
            style={{ maxWidth: '520px', width: '100%', padding: '24px' }}
          >
            <h2 id="modal-crear-titulo" ref={tituloModalCrearRef} tabIndex={-1}>
              Añadir repositorio
            </h2>
            <form onSubmit={(e) => void handleSubmitCrear(e)}>
              <div className="form-field" style={{ marginTop: '16px' }}>
                <label htmlFor="repo-input">Repositorio (owner/repo en minúsculas)</label>
                <input
                  id="repo-input"
                  ref={inputRepoRef}
                  type="text"
                  placeholder="ej. acme/backend-service"
                  value={repoInput}
                  onChange={(e) => handleRepoInputChange(e.target.value)}
                  required
                />
              </div>

              <div className="form-field" style={{ marginTop: '16px' }}>
                <label htmlFor="image-input">Prefijo de imagen en GHCR</label>
                <input
                  id="image-input"
                  type="text"
                  placeholder="ej. ghcr.io/acme/backend-service"
                  value={imageInput}
                  onChange={(e) => handleImageInputChange(e.target.value)}
                />
              </div>

              <fieldset className="fieldset" style={{ marginTop: '20px' }}>
                <legend>Checklist del App Contract</legend>
                <label className="check" style={{ display: 'flex', gap: '8px', marginBottom: '8px' }}>
                  <input
                    type="checkbox"
                    checked={contractDb}
                    onChange={(e) => setContractDb(e.target.checked)}
                    data-testid="check-contract-db"
                  />
                  <span>Lee <code>DATABASE_URL</code> del entorno de ejecución</span>
                </label>
                <label className="check" style={{ display: 'flex', gap: '8px', marginBottom: '8px' }}>
                  <input
                    type="checkbox"
                    checked={contractPort}
                    onChange={(e) => setContractPort(e.target.checked)}
                    data-testid="check-contract-port"
                  />
                  <span>Expone el listener de línea base en el puerto <code>:9090</code></span>
                </label>
                <label className="check" style={{ display: 'flex', gap: '8px' }}>
                  <input
                    type="checkbox"
                    checked={contractImage}
                    onChange={(e) => setContractImage(e.target.checked)}
                    data-testid="check-contract-image"
                  />
                  <span>Publica la imagen en GHCR al hacer push a <code>main</code></span>
                </label>
              </fieldset>

              {formError && (
                <div role="alert" className="alert-box alert-danger" style={{ marginTop: '16px' }}>
                  {formError}
                </div>
              )}
              {submitError !== null && (
                <div style={{ marginTop: '16px' }}>
                  <ApiErrorNotice error={submitError} />
                </div>
              )}

              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '12px', marginTop: '24px' }}>
                <button
                  type="button"
                  className="btn btn-secondary"
                  onClick={cerrarModalCrear}
                  disabled={enVuelo}
                >
                  Cancelar
                </button>
                <button
                  type="submit"
                  className="btn btn-primary"
                  disabled={!formValido || enVuelo}
                  data-testid="btn-submit-repo"
                >
                  {enVuelo ? 'Registrando…' : 'Registrar repositorio'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Modal Confirmar Eliminación */}
      {repoAEliminar !== null && (
        <div className="modal-backdrop open">
          <div
            role="dialog"
            aria-modal="true"
            aria-labelledby="modal-eliminar-titulo"
            className="modal-dialog"
            onKeyDown={(e) => {
              if (e.key === 'Escape') cerrarModalEliminar();
            }}
            style={{ maxWidth: '480px', width: '100%', padding: '24px' }}
          >
            <h2 id="modal-eliminar-titulo" ref={tituloModalEliminarRef} tabIndex={-1}>
              Eliminar repositorio
            </h2>
            <p style={{ marginTop: '12px' }}>
              ¿Estás seguro de que deseas eliminar el repositorio <strong>{repoAEliminar.repo}</strong> y su entorno warm asociado? Esta acción no se puede deshacer.
            </p>

            {deleteError !== null && (
              <div style={{ marginTop: '16px' }}>
                <ApiErrorNotice error={deleteError} />
              </div>
            )}

            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '12px', marginTop: '24px' }}>
              <button
                type="button"
                className="btn btn-secondary"
                onClick={cerrarModalEliminar}
                disabled={enVuelo}
              >
                Cancelar
              </button>
              <button
                type="button"
                className="btn btn-primary"
                data-testid="btn-confirm-delete"
                onClick={() => void confirmarEliminar()}
                disabled={enVuelo}
              >
                {enVuelo ? 'Eliminando…' : 'Eliminar repositorio'}
              </button>
            </div>
          </div>
        </div>
      )}
    </main>
  );
}
