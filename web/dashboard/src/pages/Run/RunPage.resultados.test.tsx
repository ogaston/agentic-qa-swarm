import { act, fireEvent, render, screen, within } from '@testing-library/react';
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
      if (verdict === 'sin-hallazgos') expect(insignia).toBeNull();
      else expect(insignia?.textContent).toBeTruthy();
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
    const pm = within(screen.getByRole('region', { name: 'Resultados de la prueba' }));
    expect(pm.getByText('Dos flujos fallidos')).toBeTruthy();
    expect(pm.getByText('/orders')).toBeTruthy();
    expect(pm.getByText('/orders/{id}')).toBeTruthy();
    expect(pm.getByText('inv a')).toBeTruthy();
    expect(pm.getByText('causa a')).toBeTruthy();
    expect(pm.queryByText('s3://aqs-evidence/runs/run-1/flow-1/result.json')).toBeNull();
    expect(pm.queryByText('s3://aqs-evidence/runs/run-1/flow-1/logs.txt')).toBeNull();
    expect(pm.queryByText('https://evidence.example/run-1/flow-2/result.json')).toBeNull();
    expect(pm.getByRole('link', { name: /Abrir evidencia/ })).toBeTruthy();
    expect(pm.getAllByRole('link', { name: /Descargar evidencia/ })).toHaveLength(2);
  });

  it('findings vacío muestra un mensaje sin hallazgos', async () => {
    ruta(`/api/runs/${ID}`, [corrida('done', { verdict: 'sin-hallazgos' })]);
    ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    ruta(`/api/reports/${ID}`, [{ json: { ...informeSinHallazgos, run_id: ID } }]);
    renderRun();
    await vaciar();
    expect(screen.queryByText('No se encontraron hallazgos en esta corrida.')).toBeNull();
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
  it('muestra el resultado real de cada flujo y permite descargar evidencia', async () => {
    ruta(`/api/runs/${ID}`, [corrida('done')]);
    ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    server.use(http.get('/api/run-evidence/:run/:flow/result.json', ({ params }) =>
      HttpResponse.json({ status: params.flow === planCon.flows[0]?.flow_id ? 'passed' : 'failed' }),
    ));
    renderRun();
    await vaciar();
    await vaciar();
    expect(screen.getByRole('heading', { name: 'Flujos principales' })).toBeTruthy();
    expect(screen.getByRole('heading', { name: 'Reglas de negocio' })).toBeTruthy();
    expect(screen.getByText('Aprobada')).toBeTruthy();
    expect(screen.getByText('Fallida')).toBeTruthy();
    const firstFlow = planCon.flows[0];
    if (!firstFlow) throw new Error('Missing flow fixture');
    const flow = screen.getByText(firstFlow.name).closest('details');
    if (!flow) throw new Error('Missing flow details');
    const summary = flow.querySelector('summary');
    if (!summary) throw new Error('Missing flow summary');
    expect(flow.open).toBe(false);
    expect(within(flow).getByRole('button', { name: 'Descargar resultados', hidden: true })).toBeTruthy();
    expect(within(flow).getByRole('button', { name: 'Descargar registros', hidden: true })).toBeTruthy();
    fireEvent.click(summary);
    expect(flow.open).toBe(true);
    fireEvent.click(summary);
    expect(flow.open).toBe(false);
  });
  it('running mantiene los flujos cerrados hasta que se abran', async () => {
    ruta(`/api/runs/${ID}`, [corrida('running')]);
    const plan = ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    renderRun();
    await vaciar();
    const region = screen.getByRole('region', { name: 'Plan de flujos' });
    const flujos = region.querySelectorAll<HTMLDetailsElement>('.flow-details');
    expect(flujos.length).toBe(2);
    flujos.forEach((flujo) => expect(flujo.open).toBe(false));
    expect(region.parentElement?.firstElementChild).toBe(region);
    // El sondeo de la corrida no vuelve a abrir lo que el usuario cierra.
    await act(async () => { if (flujos[0]) flujos[0].open = false; });
    await avanzar(9000);
    expect(flujos[0]?.open).toBe(false);
    expect(plan.n).toBe(1);
  });

  it('al pasar de rehearsing a running mantiene los flujos cerrados sin volver a pedir el plan', async () => {
    ruta(`/api/runs/${ID}`, [corrida('rehearsing'), corrida('running')]);
    const plan = ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    renderRun();
    await vaciar();
    expect(document.querySelector<HTMLDetailsElement>('.flow-details')?.open).toBe(false);
    await avanzar(3000);
    expect(document.querySelector<HTMLDetailsElement>('.flow-details')?.open).toBe(false);
    expect(plan.n).toBe(1);
  });

  it('recupera plan_not_ready durante running y deja de pedir al recibir el plan', async () => {
    ruta(`/api/runs/${ID}`, [corrida('running')]);
    const plan = ruta(`/api/runs/${ID}/plan`, [error(404, 'plan_not_ready'), { json: { ...planCon, run_id: ID } }]);
    renderRun();
    await vaciar();
    expect(plan.n).toBe(1);
    await avanzar(3000);
    expect(plan.n).toBe(2);
    expect(document.querySelector<HTMLDetailsElement>('.flow-details')?.open).toBe(false);
    await avanzar(30_000);
    expect(plan.n).toBe(2);
  });

  it('limita plan_not_ready a cinco intentos y permite reintento manual', async () => {
    ruta(`/api/runs/${ID}`, [corrida('running')]);
    const plan = ruta(`/api/runs/${ID}/plan`, [error(404, 'plan_not_ready')]);
    renderRun();
    await vaciar();
    for (let i = 0; i < 5; i++) await avanzar(3000);
    expect(plan.n).toBe(5);
    await avanzar(30_000);
    expect(plan.n).toBe(5);
    await pulsar('Reintentar');
    expect(plan.n).toBe(6);
    await avanzar(3000);
    expect(plan.n).toBe(7);
  });

  it('running consulta inmediatamente aunque se agotaran los intentos durante rehearsing', async () => {
    ruta(`/api/runs/${ID}`, [
      ...Array.from({ length: 6 }, () => corrida('rehearsing')),
      corrida('running'),
    ]);
    const plan = ruta(`/api/runs/${ID}/plan`, [
      ...Array.from({ length: 5 }, () => error(404, 'plan_not_ready')),
      { json: { ...planCon, run_id: ID } },
    ]);
    renderRun();
    await vaciar();
    for (let i = 0; i < 5; i++) await avanzar(3000);
    expect(plan.n).toBe(5);
    await avanzar(3000);
    expect(plan.n).toBe(6);
    expect(document.querySelector<HTMLDetailsElement>('.flow-details')?.open).toBe(false);
  });

  it('desmontar cancela el reintento pendiente del plan', async () => {
    ruta(`/api/runs/${ID}`, [corrida('running')]);
    const plan = ruta(`/api/runs/${ID}/plan`, [error(404, 'plan_not_ready')]);
    const { unmount } = renderRun();
    await vaciar();
    unmount();
    await avanzar(9000);
    expect(plan.n).toBe(1);
  });

  it('al terminar cancela los reintentos pendientes y conserva reintento manual', async () => {
    ruta(`/api/runs/${ID}`, [corrida('rehearsing'), corrida('failed')]);
    const plan = ruta(`/api/runs/${ID}/plan`, [error(404, 'plan_not_ready')]);
    renderRun();
    await vaciar();
    await avanzar(3000);
    const pedidosAlTerminar = plan.n;
    await avanzar(30_000);
    expect(plan.n).toBe(pedidosAlTerminar);
    expect(screen.getByTestId('run-plan-retry-button')).toBeTruthy();
  });

  it('403 del plan no reintenta automáticamente', async () => {
    ruta(`/api/runs/${ID}`, [corrida('running')]);
    const plan = ruta(`/api/runs/${ID}/plan`, [error(403, 'forbidden')]);
    renderRun();
    await vaciar();
    await avanzar(30_000);
    expect(plan.n).toBe(1);
    expect(screen.getByRole('alert').textContent).toContain('No tienes acceso');
  });

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
    expect(screen.getByText('sondeo: no vender mas stock del disponible')).toBeTruthy();
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
    expect(screen.getByText('sondeo: no vender mas stock del disponible')).toBeTruthy();
  });

  it('failed llegando desde rehearsing pide el plan y lo muestra', async () => {
    ruta(`/api/runs/${ID}`, [corrida('rehearsing'), corrida('failed')]);
    const plan = ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    renderRun();
    await vaciar();
    await avanzar(3000);
    expect(plan.n).toBe(1);
    expect(screen.getByRole('region', { name: 'Plan de flujos' })).toBeTruthy();
    expect(screen.getByText('sondeo: no vender mas stock del disponible')).toBeTruthy();
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
    expect(screen.queryByText('invalid_run')).toBeNull();
    expect(screen.getByRole('alert').textContent).toContain('No se pudo actualizar la información');
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
