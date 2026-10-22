import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { App } from './App';
import { aplicarTema, temaInicial } from './theme/tema';
import './styles.css';

// Tema antes del primer render: data-theme ya está puesto cuando se pinta la cabecera.
aplicarTema(temaInicial());

const root = document.getElementById('root');
if (root === null) throw new Error('falta #root en index.html');

createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
