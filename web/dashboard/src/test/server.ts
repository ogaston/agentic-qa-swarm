import { http, HttpResponse } from 'msw';
import { setupServer } from 'msw/node';

// Servidor MSW compartido por las pruebas. Cualquier petición sin handler falla:
// ninguna prueba puede salir a la red.
// Handler por defecto: la página de inbox pide la lista al entrar en /inbox; las pruebas
// de sesión y login no necesitan mockearla. Las pruebas del inbox lo sobrescriben con server.use.
export const server = setupServer(http.get('/api/notifications', () => HttpResponse.json([])));
