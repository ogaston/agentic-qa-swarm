import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useReveal } from '../../hooks/useReveal';

const PASOS = [
  {
    num: '01',
    titulo: 'Notify',
    texto: 'A commit, PR, or tag on GitHub lands in your inbox as a QA run proposal.',
    meta: 'event: push → main',
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
    titulo: 'Confirm',
    texto: 'A human approves the run. Nothing executes against any environment without it.',
    meta: 'gate: human approval',
    icon: (
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
        <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
        <path d="m9 12 2 2 4-4" />
      </svg>
    ),
  },
  {
    num: '03',
    titulo: 'Rehearse & run',
    texto: 'The swarm infers the external surface, generates flows, rehearses them, and executes against a warm, isolated environment.',
    meta: 'swarm: warm-07 sandbox',
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
    titulo: 'Reset & report',
    texto: 'The environment is reset and the reset is verified. You get a business-logic post-mortem, not a stack trace.',
    meta: 'parity: 0 staging drift',
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
    texto: 'Verified environment reset after every run — your warm env never drifts.',
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
    texto: 'Flows that pass rehearsal on the first try, before they ever execute.',
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
    texto: 'Autonomy-limit incidents tolerated. The agent proposes; humans decide.',
    icon: (
      <svg className="ed-stat-svg" width="44" height="44" viewBox="0 0 44 44" fill="none" aria-hidden="true">
        <path d="M22 6l13 4.5v11c0 8.5-5.5 15.5-13 17.5-7.5-2-13-9-13-17.5v-11L22 6z" stroke="var(--ed-violet)" strokeWidth="1.8" />
        <circle cx="22" cy="20" r="3" fill="var(--ed-violet)" />
        <path d="M22 23v5" stroke="var(--ed-violet)" strokeWidth="2" strokeLinecap="round" />
      </svg>
    ),
  },
] as const;

