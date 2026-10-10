import { useRef, useState, type FormEvent } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { ApiError } from '../../api/client';
import { ApiErrorNotice, useCountdownUntil } from '../../api/errors';
import { safeNext } from '../../session/next';
import { useSession } from '../../session/SessionContext';

const MSG_CREDENCIALES = 'Usuario, contraseña o código incorrectos';
const MSG_IDENTIDAD = 'Servicio de identidad no disponible';

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
    <main>
      <h1>Iniciar sesión</h1>
      <form onSubmit={(evento) => void enviar(evento)}>
        <div>
          <label htmlFor="login-usuario">Usuario</label>
          <input
            id="login-usuario"
            name="usuario"
            autoComplete="username"
            value={usuario}
            onChange={(e) => setUsuario(e.target.value)}
          />
        </div>
        <div>
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
          <div>
            <label htmlFor="login-codigo">Código de verificación</label>
            <input
              id="login-codigo"
              name="codigo"
              inputMode="numeric"
              autoComplete="one-time-code"
              maxLength={6}
              pattern="[0-9]{6}"
              value={codigo}
              onChange={(e) => setCodigo(e.target.value)}
            />
          </div>
        ) : (
          <button type="button" onClick={() => setMostrarCodigo(true)}>
            Tengo un código de verificación
          </button>
        )}
        <button type="submit" disabled={pendiente || bloqueado}>
          Entrar
        </button>
      </form>

      {bloqueado ? (
        <p role="alert">{`Demasiados intentos, espera ${restante} s`}</p>
      ) : null}
      {mensaje !== null && !bloqueado ? <p role="alert">{mensaje}</p> : null}
      {errorApi !== null ? <ApiErrorNotice error={errorApi} /> : null}
    </main>
  );
}
