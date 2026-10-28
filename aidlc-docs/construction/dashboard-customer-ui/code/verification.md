# Verificación del dashboard orientado al cliente

Fecha: 2026-10-28. Plan aprobado con «hazlo ahora!!!!».

## Resultado

Se ajustaron Inbox, Mis corridas, Entorno y la cabecera protegida. Las tarjetas de Inbox tienen superficies semánticas, iconos y descripciones; se retiraron las insignias y barras de progreso con porcentajes fijos. Los conteos conservan el significado y disponibilidad de los datos actuales. Los identificadores internos del entorno ya no aparecen.

La navegación agrupa los enlaces y distingue la página activa. Se retiró el selector de tema de la cabecera protegida; Salir conserva el cierre de sesión y añade icono y estados visuales. La pestaña Pendientes tiene espaciado lateral uniforme.

Mis corridas usa textos para el cliente y un estado vacío con ilustración SVG y enlace al Inbox. Entorno muestra disponibilidad y limpieza verificada con textos comprensibles, conservando estados, sondeo, errores y advertencias. En móvil, la tabla permite desplazamiento horizontal dentro de su contenedor.

## Comprobaciones

Desde `web/dashboard`:

| Comando | Resultado |
| --- | --- |
| `npm run build` | Correcto, TypeScript y Vite. |
| `npm run lint` | Correcto. |
| `npm test` | 21 archivos, 234 pruebas correctas. |
| `git diff --check` desde la raíz | Correcto. |

Logs: `build.log`, `lint.log`, `tests.log` y `browser.log` en este directorio. La suite emite avisos de React Router y tres solicitudes MSW sin handler al navegar desde pruebas de confirmación; no producen fallos. No se modificó ese comportamiento ajeno al ajuste visual.

Navegador: Chromium con el build local, APIs simuladas y movimiento reducido. Comando de comprobación: `node /tmp/dashboard-customer-ui-check.mjs` desde la raíz. El servidor escucha solo en loopback; no se usaron cuentas ni entornos compartidos.

- Inbox, Mis corridas vacío y Entorno inspeccionados a 1440 y 390 px.
- Sin desbordamiento horizontal de la página ni errores de JavaScript.
- Tabla desplazable en móvil; padding izquierdo de Pendientes de 16 px en escritorio y 8 px en móvil.
- Texto visible sin `warm-local`, rutas API, nombres de campos internos ni etiquetas retiradas.
- Navegación, enlace Ir al Inbox, foco por teclado y Salir comprobados.
- Estados dirty, cuarentena, idle-escalado y ready comprobados; se mantiene la advertencia de limpieza sin verificar.

Capturas inspeccionadas: `/tmp/dashboard-inbox-1440.png`, `/tmp/dashboard-inbox-390.png`, `/tmp/dashboard-runs-1440.png`, `/tmp/dashboard-runs-390.png`, `/tmp/dashboard-warm-1440.png`, `/tmp/dashboard-warm-390.png`.

Los cambios previos de landing, login y otros archivos se conservaron. No se realizó despliegue, commit ni avance de etapa global. La compuerta histórica de igualdad visual con el mockup anterior no se ejecutó porque este ajuste cambia deliberadamente su presentación; las pruebas unitarias de paridad de estilos existentes sí forman parte de las 234 pruebas correctas.
