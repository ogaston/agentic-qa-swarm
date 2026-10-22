import { Link, Navigate, Outlet, useLocation } from 'react-router-dom';
import { loginPath } from './next';
import { useSession } from './SessionContext';

/** Guarda de rutas: sin sesión redirige a /login?next=…; con sesión muestra la cabecera. */
export function ProtectedLayout() {
  const { session, logout } = useSession();
  const location = useLocation();

  if (session === null) {
    return <Navigate to={loginPath(location.pathname + location.search)} replace />;
  }

  return (
    <>
      <header className="app-header">
        <div className="app-header-inner">
          <div className="app-brand">
            <span className="app-wordmark">aqs</span>
            <span className="app-brand-tag">Agentic QA Swarm</span>
          </div>
          <nav aria-label="Principal" className="app-nav">
            <Link to="/inbox">Inbox</Link>
            <Link to="/runs">Mis corridas</Link>
            <Link to="/warm">Warm</Link>
          </nav>
          <div className="app-session">
            <span className="app-principal">{session.principal.principal_id}</span>
            <span className="app-role">{session.principal.role}</span>
            <button type="button" className="btn btn-secondary" onClick={() => void logout()}>
              Salir
            </button>
          </div>
        </div>
      </header>
      <Outlet />
    </>
  );
}
