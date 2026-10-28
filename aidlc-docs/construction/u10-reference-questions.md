# U10-T06 — Decisión sobre el contenido de referencia

La revisión independiente de la ronda 2 confirma 12/20 comparaciones visuales y 226/226 pruebas. CA-2..CA-5 y control pasan; CA-1 sigue fallando. La rama `tarea/U10-T06` conserva las correcciones. Informe: `../agentic-qa-swarm-wt-U10-T06/revisiones/U10-T06/ronda-2.md`.

## Pregunta 1
¿Cómo continuar con las diferencias de contenido entre el mockup y los datos disponibles de la aplicación?

A) Autorizar que la preparación del mockup en la prueba use contenido comparable al producto real (recomendado). Se ajustan únicamente los elementos enumerados abajo; siguen intactos comparadores, estilos medidos, tolerancias, umbral de píxeles y las 20 combinaciones. Después se miden otra vez todas las vistas y se corrige el CSS que aún falle.

B) Mantener todo el contenido actual del mockup y especificar tareas separadas para implementar las funcionalidades y los campos que faltan. Requiere ampliar el alcance de producto; no permite inventar datos en la interfaz.

C) Otra alternativa (describir después de [Answer]).

[Answer]: A

## Cambios concretos de referencia propuestos para A

| Vista | Preparación de contenido propuesta |
|---|---|
| Bandeja | Retirar el enlace sin funcionalidad «Nueva aplicación», los metadatos sin dato real «Familias activas»/«sí (100%)»; usar KPI de rechazos y contadores disponibles, igual que la aplicación. |
| Corrida | Igualar el subtítulo real, la etiqueta del estado y el contenido del hallazgo como texto plano del contrato; sin interpretar HTML arbitrario. |
| Entorno | Usar un único identificador real y descripción sin composición ni namespace inventados; conservar los estilos y la estructura de referencia. |

La opción A no modifica `mockups/swarm-mock.html` ni autoriza neutralizaciones adicionales sin documentarlas. No permite copiar estilos de la aplicación al mockup, ocultar regiones completas para evitar errores ni modificar los comparadores. El registro JSON debe enumerar cada preparación adicional por nombre.

## Motivo de la decisión y límite de evidencia

La tarea `tareas/U10-T06-cierre-de-brechas-de-fidelidad.md` prohíbe expresamente «si una diferencia es de datos y no de CSS, se reporta como candidata, no se oculta». Las autorizaciones existentes solo permiten neutralizar demostraciones y desbordamiento. Por eso el cambio de referencia necesita una decisión explícita.

Las sondas de diagnóstico reproducidas por el revisor bajan bandeja móvil de 16,04% a 4,04% y corrida móvil de 14,73% a 0,31% al igualar contenido. No sustituyen la compuerta oficial. Entorno aún requiere medir el efecto de su alineación de contenido; no se promete 20/20 con estos cambios solos. Quedan residuos geométricos menores por investigar.

La tarea sigue NO-VERDE y no se abre como lista para fusión. No se propone relajar la tolerancia.
