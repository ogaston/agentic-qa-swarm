import { Navigate, Outlet, useLocation } from 'react-router-dom';
import { loginPath } from './next';
import { NavPrincipal } from './NavPrincipal';
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
    <div className="ed-root ed-app-layout">
      <header className="az-header">
        <div className="az-header-inner">
          <div className="az-brand">
            <span className="az-logo">aqs</span>
            <span className="az-brand-sub">Agentic QA Swarm</span>
          </div>
          <NavPrincipal />
          <div className="az-header-right">
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
            <button type="button" className="btn btn-sm dashboard-logout" data-testid="session-logout-button" onClick={() => void logout()}>
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
                <path d="M9 4H4v16h5M14 8l4 4-4 4M8 12h10" />
              </svg>
              Salir
            </button>
          </div>
        </div>
      </header>
      <Outlet />
    </div>
  );
}
