import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { SelectorTema } from './SelectorTema';
import { CLAVE_TEMA, aplicarTema, temaInicial } from './tema';

// Selector de tema (U10-T01 CA-3): conmuta data-theme, persiste en localStorage, respeta
// prefers-color-scheme sin valor guardado y no lanza si el almacenamiento no está disponible.

/** Simula el sistema en modo oscuro (true) o claro (false) mediante matchMedia. */
function sistemaOscuro(oscuro: boolean): void {
  window.matchMedia = ((query: string) => ({
    matches: oscuro && query.includes('dark'),
    media: query,
    onchange: null,
    addListener: () => undefined,
    removeListener: () => undefined,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia;
}

beforeEach(() => {
  window.localStorage.clear();
  delete document.documentElement.dataset.theme;
  sistemaOscuro(false);
});

afterEach(() => {
  vi.restoreAllMocks();
  delete (window as { matchMedia?: unknown }).matchMedia;
});

describe('selector de tema', () => {
  it('conmuta data-theme claro/oscuro y persiste la elección', async () => {
    aplicarTema(temaInicial());
    render(<SelectorTema />);
    const user = userEvent.setup();
    const boton = screen.getByRole('button', { name: 'Cambiar tema visual' });

    await user.click(boton);
    expect(document.documentElement.dataset.theme).toBe('dark');
    expect(window.localStorage.getItem(CLAVE_TEMA)).toBe('dark');

    await user.click(boton);
    expect(document.documentElement.dataset.theme).toBe('light');
    expect(window.localStorage.getItem(CLAVE_TEMA)).toBe('light');
  });

  it('el valor guardado manda sobre prefers-color-scheme al arrancar', () => {
    sistemaOscuro(true);
    window.localStorage.setItem(CLAVE_TEMA, 'light');
    expect(temaInicial()).toBe('light');
  });

  it('sin valor guardado respeta prefers-color-scheme', () => {
    sistemaOscuro(true);
    expect(temaInicial()).toBe('dark');
    sistemaOscuro(false);
    expect(temaInicial()).toBe('light');
  });

  it('sin localStorage (lectura y escritura denegadas) no lanza y el tema se aplica igualmente', async () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('almacenamiento denegado');
    });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('almacenamiento denegado');
    });
    sistemaOscuro(false);
    expect(() => aplicarTema(temaInicial())).not.toThrow();
    expect(temaInicial()).toBe('light');

    render(<SelectorTema />);
    const user = userEvent.setup();
    await expect(user.click(screen.getByRole('button', { name: 'Cambiar tema visual' }))).resolves.not.toThrow();
    expect(document.documentElement.dataset.theme).toBe('dark');
  });
});
