import { BrowserRouter, Route, Routes } from 'react-router-dom';
import { InboxPage } from './pages/Inbox/InboxPage';
import { LoginPage } from './pages/Login/LoginPage';
import { ProtectedLayout } from './session/ProtectedLayout';
import { SessionProvider } from './session/SessionContext';

// Rutas vacías: las pantallas reales llegan en U6-T04 y U6-T05.
function Placeholder({ name }: { name: string }) {
  return <main><h1>{name}</h1></main>;
}

export function NotFound() {
  return <main><h1>No encontrado</h1></main>;
}

/** Rutas de la app. Sin BrowserRouter ni proveedor: las pruebas las montan en memoria. */
export function AppRoutes() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route element={<ProtectedLayout />}>
        <Route path="/inbox" element={<InboxPage />} />
        <Route path="/runs/:id" element={<Placeholder name="Corrida" />} />
        <Route path="/warm" element={<Placeholder name="Warm" />} />
      </Route>
      <Route path="*" element={<NotFound />} />
    </Routes>
  );
}

export function App() {
  return (
    <BrowserRouter>
      <SessionProvider>
        <AppRoutes />
      </SessionProvider>
    </BrowserRouter>
  );
}
