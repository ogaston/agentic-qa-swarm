import type { ReactNode } from 'react';

interface PropsKpi {
  etiqueta: string;
  valor: ReactNode;
  /** Porcentaje 0–100 para la barra de progreso inferior (.kpi-bar). Sin valor, no hay barra. */
  barra?: number;
}

/** Bloque KPI del mockup: etiqueta, valor grande y barra opcional. */
export function Kpi({ etiqueta, valor, barra }: PropsKpi) {
  const porcentaje = barra === undefined ? null : Math.min(100, Math.max(0, barra));
  return (
    <div className="card kpi-box">
      <div className="kpi-label">{etiqueta}</div>
      <div className="kpi-row">
        <span className="kpi-val">{valor}</span>
      </div>
      {porcentaje !== null && (
        <div className="kpi-bar">
          <div className="kpi-bar-fill" style={{ width: `${porcentaje}%` }} />
        </div>
      )}
    </div>
  );
}
