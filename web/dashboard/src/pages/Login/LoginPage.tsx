import { useRef, useState, type FormEvent } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { ApiError } from '../../api/client';
import { ApiErrorNotice, useCountdownUntil } from '../../api/errors';
import { safeNext } from '../../session/next';
import { useSession } from '../../session/SessionContext';

const MSG_CREDENCIALES = 'Usuario, contraseña o código incorrectos';
const MSG_IDENTIDAD = 'Servicio de identidad no disponible';

/** «Iniciar sesión» (`/login`): tarjeta centrada con marca, sin menú de sesión ni selector de tema. */
export function LoginPage() {
  const { login } = useSession();
  const navigate = useNavigate();
  const [params] = useSearchParams();

  const [usuario, setUsuario] = useState('');
  const [clave, setClave] = useState('');
  const [codigo, setCodigo] = useState('');
  const [mostrarCodigo, setMostrarCodigo] = useState(false);
  const [pendiente, setPendiente] = useState(false);
  const [mensaje, setMensaje] = useState<string | null>(null);
  const [errorApi, setErrorApi] = useState<unknown>(null);
  const [endAt, setEndAt] = useState<number | null>(null);
  const enCurso = useRef(false);

  const restante = useCountdownUntil(endAt);
  const bloqueado = restante > 0;

  async function enviar(evento: FormEvent<HTMLFormElement>) {
    evento.preventDefault();
    if (enCurso.current || bloqueado) return;
    enCurso.current = true;
    setPendiente(true);
    setMensaje(null);
    setErrorApi(null);
    try {
      const otp = codigo.trim();
      await login({
        username: usuario.trim(),
        password: clave,
        ...(otp !== '' ? { otp } : {}),
      });
      navigate(safeNext(params.get('next')), { replace: true });
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        setMensaje(MSG_CREDENCIALES);
        if (usuario.trim().toLowerCase() === 'admin') setMostrarCodigo(true);
      } else if (error instanceof ApiError && error.status === 429) {
        const espera = error.retryAfter ?? 0;
        setEndAt(espera > 0 ? Date.now() + espera * 1000 : null);
        if (espera <= 0) setMensaje('Demasiados intentos. Vuelve a intentarlo en unos instantes.');
      } else if (error instanceof ApiError && (error.status === 503 || error.status === 0)) {
        setMensaje(MSG_IDENTIDAD);
      } else {
        setErrorApi(error);
      }
    } finally {
      enCurso.current = false;
      setPendiente(false);
    }
  }

  return (
    <div className="ed-root ed-login-root">
      <header className="az-header">
        <div className="az-header-inner">
          <Link to="/" className="az-brand" title="Volver al inicio">
            <span className="az-logo">aqs</span>
            <span className="az-brand-sub">Agentic QA Swarm</span>
          </Link>
        </div>
      </header>
      <main className="az-container">
        <div className="login-wrap">
          <div className="card card-pad">
            <div className="login-marca">
              <span className="az-logo">aqs</span>
              <span className="login-marca-sub">Agentic QA Swarm</span>
            </div>
            <h1 className="login-titulo">Iniciar sesión</h1>
            <p className="login-texto">
              Introduce tus credenciales para acceder a la plataforma.
            </p>

            {bloqueado ? (
              <div role="alert" className="alert-box alert-warn">{`Demasiados intentos, espera ${restante} s`}</div>
            ) : null}
            {mensaje !== null && !bloqueado ? (
              <div role="alert" className="alert-box alert-danger">
                {mensaje}
              </div>
            ) : null}

            <form onSubmit={(evento) => void enviar(evento)}>
              <div className="form-field">
                <label htmlFor="login-usuario">Usuario</label>
                <input
                  id="login-usuario"
                  name="usuario"
                  autoComplete="username"
                  value={usuario}
                  onChange={(e) => setUsuario(e.target.value)}
                />
              </div>
              <div className="form-field">
                <label htmlFor="login-clave">Contraseña</label>
                <input
                  id="login-clave"
                  name="clave"
                  type="password"
                  autoComplete="current-password"
                  value={clave}
                  onChange={(e) => setClave(e.target.value)}
                />
              </div>
              {mostrarCodigo ? (
                <div className="form-field">
                  <label htmlFor="login-codigo">Código de verificación</label>
                  <input
                    id="login-codigo"
                    name="codigo"
                    inputMode="numeric"
                    autoComplete="one-time-code"
                    maxLength={6}
                    pattern="[0-9]{6}"
                    placeholder="123456"
                    value={codigo}
                    onChange={(e) => setCodigo(e.target.value)}
                  />
                </div>
              ) : (
                <div className="login-otp">
                  <button type="button" className="btn btn-sm btn-bloque" onClick={() => setMostrarCodigo(true)}>
                    Tengo un código de verificación
                  </button>
                </div>
              )}
              <button type="submit" className="btn btn-primary btn-bloque" disabled={pendiente || bloqueado}>
                Entrar
              </button>
            </form>

            {errorApi !== null ? <ApiErrorNotice error={errorApi} /> : null}
          </div>
        </div>
      </main>
    </div>
  );
}
