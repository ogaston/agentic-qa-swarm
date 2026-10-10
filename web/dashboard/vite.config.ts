import react from '@vitejs/plugin-react';
import { loadEnv } from 'vite';
import { defineConfig } from 'vitest/config';
import { buildProxyTable, toViteProxy } from './src/proxy/table';

// Cabeceras de seguridad (U6-T06). Arbitraje del orquestador, aprobado por el humano: la CSP
// estricta va SOLO en preview (el build no lleva código en línea). En dev, el preámbulo en línea
// de React Refresh de @vitejs/plugin-react chocaría con script-src 'self', así que dev no lleva CSP.
const BASE_HEADERS = {
  'X-Content-Type-Options': 'nosniff',
  'Referrer-Policy': 'no-referrer',
};
const CSP_STRICT = {
  'Content-Security-Policy':
    "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
    "connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'",
};
const DEV_HEADERS = BASE_HEADERS;
const PREVIEW_HEADERS = { ...BASE_HEADERS, ...CSP_STRICT };

export default defineConfig(({ command, mode }) => {
  // Las variables del proxy solo se exigen al servir (dev y preview). Build y pruebas no las necesitan.
  const serving = command === 'serve' && mode !== 'test';
  const servers = serving
    ? { proxy: toViteProxy(buildProxyTable(loadEnv(mode, '.', 'AQS_'))) }
    : {};

  return {
    plugins: [react()],
    server: { headers: DEV_HEADERS, ...servers },
    preview: { headers: PREVIEW_HEADERS, ...servers },
    test: {
      environment: 'jsdom',
      setupFiles: ['src/test/setup.ts'],
    },
  };
});
