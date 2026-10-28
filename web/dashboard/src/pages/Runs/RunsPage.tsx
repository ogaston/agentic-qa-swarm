import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { apiFetch } from '../../api/client';
import { ApiErrorNotice } from '../../api/errors';
import type { components } from '../../api/schema.gen';
import { PageHeader } from '../../components/PageHeader';

type Recibo = components['schemas']['ConfirmationReceipt'];

/**
 * «Mis corridas» (`/runs`): recibos de confirmación, más reciente primero. Carga una
 * vez, sin sondeo. Cada corrida enlaza a su vista `/runs/:id`.
 */
export function RunsPage() {
  const [recibos, setRecibos] = useState<Recibo[] | null>(null);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    let vivo = true;
    apiFetch<Recibo[]>('/api/confirmations?limit=20')
      .then((lista) => {
        if (vivo) setRecibos(lista);
      })
      .catch((e: unknown) => {
        if (vivo) setError(e);
      });
    return () => {
      vivo = false;
    };
  }, []);

  return (
    <main className="az-container">
      <PageHeader
        titulo="Mis corridas"
        descripcion="Consulta las pruebas que has autorizado y sigue sus resultados. Las más recientes aparecen primero."
      />
      {error !== null && <ApiErrorNotice error={error} />}
      {error === null && recibos === null && <p>Cargando…</p>}
      {recibos !== null && (
        <div className="card">
          {recibos.length === 0 ? (
            <section className="dashboard-empty" aria-labelledby="runs-empty-title">
              <div className="dashboard-empty-art" aria-hidden="true">
                <svg viewBox="0 0 64 64" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                  <rect x="13" y="9" width="38" height="46" rx="5" />
                  <path d="M23 9v6h18V9M22 27h20M22 34h13M22 41h8" />
                  <circle cx="48" cy="46" r="12" /><path d="m44 46 3 3 6-7" />
                </svg>
              </div>
              <h2 id="runs-empty-title">Tu primera corrida empieza en el Inbox</h2>
              <p>Aún no tienes corridas confirmadas. Revisa una notificación y confirma las pruebas que quieres ejecutar. Aquí podrás seguir sus resultados.</p>
              <Link to="/inbox" className="btn btn-primary" data-testid="runs-empty-inbox-link">Ir al Inbox <span aria-hidden="true">→</span></Link>
            </section>
          ) : (
            <div className="table-wrap">
              <table className="data-table">
                <thead>
                  <tr>
                    <th scope="col">
                      Corrida
                    </th>
                    <th scope="col">
                      Notificación
                    </th>
                    <th scope="col">Confirmada (hora en UTC)</th>
                    <th scope="col">Confirmada por</th>
                    <th scope="col">
                      <span className="sr-only">Acción</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {recibos.map((r) => (
                    <tr key={`${r.run_id}-${r.notification_id}`}>
                      <td data-label="Corrida">
                        <Link className="mono enlace-corrida" to={`/runs/${r.run_id}`}>
                          {r.run_id}
                        </Link>
                      </td>
                      <td data-label="Notificación" className="mono">
                        {r.notification_id}
                      </td>
                      <td data-label="Confirmada">{new Date(r.confirmed_at).toLocaleString('es-ES', { hour12: false, timeZone: 'UTC' })}</td>
                      <td data-label="Confirmada por">
                        <code>{r.confirmed_by}</code>
                      </td>
                      <td data-label="Acción">
                        <Link className="btn btn-sm" to={`/runs/${r.run_id}`}>
                          Ver corrida →
                        </Link>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
    </main>
  );
}
