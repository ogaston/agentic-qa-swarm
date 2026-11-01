import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useReveal } from '../../hooks/useReveal';

const PASOS = [
  {
    num: '01',
    titulo: 'Notificar',
    texto: 'Un commit, PR o tag en GitHub llega a tu bandeja de entrada como propuesta de corrida de QA.',
    meta: 'evento: push → main',
    icon: (
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
        <circle cx="12" cy="18" r="3" />
        <circle cx="6" cy="6" r="3" />
        <circle cx="18" cy="6" r="3" />
        <path d="M18 9v2a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2V9" />
        <path d="M12 13v2" />
      </svg>
    ),
  },
  {
    num: '02',
    titulo: 'Confirmar',
    texto: 'Un humano aprueba la corrida. Nada se ejecuta contra ningún entorno sin su confirmación.',
    meta: 'control: aprobación humana',
    icon: (
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
        <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
        <path d="m9 12 2 2 4-4" />
      </svg>
    ),
  },
  {
    num: '03',
    titulo: 'Ensayar y ejecutar',
    texto: 'El enjambre infiere la superficie externa, genera flujos, los ensaya y los ejecuta contra un entorno warm aislado.',
    meta: 'enjambre: sandbox warm-07',
    icon: (
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
        <rect x="2" y="3" width="7" height="7" rx="1" />
        <rect x="15" y="3" width="7" height="7" rx="1" />
        <rect x="8.5" y="14" width="7" height="7" rx="1" />
        <path d="M5.5 10v2a2 2 0 0 0 2 2h1" />
        <path d="M18.5 10v2a2 2 0 0 1-2 2h-1" />
      </svg>
    ),
  },
  {
    num: '04',
    titulo: 'Reiniciar y reportar',
    texto: 'El entorno se reinicia y el reinicio se verifica. Obtienes un post-mortem de lógica de negocio, no un stack trace.',
    meta: 'paridad: 0 desvío de staging',
    icon: (
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
        <path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8" />
        <path d="M3 3v5h5" />
        <path d="m9 12 2 2 4-4" />
      </svg>
    ),
  },
] as const;

const METRICAS = [
  {
    valor: '100%',
    texto: 'Reinicio de entorno verificado tras cada corrida: tu entorno warm nunca sufre desvíos.',
    icon: (
      <svg className="ed-stat-svg" width="44" height="44" viewBox="0 0 44 44" fill="none" aria-hidden="true">
        <circle cx="22" cy="22" r="18" stroke="var(--ed-hairline)" strokeWidth="2" />
        <circle
          cx="22"
          cy="22"
          r="18"
          stroke="var(--ed-violet)"
          strokeWidth="2.5"
          strokeDasharray="113"
          strokeDashoffset="0"
          strokeLinecap="round"
          transform="rotate(-90 22 22)"
          className="ed-circle-fill"
        />
        <path d="M14 22l5 5 11-11" stroke="var(--ed-violet)" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
    ),
  },
  {
    valor: '>70%',
    texto: 'Flujos que superan el ensayo al primer intento, antes de ejecutarse.',
    icon: (
      <svg className="ed-stat-svg" width="48" height="44" viewBox="0 0 48 44" fill="none" aria-hidden="true">
        <rect x="4" y="24" width="5" height="16" rx="1" fill="var(--ed-stone)" />
        <rect x="13" y="18" width="5" height="22" rx="1" fill="var(--ed-stone)" />
        <rect x="22" y="12" width="5" height="28" rx="1" fill="var(--ed-violet)" />
        <rect x="31" y="8" width="5" height="32" rx="1" fill="var(--ed-violet)" />
        <rect x="40" y="6" width="5" height="34" rx="1" fill="var(--ed-violet)" />
        <line x1="0" y1="40" x2="48" y2="40" stroke="var(--ed-hairline)" strokeWidth="1" />
      </svg>
    ),
  },
  {
    valor: '0',
    texto: 'Incidentes de límite de autonomía tolerados. El agente propone; los humanos deciden.',
    icon: (
      <svg className="ed-stat-svg" width="44" height="44" viewBox="0 0 44 44" fill="none" aria-hidden="true">
        <path d="M22 6l13 4.5v11c0 8.5-5.5 15.5-13 17.5-7.5-2-13-9-13-17.5v-11L22 6z" stroke="var(--ed-violet)" strokeWidth="1.8" />
        <circle cx="22" cy="20" r="3" fill="var(--ed-violet)" />
        <path d="M22 23v5" stroke="var(--ed-violet)" strokeWidth="2" strokeLinecap="round" />
      </svg>
    ),
  },
] as const;

/**
 * Landing pública (`/`): inspirada en el sistema editorial cálido («Intercom — warm cream editorial»).
 * Sin sesión ni cabecera de la app; las CTA llevan a `/login`.
 */
