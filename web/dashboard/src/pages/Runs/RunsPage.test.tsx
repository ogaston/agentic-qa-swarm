import { render, screen, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { server } from '../../test/server';
import { RunsPage } from './RunsPage';

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
    const hora = new Date(primero.confirmed_at).toLocaleString();
    expect(screen.getByText(hora)).toBeTruthy();
  });

  it('lista vacía muestra el mensaje de vacío', async () => {
    server.use(http.get('/api/confirmations', () => HttpResponse.json([])));
    renderMisCorridas();
    expect(await screen.findByText('Aún no tienes corridas confirmadas.')).toBeTruthy();
  });
});
