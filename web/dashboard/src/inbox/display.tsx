import { useEffect, useId, useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';

/** Texto relativo sin dependencias, en el idioma de la interfaz. */
export function relativeTime(value: string | undefined, now: number): string {
  const timestamp = value === undefined ? NaN : Date.parse(value);
  if (!Number.isFinite(timestamp)) return '—';
  const seconds = Math.max(0, Math.floor((now - timestamp) / 1000));
  if (seconds < 60) return 'hace unos segundos';
  const units = [[86400, 'day'], [3600, 'hour'], [60, 'minute']] as const;
  const [size, unit] = units.find(([size]) => seconds >= size) ?? units[2];
  return new Intl.RelativeTimeFormat('es', { numeric: 'always' }).format(-Math.floor(seconds / size), unit);
}

export function ArtifactLabel({ artifact }: { artifact: { kind: string; ref: string } }) {
  const kind = artifact.kind === 'published-image' ? 'image' : artifact.kind === 'build-from-repo' ? 'build_from_repo' : artifact.kind;
  const id = useId();
  const trigger = useRef<HTMLSpanElement>(null);
  const tooltip = useRef<HTMLSpanElement>(null);
  const closeTimer = useRef<ReturnType<typeof setTimeout>>();
  const [open, setOpen] = useState(false);
  const [position, setPosition] = useState({ left: 0, top: 0 });
  const show = () => { clearTimeout(closeTimer.current); setOpen(true); };
  const hide = () => { closeTimer.current = setTimeout(() => setOpen(false), 150); };
  useEffect(() => () => clearTimeout(closeTimer.current), []);
  useLayoutEffect(() => {
    if (!open) return;
    const place = () => {
      if (!trigger.current || !tooltip.current) return;
      const anchor = trigger.current.getBoundingClientRect();
      const bubble = tooltip.current.getBoundingClientRect();
      const left = Math.max(8, Math.min(anchor.left, window.innerWidth - bubble.width - 8));
      const top = anchor.top >= bubble.height + 16 ? anchor.top - bubble.height - 8 : Math.max(8, Math.min(anchor.bottom + 8, window.innerHeight - bubble.height - 8));
      setPosition({ left, top });
    };
    place();
    window.addEventListener('resize', place);
    window.addEventListener('scroll', place, true);
    return () => {
      window.removeEventListener('resize', place);
      window.removeEventListener('scroll', place, true);
    };
  }, [open]);
  return (
    <>
      <span ref={trigger} className="artifact-tooltip mono" tabIndex={0} data-testid="inbox-artifact-tooltip"
        aria-describedby={open ? id : undefined} onMouseEnter={show} onMouseLeave={hide}
        onFocus={show} onBlur={() => setOpen(false)} onKeyDown={(event) => { if (event.key === 'Escape') setOpen(false); }}>
        {kind}: {artifact.ref.slice(-7)}
      </span>
      {open ? createPortal(<span ref={tooltip} id={id} className="artifact-tooltip-content mono" role="tooltip"
        style={position} onMouseEnter={show} onMouseLeave={hide}>{kind}: {artifact.ref}</span>, document.body) : null}
    </>
  );
}

export function GithubLink({ repo, sha, children }: { repo: string; sha?: string; children: ReactNode }) {
  // Solo nombres owner/repo: el origen nunca viene de texto controlado por el evento.
  if (!/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(repo) || (sha !== undefined && !/^[0-9a-f]+$/i.test(sha))) return <>{children}</>;
  const href = `https://github.com/${repo.split('/').map(encodeURIComponent).join('/')}${sha === undefined ? '' : `/commit/${encodeURIComponent(sha)}`}`;
  return <a href={href} target="_blank" rel="noopener noreferrer" className="notification-external-link" data-testid={sha === undefined ? 'inbox-repository-link' : 'inbox-commit-link'}>
    {children}<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true"><path d="M4 12 12 4M4 4h8v8" /></svg><span className="sr-only"> (abre en una pestaña nueva)</span>
  </a>;
}
