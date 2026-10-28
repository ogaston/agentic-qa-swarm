import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { LandingPage } from './LandingPage';

function renderLanding() {
  return render(
    <MemoryRouter>
      <LandingPage />
    </MemoryRouter>,
  );
}

describe('LandingPage', () => {
  it('muestra el titular del hero y la marca', () => {
    renderLanding();
    expect(
      screen.getByRole('heading', {
        level: 1,
        name: /Your releases, tested by a swarm/,
      }),
    ).toBeTruthy();
    expect(screen.getAllByText('Agentic QA Swarm').length).toBeGreaterThan(0);
  });

  it('las CTA llevan a /login', () => {
    renderLanding();
    const ctas = screen.getAllByRole('link', { name: /Request access/ });
    expect(ctas.length).toBeGreaterThan(0);
    for (const cta of ctas) {
      expect(cta.getAttribute('href')).toBe('/login');
    }
    expect(screen.getByRole('link', { name: 'Sign in' }).getAttribute('href')).toBe('/login');
  });

  it('los .ed-reveal quedan visibles sin IntersectionObserver (jsdom)', () => {
    const { container } = renderLanding();
    const revelados = container.querySelectorAll('.ed-reveal');
    expect(revelados.length).toBeGreaterThan(0);
    revelados.forEach((el) => expect(el.classList.contains('ed-visible')).toBe(true));
  });

  it('las secciones de la landing usan el contenedor .ed-container', () => {
    const { container } = renderLanding();
    const contenedores = container.querySelectorAll('.ed-container');
    expect(contenedores.length).toBeGreaterThanOrEqual(7);
  });
});
