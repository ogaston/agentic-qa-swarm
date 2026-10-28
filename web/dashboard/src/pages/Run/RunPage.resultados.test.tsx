import { act, render, screen, within } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import informeDosHallazgos from '../../../../../contracts/plans/examples/valid/report.dos-hallazgos.json';
import informeSinHallazgos from '../../../../../contracts/plans/examples/valid/report.sin-hallazgos.json';
import informeInconcluso from '../../../../../contracts/plans/examples/valid/report.inconcluso.json';
import planCon from '../../../../../contracts/plans/examples/valid/flow-plan.body-y-sondeo.json';
import { server } from '../../test/server';
import { TIMERS_FALSOS, avanzar, vaciar } from '../../test/tiempo';
import { RunPage } from './RunPage';

// Fixtures: los ejemplos válidos de contracts/plans/examples, con el run_id de la ruta.
const ID = 'run-0123456789abcdef';

type Peticion = { status?: number; json: unknown };
type Contador = { n: number };

/** Respuestas en orden para una ruta; la última se repite cuando se agotan. */
function ruta(url: string, respuestas: Peticion[]): Contador {
  const contador: Contador = { n: 0 };
  server.use(
    http.get(url, () => {
      const i = Math.min(contador.n, respuestas.length - 1);
      contador.n++;
      const r = respuestas[i] ?? { json: {} };
      return HttpResponse.json(r.json as object, { status: r.status ?? 200 });
    }),
  );
  return contador;
}

function corrida(state: string, extra: Record<string, unknown> = {}): Peticion {
  return { json: { id: ID, state, trace_id: 'tr-1', ...extra } };
}

function error(status: number, code: string): Peticion {
  return { status, json: { code, message: `mensaje ${code}` } };
}

function sinPeticiones(contador: Contador) {
  expect(contador.n).toBe(0);
}

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

async function pulsar(nombre: string) {
  await act(async () => {
    screen.getByRole('button', { name: nombre }).click();
  });
  await vaciar();
}

beforeEach(() => {
  vi.useFakeTimers(TIMERS_FALSOS);
});

afterEach(() => {
  setVisibilidad('visible');
  vi.useRealTimers();
});

describe('RunPage: veredicto', () => {
  it('los tres veredictos muestran texto distinto', async () => {
    const textos: string[] = [];
    for (const verdict of ['bug', 'sin-hallazgos', 'inconcluso'] as const) {
      ruta(`/api/runs/${ID}`, [corrida('done', { verdict })]);
      ruta(`/api/reports/${ID}`, [{ json: { ...informeDosHallazgos, run_id: ID, verdict } }]);
      ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
      const { unmount } = renderRun();
      await vaciar();
      const insignia = document.querySelector(`[data-veredicto="${verdict}"]`);
      expect(insignia?.textContent).toBeTruthy();
      textos.push(insignia?.textContent ?? '');
      unmount();
    }
    expect(new Set(textos).size).toBe(3);
  });
});

