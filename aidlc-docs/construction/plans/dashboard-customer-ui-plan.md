# Dashboard: interfaz orientada al cliente

## Contexto y alcance

Seguimiento visual de U6/U10 en CONSTRUCTION / Code Generation. La solicitud cubre Inbox, Mis corridas, Entorno y la cabecera protegida. Historias: consultar notificaciones y confirmar explícitamente; consultar corridas confirmadas; conocer disponibilidad y limpieza del entorno. Las interfaces existentes de U1/U2/U4 permanecen como fuente de datos.

No se requieren etapas de diseño funcional, NFR ni infraestructura: el cambio es de presentación sobre componentes React/CSS existentes. Conservar los cambios locales previos, incluidos landing, login, App y estilos. No desplegar ni operar el clúster.

## Revisión completada

- [x] Leer estado AI-DLC, reglas activas y contexto de la solicitud.
- [x] Consultar grafo y cobertura, contrastar con las páginas y componentes actuales.
- [x] Identificar causas: ruta API expuesta en Runs y Warm; etiquetas internas en Warm; insignias y porcentajes fijos en Inbox; selector de tema junto al usuario; `padding-left: 0` en la primera pestaña.
- [x] Preparar plan y propuesta de contenido revisables.

## Propuesta concreta

Mantener la tipografía editorial y el fondo cálido. Introducir superficies suavemente coloreadas, iconos SVG coherentes, bordes discretos y estados activos claros.

| Elemento | Presentación propuesta |
| --- | --- |
| Notificaciones pendientes | Ámbar suave, cifra real y «Cambios por revisar antes de iniciar las pruebas». |
| Corridas confirmadas | Azul suave, cifra real y «Pruebas autorizadas por tu equipo». |
| Estado del entorno | Verde cuando esté listo; advertencia o neutral según estado real. Descripción sobre disponibilidad y limpieza verificada. |
| Rechazadas | Rosa suave, cifra real y «Cambios descartados por tu equipo». |

Retirar «esperando confirm», «auditable», «ready» y barras con porcentajes fijos. Mantener «—» si aún no hay un conteo real; mostrar 0 solo cuando los datos lo indiquen. No convertir el conteo de notificaciones confirmadas en una estadística de ejecuciones terminadas.

Retirar el identificador `warm-local` de Inbox y Entorno. La cabecera del Inbox mostrará únicamente el estado del entorno y la verificación de limpieza. Conservar confirmación humana y advertencias relevantes en lenguaje claro.

Cabecera: navegación agrupada con selección visible, espaciado consistente e iconos legibles; retirar el selector de tema de la cabecera protegida; conservar la identidad del usuario; mejorar «Salir» con icono y estados de hover/foco, conservando su acción de cierre de sesión.

Mis corridas: descripción «Consulta las pruebas que has autorizado y sigue sus resultados». Retirar rutas y nombres de campos API de los encabezados. Estado vacío con icono, «Tu primera corrida empieza en el Inbox», explicación de confirmación previa y enlace «Ir al Inbox» a `/inbox`.

Entorno: descripción «Consulta si tu entorno de pruebas está disponible para la próxima corrida». Presentar estado y limpieza verificada con explicaciones útiles. Retirar ruta API, frecuencia de sondeo, nombres de campos, identificador interno y versión base de la vista principal. Conservar visualización de errores, indisponibilidad, cuarentena y datos antiguos, usando lenguaje para el cliente.

## Secuencia de implementación

1. [x] Ajustar `web/dashboard/src/session/ProtectedLayout.tsx`, `session/NavPrincipal.tsx` y estilos de cabecera. Añadir identificadores de automatización estables a controles nuevos.
2. [x] Ajustar `web/dashboard/src/pages/Inbox/InboxPage.tsx` y `components/Kpi.tsx`: contenido, colores semánticos, indicadores reales y espaciado de la pestaña Pendientes. Acotar las reglas nuevas al dashboard protegido para preservar la landing y el login.
3. [x] Ajustar `web/dashboard/src/pages/Runs/RunsPage.tsx`: descripción, encabezados y estado vacío con enlace al Inbox.
4. [x] Ajustar `web/dashboard/src/pages/Warm/WarmPage.tsx`: información útil al cliente, estados y advertencias comprensibles. Conservar sondeo, reintento y refresco existentes.
5. [x] Actualizar las aserciones existentes afectadas por cambios de texto/controles en pruebas de cabecera, Inbox, Runs y Warm; conservar pruebas de confirmación, cierre de sesión, navegación y estados no saludables.
6. [x] Ejecutar build, lint y pruebas existentes del dashboard. Inspeccionar Inbox, Mis corridas vacío y Entorno en navegador local a 1440px y 390px; verificar foco, navegación, ausencia de errores y desbordamiento. Usar datos simulados de solo lectura para la revisión visual.
7. [x] Registrar evidencia en `aidlc-docs/construction/dashboard-customer-ui/code/verification.md`, marcar pasos y actualizar `aidlc-state.md` sin avanzar la etapa global.

## Comandos de aceptación

Desde `web/dashboard`:

```sh
npm run build
npm run lint
npm test
```

Para revisión visual: `npm run dev -- --host 127.0.0.1`, navegador local con Playwright y mocks de endpoints existentes. Verificar `scrollWidth <= innerWidth`, enlaces correctos, ausencia de `pageerror`, ausencia de rutas API y `warm-local` en el texto visible de las tres páginas, y separación entre estado vacío, carga y error.

## Aprobación

Aprobado explícitamente por el usuario: «hazlo ahora!!!!». Implementación y verificación local autorizadas.
