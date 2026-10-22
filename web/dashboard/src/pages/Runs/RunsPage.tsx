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
        descripcion={
          <>
            Recibos de confirmación auditables (<code>GET /api/confirmations?limit=20</code>), ordenados del más
            reciente al más antiguo.
          </>
        }
      />
      {error !== null && <ApiErrorNotice error={error} />}
      {error === null && recibos === null && <p>Cargando…</p>}
      {recibos !== null && (
        <div className="card">
          {recibos.length === 0 ? (
            <p className="empty">Aún no tienes corridas confirmadas.</p>
          ) : (
            <div className="table-wrap">
              <table className="data-table">
                <thead>
                  <tr>
                    <th scope="col">
                      Corrida (<code>run_id</code>)
                    </th>
                    <th scope="col">
                      Notificación (<code>notification_id</code>)
                    </th>
                    <th scope="col">Confirmada (hora local)</th>
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
                        <Link className="mono" to={`/runs/${r.run_id}`}>
                          {r.run_id}
                        </Link>
                      </td>
                      <td data-label="Notificación" className="mono">
                        {r.notification_id}
                      </td>
                      <td data-label="Confirmada">{new Date(r.confirmed_at).toLocaleString()}</td>
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
