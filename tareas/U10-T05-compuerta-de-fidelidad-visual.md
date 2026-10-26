# U10-T05 — Compuerta de fidelidad visual (mockup vs. dashboard)

**Unidad:** U10 — Fidelidad visual del dashboard al mockup
**Historias que implementa:** S5.
**Depende de:** U10-T01 **fusionada** (corre en paralelo con T02–T04; su salida se engancha al cerrar). **Ola 2.** **Tope: 3 rondas**.

## Por qué existe
U9-T04 pasó con tokens y contraste y nada que comparara el resultado con el mockup. Esta tarea hace que «se ve distinto» falle una prueba.

## Alcance
**Dentro:**
> Un script `npm run e2e:fidelidad` que abre `mockups/swarm-mock.html` y el dashboard (servido con datos MSW fijos), y compara por vista, tema (claro/oscuro) y ancho (1280, 375).

- Estilos calculados de elementos ancla (cabecera 60 px, alto/borde/radio de tarjetas, tamaño/peso de títulos, color y fondo de insignias, separaciones de la rejilla) con tolerancia de ±1 px y colores exactos.
- Estructura: misma secuencia de regiones/encabezados por vista (por rol y texto de etiqueta).
- Captura de ambas páginas y diferencia de píxeles por vista con umbral declarado (p. ej. ≤ 5 % del área) y un informe en `bitacoras/U10-T05/` con las imágenes lado a lado.
- Playwright (Chromium) como `devDependency`, justificada en la bitácora; sin CDN ni cambios de CSP del build.
**Fuera:** pruebas de comportamiento (ya existen en Vitest); cambios en vistas (T02–T04).

## Criterios de aceptación
- [ ] **CA-1** — `npm run -s e2e:fidelidad` existe y devuelve código ≠ 0 mientras haya diferencias reales entre el dashboard y el mockup (rojo primero: se pega la salida). *Cambio del humano (2026-10-26):* ya no se exige 0 tras T02–T04; cerrar las brechas es U10-T06. Se acepta MSW sustituido por `page.route` con fixtures de `contracts/**/examples/valid` (precedente de T03).
- [ ] **CA-2** — El informe lista, por vista/tema/ancho, las diferencias de estilo calculado y el % de píxeles distintos.
- [ ] **CA-3** — Se engancha a `scripts/kind/dashboard-smoke.sh` como `OK fidelidad-visual` (dentro de `flock ${XDG_RUNTIME_DIR:-/tmp}/aqs-kind.lock`).
- [ ] **CA-4** — Alcance: solo `web/dashboard/**`, `scripts/kind/dashboard-smoke.sh` y archivos de la tarea.
- [ ] **CA-5** — Control de ruido: una ejecución que compara el mockup contra sí mismo, con los mismos viewports y temas, sale con código 0 y 0 % de píxeles distintos (`npm run -s e2e:fidelidad -- --control`). Un fixture compartido entre mockup y app queda como candidata de T06.

_Creado con ❤️ por Luis Felipe Ariza Vesga._
