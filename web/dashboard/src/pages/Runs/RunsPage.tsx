import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { apiFetch } from '../../api/client';
import { ApiErrorNotice } from '../../api/errors';
import type { components } from '../../api/schema.gen';

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
    <main>
      <h1>Mis corridas</h1>
      {error !== null && <ApiErrorNotice error={error} />}
      {error === null && recibos === null && <p>Cargando…</p>}
      {recibos !== null && recibos.length === 0 && <p>Aún no tienes corridas confirmadas.</p>}
      {recibos !== null && recibos.length > 0 && (
        <table>
          <thead>
            <tr>
              <th>Corrida</th>
              <th>Notificación</th>
              <th>Confirmada</th>
              <th>Por</th>
            </tr>
          </thead>
          <tbody>
            {recibos.map((r) => (
              <tr key={`${r.run_id}-${r.notification_id}`}>
                <td>
                  <Link to={`/runs/${r.run_id}`}>{r.run_id}</Link>
                </td>
                <td>{r.notification_id}</td>
                <td>{new Date(r.confirmed_at).toLocaleString()}</td>
                <td>{r.confirmed_by}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </main>
  );
}