describe('RunPage: post-mortem', () => {
  it('done pide GET /reports exactamente una vez y ninguna más en 30 s', async () => {
    ruta(`/api/runs/${ID}`, [corrida('done', { verdict: 'bug' })]);
    ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    const reporte = ruta(`/api/reports/${ID}`, [{ json: { ...informeDosHallazgos, run_id: ID } }]);
    renderRun();
    await vaciar();
    await avanzar(30_000);
    expect(reporte.n).toBe(1);
  });

  it('antes de done no pide GET /reports', async () => {
    ruta(`/api/runs/${ID}`, [corrida('reporting')]);
    ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    const reporte = ruta(`/api/reports/${ID}`, [{ json: informeDosHallazgos }]);
    renderRun();
    await vaciar();
    await avanzar(10_000);
    sinPeticiones(reporte);
  });

  it('muestra summary, método, ruta, invariante, causa raíz y evidencias como texto', async () => {
    ruta(`/api/runs/${ID}`, [corrida('done', { verdict: 'bug' })]);
    ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    ruta(`/api/reports/${ID}`, [{ json: { ...informeDosHallazgos, run_id: ID } }]);
    renderRun();
    await vaciar();
    const pm = within(screen.getByRole('region', { name: 'Informe post-mortem' }));
    expect(pm.getByText('Dos flujos fallidos')).toBeTruthy();
    expect(pm.getByText('/orders')).toBeTruthy();
    expect(pm.getByText('/orders/{id}')).toBeTruthy();
    expect(pm.getByText('inv a')).toBeTruthy();
    expect(pm.getByText('causa a')).toBeTruthy();
    expect(pm.getByText('s3://aqs-evidence/runs/run-1/flow-1/result.json')).toBeTruthy();
    expect(pm.getByText('s3://aqs-evidence/runs/run-1/flow-1/logs.txt')).toBeTruthy();
    expect(pm.getByText('https://evidence.example/run-1/flow-2/result.json')).toBeTruthy();
    expect(pm.queryByRole('link')).toBeNull();
  });

  it('findings vacío muestra un mensaje sin hallazgos', async () => {
    ruta(`/api/runs/${ID}`, [corrida('done', { verdict: 'sin-hallazgos' })]);
    ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    ruta(`/api/reports/${ID}`, [{ json: { ...informeSinHallazgos, run_id: ID } }]);
    renderRun();
    await vaciar();
    expect(screen.getByText('No se encontraron hallazgos en esta corrida.')).toBeTruthy();
  });

  it('404 report_not_ready muestra el aviso y «Reintentar» hace una petición más', async () => {
    ruta(`/api/runs/${ID}`, [corrida('done', { verdict: 'bug' })]);
    ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    const reporte = ruta(`/api/reports/${ID}`, [
      error(404, 'report_not_ready'),
      { json: { ...informeInconcluso, run_id: ID } },
    ]);
    renderRun();
    await vaciar();
    expect(screen.getByText('El informe aún no está disponible.')).toBeTruthy();
    expect(reporte.n).toBe(1);
    await pulsar('Reintentar');
    expect(reporte.n).toBe(2);
    expect(screen.getByText('Fallo de formato de la entrada')).toBeTruthy();
  });

  it('403 muestra «Sin acceso al informe»', async () => {
    ruta(`/api/runs/${ID}`, [corrida('done', { verdict: 'bug' })]);
    ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    ruta(`/api/reports/${ID}`, [error(403, 'forbidden')]);
    renderRun();
    await vaciar();
    expect(screen.getByText('Sin acceso al informe.')).toBeTruthy();
  });

  it('502 muestra un aviso y «Reintentar»', async () => {
    ruta(`/api/runs/${ID}`, [corrida('done', { verdict: 'bug' })]);
    ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    ruta(`/api/reports/${ID}`, [error(502, 'report_mismatch')]);
    renderRun();
    await vaciar();
    expect(screen.getByRole('alert').textContent).toMatch(/No se pudo cargar el informe/);
    expect(screen.getByRole('button', { name: 'Reintentar' })).toBeTruthy();
  });

  it('un summary con HTML se muestra como texto y no crea ningún img', async () => {
    const malicioso = '<img src=x onerror=alert(1)>';
    ruta(`/api/runs/${ID}`, [corrida('done', { verdict: 'bug' })]);
    ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    ruta(`/api/reports/${ID}`, [{ json: { ...informeDosHallazgos, run_id: ID, summary: malicioso } }]);
    renderRun();
    await vaciar();
    expect(screen.getByText(malicioso)).toBeTruthy();
    expect(document.querySelectorAll('img').length).toBe(0);
  });

  it('failed sin report_uri no pide GET /reports', async () => {
    ruta(`/api/runs/${ID}`, [corrida('rehearsing'), corrida('failed')]);
    ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    const reporte = ruta(`/api/reports/${ID}`, [{ json: informeDosHallazgos }]);
    renderRun();
    await vaciar();
    await avanzar(10_000);
    sinPeticiones(reporte);
  });

  it('failed con report_uri pide el informe una vez', async () => {
    ruta(`/api/runs/${ID}`, [
      corrida('reporting'),
      corrida('failed', { report_uri: 's3://aqs-reports/x.json' }),
    ]);
    ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    const reporte = ruta(`/api/reports/${ID}`, [{ json: { ...informeDosHallazgos, run_id: ID } }]);
    renderRun();
    await vaciar();
    await avanzar(10_000);
    expect(reporte.n).toBe(1);
  });
});

