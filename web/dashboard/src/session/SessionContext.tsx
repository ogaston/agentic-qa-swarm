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
import { ApiError, apiFetch, setAuthToken, setUnauthorizedHandler } from '../api/client';
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

/** Sesión validada por el servidor; solo token y caducidad se persisten. */
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
  restoring: boolean;
  login: (input: LoginInput) => Promise<void>;
  logout: () => Promise<void>;
};

const SessionContext = createContext<SessionContextValue | null>(null);

// setTimeout no admite más de ~24.8 días; se re-arma si hace falta.
const MAX_TIMER_MS = 2_147_483_647;

export const SESSION_STORAGE_KEY = 'aqs.session.v1';

function persistSession(value: { token: string; expiresAt: string } | null) {
  try {
    if (value === null) localStorage.removeItem(SESSION_STORAGE_KEY);
    else localStorage.setItem(SESSION_STORAGE_KEY, JSON.stringify({ version: 1, ...value }));
  } catch {
    // El navegador puede bloquear storage: la sesión sigue disponible en memoria.
  }
}

function readSession(): { token: string; expiresAt: string } | null {
  try {
    const raw = localStorage.getItem(SESSION_STORAGE_KEY);
    if (raw === null) return null;
    const value: unknown = JSON.parse(raw);
    if (typeof value === 'object' && value !== null) {
      const v = value as Record<string, unknown>;
      if (v.version === 1 && typeof v.token === 'string' && v.token.trim() !== '' &&
          typeof v.expiresAt === 'string' && Date.parse(v.expiresAt) > Date.now()) {
        return { token: v.token, expiresAt: v.expiresAt };
      }
    }
  } catch {
    // Un registro ilegible o no válido nunca concede acceso.
  }
  persistSession(null);
  return null;
}

function validPrincipal(value: SessionBody): boolean {
  return typeof value?.principal_id === 'string' && value.principal_id !== '' &&
    (value.role === 'user' || value.role === 'admin') && Date.parse(value.expires_at) > Date.now();
}

export function SessionProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(null);
  const [restoring, setRestoring] = useState(true);
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
    persistSession(null);
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

  useEffect(() => {
    let active = true;
    const stored = readSession();
    setAuthToken(stored?.token ?? null);
    if (stored === null) {
      setRestoring(false);
      return;
    }
    void (async () => {
      try {
        const body = await apiFetch<SessionBody>('/api/auth/session');
        if (!active) return;
        if (!validPrincipal(body)) throw new Error('Sesión no válida');
        const expiresAt = new Date(Math.min(Date.parse(stored.expiresAt), Date.parse(body.expires_at))).toISOString();
        if (Date.parse(expiresAt) <= Date.now()) throw new Error('Sesión caducada');
        persistSession({ token: stored.token, expiresAt });
        setSession({ token: stored.token, expiresAt, principal: { principal_id: body.principal_id, role: body.role } });
      } catch {
        if (active) borrarLocal();
      } finally {
        if (active) setRestoring(false);
      }
    })();
    return () => { active = false; };
  }, [borrarLocal]);

  // Temporizador: al llegar a expires_at se cierra la sesión local.
  useEffect(() => {
    if (session === null) return;
    let id: ReturnType<typeof setTimeout> | undefined;
    const armar = () => {
      const restante = Date.parse(session.expiresAt) - Date.now();
      // NaN (expires_at no parseable) no es sesión válida: nunca se re-arma.
      if (!Number.isFinite(restante) || restante <= 0) {
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
      // Un login nuevo no lleva el Bearer de una sesión previa: se descarta antes de la petición.
      borrarLocal();
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
      // F-02: sin fecha de expiración válida no hay sesión, y el token no llega a fijarse.
      if (typeof respuesta.token !== 'string' || respuesta.token.trim() === '' || !(Date.parse(respuesta.expires_at) > Date.now())) {
        throw new ApiError({
          status: 200,
          code: 'unexpected_response',
          message: 'expires_at no es una fecha válida',
          requestId: 'sin-id',
        });
      }
      setAuthToken(respuesta.token);
      try {
        const sesion = await apiFetch<SessionBody>('/api/auth/session');
        if (!validPrincipal(sesion)) throw new Error('Sesión no válida');
        const expiresAt = new Date(Math.min(Date.parse(respuesta.expires_at), Date.parse(sesion.expires_at))).toISOString();
        persistSession({ token: respuesta.token, expiresAt });
        setSession({
          token: respuesta.token,
          expiresAt,
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

  const valor = useMemo(() => ({ session, restoring, login, logout }), [session, restoring, login, logout]);

  return <SessionContext.Provider value={valor}>{restoring ? <p role="status">Restaurando sesión…</p> : children}</SessionContext.Provider>;
}

export function useSession(): SessionContextValue {
  const valor = useContext(SessionContext);
  if (valor === null) throw new Error('useSession fuera de SessionProvider');
  return valor;
}
