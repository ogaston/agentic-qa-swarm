import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
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

  // Modal Eliminar repositorio
  const [repoAEliminar, setRepoAEliminar] = useState<Repository | null>(null);
  const [deleteError, setDeleteError] = useState<unknown>(null);

  // Focus refs
  const btnAnadirRef = useRef<HTMLAnchorElement>(null);
  const tituloModalEliminarRef = useRef<HTMLHeadingElement>(null);

  // La URL debe apuntar al intake público, no necesariamente al host del dashboard.
  const webhookUrl = import.meta.env.VITE_PUBLIC_WEBHOOK_URL?.trim() || '';

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
    if (webhookUrl && typeof navigator !== 'undefined' && navigator.clipboard) {
      navigator.clipboard.writeText(webhookUrl).catch(() => {});
      setCopiado(true);
      setTimeout(() => setCopiado(false), 2000);
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
      await apiFetch<undefined>(`/api/repositories/${repoAEliminar.repo}`, {
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

  return (
    <main className="az-container repositories-page">
      <PageHeader
        titulo="Repositorios"
        descripcion="Configura tus repositorios de GitHub para pruebas continuas de caja negra."
        acciones={
          <Link
            ref={btnAnadirRef}
            to="/repositories/new"
            className="btn btn-primary"
            data-testid="btn-anadir-repo"
          >
            Añadir repositorio
          </Link>
        }
      />

      {error !== null && <ApiErrorNotice error={error} />}

      {/* Panel de instrucciones de Webhook */}
      <Card
        titulo="Configuración de Webhooks en GitHub"
        sub="Configura tu repositorio en GitHub para enviar eventos de push, pull request o tag a este clúster."
      >
        <dl className="kv-list" style={{ marginTop: '12px' }}>
          <dt>Payload URL</dt>
          <dd style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
            {webhookUrl ? (
              <>
                <code data-testid="webhook-url">{webhookUrl}</code>
                <button type="button" className="btn btn-sm" data-testid="btn-copiar-webhook-url" onClick={copiarUrl}>
                  {copiado ? '¡Copiado!' : 'Copiar URL'}
                </button>
              </>
            ) : (
              <span>La URL pública del webhook no está configurada. Solicítala a tu operador.</span>
            )}
          </dd>
          <dt>Content type</dt>
          <dd>
            <code>application/json</code>
          </dd>
          <dt>Events</dt>
          <dd>
            Selecciona <code>push</code>, <code>pull_request</code> y <code>release</code>.
          </dd>
          <dt>Secret</dt>
          <dd>
            <em>
              Cada nuevo proyecto dispone de un secreto HMAC SHA-256 exclusivo generado durante el asistente de conexión.
              Para repositorios heredados, se utiliza el secreto global de la plataforma.
            </em>
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
              <Link
                to="/repositories/new"
                className="btn btn-primary"
                style={{ marginTop: '12px' }}
              >
                Añadir repositorio
              </Link>
            </div>
          ) : (
            <div className="table-wrap">
              <table className="data-table" data-testid="repos-table">
                <thead>
                  <tr>
                    <th scope="col">Proyecto / Repositorio</th>
                    <th scope="col">Recursos</th>
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
                        <td data-label="Proyecto / Repositorio">
                          {repo.name && repo.name !== repo.repo && (
                            <div style={{ fontWeight: 600 }}>{repo.name}</div>
                          )}
                          <span
                            className="mono"
                            style={{
                              fontSize: repo.name && repo.name !== repo.repo ? '12px' : 'inherit',
                              color: repo.name && repo.name !== repo.repo ? 'var(--text-muted)' : 'inherit',
                            }}
                          >
                            <strong>{repo.repo}</strong>
                          </span>
                        </td>
                        <td data-label="Recursos">
                          {repo.resources && repo.resources.length > 0 ? (
                            <div style={{ display: 'flex', gap: '4px', flexWrap: 'wrap' }}>
                              {repo.resources.map((res) => (
                                <span key={res} className="badge badge-neutral mono" style={{ fontSize: '11px' }}>
                                  {res}
                                </span>
                              ))}
                            </div>
                          ) : (
                            <span className="badge badge-neutral mono" style={{ fontSize: '11px' }}>
                              stateless
                            </span>
                          )}
                        </td>
                        <td data-label="Prefijo de imagen" className="mono">
                          <code>{repo.image_repo}</code>
                        </td>
                        <td data-label="Entorno Warm">
                          <div style={{ display: 'flex', gap: '4px', alignItems: 'center', flexWrap: 'wrap' }}>
                            <Badge tono={COLOR_WARM[warmState] ?? 'neutral'} punto>
                              {TEXTO_WARM[warmState] ?? warmState}
                            </Badge>
                            {repo.warm?.infrastructure_ready && (
                              <span className="badge badge-ok" title="Infraestructura lista" style={{ fontSize: '11px' }}>
                                infra lista
                              </span>
                            )}
                          </div>
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
