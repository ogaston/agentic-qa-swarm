/** Tema visual: claro u oscuro. Se refleja en `data-theme` de <html> (tokens.css). */
export type Tema = 'light' | 'dark';

/** Clave de localStorage. Sin acceso a almacenamiento (privado o denegado) el tema vale solo esta carga. */
export const CLAVE_TEMA = 'aqs-tema';

/** Tema que indica el sistema (prefers-color-scheme). Sin matchMedia, claro. */
export function temaSistema(): Tema {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return 'light';
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

/** Valor guardado por el usuario, o null si no hay (o no se puede leer). */
export function temaGuardado(): Tema | null {
  try {
    const valor = window.localStorage.getItem(CLAVE_TEMA);
    return valor === 'light' || valor === 'dark' ? valor : null;
  } catch {
    return null;
  }
}

/** Tema de arranque: lo guardado; si no hay, el del sistema. */
export function temaInicial(): Tema {
  return temaGuardado() ?? temaSistema();
}

/** Escribe `data-theme` en <html>. */
export function aplicarTema(tema: Tema): void {
  document.documentElement.dataset.theme = tema;
}

/** Persiste la elección. Si el almacenamiento falla, no lanza: el tema sigue aplicado en la página. */
export function guardarTema(tema: Tema): void {
  try {
    window.localStorage.setItem(CLAVE_TEMA, tema);
  } catch {
    // Sin almacenamiento disponible: nada que persistir.
  }
}

/** Tema efectivo ahora: el de `data-theme` si ya está puesto, si no el de arranque. */
export function temaActual(): Tema {
  const valor = document.documentElement.dataset.theme;
  return valor === 'light' || valor === 'dark' ? valor : temaInicial();
}
