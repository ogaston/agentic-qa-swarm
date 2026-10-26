import { render, screen, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { server } from '../../test/server';
import { RunsPage } from './RunsPage';

// Mismo patrón que Card.test.tsx: se lee la hoja desde el cwd, sin navegador.
const fsModulo = 'node:fs';
const { readFileSync } = (await import(/* @vite-ignore */ fsModulo)) as {
  readFileSync: (ruta: string, codificacion: 'utf8') => string;
};
const proceso = (globalThis as unknown as { process: { cwd: () => string } }).process;
const CSS = readFileSync(`${proceso.cwd()}/src/styles.css`, 'utf8');

/** Bloque de declaraciones de la primera regla cuyo selector es exactamente `selector`. */
function declaracionesDe(selector: string): string {
  const re = new RegExp(`(^|\\n)${selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')} \\{([^}]*)\\}`);
  return re.exec(CSS)?.[2] ?? '';
}

const RECIBOS = [
  {
    run_id: 'run-' + 'a'.repeat(32),
    notification_id: 'not-1',
    confirmed_by: 'ana',
    confirmed_at: '2026-10-09T12:00:00Z',
  },
  {
    run_id: 'run-' + 'b'.repeat(32),
    notification_id: 'not-2',
    confirmed_by: 'ana',
    confirmed_at: '2026-10-08T09:30:00Z',
  },
];

function renderMisCorridas() {
  return render(
    <MemoryRouter initialEntries={['/runs']}>
      <Routes>
        <Route path="/runs" element={<RunsPage />} />
        <Route path="/runs/:id" element={<p>destino corrida</p>} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('RunsPage (Mis corridas)', () => {
  it('pide /api/confirmations?limit=20', async () => {
    const urls: string[] = [];
    server.use(
      http.get('/api/confirmations', ({ request }) => {
        urls.push(new URL(request.url).search);
        return HttpResponse.json([]);
      }),
    );
    renderMisCorridas();
    await waitFor(() => expect(urls).toEqual(['?limit=20']));
  });

  it('cada run_id enlaza a /runs/<run_id> y muestra notification_id, hora local y confirmed_by', async () => {
    server.use(http.get('/api/confirmations', () => HttpResponse.json(RECIBOS)));
    renderMisCorridas();
    const [primero] = RECIBOS;
    if (primero === undefined) throw new Error('fixture vacía');
    const enlace = await screen.findByRole('link', { name: primero.run_id });
    expect(enlace.getAttribute('href')).toBe(`/runs/${primero.run_id}`);
    expect(screen.getByText('not-1')).toBeTruthy();
    expect(screen.getAllByText('ana').length).toBe(2);
    const hora = new Date(primero.confirmed_at).toLocaleString('es-ES', { hour12: false });
    expect(screen.getByText(hora)).toBeTruthy();
  });

  it('lista vacía muestra el mensaje de vacío', async () => {
    server.use(http.get('/api/confirmations', () => HttpResponse.json([])));
    renderMisCorridas();
    expect(await screen.findByText('Aún no tienes corridas confirmadas.')).toBeTruthy();
  });
});

// U10-T04 CA-2: estructura de «Mis corridas» según el mockup (encabezado, tarjeta con tabla).
describe('RunsPage (estructura del mockup, U10-T04)', () => {
  it('el título «Mis corridas» es el h1 de la página', () => {
    server.use(http.get('/api/confirmations', () => HttpResponse.json([])));
    renderMisCorridas();
    expect(screen.getByRole('heading', { level: 1, name: 'Mis corridas' })).toBeTruthy();
  });

  it('la tabla vive en una tarjeta y tiene las columnas del mockup', async () => {
    server.use(http.get('/api/confirmations', () => HttpResponse.json(RECIBOS)));
    renderMisCorridas();
    const tabla = await screen.findByRole('table');
    expect(tabla.closest('.card')).not.toBeNull();
    const cabeceras = screen.getAllByRole('columnheader').map((th) => th.textContent ?? '');
    expect(cabeceras.some((t) => /^Corrida/.test(t))).toBe(true);
    expect(cabeceras.some((t) => /^Notificación/.test(t))).toBe(true);
    expect(cabeceras.some((t) => /^Confirmada \(hora local\)/.test(t))).toBe(true);
    expect(cabeceras.some((t) => t === 'Confirmada por')).toBe(true);
  });

  it('cada corrida ofrece «Ver corrida →» hacia su vista', async () => {
    server.use(http.get('/api/confirmations', () => HttpResponse.json(RECIBOS)));
    renderMisCorridas();
    const enlaces = await screen.findAllByRole('link', { name: 'Ver corrida →' });
    expect(enlaces.map((e) => e.getAttribute('href'))).toEqual(RECIBOS.map((r) => `/runs/${r.run_id}`));
  });

  it('el vacío muestra el mensaje dentro de la tarjeta', async () => {
    server.use(http.get('/api/confirmations', () => HttpResponse.json([])));
    renderMisCorridas();
    const mensaje = await screen.findByText('Aún no tienes corridas confirmadas.');
    expect(mensaje.closest('.card')).not.toBeNull();
  });

  // U10-T04 F-01: el enlace run_id tiene que verse como enlace (acento y negrita, como el mockup).
  it('el enlace run_id usa la clase de enlace de corrida, con acento y negrita del mockup', async () => {
    server.use(http.get('/api/confirmations', () => HttpResponse.json(RECIBOS)));
    renderMisCorridas();
    const enlace = await screen.findByRole('link', { name: RECIBOS[0]?.run_id ?? '' });
    expect(enlace.classList.contains('enlace-corrida')).toBe(true);
    expect(declaracionesDe('.enlace-corrida')).toMatch(/color:\s*var\(--accent-text\)/);
    expect(declaracionesDe('.enlace-corrida')).toMatch(/font-weight:\s*700/);
    expect(declaracionesDe('.enlace-corrida')).toMatch(/text-decoration:\s*underline/);
  });

  // U10-T04 F-02 (AMARILLO): los nombres de campo se acotan a las vistas de T04, sin regla global sobre th/dt.
  it('ninguna regla global de la hoja apunta a th code ni a dt code', () => {
    expect(CSS).not.toMatch(/(^|[\s,])th code\s*[,{]/);
    expect(CSS).not.toMatch(/(^|[\s,])dl\.kv-list dt code\s*[,{]/);
  });
});
