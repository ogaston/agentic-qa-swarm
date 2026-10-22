import { useState } from 'react';
import { aplicarTema, guardarTema, temaActual, type Tema } from './tema';

/** Selector de tema del mockup (btn-icon en la cabecera): alterna claro/oscuro y lo persiste. */
export function SelectorTema() {
  const [tema, setTema] = useState<Tema>(() => temaActual());

  function alternar() {
    const siguiente: Tema = tema === 'dark' ? 'light' : 'dark';
    aplicarTema(siguiente);
    guardarTema(siguiente);
    setTema(siguiente);
  }

  // Etiqueta dinámica, como el mockup: describe la acción que hará el próximo clic.
  const etiqueta = tema === 'dark' ? 'Cambiar a tema claro' : 'Cambiar a tema oscuro';

  return (
    <button type="button" className="btn btn-icon" onClick={alternar} aria-label={etiqueta} title={etiqueta}>
      <svg className="theme-icon-sun" viewBox="0 0 24 24" fill="none" aria-hidden="true" focusable="false">
        <circle cx="12" cy="12" r="4" stroke="currentColor" strokeWidth="1.8" />
        <path
          d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41"
          stroke="currentColor"
          strokeWidth="1.8"
          strokeLinecap="round"
        />
      </svg>
      <svg className="theme-icon-moon" viewBox="0 0 24 24" fill="none" aria-hidden="true" focusable="false">
        <path
          d="M21 12.79A9 9 0 1111.21 3 7 7 0 0021 12.79z"
          stroke="currentColor"
          strokeWidth="1.8"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>
    </button>
  );
}
