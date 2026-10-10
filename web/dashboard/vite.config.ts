import react from '@vitejs/plugin-react';
import { loadEnv } from 'vite';
import { defineConfig } from 'vitest/config';
import { buildProxyTable, toViteProxy } from './src/proxy/table';

// Cabeceras de seguridad de dev y preview (U6-T06). El build no lleva código en línea, así que
// script-src y style-src son 'self' sin excepciones.
const SECURITY_HEADERS = {
  'Content-Security-Policy':
    "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
    "connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'",
  'X-Content-Type-Options': 'nosniff',
  'Referrer-Policy': 'no-referrer',
};

export default defineConfig(({ command, mode }) => {
  // Las variables del proxy solo se exigen al servir (dev y preview). Build y pruebas no las necesitan.
  const serving = command === 'serve' && mode !== 'test';
  const servers = serving
    ? { proxy: toViteProxy(buildProxyTable(loadEnv(mode, '.', 'AQS_'))) }
    : {};

  return {
    plugins: [react()],
    server: { headers: SECURITY_HEADERS, ...servers },
    preview: { headers: SECURITY_HEADERS, ...servers },
    test: {
      environment: 'jsdom',
      setupFiles: ['src/test/setup.ts'],
    },
  };
});
