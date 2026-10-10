import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import type { components } from '../api/schema.gen';
import { apiFetch, setAuthToken, setUnauthorizedHandler } from '../api/client';
import { loginPath } from './next';

type SessionBody = components['schemas']['Session'];

type LoginBody = {
  token: string;
  expires_at: string;
  session_id?: string;
};

export type Principal = {
  principal_id: string;
  role: SessionBody['role'];
};

/** Sesión solo en memoria: nunca se escribe en almacenamiento ni en cookies. */
export type Session = {
  token: string;
  expiresAt: string;
  principal: Principal;
};

export type LoginInput = {
  username: string;
  password: string;
  otp?: string;
};

type SessionContextValue = {
  session: Session | null;
  login: (input: LoginInput) => Promise<void>;
  logout: () => Promise<void>;
};

const SessionContext = createContext<SessionContextValue | null>(null);

// setTimeout no admite más de ~24.8 días; se re-arma si hace falta.
const MAX_TIMER_MS = 2_147_483_647;

export function SessionProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(null);
  const navigate = useNavigate();
  const location = useLocation();
  const rutaActual = location.pathname + location.search;
  const rutaRef = useRef(rutaActual);
  const cerrando = useRef(false);

  useEffect(() => {
    rutaRef.current = rutaActual;
  });

  const borrarLocal = useCallback(() => {
    setAuthToken(null);
    setSession(null);
  }, []);

  // Expulsión por sesión caducada o 401: borra el estado y lleva a login con next.
  const expulsar = useCallback(() => {
    borrarLocal();
    const ruta = rutaRef.current;
    navigate(ruta.startsWith('/login') ? '/login' : loginPath(ruta));
  }, [borrarLocal, navigate]);

  useEffect(() => {
    setUnauthorizedHandler(() => {
      if (!cerrando.current) expulsar();
    });
    return () => setUnauthorizedHandler(null);
  }, [expulsar]);

  // Temporizador: al llegar a expires_at se cierra la sesión local.
  useEffect(() => {
    if (session === null) return;
    let id: ReturnType<typeof setTimeout> | undefined;
    const armar = () => {
      const restante = Date.parse(session.expiresAt) - Date.now();
      if (restante <= 0) {
        expulsar();
        return;
      }
      id = setTimeout(armar, Math.min(restante, MAX_TIMER_MS));
    };
    armar();
    return () => {
      if (id !== undefined) clearTimeout(id);
    };
  }, [session, expulsar]);

  const login = useCallback(
    async (input: LoginInput) => {
      const cuerpo: Record<string, string> = {
        username: input.username,
        password: input.password,
      };
      if (input.otp !== undefined && input.otp !== '') cuerpo['otp'] = input.otp;

      const respuesta = await apiFetch<LoginBody>('/api/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(cuerpo),
      });
      setAuthToken(respuesta.token);
      try {
        const sesion = await apiFetch<SessionBody>('/api/auth/session');
        setSession({
          token: respuesta.token,
          expiresAt: respuesta.expires_at,
          principal: { principal_id: sesion.principal_id, role: sesion.role },
        });
      } catch (error) {
        borrarLocal();
        throw error;
      }
    },
    [borrarLocal],
  );

  const logout = useCallback(async () => {
    cerrando.current = true;
    try {
      await apiFetch<unknown>('/api/auth/logout', { method: 'POST' });
    } catch {
      // El borrado local ocurre igual aunque el servidor falle.
    } finally {
      cerrando.current = false;
      borrarLocal();
      navigate('/login');
    }
  }, [borrarLocal, navigate]);

  const valor = useMemo(() => ({ session, login, logout }), [session, login, logout]);

  return <SessionContext.Provider value={valor}>{children}</SessionContext.Provider>;
}

export function useSession(): SessionContextValue {
  const valor = useContext(SessionContext);
  if (valor === null) throw new Error('useSession fuera de SessionProvider');
  return valor;
}