/* Tira gráfica abstracta: texturas cálidas y trazos finos, sin fotografía de stock. */
const MOSAICO = [
  <svg key="rings" className="ed-tile" viewBox="0 0 120 120">
    <rect width="120" height="120" fill="#f1eee9" />
    <circle cx="60" cy="60" r="36" fill="none" stroke="#b8b8b8" strokeWidth="1" />
    <circle cx="60" cy="60" r="23" fill="none" stroke="#585858" strokeWidth="1" />
    <circle cx="60" cy="60" r="10" fill="none" stroke="#111111" strokeWidth="1" />
  </svg>,
  <svg key="plus" className="ed-tile" viewBox="0 0 120 120">
    <rect width="120" height="120" fill="#faf9f6" />
    <path d="M60 44v32M44 60h32" stroke="#0007cb" strokeWidth="2" />
  </svg>,
  <svg key="dots" className="ed-tile" viewBox="0 0 120 120">
    <rect width="120" height="120" fill="#ffffff" />
    <pattern id="ed-dots" width="20" height="20" patternUnits="userSpaceOnUse">
      <circle cx="4" cy="4" r="1.5" fill="#b8b8b8" />
    </pattern>
    <rect width="120" height="120" fill="url(#ed-dots)" />
  </svg>,
  <svg key="diag" className="ed-tile" viewBox="0 0 120 120">
    <rect width="120" height="120" fill="#d3cec6" />
    <pattern id="ed-diag" width="12" height="12" patternUnits="userSpaceOnUse" patternTransform="rotate(45)">
      <line x1="0" y1="0" x2="0" y2="12" stroke="#b5afa5" strokeWidth="1" />
    </pattern>
    <rect width="120" height="120" fill="url(#ed-diag)" />
  </svg>,
  <svg key="arc" className="ed-tile" viewBox="0 0 120 120">
    <rect width="120" height="120" fill="#f1eee9" />
    <circle cx="60" cy="120" r="56" fill="none" stroke="#111111" strokeWidth="1" />
    <circle cx="60" cy="120" r="36" fill="none" stroke="#888888" strokeWidth="1" />
    <circle cx="60" cy="120" r="4" fill="#0007cb" />
  </svg>,
  <svg key="lines" className="ed-tile" viewBox="0 0 120 120">
    <rect width="120" height="120" fill="#faf9f6" />
    <rect x="24" y="34" width="72" height="2" fill="#dedbd6" />
    <rect x="24" y="46" width="56" height="2" fill="#dedbd6" />
    <rect x="24" y="58" width="64" height="2" fill="#dedbd6" />
    <rect x="24" y="70" width="20" height="2" fill="#0007cb" />
    <rect x="48" y="70" width="40" height="2" fill="#dedbd6" />
    <rect x="24" y="82" width="48" height="2" fill="#dedbd6" />
  </svg>,
];

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
            <a href="#how-it-works">How it works</a>
            <a href="#trust">Trust</a>
          </nav>
          <div className="ed-nav-actions">
            <Link className="ed-btn-ghost" to="/login">
              Sign in
            </Link>
            <Link className="ed-btn" to="/login">
              Request access <span className="ed-arrow" aria-hidden="true">→</span>
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
                  Black-box QA, agent-run
                </span>
                <h1 className="ed-display ed-rise ed-rise-2">
                  Your releases, tested by a swarm. Your staging, never touched.
                </h1>
              </div>
              <div className="ed-hero-aside ed-rise ed-rise-3">
                <p className="ed-serif">
                  Agentic QA Swarm turns a GitHub event into a full black-box QA run against a warm
                  environment we own — never yours. Agents propose, rehearse, and execute
                  transactional flows; a human confirms every run; the environment is verifiably
                  reset afterwards.
                </p>
                <div className="ed-hero-ctas">
                  <Link className="ed-btn" to="/login">
                    Request access <span className="ed-arrow" aria-hidden="true">→</span>
                  </Link>
                  <a className="ed-btn-ghost" href="#how-it-works">
                    See how it works
                  </a>
                </div>
              </div>
            </div>

            {/* Deck interactivo con estética editorial de Intercom */}
            <div className="ed-bento-wrap ed-rise ed-rise-3" aria-label="Live rehearsal and confirmation deck">
              <span className="ed-cross ed-cross-tl" aria-hidden="true" />
              <span className="ed-cross ed-cross-tr" aria-hidden="true" />
              <span className="ed-cross ed-cross-bl" aria-hidden="true" />
              <span className="ed-cross ed-cross-br" aria-hidden="true" />

              <div className="ed-bento-deck">
                <div className="ed-bento-bar">
                  <div className="ed-bento-tabs" role="tablist" aria-label="Perspective switcher">
                    <button
                      type="button"
                      role="tab"
                      aria-selected={activeHeroTab === 'swarm'}
                      className={`ed-bento-tab ${activeHeroTab === 'swarm' ? 'is-active' : ''}`}
                      onClick={() => setActiveHeroTab('swarm')}
                    >
                      <span className="ed-dot" aria-hidden="true" />
                      01 Swarm Rehearsal Engine
                    </button>
                    <button
                      type="button"
                      role="tab"
                      aria-selected={activeHeroTab === 'gate'}
                      className={`ed-bento-tab ${activeHeroTab === 'gate' ? 'is-active' : ''}`}
                      onClick={() => setActiveHeroTab('gate')}
                    >
                      <span className="ed-dot" aria-hidden="true" />
                      02 Human Gate & Reset
                    </button>
                  </div>
                </div>

                {activeHeroTab === 'swarm' ? (
                  <div className="ed-bento-body">
                    <div className="ed-flow-card-list">
                      <div className="ed-flow-card">
                        <div>
                          <div className="ed-flow-name">POST /api/orders/checkout</div>
                          <span className="ed-flow-sub">Idempotent purchase flow · commit a1b2c3d</span>
                        </div>
                        <span className="ed-badge-ok">201 OK · 14ms</span>
                      </div>
                      <div className="ed-flow-card">
                        <div>
                          <div className="ed-flow-name">GET /api/inventory/invariants</div>
                          <span className="ed-flow-sub">Concurrent reservation check</span>
                        </div>
                        <span className="ed-badge-ok">PASS · 0 DRIFT</span>
                      </div>
                      <div className="ed-flow-card">
                        <div>
                          <div className="ed-flow-name">DELETE /api/orders/temp_cart</div>
                          <span className="ed-flow-sub">Isolation boundary validation</span>
                        </div>
                        <span className="ed-badge-violet">REHEARSED</span>
                      </div>
                    </div>

                    <div className="ed-terminal">
                      <div>
                        <div className="ed-term-head">
                          <span className="ed-term-dot" />
                          <span className="ed-term-dot" />
                          <span className="ed-term-dot" />
                          <span style={{ marginLeft: 'auto' }}>rehearsal-engine v1.4.2</span>
                        </div>
                        <div className="ed-term-lines">
                          <div className="ed-term-line">
                            <span className="ed-term-prompt">&gt;</span>
                            <span>[swarm] Inferred surface: 14 endpoints</span>
                          </div>
                          <div className="ed-term-line">
                            <span className="ed-term-prompt">&gt;</span>
                            <span>[swarm] Generated 6 transactional flows</span>
                          </div>
                          <div className="ed-term-line">
                            <span className="ed-term-prompt">&gt;</span>
                            <span>[swarm] 5/5 passing against warm-07 sandbox</span>
                          </div>
                          <div className="ed-term-line">
                            <span className="ed-term-prompt">&gt;</span>
                            <span className="ed-term-accent">0 autonomy incidents. Human gate ready.</span>
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
                          <dt>Target Repository</dt>
                          <dd>acme-corp/storefront @ commit a1b2c3d</dd>
                        </div>
                        <div className="ed-gate-item">
                          <dt>Proposed Runs</dt>
                          <dd>6 flows proposed · 5 rehearsed · 0 regressions</dd>
                        </div>
                        <div className="ed-gate-item">
                          <dt>Autonomous Policy</dt>
                          <dd>Human confirmation required · 0 auto-runs</dd>
                        </div>
                      </div>
                      <div>
                        <Link to="/login" className="ed-btn ed-btn-compact">
                          Review in Inbox <span className="ed-arrow" aria-hidden="true">→</span>
                        </Link>
                      </div>
                    </div>

                    <div className="ed-stamp-wrap">
                      <div className="ed-stamp">Reset Verified</div>
                      <p style={{ margin: 0, fontSize: '13px', color: 'var(--ed-graphite)', lineHeight: 1.4 }}>
                        Warm sandbox returned to baseline version in 41s. 0 residual rows, 0 leaked tokens, 0 schema mutations.
                      </p>
                    </div>
                  </div>
                )}
              </div>
            </div>
          </div>
        </section>

        <section className="ed-strip" aria-hidden="true">
          <div className="ed-container">
            <div className="ed-strip-track">
              {MOSAICO.map((tile, i) => (
                <span key={tile.key} className={`ed-reveal ed-reveal-delay-${i % 6}`}>
                  {tile}
                </span>
              ))}
            </div>
          </div>
        </section>

        <section className="ed-section ed-section-warm" id="how-it-works">
          <div className="ed-container">
            <div className="ed-section-head ed-reveal">
              <div>
                <span className="ed-tag">How it works</span>
                <h2 className="ed-heading">From commit to post-mortem, with a human in the loop</h2>
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
                <span className="ed-tag">Trust by construction</span>
                <h2 className="ed-heading">Numbers we hold ourselves to</h2>
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
            <span className="ed-tag">Get started</span>
            <h2 className="ed-heading">Ship without the QA backlog</h2>
            <Link className="ed-btn" to="/login">
              Request access <span className="ed-arrow" aria-hidden="true">→</span>
            </Link>
          </div>
        </section>
      </main>

      <div className="ed-container">
        <footer className="ed-footer">
          <small>Agentic QA Swarm</small>
          <small>Agent, not autonomous</small>
        </footer>
      </div>
    </div>
  );
}