describe('RunPage: plan de flujos', () => {
  it('antes de rehearsing no pide el plan', async () => {
    ruta(`/api/runs/${ID}`, [corrida('inferring')]);
    const plan = ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    renderRun();
    await vaciar();
    await avanzar(10_000);
    sinPeticiones(plan);
  });

  it('al llegar a rehearsing pide el plan una vez y muestra el body en <pre>', async () => {
    ruta(`/api/runs/${ID}`, [corrida('rehearsing')]);
    const plan = ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    renderRun();
    await vaciar();
    await avanzar(10_000);
    expect(plan.n).toBe(1);
    expect(screen.getByRole('region', { name: 'Plan de flujos' })).toBeTruthy();
    expect(screen.getByText('1. sondeo: no vender mas stock del disponible')).toBeTruthy();
    const pre = document.querySelector('pre');
    expect(pre?.textContent).toBe(JSON.stringify({ product_id: 1, qty: 11 }, null, 2));
  });

  it('404 plan_not_ready muestra «Plan aún no disponible»', async () => {
    ruta(`/api/runs/${ID}`, [corrida('rehearsing')]);
    ruta(`/api/runs/${ID}/plan`, [error(404, 'plan_not_ready')]);
    renderRun();
    await vaciar();
    expect(screen.getByText('Plan aún no disponible.')).toBeTruthy();
  });

  it('failed abierto directamente pide el plan una vez y muestra «Flujos ejecutados»', async () => {
    ruta(`/api/runs/${ID}`, [corrida('failed')]);
    const plan = ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    renderRun();
    await vaciar();
    await avanzar(10_000);
    expect(plan.n).toBe(1);
    expect(screen.getByRole('region', { name: 'Plan de flujos' })).toBeTruthy();
    expect(screen.getByText('1. sondeo: no vender mas stock del disponible')).toBeTruthy();
  });

  it('failed llegando desde rehearsing pide el plan y lo muestra', async () => {
    ruta(`/api/runs/${ID}`, [corrida('rehearsing'), corrida('failed')]);
    const plan = ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    renderRun();
    await vaciar();
    await avanzar(3000);
    expect(plan.n).toBe(1);
    expect(screen.getByRole('region', { name: 'Plan de flujos' })).toBeTruthy();
    expect(screen.getByText('1. sondeo: no vender mas stock del disponible')).toBeTruthy();
  });

  it('failed sin plan muestra «Plan aún no disponible»', async () => {
    ruta(`/api/runs/${ID}`, [corrida('failed')]);
    ruta(`/api/runs/${ID}/plan`, [error(404, 'plan_not_ready')]);
    renderRun();
    await vaciar();
    expect(screen.getByText('Plan aún no disponible.')).toBeTruthy();
  });

  it('404 de reports con otro código muestra «Sin acceso al informe»', async () => {
    ruta(`/api/runs/${ID}`, [corrida('done', { verdict: 'bug' })]);
    ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    ruta(`/api/reports/${ID}`, [error(404, 'not_found')]);
    renderRun();
    await vaciar();
    expect(screen.getByText('Sin acceso al informe.')).toBeTruthy();
  });

  it('error de red en reports muestra el aviso con «Reintentar»', async () => {
    ruta(`/api/runs/${ID}`, [corrida('done', { verdict: 'bug' })]);
    ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    server.use(http.get(`/api/reports/${ID}`, () => HttpResponse.error()));
    renderRun();
    await vaciar();
    expect(screen.getByRole('alert').textContent).toMatch(/No se pudo cargar el informe/);
    expect(screen.getByRole('button', { name: 'Reintentar' })).toBeTruthy();
  });

  it('error genérico de reports usa el aviso común de error', async () => {
    ruta(`/api/runs/${ID}`, [corrida('done', { verdict: 'bug' })]);
    ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    ruta(`/api/reports/${ID}`, [error(400, 'invalid_run')]);
    renderRun();
    await vaciar();
    expect(screen.getByText('invalid_run')).toBeTruthy();
  });

  it('502 en el plan muestra «Reintentar» y lo vuelve a pedir', async () => {
    ruta(`/api/runs/${ID}`, [corrida('rehearsing')]);
    const plan = ruta(`/api/runs/${ID}/plan`, [error(502, 'bad_gateway'), { json: { ...planCon, run_id: ID } }]);
    renderRun();
    await vaciar();
    expect(screen.getByRole('button', { name: 'Reintentar' })).toBeTruthy();
    await pulsar('Reintentar');
    expect(plan.n).toBe(2);
    expect(screen.getByRole('region', { name: 'Plan de flujos' })).toBeTruthy();
  });
});
