import { BrowserRouter, Route, Routes } from 'react-router-dom';

// Rutas vacías: las pantallas reales llegan en U6-T03…T05.
function Placeholder({ name }: { name: string }) {
  return <main><h1>{name}</h1></main>;
}

export function NotFound() {
  return <main><h1>No encontrado</h1></main>;
}

export function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<Placeholder name="Login" />} />
        <Route path="/inbox" element={<Placeholder name="Inbox" />} />
        <Route path="/runs/:id" element={<Placeholder name="Corrida" />} />
        <Route path="/warm" element={<Placeholder name="Warm" />} />
        <Route path="*" element={<NotFound />} />
      </Routes>
    </BrowserRouter>
  );
}
