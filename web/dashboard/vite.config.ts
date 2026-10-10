import react from '@vitejs/plugin-react';
import { loadEnv } from 'vite';
import { defineConfig } from 'vitest/config';
import { buildProxyTable, toViteProxy } from './src/proxy/table';

export default defineConfig(({ command, mode }) => {
  // Las variables del proxy solo se exigen al servir (dev y preview). Build y pruebas no las necesitan.
  const serving = command === 'serve' && mode !== 'test';
  const servers = serving
    ? { proxy: toViteProxy(buildProxyTable(loadEnv(mode, '.', 'AQS_'))) }
    : {};

  return {
    plugins: [react()],
    server: servers,
    preview: servers,
    test: {
      environment: 'jsdom',
      setupFiles: ['src/test/setup.ts'],
    },
  };
});
