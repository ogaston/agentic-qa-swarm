import { act, render, screen } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { MemoryRouter, Route, Routes, useNavigate } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { server } from '../../test/server';
import { TIMERS_FALSOS, avanzar, vaciar } from '../../test/tiempo';
import { RunPage } from './RunPage';

const ID = 'run-0123456789abcdef';

function setVisibilidad(valor: 'visible' | 'hidden') {
  Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => valor });
  document.dispatchEvent(new Event('visibilitychange'));
}

function renderRun(id = ID) {
  return render(
    <MemoryRouter initialEntries={[`/runs/${id}`]}>
      <Routes>
        <Route path="/runs/:id" element={<RunPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

/** Respuestas de la API en orden; la última se repite cuando se agotan. */
function respuestas(...estados: Array<string | { status: number; body?: unknown }>) {
  const contador = { peticiones: 0 };
  server.use(
    http.get(`/api/runs/${ID}`, () => {
      const i = Math.min(contador.peticiones, estados.length - 1);
      contador.peticiones++;
      const e = estados[i];
      if (typeof e === 'string') return HttpResponse.json({ id: ID, state: e, trace_id: 'tr-1' });
      if (e === undefined) return HttpResponse.json({ id: ID, state: 'running' });
      const cuerpo = e.body ?? { code: 'err', message: 'fallo' };
      return HttpResponse.json(cuerpo, { status: e.status });
    }),
  );
  return contador;
}

beforeEach(() => {
  vi.useFakeTimers(TIMERS_FALSOS);
});

afterEach(() => {
  setVisibilidad('visible');
  vi.useRealTimers();
});

const ID_B = 'run-' + 'b'.repeat(16);

function Saltar({ a }: { a: string }) {
  const navegar = useNavigate();
  return <button type="button" onClick={() => navegar(a)}>ir</button>;
}

describe('RunPage', () => {
  it('cambiar de :id no muestra datos de la corrida anterior ni vuelve a pedirla', async () => {
    const pedidasA = { n: 0 };
    server.use(
      http.get(`/api/runs/${ID}`, () => {
        pedidasA.n++;
        return HttpResponse.json({ id: ID, state: 'running', trace_id: 'tr-A' });
      }),
      // B nunca responde: lo que se vea después es lo que quedó de A.
      http.get(`/api/runs/${ID_B}`, () => new Promise(() => {})),
    );
    render(
      <MemoryRouter initialEntries={[`/runs/${ID}`]}>
        <Routes>
          <Route
            path="/runs/:id"
            element={
              <>
                <RunPage />
                <Saltar a={`/runs/${ID_B}`} />
              </>
            }
          />
        </Routes>
      </MemoryRouter>,
    );
    await vaciar();
    expect(screen.getByText('Ejecutando pruebas', { selector: '.phase-name' }).closest('li')?.getAttribute('aria-current')).toBe('step');
    const antes = pedidasA.n;
    act(() => {
      screen.getByRole('button', { name: 'ir' }).click();
    });
    await avanzar(12_000);
    expect(screen.queryByText('tr-A')).toBeNull();
    expect(pedidasA.n).toBe(antes);
  });

  it('running marca 6 fases como hechas o actuales y running como actual', async () => {
    respuestas('running', 'running');
    renderRun();
    await vaciar();
    const actual = screen.getByText('Ejecutando pruebas', { selector: '.phase-name' }).closest('li');
    expect(actual?.getAttribute('aria-current')).toBe('step');
    const marcadas = document.querySelectorAll('[data-fase-estado="hecha"], [data-fase-estado="actual"]');
    expect(marcadas.length).toBe(6);
  });

  it('secuencia deploying → running → done: exactamente 3 peticiones y ninguna tras done', async () => {
    const contador = respuestas('deploying', 'running', 'done', 'done');
    renderRun();
    await vaciar();
    await avanzar(3000);
    await avanzar(3000);
    expect(contador.peticiones).toBe(3);
    await avanzar(10_000);
    expect(contador.peticiones).toBe(3);
    expect(screen.getByText('Completada', { selector: '.az-meta-value' })).toBeTruthy();
  });

  it('failed es terminal y se distingue sin depender del color', async () => {
    const contador = respuestas('inferring', 'failed');
    renderRun();
    await vaciar();
    await avanzar(3000);
    const aviso = screen.getByText(/Fallida/);
    expect(aviso.getAttribute('data-estado')).toBe('failed');
    expect(aviso.classList.contains('status-failed')).toBe(true);
    expect(aviso.textContent).toContain('Fallida');
    expect(screen.getByText(/última fase alcanzada: Analizando tu aplicación/)).toBeTruthy();
    await avanzar(10_000);
    expect(contador.peticiones).toBe(2);
  });

  it('pestaña oculta: 0 peticiones en 9 s y se reanuda al volver', async () => {
    const contador = respuestas('running');
    renderRun();
    await vaciar();
    expect(contador.peticiones).toBe(1);
    setVisibilidad('hidden');
    await avanzar(9000);
    expect(contador.peticiones).toBe(1);
    setVisibilidad('visible');
    await vaciar();
    expect(contador.peticiones).toBe(2);
  });

  it('desmontar la vista deja de pedir', async () => {
    const contador = respuestas('running');
    const { unmount } = renderRun();
    await vaciar();
    unmount();
    const antes = contador.peticiones;
    await avanzar(12_000);
    expect(contador.peticiones).toBe(antes);
  });

  it('5 503 seguidos: deja de sondear y aparece «Reintentar»', async () => {
    const contador = respuestas({ status: 503 }, { status: 503 }, { status: 503 }, { status: 503 }, { status: 503 });
    renderRun();
    await vaciar();
    await avanzar(30_000);
    expect(contador.peticiones).toBe(5);
    expect(screen.getByRole('button', { name: 'Reintentar' })).toBeTruthy();
  });

  it('404 inicial reintenta y recupera confirmed → running → done sin recargar', async () => {
    const contador = respuestas({ status: 404 }, 'confirmed', 'running', 'done');
    renderRun();
    await vaciar();
    expect(screen.queryByText('Corrida no encontrada')).toBeNull();
    expect(screen.getByRole('status').textContent).toContain('Esperando a que la corrida esté disponible');
    expect(screen.getByText('Esperando corrida')).toBeTruthy();
    await avanzar(3000);
    expect(contador.peticiones).toBe(2);
    expect(screen.getByText('Confirmada', { selector: '.phase-name' }).closest('li')?.getAttribute('aria-current')).toBe('step');
    expect(screen.queryByText(/Esperando a que la corrida esté disponible/)).toBeNull();
    await avanzar(3000);
    expect(screen.getByText('Ejecutando pruebas', { selector: '.phase-name' }).closest('li')?.getAttribute('aria-current')).toBe('step');
    await avanzar(3000);
    expect(screen.getByText('Completada', { selector: '.az-meta-value' })).toBeTruthy();
    await avanzar(12_000);
    expect(contador.peticiones).toBe(4);
  });

  it('5 404 iniciales agotan el sondeo y muestran «Reintentar» sin seguir cargando', async () => {
    const contador = respuestas({ status: 404 });
    renderRun();
    await vaciar();
    for (let i = 0; i < 4; i++) await avanzar(3000);
    expect(contador.peticiones).toBe(5);
    expect(screen.getByText('Corrida no encontrada')).toBeTruthy();
    expect(screen.getByText('No encontrada')).toBeTruthy();
    expect(screen.queryByText('Cargando…')).toBeNull();
    expect(screen.queryByText(/Esperando a que la corrida esté disponible/)).toBeNull();
    expect(screen.getByRole('button', { name: 'Reintentar' })).toBeTruthy();
    await avanzar(12_000);
    expect(contador.peticiones).toBe(5);
  });

  it('reintentar tras agotar 404 permite esperar otra vez y recuperar la corrida', async () => {
    const contador = respuestas(
      { status: 404 }, { status: 404 }, { status: 404 }, { status: 404 }, { status: 404 },
      { status: 404 }, 'confirmed',
    );
    renderRun();
    await vaciar();
    for (let i = 0; i < 4; i++) await avanzar(3000);
    act(() => screen.getByRole('button', { name: 'Reintentar' }).click());
    await vaciar();
    expect(contador.peticiones).toBe(6);
    expect(screen.queryByText('Corrida no encontrada')).toBeNull();
    expect(screen.getByRole('status').textContent).toContain('Esperando a que la corrida esté disponible');
    await avanzar(3000);
    expect(contador.peticiones).toBe(7);
    expect(screen.getByText('Confirmada', { selector: '.phase-name' }).closest('li')?.getAttribute('aria-current')).toBe('step');
    expect(screen.queryByRole('button', { name: 'Reintentar' })).toBeNull();
  });

  it('desmontar durante la espera inicial cancela los reintentos', async () => {
    const contador = respuestas({ status: 404 }, 'confirmed');
    const { unmount } = renderRun();
    await vaciar();
    unmount();
    await avanzar(12_000);
    expect(contador.peticiones).toBe(1);
  });

  it('cambiar de corrida cancela la espera anterior y reinicia el presupuesto de 404', async () => {
    const contador = respuestas({ status: 404 });
    let peticionesB = 0;
    server.use(http.get(`/api/runs/${ID_B}`, () => {
      peticionesB++;
      return peticionesB === 1
        ? HttpResponse.json({ code: 'not_found', message: 'pendiente' }, { status: 404 })
        : HttpResponse.json({ id: ID_B, state: 'confirmed', trace_id: 'tr-B' });
    }));
    render(
      <MemoryRouter initialEntries={[`/runs/${ID}`]}>
        <Routes>
          <Route path="/runs/:id" element={<><RunPage /><Saltar a={`/runs/${ID_B}`} /></>} />
        </Routes>
      </MemoryRouter>,
    );
    await vaciar();
    for (let i = 0; i < 3; i++) await avanzar(3000);
    expect(contador.peticiones).toBe(4);
    act(() => screen.getByRole('button', { name: 'ir' }).click());
    await vaciar();
    expect(peticionesB).toBe(1);
    expect(screen.queryByText('Corrida no encontrada')).toBeNull();
    await avanzar(3000);
    expect(peticionesB).toBe(2);
    expect(screen.getByText('Confirmada', { selector: '.phase-name' }).closest('li')?.getAttribute('aria-current')).toBe('step');
    expect(contador.peticiones).toBe(4);
  });

  it('id inválido no hace ninguna petición', async () => {
    const contador = respuestas('running');
    renderRun('run-XYZ');
    await vaciar();
    await avanzar(10_000);
    expect(contador.peticiones).toBe(0);
    expect(screen.getByText(/identificador de corrida no válido/i)).toBeTruthy();
  });

  it('lectura buena y luego 503: conserva la línea de tiempo y avisa del dato desactualizado', async () => {
    respuestas('running', { status: 503 });
    renderRun();
    await vaciar();
    await avanzar(3000);
    expect(screen.getByText('Mostrando la última lectura correcta.')).toBeTruthy();
    expect(screen.getByText('Ejecutando pruebas', { selector: '.phase-name' }).closest('li')?.getAttribute('aria-current')).toBe('step');
  });

  it('lectura buena y luego 404: no muestra el aviso de dato desactualizado', async () => {
    const contador = respuestas('running', { status: 404 });
    renderRun();
    await vaciar();
    await avanzar(3000);
    expect(screen.getByText('Corrida no encontrada')).toBeTruthy();
    expect(screen.queryByText('Mostrando la última lectura correcta.')).toBeNull();
    expect(screen.getByText('No encontrada')).toBeTruthy();
    await avanzar(12_000);
    expect(contador.peticiones).toBe(2);
  });

  it('id de 36 caracteres (formato viejo) no hace ninguna petición', async () => {
    const viejo = 'run-' + '0123456789abcdef'.repeat(2);
    const contador = respuestas('running');
    renderRun(viejo);
    await vaciar();
    await avanzar(10_000);
    expect(contador.peticiones).toBe(0);
    expect(screen.getByText(/identificador de corrida no válido/i)).toBeTruthy();
  });

  it('no expone trazas ni componentes internos', async () => {
    respuestas('running');
    renderRun();
    await vaciar();
    expect(screen.queryByText('tr-1')).toBeNull();
    expect(document.body.textContent).not.toMatch(/Trace:|go-run-controller|agent-reporter|invariant_probe|workflow:|GET \/api/);
  });
});
