import { setupServer } from 'msw/node';

// Servidor MSW compartido por las pruebas. Cualquier petición sin handler falla:
// ninguna prueba puede salir a la red.
export const server = setupServer();
