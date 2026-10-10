// Tabla de enrutado del proxy de desarrollo/preview. Función pura: no importa Vite,
// para poder probarla sin levantar el servidor.
//
// Orden importa: la primera regla que casa gana. Las reglas más específicas van antes.
// Casar es por segmento: /api/authz no casa con /api/auth.

export interface ProxyRule {
  readonly prefix: string;
  readonly envVar: string;
  readonly target: string;
}

export interface ProxyResolution {
  readonly target: string;
  readonly path: string;
}

const RULE_SPECS = [
  { prefix: '/api/auth', envVar: 'AQS_IDENTITY_URL' },
  { prefix: '/api/runs', envVar: 'AQS_RUN_CONTROLLER_URL' },
  { prefix: '/api', envVar: 'AQS_UI_API_URL' },
] as const;

export function buildProxyTable(env: Record<string, string | undefined>): ProxyRule[] {
  return RULE_SPECS.map(({ prefix, envVar }) => {
    const target = env[envVar];
    if (target === undefined || target.trim() === '') {
      throw new Error(
        `falta la variable de entorno ${envVar} (necesaria para el proxy de ${prefix})`,
      );
    }
    return { prefix, envVar, target: target.trim() };
  });
}

function matches(prefix: string, path: string): boolean {
  return path === prefix || path.startsWith(`${prefix}/`);
}

// Quita el prefijo /api. /api y /api/ quedan como /.
function stripApi(path: string): string {
  const rest = path.slice('/api'.length);
  return rest === '' ? '/' : rest;
}

export function resolveProxy(table: readonly ProxyRule[], path: string): ProxyResolution | null {
  const rule = table.find((r) => matches(r.prefix, path));
  if (rule === undefined) return null;
  return { target: rule.target, path: stripApi(path) };
}

export interface ViteProxyOptions {
  target: string;
  changeOrigin: boolean;
  rewrite: (path: string) => string;
}

// Vite trata las claves que empiezan por ^ como regex. Así casan por segmento, igual que resolveProxy.
export function toViteProxy(table: readonly ProxyRule[]): Record<string, ViteProxyOptions> {
  const out: Record<string, ViteProxyOptions> = {};
  for (const rule of table) {
    out[`^${rule.prefix}(/|$)`] = {
      target: rule.target,
      changeOrigin: true,
      rewrite: stripApi,
    };
  }
  return out;
}
