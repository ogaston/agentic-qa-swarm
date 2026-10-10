import { render } from '@testing-library/react';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { AppRoutes } from '../App';
import { SessionProvider } from '../session/SessionContext';

/** Muestra la ruta actual (pathname + search) para que las pruebas la comprueben. */
export function LocationProbe() {
  const location = useLocation();
  return <output data-testid="location">{location.pathname + location.search}</output>;
}

export function currentLocation(): string {
  return document.querySelector('[data-testid="location"]')?.textContent ?? '';
}

/** Monta la app real (rutas + sesión) en memoria, en la entrada indicada. */
export function renderApp(initialEntry: string) {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <SessionProvider>
        <AppRoutes />
        <LocationProbe />
      </SessionProvider>
    </MemoryRouter>,
  );
}
