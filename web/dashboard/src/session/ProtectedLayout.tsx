import { Navigate, NavLink, Outlet, useLocation } from 'react-router-dom';
import { SelectorTema } from '../theme/SelectorTema';
import { loginPath } from './next';
import { useSession } from './SessionContext';

/** Iniciales del avatar: primera letra de las dos primeras palabras del principal, o sus dos primeros caracteres. */
function iniciales(principalId: string): string {
  const partes = principalId.split(/[^A-Za-z0-9]+/).filter((p) => p !== '');
  const dos = partes.length >= 2 ? `${partes[0]?.charAt(0) ?? ''}${partes[1]?.charAt(0) ?? ''}` : principalId.slice(0, 2);
  return dos.toUpperCase();
}

/** Guarda de rutas: sin sesión redirige a /login?next=…; con sesión muestra la cabecera del mockup. */
export function ProtectedLayout() {
  const { session, logout } = useSession();
  const location = useLocation();

  if (session === null) {
    return <Navigate to={loginPath(location.pathname + location.search)} replace />;
  }

  const { principal_id: principalId, role } = session.principal;

  return (
    <>
      <header className="az-header">
        <div className="az-header-inner">
          <div className="az-brand">
            <span className="az-logo">aqs</span>
            <span className="az-brand-sub">Agentic QA Swarm</span>
          </div>
          <nav aria-label="Principal" className="az-nav">
            <NavLink to="/inbox" className="az-nav-link">
              <svg viewBox="0 0 24 24" fill="none" aria-hidden="true" focusable="false">
                <path d="M4 6h16v12H4z" stroke="currentColor" strokeWidth="1.8" />
                <path d="M4 13h5l2 3h2l2-3h5" stroke="currentColor" strokeWidth="1.8" />
              </svg>
              Inbox
            </NavLink>
            <NavLink to="/runs" className="az-nav-link">
              <svg viewBox="0 0 24 24" fill="none" aria-hidden="true" focusable="false">
                <path
                  d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01"
                  stroke="currentColor"
                  strokeWidth="1.8"
                  strokeLinecap="round"
                />
              </svg>
              Mis corridas
            </NavLink>
            <NavLink to="/warm" className="az-nav-link">
              <svg viewBox="0 0 24 24" fill="none" aria-hidden="true" focusable="false">
                <path d="M12 3v18M5 8l14 8M19 8L5 16" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
              </svg>
              Entorno
            </NavLink>
          </nav>
          <div className="az-header-right">
            <SelectorTema />
            <div className="az-session-chip">
              <span className="az-avatar" aria-hidden="true">
                {iniciales(principalId)}
              </span>
              <div className="az-principal">
                <strong>{principalId}</strong>
                <span>
                  Rol: <span>{role}</span>
                </span>
              </div>
            </div>
            <button type="button" className="btn btn-sm" onClick={() => void logout()}>
              Salir
            </button>
          </div>
        </div>
      </header>
      <Outlet />
    </>
  );
}
