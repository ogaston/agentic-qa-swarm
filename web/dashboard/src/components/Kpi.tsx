import type { ReactNode } from 'react';

interface PropsKpi {
  etiqueta: string;
  valor: ReactNode;
  /** Insignia junto al valor (.badge del mockup). Su clase es la variante semántica y el texto va escrito. */
  insignia?: { clase: string; texto: string; punto?: boolean } | undefined;
  /** Porcentaje 0–100 para la barra de progreso inferior (.kpi-bar). Sin valor, no hay barra. */
  barra?: number;
  /** Color de la barra: variable CSS del mockup (p. ej. «var(--info)»). Por defecto, el acento. */
  colorBarra?: string;
  /** Clase extra del valor (p. ej. «kpi-valor-20»: valor de texto a 20 px, como el mockup). */
  claseValor?: string;
}

/** Bloque KPI del mockup: etiqueta, valor grande, insignia y barra opcionales. */
export function Kpi({ etiqueta, valor, insignia, barra, colorBarra, claseValor }: PropsKpi) {
  const porcentaje = barra === undefined ? null : Math.min(100, Math.max(0, barra));
  return (
    <div className="card kpi-box">
      <div className="kpi-label">{etiqueta}</div>
      <div className="kpi-row">
        <span className={claseValor === undefined ? 'kpi-val' : `kpi-val ${claseValor}`}>{valor}</span>
        {insignia !== undefined && (
          <span className={`badge ${insignia.clase}`}>
            {insignia.punto === true && <span className="badge-dot" />}
            {insignia.texto}
          </span>
        )}
      </div>
      {porcentaje !== null && (
        <div className="kpi-bar">
          <div
            className="kpi-bar-fill"
            style={{ width: `${porcentaje}%`, background: colorBarra ?? 'var(--accent)' }}
          />
        </div>
      )}
    </div>
  );
}
