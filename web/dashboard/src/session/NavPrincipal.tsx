import { NavLink } from 'react-router-dom';

/** Navegación «Principal» del mockup (cabecera de sesión y cabecera pública de /login). */
export function NavPrincipal() {
  return (
    <nav aria-label="Principal" className="az-nav">
      <NavLink to="/inbox" className="az-nav-link" data-testid="nav-inbox-link">
        <svg viewBox="0 0 24 24" fill="none" aria-hidden="true" focusable="false">
          <path d="M4 6h16v12H4z" stroke="currentColor" strokeWidth="1.8" />
          <path d="M4 13h5l2 3h2l2-3h5" stroke="currentColor" strokeWidth="1.8" />
        </svg>
        Inbox
      </NavLink>
      <NavLink to="/runs" className="az-nav-link" data-testid="nav-runs-link">
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
      <NavLink to="/warm" className="az-nav-link" data-testid="nav-warm-link">
        <svg viewBox="0 0 24 24" fill="none" aria-hidden="true" focusable="false">
          <path d="M4 4h16v12H4zM8 20h8M12 16v4M7 10h3l2-3 2 6 1-3h2" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
        Entorno
      </NavLink>
    </nav>
  );
}
