import { BrowserRouter, Route, Routes } from 'react-router-dom';
import { InboxPage } from './pages/Inbox/InboxPage';
import { LoginPage } from './pages/Login/LoginPage';
import { RunPage } from './pages/Run/RunPage';
import { RunsPage } from './pages/Runs/RunsPage';
import { WarmPage } from './pages/Warm/WarmPage';
import { ProtectedLayout } from './session/ProtectedLayout';
import { SessionProvider } from './session/SessionContext';

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
        <Route path="/runs" element={<RunsPage />} />
        <Route path="/runs/:id" element={<RunPage />} />
        <Route path="/warm" element={<WarmPage />} />
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
