import { useEffect, useRef, useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { apiFetch, ApiError } from '../../api/client';
import { ApiErrorNotice } from '../../api/errors';
import type { components } from '../../api/schema.gen';
import { Badge } from '../../components/Badge';
import { PageHeader } from '../../components/PageHeader';
import { usePolling } from '../../hooks/usePolling';

type Repository = components['schemas']['Repository'];
type RepositoryCredentials = components['schemas']['RepositoryCredentials'];
type RepositoryActivity = components['schemas']['RepositoryActivity'];

const REPO_REGEX = /^[a-z0-9-]+\/[a-z0-9._-]+$/;

const WIZARD_STEPS = [
  { title: 'Detalles del proyecto' },
  { title: 'Recursos' },
  { title: 'Repositorio' },
  { title: 'Verificación' },
] as const;

interface PollResult {
  repo: Repository;
  act: RepositoryActivity;
}

function isRepositoryCredentials(value: unknown): value is RepositoryCredentials {
  if (typeof value !== 'object' || value === null) return false;
  const candidate = value as Record<string, unknown>;
  return typeof candidate.webhook_secret === 'string' && typeof candidate.repository === 'object' && candidate.repository !== null;
}

function isProjectContractMismatch(error: unknown): error is ApiError {
  return (
    error instanceof ApiError &&
    error.status === 400 &&
    error.code === 'bad_request' &&
    error.message === 'JSON inválido'
  );
}

export function ProjectWizardPage() {
  const [paso, setPaso] = useState<1 | 2 | 3 | 4>(1);

  // Paso 1: Proyecto y App Contract
  const [nombre, setNombre] = useState('');
  const [descripcion, setDescripcion] = useState('');
  const [contractPort, setContractPort] = useState(false);
  const [contractImage, setContractImage] = useState(false);
  const [contractDb, setContractDb] = useState(false);

  // Paso 2: Recursos
  const [recursos, setRecursos] = useState<('postgresql' | 'redis')[]>(['postgresql', 'redis']);

  // Paso 3: Repositorio GitHub
  const [repo, setRepo] = useState('');
  const [imageRepo, setImageRepo] = useState('');
  const [imageTouched, setImageTouched] = useState(false);

  // Errores de validación
  const [erroresPaso1, setErroresPaso1] = useState<string[]>([]);
  const [erroresPaso3, setErroresPaso3] = useState<string[]>([]);
  const [errorEnvio, setErrorEnvio] = useState<unknown>(null);
  const [enVuelo, setEnVuelo] = useState(false);

  // Paso 4: Credenciales y Espera
  const [credenciales, setCredenciales] = useState<RepositoryCredentials | null>(null);
  const [copiadoUrl, setCopiadoUrl] = useState(false);
  const [copiadoSecret, setCopiadoSecret] = useState(false);
  const [rotando, setRotando] = useState(false);
  const [mensajeRotacion, setMensajeRotacion] = useState<string | null>(null);
  const [tiempoEspera, setTiempoEspera] = useState(0);

  // Refs para accesibilidad y foco
  const encabezadoRef = useRef<HTMLHeadingElement>(null);

  const webhookPublicUrl =
    import.meta.env.VITE_PUBLIC_WEBHOOK_URL?.trim() ||
    (typeof window !== 'undefined' ? `${window.location.origin}/api/webhooks/github` : 'https://qa.agentic-swarm.local/api/v1/webhooks/github');

  // Enfoque al cambiar de paso
  useEffect(() => {
    encabezadoRef.current?.focus();
  }, [paso]);

  // Contador de tiempo de espera en paso 4
  useEffect(() => {
    if (paso !== 4) return;
    const timer = setInterval(() => setTiempoEspera((t) => t + 1), 1000);
    return () => clearInterval(timer);
  }, [paso]);

  // Sondeo en paso 4
  const pollTarget = credenciales?.repository.repo;
  const polling = usePolling<PollResult>(
    async () => {
      if (!pollTarget) throw new Error('Sin repositorio');
      const [repoRes, actRes] = await Promise.all([
        apiFetch<Repository>(`/api/repositories/${pollTarget}`),
        apiFetch<RepositoryActivity>(`/api/repositories/${pollTarget}/activity`),
      ]);
      return { repo: repoRes, act: actRes };
    },
    2000,
    {
      enabled: paso === 4 && !!pollTarget,
      isTerminal: (data) => data.repo.warm.infrastructure_ready && data.act.has_webhook,
      maxFailures: 30,
    },
  );

  const infraestructuraLista = polling.data?.repo.warm.infrastructure_ready ?? false;
  const webhookVerificado = polling.data?.act.has_webhook ?? false;
  const completado = infraestructuraLista && webhookVerificado;

  // Manejadores Paso 1
  function validarPaso1(): boolean {
    const errs: string[] = [];
    if (!nombre.trim()) {
      errs.push('El nombre del proyecto es obligatorio.');
    }
    if (!contractPort) {
      errs.push('Debes confirmar la exposición de HTTP en el puerto 8080.');
    }
    if (!contractImage) {
      errs.push('Debes confirmar la publicación de imágenes en GHCR.');
    }
    if (recursos.includes('postgresql') && !contractDb) {
      errs.push('Debes confirmar el uso de DATABASE_URL y ejecución de migraciones.');
    }
    setErroresPaso1(errs);
    return errs.length === 0;
  }

  function avanzarPaso1() {
    if (validarPaso1()) {
      setPaso(2);
    }
  }

  // Manejadores Paso 2
  function alternarRecurso(r: 'postgresql' | 'redis') {
    setRecursos((prev) => (prev.includes(r) ? prev.filter((x) => x !== r) : [...prev, r]));
  }

  // Manejadores Paso 3
  function handleRepoChange(val: string) {
    const limpio = val.trim().toLowerCase();
    setRepo(limpio);
    if (!imageTouched) {
      setImageRepo(limpio ? `ghcr.io/${limpio}` : '');
    }
    if (erroresPaso3.length > 0 && REPO_REGEX.test(limpio)) {
      setErroresPaso3([]);
    }
  }

  function validarPaso3(): boolean {
    const errs: string[] = [];
    if (!REPO_REGEX.test(repo)) {
      errs.push('El repositorio debe tener formato owner/repo en minúsculas (ej. acme/api).');
    }
    setErroresPaso3(errs);
    return errs.length === 0;
  }

  async function handleCrearProyecto(e: FormEvent) {
    e.preventDefault();
    if (!validarPaso1() || !validarPaso3() || enVuelo) return;

    setEnVuelo(true);
    setErrorEnvio(null);
    try {
      const res = await apiFetch<unknown>('/api/repositories', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: nombre.trim(),
          description: descripcion.trim() || undefined,
          repo,
          image_repo: imageRepo.trim() || undefined,
          resources: recursos,
        }),
      });
      if (!isRepositoryCredentials(res)) {
        throw new ApiError({
          status: 503,
          code: 'project_credentials_unavailable',
          message: 'El control plane no devolvió las credenciales del proyecto. Actualiza sus servicios y vuelve a intentarlo.',
          requestId: 'project-credentials-response',
        });
      }
      setCredenciales(res);
      setPaso(4);
    } catch (err) {
      setErrorEnvio(err);
    } finally {
      setEnVuelo(false);
    }
  }

  // Copia de credenciales al portapapeles
  function copiarTexto(texto: string, setFn: (v: boolean) => void) {
    if (typeof navigator !== 'undefined' && navigator.clipboard) {
      navigator.clipboard.writeText(texto).catch(() => {});
      setFn(true);
      setTimeout(() => setFn(false), 2000);
    }
  }

  // Rotación de credenciales
  async function rotarSecreto() {
    if (!pollTarget || rotando) return;
    setRotando(true);
    setMensajeRotacion(null);
    try {
      const res = await apiFetch<RepositoryCredentials>(`/api/repositories/${pollTarget}/webhook-secret/rotate`, {
        method: 'POST',
      });
      setCredenciales(res);
      setMensajeRotacion('Nuevo secreto generado con éxito. Actualízalo en GitHub Settings → Webhooks.');
    } catch (err) {
      if (err instanceof ApiError) {
        setMensajeRotacion(`Error al rotar secreto: ${err.message}`);
      } else {
        setMensajeRotacion('Error al rotar secreto.');
      }
    } finally {
      setRotando(false);
    }
  }

  return (
    <main className="az-container project-wizard-page">
      <PageHeader
        titulo="Conectar nueva aplicación"
        descripcion="Asistente de configuración de proyecto, recursos de infraestructura y webhook de GitHub."
      />

      <nav aria-label="Progreso del asistente" className="wizard-stepper" data-testid="wizard-stepper">
        <ol className="wizard-stepper-list">
          {WIZARD_STEPS.map((step, index) => {
            const number = index + 1;
            const isCompleted = paso > number;
            const isCurrent = paso === number;
            const status = isCompleted ? 'Completado' : isCurrent ? 'En curso' : 'Pendiente';
            const state = isCompleted ? 'completed' : isCurrent ? 'active' : 'pending';

            return (
              <li
                key={step.title}
                className={`wizard-stepper-item ${state}`}
                aria-current={isCurrent ? 'step' : undefined}
                data-testid={`wizard-step-${number}`}
              >
                <div className="wizard-step-track" aria-hidden="true">
                  <span className="wizard-step-circle">
                    {isCompleted ? (
                      <svg viewBox="0 0 16 16" focusable="false">
                        <path d="m3.25 8.25 2.9 2.9 6.6-6.6" fill="none" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" />
                      </svg>
                    ) : (
                      number
                    )}
                  </span>
                  {number < WIZARD_STEPS.length && <span className="wizard-step-connector" />}
                </div>
                <div className="wizard-step-copy">
                  <span className="wizard-step-eyebrow">Paso {number}</span>
                  <strong className="wizard-step-title">{step.title}</strong>
                  <span className="wizard-step-status">{status}</span>
                </div>
              </li>
            );
          })}
        </ol>
      </nav>

      <div className="card card-pad wizard-card">
        {/* PASO 1: PROYECTO Y APP CONTRACT */}
        {paso === 1 && (
          <section aria-labelledby="step-1-title">
            <h2 id="step-1-title" ref={encabezadoRef} tabIndex={-1} className="card-title">
              1. Información del proyecto y App Contract
            </h2>
            <p className="card-sub">
              Define el nombre visible de tu aplicación y confirma los requisitos mínimos del contenedor.
            </p>

            {erroresPaso1.length > 0 && (
              <div role="alert" className="alert-box alert-danger" style={{ marginBottom: '1rem' }}>
                <ul style={{ margin: 0, paddingLeft: '1.25rem' }}>
                  {erroresPaso1.map((err, i) => (
                    <li key={i}>{err}</li>
                  ))}
                </ul>
              </div>
            )}

            <div className="form-field">
              <label htmlFor="wizard-project-name">
                Nombre del proyecto <span aria-hidden="true">*</span>
              </label>
              <input
                id="wizard-project-name"
                data-testid="wizard-project-name"
                type="text"
                required
                maxLength={120}
                value={nombre}
                onChange={(e) => setNombre(e.target.value)}
                placeholder="Ej. Pasarela de Pagos"
              />
            </div>

            <div className="form-field">
              <label htmlFor="wizard-project-desc">Descripción (opcional)</label>
              <textarea
                id="wizard-project-desc"
                data-testid="wizard-project-desc"
                rows={2}
                maxLength={500}
                value={descripcion}
                onChange={(e) => setDescripcion(e.target.value)}
                placeholder="Breve descripción del servicio o dominio de negocio"
              />
            </div>

            <fieldset className="fieldset-box" style={{ marginTop: '1.5rem' }}>
              <legend style={{ fontWeight: 600, fontSize: '13px', color: 'var(--text)' }}>
                Requisitos del App Contract (Caja Negra)
              </legend>
              <p style={{ fontSize: '12px', color: 'var(--text-muted)', marginBottom: '0.75rem' }}>
                La plataforma ejecuta pruebas de caja negra contra tu servicio desplegado en un sandbox warm aislado.
              </p>

              <div className="contract-checkbox-item">
                <input
                  id="wizard-contract-port"
                  data-testid="wizard-contract-port"
                  type="checkbox"
                  checked={contractPort}
                  onChange={(e) => setContractPort(e.target.checked)}
                />
                <label htmlFor="wizard-contract-port" style={{ fontSize: '13px', cursor: 'pointer' }}>
                  <strong>Puerto HTTP 8080:</strong> La aplicación escucha en el puerto 8080 y expone un endpoint{' '}
                  <code>/healthz</code> para probes de disponibilidad.
                </label>
              </div>

              <div className="contract-checkbox-item">
                <input
                  id="wizard-contract-image"
                  data-testid="wizard-contract-image"
                  type="checkbox"
                  checked={contractImage}
                  onChange={(e) => setContractImage(e.target.checked)}
                />
                <label htmlFor="wizard-contract-image" style={{ fontSize: '13px', cursor: 'pointer' }}>
                  <strong>Imagen en GHCR:</strong> Tu pipeline de CI publica la imagen compilada en GitHub Container
                  Registry (ghcr.io) con tag por commit SHA.
                </label>
              </div>

              {recursos.includes('postgresql') && (
                <div className="contract-checkbox-item">
                  <input
                    id="wizard-contract-db"
                    data-testid="wizard-contract-db"
                    type="checkbox"
                    checked={contractDb}
                    onChange={(e) => setContractDb(e.target.checked)}
                  />
                  <label htmlFor="wizard-contract-db" style={{ fontSize: '13px', cursor: 'pointer' }}>
                    <strong>Base de Datos Relacional:</strong> La aplicación lee su conexión desde la variable{' '}
                    <code>DATABASE_URL</code> y aplica sus propias migraciones al inicio.
                  </label>
                </div>
              )}
            </fieldset>

            <div className="wizard-buttons-row">
              <Link to="/inbox" className="btn btn-secundario">
                Cancelar
              </Link>
              <button
                type="button"
                className="btn btn-primary"
                data-testid="wizard-btn-next-step1"
                onClick={avanzarPaso1}
              >
                Siguiente: Recursos →
              </button>
            </div>
          </section>
        )}

        {/* PASO 2: RECURSOS */}
        {paso === 2 && (
          <section aria-labelledby="step-2-title">
            <h2 id="step-2-title" ref={encabezadoRef} tabIndex={-1} className="card-title">
              2. Recursos de infraestructura
            </h2>
            <p className="card-sub">
              Selecciona los servicios auxiliares aprovisionados en el entorno warm aislado. Puedes seleccionar uno, ambos
              o ninguno (aplicación stateless).
            </p>

            <section className="resource-category" aria-labelledby="resource-databases-title">
              <div className="resource-category-heading">
                <h3 id="resource-databases-title">Bases de datos</h3>
                <p>Almacena estado persistente de forma aislada para cada proyecto.</p>
              </div>
              <div className="resource-cards-grid">
              {/* PostgreSQL */}
              <label
                className={`resource-card${recursos.includes('postgresql') ? ' selected' : ''}`}
                htmlFor="res-postgresql"
              >
                <div className="resource-card-header">
                  <img className="resource-card-logo" src="/logos/postgresql.svg" alt="" data-testid="resource-logo-postgresql" />
                  <input
                    id="res-postgresql"
                    data-testid="res-postgresql"
                    type="checkbox"
                    checked={recursos.includes('postgresql')}
                    onChange={() => alternarRecurso('postgresql')}
                  />
                </div>
                <div className="resource-card-title">PostgreSQL 16.4</div>
                <div className="resource-card-desc">
                  Base de datos relacional aislada con protocolo de reset TRUNCATE + Seed.
                </div>
                <div style={{ marginTop: '0.75rem' }}>
                  <Badge tono="ok">Disponible</Badge>
                </div>
              </label>

              {/* MySQL (Disabled) */}
              <div className="resource-card disabled" aria-disabled="true">
                <div className="resource-card-header">
                  <img className="resource-card-logo" src="/logos/mysql.svg" alt="" data-testid="resource-logo-mysql" />
                  <input type="checkbox" disabled checked={false} aria-label="MySQL (deshabilitado)" />
                </div>
                <div className="resource-card-title">MySQL 8.4</div>
                <div className="resource-card-desc">Base de datos relacional auxiliar.</div>
                <div style={{ marginTop: '0.75rem' }}>
                  <Badge tono="neutral">Próximamente</Badge>
                </div>
              </div>

              {/* MongoDB (Disabled) */}
              <div className="resource-card disabled" aria-disabled="true">
                <div className="resource-card-header">
                  <img className="resource-card-logo" src="/logos/mongodb.svg" alt="" data-testid="resource-logo-mongodb" />
                  <input type="checkbox" disabled checked={false} aria-label="MongoDB (deshabilitado)" />
                </div>
                <div className="resource-card-title">MongoDB 7.0</div>
                <div className="resource-card-desc">Base de datos documental auxiliar.</div>
                <div style={{ marginTop: '0.75rem' }}>
                  <Badge tono="neutral">Próximamente</Badge>
                </div>
              </div>
              </div>
            </section>

            <section className="resource-category" aria-labelledby="resource-cache-title">
              <div className="resource-category-heading">
                <h3 id="resource-cache-title">Caché y mensajería</h3>
                <p>Reduce latencia y mantiene datos efímeros dentro del entorno warm.</p>
              </div>
              <div className="resource-cards-grid resource-cards-grid-compact">
                <label
                  className={`resource-card${recursos.includes('redis') ? ' selected' : ''}`}
                  htmlFor="res-redis"
                >
                  <div className="resource-card-header">
                    <img className="resource-card-logo" src="/logos/redis.svg" alt="" data-testid="resource-logo-redis" />
                    <input
                      id="res-redis"
                      data-testid="res-redis"
                      type="checkbox"
                      checked={recursos.includes('redis')}
                      onChange={() => alternarRecurso('redis')}
                    />
                  </div>
                  <div className="resource-card-title">Redis 7.4.1</div>
                  <div className="resource-card-desc">
                    Servidor de caché y broker en memoria con protocolo de reset FLUSHDB.
                  </div>
                  <div style={{ marginTop: '0.75rem' }}>
                    <Badge tono="ok">Disponible</Badge>
                  </div>
                </label>
                <div className="resource-card disabled" aria-disabled="true">
                  <div className="resource-card-header">
                    <img className="resource-card-logo" src="/logos/memcached.svg" alt="" data-testid="resource-logo-memcached" />
                    <input type="checkbox" disabled checked={false} aria-label="Memcached (deshabilitado)" />
                  </div>
                  <div className="resource-card-title">Memcached</div>
                  <div className="resource-card-desc">Caché distribuida en memoria para lecturas de baja latencia.</div>
                  <div style={{ marginTop: '0.75rem' }}><Badge tono="neutral">Próximamente</Badge></div>
                </div>
                <div className="resource-card disabled" aria-disabled="true">
                  <div className="resource-card-header">
                    <img className="resource-card-logo" src="/logos/valkey.svg" alt="" data-testid="resource-logo-valkey" />
                    <input type="checkbox" disabled checked={false} aria-label="Valkey (deshabilitado)" />
                  </div>
                  <div className="resource-card-title">Valkey</div>
                  <div className="resource-card-desc">Caché compatible en memoria para cargas de trabajo futuras.</div>
                  <div style={{ marginTop: '0.75rem' }}><Badge tono="neutral">Próximamente</Badge></div>
                </div>
              </div>
            </section>

            <div className="wizard-buttons-row">
              <button type="button" className="btn btn-secundario" onClick={() => setPaso(1)}>
                ← Anterior
              </button>
              <button
                type="button"
                className="btn btn-primary"
                data-testid="wizard-btn-next-step2"
                onClick={() => setPaso(3)}
              >
                Siguiente: Repositorio GitHub →
              </button>
            </div>
          </section>
        )}

        {/* PASO 3: REPOSITORIO GITHUB Y ENVÍO */}
        {paso === 3 && (
          <form onSubmit={handleCrearProyecto} aria-labelledby="step-3-title">
            <h2 id="step-3-title" ref={encabezadoRef} tabIndex={-1} className="card-title">
              3. Repositorio GitHub y contenedor
            </h2>
            <p className="card-sub">
              Indica las coordenadas del repositorio de GitHub y el prefijo de la imagen en GHCR.
            </p>

            {erroresPaso3.length > 0 && (
              <div role="alert" className="alert-box alert-danger" style={{ marginBottom: '1rem' }}>
                <ul style={{ margin: 0, paddingLeft: '1.25rem' }}>
                  {erroresPaso3.map((err, i) => (
                    <li key={i}>{err}</li>
                  ))}
                </ul>
              </div>
            )}

            {errorEnvio !== null && (
              <div className="alert-box alert-danger project-creation-error">
                <ApiErrorNotice error={errorEnvio} />
                {errorEnvio instanceof ApiError && errorEnvio.code === 'project_credentials_unavailable' && (
                  <p>El proyecto no se puede completar sin sus credenciales. Actualiza el control plane y vuelve a intentarlo.</p>
                )}
                {isProjectContractMismatch(errorEnvio) && (
                  <p>
                    El control plane activo no reconoce los campos de Proyecto (nombre, descripción o recursos). Actualiza
                    go-governance y ui-api al mismo release y vuelve a intentarlo.
                  </p>
                )}
              </div>
            )}

            <div className="form-field">
              <label htmlFor="wizard-repo-name">
                Repositorio en GitHub (owner/repo) <span aria-hidden="true">*</span>
              </label>
              <input
                id="wizard-repo-name"
                data-testid="wizard-repo-name"
                type="text"
                required
                value={repo}
                onChange={(e) => handleRepoChange(e.target.value)}
                onBlur={validarPaso3}
                placeholder="acme/payments-api"
              />
              <span style={{ fontSize: '12px', color: 'var(--text-muted)', marginTop: '4px', display: 'block' }}>
                Formato en minúsculas. Ejemplo: <code>organizacion/servicio</code>
              </span>
            </div>

            <div className="form-field">
              <label htmlFor="wizard-image-repo">Prefijo de imagen en GHCR</label>
              <input
                id="wizard-image-repo"
                data-testid="wizard-image-repo"
                type="text"
                value={imageRepo}
                onChange={(e) => {
                  setImageTouched(true);
                  setImageRepo(e.target.value);
                }}
                placeholder="ghcr.io/acme/payments-api"
              />
              <span style={{ fontSize: '12px', color: 'var(--text-muted)', marginTop: '4px', display: 'block' }}>
                Predeterminado: <code>ghcr.io/{repo || 'owner/repo'}</code>
              </span>
            </div>

            <div className="wizard-buttons-row">
              <button
                type="button"
                className="btn btn-secundario"
                disabled={enVuelo}
                onClick={() => setPaso(2)}
              >
                ← Anterior
              </button>
              <button
                type="submit"
                className="btn btn-primary"
                data-testid="wizard-btn-submit"
                disabled={enVuelo || !REPO_REGEX.test(repo)}
              >
                {enVuelo ? 'Creando proyecto…' : 'Crear proyecto y generar credenciales'}
              </button>
            </div>
          </form>
        )}

        {/* PASO 4: CREDENCIALES & VERIFICACIÓN EN VIVO */}
        {paso === 4 && credenciales && (
          <section aria-labelledby="step-4-title" className="waiting-box">
            <h2 id="step-4-title" ref={encabezadoRef} tabIndex={-1} className="card-title">
              4. Configuración del Webhook & Verificación en vivo
            </h2>
            <p className="card-sub">
              Configura el webhook en GitHub Settings con las credenciales aisladas generadas para este proyecto. La
              plataforma verificará automáticamente la recepción del evento <code>ping</code> y la preparación del
              entorno.
            </p>

            {/* Región accesible en vivo */}
            <div aria-live="polite" className="sr-only">
              {completado
                ? 'El proyecto está completamente verificado e infra listo.'
                : `Esperando verificación. Infraestructura: ${infraestructuraLista ? 'Lista' : 'Aprovisionando'}. Webhook: ${webhookVerificado ? 'Verificado' : 'Esperando'}.`}
            </div>

            {/* Cuadro de Instrucciones GitHub */}
            <div className="instruction-step">
              <h3 style={{ fontSize: '14px', fontWeight: 700, marginBottom: '0.75rem' }}>
                Instrucciones para GitHub Settings → Webhooks → Add webhook
              </h3>

              <div style={{ marginBottom: '1rem' }}>
                <label style={{ fontSize: '12px', fontWeight: 600 }}>Payload URL</label>
                <div className="copy-input-row">
                  <input readOnly value={webhookPublicUrl} data-testid="wizard-webhook-url" />
                  <button
                    type="button"
                    className="btn btn-sm btn-secundario"
                    data-testid="wizard-btn-copy-url"
                    onClick={() => copiarTexto(webhookPublicUrl, setCopiadoUrl)}
                  >
                    {copiadoUrl ? 'Copiado' : 'Copiar URL'}
                  </button>
                </div>
              </div>

              <div style={{ marginBottom: '1rem' }}>
                <label style={{ fontSize: '12px', fontWeight: 600 }}>
                  Secret (HMAC SHA-256 exclusivo de este proyecto)
                </label>
                <div className="copy-input-row">
                  <input
                    readOnly
                    type="text"
                    value={credenciales.webhook_secret}
                    data-testid="wizard-webhook-secret"
                  />
                  <button
                    type="button"
                    className="btn btn-sm btn-secundario"
                    data-testid="wizard-btn-copy-secret"
                    onClick={() => copiarTexto(credenciales.webhook_secret, setCopiadoSecret)}
                  >
                    {copiadoSecret ? 'Copiado' : 'Copiar Secreto'}
                  </button>
                  <button
                    type="button"
                    className="btn btn-sm"
                    data-testid="wizard-btn-rotate-secret"
                    disabled={rotando}
                    onClick={rotarSecreto}
                    title="Incrementa la versión del secreto si necesitas regenerarlo"
                  >
                    {rotando ? 'Rotando…' : 'Rotar secreto'}
                  </button>
                </div>
                {mensajeRotacion && (
                  <p style={{ fontSize: '12px', color: 'var(--accent)', marginTop: '4px' }}>
                    {mensajeRotacion}
                  </p>
                )}
                <span style={{ fontSize: '11.5px', color: 'var(--text-muted)', display: 'block', marginTop: '4px' }}>
                  Nota de seguridad: Este secreto solo se muestra en esta pantalla. No se almacena en el navegador ni se
                  retornará en consultas futuras.
                </span>
              </div>

              <div style={{ fontSize: '12.5px', color: 'var(--text)', lineHeight: 1.5 }}>
                <div>
                  <strong>Content type:</strong> <code>application/json</code>
                </div>
                <div>
                  <strong>Which events would you like to trigger this webhook?</strong>
                </div>
                <div style={{ paddingLeft: '1rem', marginTop: '4px' }}>
                  Selecciona <em>«Let me select individual events»</em> y marca:
                  <ul style={{ margin: '4px 0', paddingLeft: '1.25rem' }}>
                    <li>
                      <strong>Pushes</strong> (para cambios en ramas)
                    </li>
                    <li>
                      <strong>Pull requests</strong> (para verificación de PRs)
                    </li>
                    <li>
                      <strong>Releases</strong> (para eventos de publicación / tags)
                    </li>
                  </ul>
                </div>
              </div>
            </div>

            {/* Franja de Estado de Verificación */}
            <div className="waiting-status-strip" data-testid="wizard-status-strip">
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <strong style={{ fontSize: '14px' }}>Estado de la verificación</strong>
                <span style={{ fontSize: '12px', color: 'var(--text-muted)' }}>
                  Tiempo transcurrido: {tiempoEspera} s
                </span>
              </div>

              <div className="waiting-status-row">
                <span style={{ fontSize: '13px' }}>Infraestructura warm aislada (K8s &amp; Recursos):</span>
                {infraestructuraLista ? (
                  <Badge tono="ok" data-testid="badge-infra-ready">
                    Listo (Infraestructura lista)
                  </Badge>
                ) : (
                  <Badge tono="warn" data-testid="badge-infra-waiting">
                    Aprovisionando recursos…
                  </Badge>
                )}
              </div>

              <div className="waiting-status-row">
                <span style={{ fontSize: '13px' }}>Webhook de GitHub (Firma HMAC verificada):</span>
                {webhookVerificado ? (
                  <Badge tono="ok" data-testid="badge-webhook-ready">
                    Verificado (Evento recibido)
                  </Badge>
                ) : (
                  <Badge tono="warn" data-testid="badge-webhook-waiting">
                    Esperando evento ping / push / pr…
                  </Badge>
                )}
              </div>

              {polling.agotado && (
                <div role="alert" className="alert-box alert-warn" style={{ marginTop: '0.5rem' }}>
                  El sondeo ha alcanzado el límite de intentos. Puedes reintentar la comprobación manualmente cuando
                  hayas configurado el webhook en GitHub.
                </div>
              )}

              <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: '0.5rem' }}>
                <button
                  type="button"
                  className="btn btn-sm btn-secundario"
                  data-testid="wizard-btn-refresh-poll"
                  onClick={() => polling.refrescar()}
                >
                  Comprobar ahora
                </button>
              </div>
            </div>

            {/* Estado Final Exitoso */}
            {completado && (
              <div
                role="status"
                className="alert-box alert-ok"
                data-testid="wizard-success-banner"
                style={{ padding: '1.25rem' }}
              >
                <h3 style={{ margin: '0 0 0.5rem 0', fontSize: '15px' }}>
                  ✓ ¡Proyecto conectado y verificado exitosamente!
                </h3>
                <p style={{ margin: '0 0 1rem 0', fontSize: '13px' }}>
                  El entorno warm aislado para <code>{credenciales.repository.repo}</code> está aprovisionado y el
                  webhook de GitHub ha sido validado mediante firma criptográfica HMAC SHA-256.
                </p>
                <div style={{ display: 'flex', gap: '0.75rem', flexWrap: 'wrap' }}>
                  <Link
                    to={`/inbox?project=${encodeURIComponent(credenciales.repository.repo)}`}
                    className="btn btn-primary"
                    data-testid="wizard-link-inbox"
                  >
                    Ir al Inbox del proyecto →
                  </Link>
                  <Link to="/inbox" className="btn btn-secundario" data-testid="wizard-link-repos">
                    Volver al Inbox
                  </Link>
                </div>
              </div>
            )}
          </section>
        )}
      </div>
    </main>
  );
}