export function LandingPage() {
  const rootRef = useReveal<HTMLDivElement>();
  const [activeHeroTab, setActiveHeroTab] = useState<'swarm' | 'gate'>('swarm');

  return (
    <div className="ed-root" ref={rootRef}>
      <div className="ed-container">
        <header className="ed-nav">
          <div className="az-brand">
            <span className="az-logo">aqs</span>
            <span className="az-brand-sub">Agentic QA Swarm</span>
          </div>
          <nav className="ed-nav-links" aria-label="Principal">
            <a href="#how-it-works">Cómo funciona</a>
            <a href="#trust">Confianza</a>
          </nav>
          <div className="ed-nav-actions">
            <Link className="ed-btn-ghost" to="/login">
              Iniciar sesión
            </Link>
            <Link className="ed-btn" to="/login">
              Solicitar acceso <span className="ed-arrow" aria-hidden="true">→</span>
            </Link>
          </div>
        </header>
      </div>

      <main>
        <section className="ed-hero">
          <div className="ed-container">
            <div className="ed-hero-grid">
              <div>
                <span className="ed-tag ed-rise ed-rise-1">
                  <span className="ed-dot ed-beacon" aria-hidden="true" />
                  QA de caja negra, operado por agentes
                </span>
                <h1 className="ed-display ed-rise ed-rise-2">
                  Tus releases, probados por un enjambre. Tu staging, intacto.
                </h1>
              </div>
              <div className="ed-hero-aside ed-rise ed-rise-3">
                <p className="ed-serif">
                  Agentic QA Swarm convierte un evento de GitHub en una corrida completa de QA de caja negra contra un
                  entorno warm propio — nunca el tuyo. Los agentes proponen, ensayan y ejecutan
                  flujos transaccionales; un humano confirma cada corrida; el entorno se reinicia
                  de forma verificada al terminar.
                </p>
                <div className="ed-hero-ctas">
                  <Link className="ed-btn" to="/login">
                    Solicitar acceso <span className="ed-arrow" aria-hidden="true">→</span>
                  </Link>
                  <a className="ed-btn-ghost" href="#how-it-works">
                    Ver cómo funciona
                  </a>
                </div>
              </div>
            </div>

            {/* Deck interactivo con estética editorial de Intercom */}
            <div className="ed-bento-wrap ed-rise ed-rise-3" aria-label="Panel de ensayo y confirmación en vivo">
              <span className="ed-cross ed-cross-tl" aria-hidden="true" />
              <span className="ed-cross ed-cross-tr" aria-hidden="true" />
              <span className="ed-cross ed-cross-bl" aria-hidden="true" />
              <span className="ed-cross ed-cross-br" aria-hidden="true" />

              <div className="ed-bento-deck">
                <div className="ed-bento-bar">
                  <div className="ed-bento-tabs" role="tablist" aria-label="Selector de perspectiva">
                    <button
                      type="button"
                      role="tab"
                      aria-selected={activeHeroTab === 'swarm'}
                      className={`ed-bento-tab ${activeHeroTab === 'swarm' ? 'is-active' : ''}`}
                      onClick={() => setActiveHeroTab('swarm')}
                    >
                      <span className="ed-dot" aria-hidden="true" />
                      01 Motor de ensayo del enjambre
                    </button>
                    <button
                      type="button"
                      role="tab"
                      aria-selected={activeHeroTab === 'gate'}
                      className={`ed-bento-tab ${activeHeroTab === 'gate' ? 'is-active' : ''}`}
                      onClick={() => setActiveHeroTab('gate')}
                    >
                      <span className="ed-dot" aria-hidden="true" />
                      02 Control humano y reinicio
                    </button>
                  </div>
                </div>

                {activeHeroTab === 'swarm' ? (
                  <div className="ed-bento-body">
                    <div className="ed-flow-card-list">
                      <div className="ed-flow-card">
                        <div>
                          <div className="ed-flow-name">POST /api/orders/checkout</div>
                          <span className="ed-flow-sub">Flujo de compra idempotente · commit a1b2c3d</span>
                        </div>
                        <span className="ed-badge-ok">201 OK · 14ms</span>
                      </div>
                      <div className="ed-flow-card">
                        <div>
                          <div className="ed-flow-name">GET /api/inventory/invariants</div>
                          <span className="ed-flow-sub">Verificación de invariantes concurrentes</span>
                        </div>
                        <span className="ed-badge-ok">CORRECTO · 0 DESVÍO</span>
                      </div>
                      <div className="ed-flow-card">
                        <div>
                          <div className="ed-flow-name">DELETE /api/orders/temp_cart</div>
                          <span className="ed-flow-sub">Validación del límite de aislamiento</span>
                        </div>
                        <span className="ed-badge-violet">ENSAYADO</span>
                      </div>
                    </div>

                    <div className="ed-terminal">
                      <div>
                        <div className="ed-term-head">
                          <span className="ed-term-dot" />
                          <span className="ed-term-dot" />
                          <span className="ed-term-dot" />
                          <span style={{ marginLeft: 'auto' }}>motor-ensayo v1.4.2</span>
                        </div>
                        <div className="ed-term-lines">
                          <div className="ed-term-line">
                            <span className="ed-term-prompt">&gt;</span>
                            <span>[swarm] Superficie inferida: 14 endpoints</span>
                          </div>
                          <div className="ed-term-line">
                            <span className="ed-term-prompt">&gt;</span>
                            <span>[swarm] 6 flujos transaccionales generados</span>
                          </div>
                          <div className="ed-term-line">
                            <span className="ed-term-prompt">&gt;</span>
                            <span>[swarm] 5/5 superados en sandbox warm-07</span>
                          </div>
                          <div className="ed-term-line">
                            <span className="ed-term-prompt">&gt;</span>
                            <span className="ed-term-accent">0 incidentes de autonomía. Control humano listo.</span>
                            <span className="ed-cursor" aria-hidden="true" />
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>
                ) : (
                  <div className="ed-bento-body">
                    <div className="ed-gate-preview">
                      <div className="ed-gate-receipt">
                        <div className="ed-gate-item">
                          <dt>Repositorio objetivo</dt>
                          <dd>acme-corp/storefront @ commit a1b2c3d</dd>
                        </div>
                        <div className="ed-gate-item">
                          <dt>Corridas propuestas</dt>
                          <dd>6 flujos propuestos · 5 ensayados · 0 regresiones</dd>
                        </div>
                        <div className="ed-gate-item">
                          <dt>Política de autonomía</dt>
                          <dd>Aprobación humana requerida · 0 ejecuciones automáticas</dd>
                        </div>
                      </div>
                      <div>
                        <Link to="/login" className="ed-btn ed-btn-compact">
                          Revisar en bandeja <span className="ed-arrow" aria-hidden="true">→</span>
                        </Link>
                      </div>
                    </div>

                    <div className="ed-stamp-wrap">
                      <div className="ed-stamp">Reinicio verificado</div>
                      <p style={{ margin: 0, fontSize: '13px', color: 'var(--ed-graphite)', lineHeight: 1.4 }}>
                        Sandbox warm restablecido a la versión base en 41s. 0 filas residuales, 0 tokens filtrados, 0 mutaciones de esquema.
                      </p>
                    </div>
                  </div>
                )}
              </div>
            </div>
          </div>
        </section>

        <section className="ed-section ed-section-warm" id="how-it-works">
          <div className="ed-container">
            <div className="ed-section-head ed-reveal">
              <div>
                <span className="ed-tag">Cómo funciona</span>
                <h2 className="ed-heading">Del commit al post-mortem, con intervención humana</h2>
              </div>
            </div>
            <div className="ed-steps">
              {PASOS.map((paso, i) => (
                <article
                  key={paso.num}
                  className={`ed-step ed-reveal ed-reveal-delay-${i}`}
                >
                  <span className="ed-step-progress-bar" aria-hidden="true" />
                  <div className="ed-step-top">
                    <span className="ed-step-num">{paso.num}</span>
                    <div className="ed-step-icon">{paso.icon}</div>
                  </div>
                  <h3>{paso.titulo}</h3>
                  <p>{paso.texto}</p>
                  <span className="ed-step-meta">{paso.meta}</span>
                </article>
              ))}
            </div>
          </div>
        </section>

        <section className="ed-section" id="trust">
          <div className="ed-container">
            <div className="ed-section-head ed-reveal">
              <div>
                <span className="ed-tag">Confianza por construcción</span>
                <h2 className="ed-heading">Métricas con las que nos comprometemos</h2>
              </div>
            </div>
            <div className="ed-stats">
              {METRICAS.map((m, i) => (
                <div key={m.valor} className={`ed-stat-card ed-reveal ed-reveal-delay-${i}`}>
                  <div className="ed-stat-header">
                    <div className="ed-stat-value">{m.valor}</div>
                    {m.icon}
                  </div>
                  <p className="ed-stat-label">{m.texto}</p>
                </div>
              ))}
            </div>
          </div>
        </section>

        <section className="ed-section ed-cta">
          <div className="ed-container ed-reveal">
            <span className="ed-tag">Comenzar</span>
            <h2 className="ed-heading">Despliega sin acumulación de QA</h2>
            <Link className="ed-btn" to="/login">
              Solicitar acceso <span className="ed-arrow" aria-hidden="true">→</span>
            </Link>
          </div>
        </section>
      </main>

      <div className="ed-container">
        <footer className="ed-footer">
          <small>Agentic QA Swarm</small>
          <small>Agente, no autónomo</small>
        </footer>
      </div>
    </div>
  );
}
