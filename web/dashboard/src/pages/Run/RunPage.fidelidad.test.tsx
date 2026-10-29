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

// U10-T03 (CA-2): pruebas por estado de la «Vista de corrida» del mockup. Se buscan por rol o texto.
const ID = 'run-0123456789abcdef';

function ruta(url: string, estados: Array<{ status?: number; json: unknown }>) {
  const contador = { n: 0 };
  server.use(
    http.get(url, () => {
      const i = Math.min(contador.n, estados.length - 1);
      contador.n++;
      const r = estados[i] ?? { json: {} };
      return HttpResponse.json(r.json as object, { status: r.status ?? 200 });
    }),
  );
  return contador;
}

function corrida(state: string, extra: Record<string, unknown> = {}) {
  return { json: { id: ID, state, trace_id: 'tr-fid', ...extra } };
}

function renderRun() {
  return render(
    <MemoryRouter initialEntries={[`/runs/${ID}`]}>
      <Routes>
        <Route path="/runs/:id" element={<RunPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.useFakeTimers(TIMERS_FALSOS);
});

afterEach(() => {
  vi.useRealTimers();
});

const FASES_ESPERADAS = [
  'Confirmada',
  'Entorno listo',
  'Preparando la versión',
  'Analizando tu aplicación',
  'Validando las pruebas',
  'Ejecutando pruebas',
  'Restaurando el entorno',
  'Generando informe',
  'Completada',
];

describe('RunPage fidelidad: encabezado y línea de tiempo', () => {
  it('encabezado con h1 «Corrida», código del id, trace id y franja de metadatos', async () => {
    ruta(`/api/runs/${ID}`, [corrida('running')]);
    renderRun();
    await vaciar();
    expect(screen.getByRole('heading', { level: 1, name: /Resumen de la corrida/ })).toBeTruthy();
    expect(screen.queryByText('tr-fid')).toBeNull();
    expect(screen.getByText('Estado de corrida')).toBeTruthy();
    expect(screen.queryByText('Sondeo')).toBeNull();
  });

  it('9 fases con indicador accesible de carga además del color', async () => {
    ruta(`/api/runs/${ID}`, [corrida('running')]);
    renderRun();
    await vaciar();
    const lista = screen.getByRole('list', { name: 'Fases de la corrida' });
    const items = within(lista).getAllByRole('listitem');
    expect(items.length).toBe(9);
    const textos = items.map((li) => li.textContent ?? '');
    FASES_ESPERADAS.forEach((etiqueta, i) => expect(textos[i]).toContain(etiqueta));
    expect(items.filter((li) => li.textContent?.includes('hecha')).length).toBe(5);
    const actual = items[5];
    expect(actual?.getAttribute('aria-current')).toBe('step');
    expect(actual?.textContent).toContain('En curso');
    expect(actual?.textContent).not.toContain('actual');
    expect(actual?.querySelector('.phase-spinner')?.getAttribute('aria-hidden')).toBe('true');
    expect(items[8]?.textContent).toContain('pendiente');
    // Ningún ítem depende solo del color: cada uno lleva su estado escrito.
    for (const li of items) {
      expect(/hecha|En curso|pendiente/.test(li.textContent ?? '')).toBe(true);
    }
  });

  it('la línea de tiempo vive en una columna lateral con título propio', async () => {
    ruta(`/api/runs/${ID}`, [corrida('running')]);
    renderRun();
    await vaciar();
    expect(screen.getByRole('region', { name: 'Línea de tiempo de fases' })).toBeTruthy();
  });
});

describe('RunPage fidelidad: estados', () => {
  it('fallida: banner role=status con data-estado y clase de alerta roja', async () => {
    ruta(`/api/runs/${ID}`, [corrida('rehearsing'), corrida('failed')]);
    renderRun();
    await vaciar();
    await avanzar(3000);
    const banner = document.querySelector('[data-estado="failed"]');
    expect(banner).not.toBeNull();
    expect(banner?.getAttribute('role')).toBe('status');
    expect(banner?.classList.contains('alert-box')).toBe(true);
    expect(banner?.classList.contains('alert-danger')).toBe(true);
    expect(banner?.getAttribute('style')).toBeNull();
    expect(banner?.textContent).toContain('Fallida');
  });

  it('sin reporte aún: la tarjeta de post-mortem existe y lo dice', async () => {
    ruta(`/api/runs/${ID}`, [corrida('inferring')]);
    renderRun();
    await vaciar();
    const pm = screen.getByRole('region', { name: 'Resultados de la prueba' });
    expect(within(pm).getByText('Los resultados estarán disponibles al finalizar la prueba.')).toBeTruthy();
  });

  it('reporte con bug: veredicto con texto y hallazgo con método, invariante, causa y evidencias copiables', async () => {
    ruta(`/api/runs/${ID}`, [corrida('done', { verdict: 'bug' })]);
    ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    ruta(`/api/reports/${ID}`, [{ json: { ...informeDosHallazgos, run_id: ID } }]);
    renderRun();
    await vaciar();
    const badge = document.querySelector('[data-veredicto="bug"]');
    expect(badge?.textContent).toBe('Fallos encontrados');
    expect(badge?.classList.contains('badge-danger')).toBe(true);
    const pm = within(screen.getByRole('region', { name: 'Resultados de la prueba' }));
    expect(pm.getByText('Dos flujos fallidos')).toBeTruthy();
    expect(pm.getAllByText('POST').length).toBeGreaterThan(0);
    expect(pm.getByText('/orders')).toBeTruthy();
    expect(pm.getByText('inv a')).toBeTruthy();
    expect(pm.getByText('causa a')).toBeTruthy();
    const uris = document.querySelectorAll('[data-testid="run-evidence-copy-button"]');
    expect(uris.length).toBe(3);
  });

  it('sin hallazgos: insignia «Sin hallazgos» y mensaje propio', async () => {
    ruta(`/api/runs/${ID}`, [corrida('done', { verdict: 'sin-hallazgos' })]);
    ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    ruta(`/api/reports/${ID}`, [{ json: { ...informeSinHallazgos, run_id: ID } }]);
    renderRun();
    await vaciar();
    const badge = document.querySelector('[data-veredicto="sin-hallazgos"]');
    expect(badge?.textContent).toBe('Sin hallazgos');
    expect(badge?.classList.contains('badge-ok')).toBe(true);
    expect(screen.getByText('No se encontraron hallazgos en esta corrida.')).toBeTruthy();
  });

  it('inconcluso: insignia «Inconcluso» con aviso de color y texto', async () => {
    ruta(`/api/runs/${ID}`, [corrida('done', { verdict: 'inconcluso' })]);
    ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    ruta(`/api/reports/${ID}`, [{ json: { ...informeInconcluso, run_id: ID } }]);
    renderRun();
    await vaciar();
    const badge = document.querySelector('[data-veredicto="inconcluso"]');
    expect(badge?.textContent).toBe('Resultado inconcluso');
    expect(badge?.classList.contains('badge-warn')).toBe(true);
  });

  it('plan no disponible: texto explícito en la tarjeta del plan', async () => {
    ruta(`/api/runs/${ID}`, [corrida('failed')]);
    ruta(`/api/runs/${ID}/plan`, [{ status: 404, json: { code: 'plan_not_ready', message: 'x' } }]);
    renderRun();
    await vaciar();
    expect(screen.getByText('Plan aún no disponible.')).toBeTruthy();
  });
});

describe('RunPage fidelidad: plan de flujos plegable', () => {
  it('la tarjeta «Plan de flujos» se pliega; cada flujo va cerrado por defecto', async () => {
    ruta(`/api/runs/${ID}`, [corrida('rehearsing')]);
    ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    renderRun();
    await vaciar();
    const plan = screen.getByRole('region', { name: 'Plan de flujos' }).querySelector('details');
    expect(plan).not.toBeNull();
    expect(plan?.tagName).toBe('DETAILS');
    const flujos = plan?.querySelectorAll('details.flow-block');
    expect(flujos?.length).toBe(2);
    flujos?.forEach((f) => expect(f.hasAttribute('open')).toBe(false));
  });

  it('insignia de método por verbo; estado esperado como código, sin insignia (mockup); cuerpo en pre.json-body', async () => {
    ruta(`/api/runs/${ID}`, [corrida('rehearsing')]);
    ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    renderRun();
    await vaciar();
    const metodo = screen.getByText('POST', { selector: 'span.badge' });
    expect(metodo.classList.contains('badge-info')).toBe(true);
    const esperado = screen.getByText('409', { selector: 'code' });
    expect(esperado.closest('.badge')).toBeNull();
    const cuerpo = document.querySelector('pre.json-body');
    expect(cuerpo?.textContent).toBe(JSON.stringify({ product_id: 1, qty: 11 }, null, 2));
  });

  it('ninguna insignia depende solo del color: todas llevan texto (método, esperado, veredicto, estado)', async () => {
    ruta(`/api/runs/${ID}`, [corrida('done', { verdict: 'bug' })]);
    ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    ruta(`/api/reports/${ID}`, [{ json: { ...informeDosHallazgos, run_id: ID } }]);
    renderRun();
    await vaciar();
    const insignias = Array.from(document.querySelectorAll('span.badge'));
    expect(insignias.length).toBeGreaterThan(5);
    for (const b of insignias) {
      expect((b.textContent ?? '').trim().length).toBeGreaterThan(0);
    }
  });
});


describe('Contenido para clientes', () => {
  it('oculta referencias operativas y conserva copiables las evidencias originales', async () => {
    const copiar = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: copiar } });
    ruta(`/api/runs/${ID}`, [corrida('done', { verdict: 'bug' })]);
    ruta(`/api/runs/${ID}/plan`, [{ json: { ...planCon, run_id: ID } }]);
    ruta(`/api/reports/${ID}`, [{ json: { ...informeDosHallazgos, run_id: ID,
      summary: 'agent-reporter guardó el informe en MinIO s3://private-bucket/result.json' } }]);
    renderRun();
    await vaciar();
    expect(document.body.textContent).not.toMatch(/agent-reporter|MinIO|private-bucket|invariant_probe|workflow:|tr-fid|GET \/api/);
    const botones = screen.getAllByTestId('run-evidence-copy-button');
    await act(async () => { botones[0]?.click(); });
    expect(copiar).toHaveBeenCalledWith(informeDosHallazgos.findings[0]?.evidence_uris[0]);
    expect(screen.getByText('Referencia copiada')).toBeTruthy();
    copiar.mockRejectedValueOnce(new Error('denied'));
    await act(async () => { botones[0]?.click(); });
    expect(screen.getByText(/No se pudo copiar la referencia/)).toBeTruthy();
  });
});
