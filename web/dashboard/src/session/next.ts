/** Ruta de login que recuerda a dónde volver tras entrar. */
export function loginPath(destino: string): string {
  return `/login?next=${encodeURIComponent(destino)}`;
}

/**
 * Acepta `next` solo si es una ruta interna: empieza por `/` y no por `//` ni
 * por `/\` (los navegadores tratan ambas como host externo). Si no, vale `/inbox`.
 */
export function safeNext(raw: string | null): string {
  if (raw === null) return '/inbox';
  if (!raw.startsWith('/') || raw.startsWith('//') || raw.startsWith('/\\')) return '/inbox';
  // A-1: el WHATWG elimina tabuladores y saltos de línea («/\t/host» se vuelve «//host»).
  // Se rechazan controles, espacios y barras invertidas, y se comprueba el origen real.
  if (/[\u0000-\u0020\u007f\\]/.test(raw)) return '/inbox';
  const base = 'https://destino.invalid';
  if (new URL(raw, base).origin !== base) return '/inbox';
  return raw;
}
