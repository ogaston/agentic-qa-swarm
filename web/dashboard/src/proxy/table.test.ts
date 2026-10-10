import { describe, expect, it } from 'vitest';
import { buildProxyTable, resolveProxy, toViteProxy } from './table';

const ENV = {
  AQS_IDENTITY_URL: 'http://identity.local:8081',
  AQS_RUN_CONTROLLER_URL: 'http://runs.local:8082',
  AQS_UI_API_URL: 'http://ui-api.local:8083',
};

describe('tabla de proxy', () => {
  const table = buildProxyTable(ENV);

  it('/api/auth/login va a identidad sin el prefijo /api', () => {
    expect(resolveProxy(table, '/api/auth/login')).toEqual({
      target: ENV.AQS_IDENTITY_URL,
      path: '/auth/login',
    });
  });

  it('/api/runs/x va al controlador sin el prefijo /api', () => {
    expect(resolveProxy(table, '/api/runs/x')).toEqual({
      target: ENV.AQS_RUN_CONTROLLER_URL,
      path: '/runs/x',
    });
  });

  it('/api/notifications y /api/warm van a ui-api sin el prefijo /api', () => {
    expect(resolveProxy(table, '/api/notifications')).toEqual({
      target: ENV.AQS_UI_API_URL,
      path: '/notifications',
    });
    expect(resolveProxy(table, '/api/warm')).toEqual({
      target: ENV.AQS_UI_API_URL,
      path: '/warm',
    });
  });

  it('una ruta del SPA no se proxea', () => {
    expect(resolveProxy(table, '/runs/abc')).toBeNull();
    expect(resolveProxy(table, '/inbox')).toBeNull();
  });

  it('el prefijo se compara por segmento: /api/authz no es identidad', () => {
    expect(resolveProxy(table, '/api/authz')).toEqual({
      target: ENV.AQS_UI_API_URL,
      path: '/authz',
    });
  });

  it('sin una de las tres variables falla y nombra la variable que falta', () => {
    for (const name of Object.keys(ENV) as (keyof typeof ENV)[]) {
      const partial: Record<string, string | undefined> = Object.fromEntries(
        Object.entries(ENV).filter(([key]) => key !== name),
      );
      expect(() => buildProxyTable(partial)).toThrow(name);
    }
  });

  it('las claves de toViteProxy casan por segmento, igual que resolveProxy', () => {
    // Vite toma las claves string por prefijo literal y las que empiezan por ^ como regex.
    const vite = toViteProxy(table);
    const viteTarget = (path: string): string | undefined => {
      for (const [key, opts] of Object.entries(vite)) {
        const hit = key.startsWith('^') ? new RegExp(key.slice(1)).test(path) : path.startsWith(key);
        if (hit) return opts.target;
      }
      return undefined;
    };
    for (const path of ['/api/authz', '/api/runs2/x', '/apix', '/api/auth/login', '/api/warm']) {
      expect(viteTarget(path)).toBe(resolveProxy(table, path)?.target);
    }
    expect(viteTarget('/api/authz')).toBe(ENV.AQS_UI_API_URL);
  });

  it('toViteProxy reescribe quitando /api y conserva el destino', () => {
    const vite = toViteProxy(table);
    const rule = vite['^/api/runs(/|$)'];
    expect(rule).toBeDefined();
    expect(rule?.target).toBe(ENV.AQS_RUN_CONTROLLER_URL);
    expect(rule?.rewrite?.('/api/runs/x')).toBe('/runs/x');
  });
});
