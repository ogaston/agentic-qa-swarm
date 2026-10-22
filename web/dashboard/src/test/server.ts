import { http, HttpResponse } from 'msw';
import { setupServer } from 'msw/node';

// Servidor MSW compartido por las pruebas. Cualquier petición sin handler falla:
// ninguna prueba puede salir a la red.
// Handlers por defecto: la página de inbox pide la lista al entrar en /inbox; las pruebas
// de sesión y login no necesitan mockearla. Las pruebas del inbox lo sobrescriben con server.use.
// La vista de corrida pide el informe y el plan al llegar a sus estados: por defecto, «aún no».
export const server = setupServer(
  http.get('/api/notifications', () => HttpResponse.json([])),
  // La cabecera del inbox lee el estado del warm (U10-T02): por defecto, «no disponible».
  http.get('/api/warm', () =>
    HttpResponse.json({ code: 'warm_unavailable', message: 'warm no disponible' }, { status: 503 }),
  ),
  http.get('/api/reports/:run', () =>
    HttpResponse.json({ code: 'report_not_ready', message: 'el informe aún no está disponible' }, { status: 404 }),
  ),
  http.get('/api/runs/:id/plan', () =>
    HttpResponse.json({ code: 'plan_not_ready', message: 'plan aún no disponible' }, { status: 404 }),
  ),
);
