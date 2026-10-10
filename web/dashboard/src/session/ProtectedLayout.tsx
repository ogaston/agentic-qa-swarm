import { Navigate, Outlet, useLocation } from 'react-router-dom';
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
      <header>
        <span>Agentic QA Swarm</span>{' '}
        <span>{session.principal.principal_id}</span>{' '}
        <span>{session.principal.role}</span>{' '}
        <button type="button" onClick={() => void logout()}>
          Salir
        </button>
      </header>
      <Outlet />
    </>
  );
}
