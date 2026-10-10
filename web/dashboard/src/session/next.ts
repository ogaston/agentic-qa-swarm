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
  return raw;
}
