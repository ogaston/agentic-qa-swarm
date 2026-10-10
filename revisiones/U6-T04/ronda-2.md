# Ronda 2 — U6-T04

VEREDICTO: VERDE

Corrí los 5 criterios de aceptación yo mismo y pasan. No queda ningún ROJO ni NARANJA. F-01 y F-02 (NARANJA en la ronda 1) quedan cerrados con mutaciones que yo repetí. Los AMARILLO de esta ronda no bloquean.

Salida completa de vitest: `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/34f01c05-e8aa-4201-b219-51a4905daf5f/scratchpad/full2.txt`. En esa salida no aparece ninguna línea `[MSW]`.

## Criterios de aceptación, verificados por mí
Desde `web/dashboard/` en el worktree, sha 7418d7f.

| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Lo anterior en verde | `npm run -s lint`, `typecheck`, `check:api`, `npx vitest run \| grep -c -E 'failed\|FAIL'` | pasa. Los tres con exit 0 y el conteo da `0`. 6 ficheros y 66 pruebas pasan. |
| 2 | Lista y filtros | `npx vitest run src/pages/Inbox -t 'lista'` | pasa. `9 passed \| 12 skipped`. |
| 3 | Confirmación explícita | `... -t 'confirma'` | pasa. `13 passed \| 8 skipped`. |
| 4 | Nunca se confirma solo | `... -t 'sin auto'` | pasa. `1 passed \| 20 skipped`. El POST se cuenta con un handler registrado, así que el handler por defecto no lo oculta. |
| 5 | Alcance | `git status --short \| wc -l`; `git diff --name-only $(git merge-base HEAD origin/main) \| grep -v -E '^(web/dashboard/\|bitacoras/U6-T04\.md\|revisiones/U6-T04/)' \| wc -l` | pasa. Da `0` y `0`. |

El worktree quedó limpio tras mis ejecuciones. Todas las mutaciones las hice sobre una copia en `.../scratchpad/rv-t04/d`. Restauré la copia y comprobé con `diff -r` que quedó idéntica al código de la ronda 2.

## Cierre de hallazgos de la ronda 1

**F-01 (polling) — cerrado.** La prueba ahora instala `vi.useFakeTimers({ shouldAdvanceTime: true })` antes de montar y avanza 6 s.
- Mutación `setInterval(() => cargar(estado), 1900)`: falla «sin polling...». Hay 2 fallos en total y 19 pruebas pasan.
- Mutación con un intervalo de 5500 ms: también falla, con 1 fallo y 20 pruebas pasando.
- En la ronda 1 esa misma mutación pasaba las 16 pruebas.

**F-02 (pestaña con POST en vuelo y respuesta vieja) — cerrado.**
- Caso A (409 con cambio de pestaña) y caso H (400 con cambio de pestaña): el código ahora deshabilita las pestañas y «Ver detalle» mientras hay un POST, y `elegirPestana` tiene la misma guarda. Las pruebas nuevas comprueban que «Pendientes» sigue seleccionada, que la recarga pide `pending`, que `acme/api` está en la tabla, que `acme/confirmado` no está, y que el aviso del 400 sigue visible con el diálogo abierto.
- Quité las dos guardas (`disabled` y `enCurso` de las pestañas): fallan las dos pruebas nuevas. Con `cargar(estado)` en lugar de `cargar(estadoRef.current)` y las guardas quitadas, también fallan.
- Quité `if (id !== peticion.current) return;` de la rama de éxito: falla «descarta la respuesta vieja...». Con 400 ms de retraso en `pending`, esa prueba solo pasa si el descarte funciona.

**F-04 (run_id) — cerrado en lo que pidió el hallazgo.** Cambié `if (!RUN_ID.test(recibo.run_id))` por `if (false)`: falla «201 con run_id no válido no navega y avisa». Ya no se navega a `/` ni a `/runs/`. Se avisa con el id de la notificación y se recarga la lista. Ver AMARILLO Y-02 sobre el alcance de esa prueba.

**F-05 (evidencia) — cerrado.**
- Rojo literal: reproduje el rojo con el `App.tsx` de 94fc8e8 y las pruebas de la ronda: `Tests 21 failed (21)`, igual que dice la bitácora.
- Salida de CA-5: ahora está en la bitácora y coincide con la mía (`0` y `0`).
- Las 3 pruebas nuevas de la ronda tienen rojo documentado contra el código de la ronda 1. Las otras dos (polling y respuesta vieja) se justifican con mutación, y la bitácora lo dice expresamente. Lo acepto: una prueba de ausencia no puede estar roja antes de la mutación.

## Handler por defecto en `src/test/server.ts` (F-03)
Veredicto: aceptable, no oculta lo que yo pedía.
- Se queda `onUnhandledRequest: 'error'` en `setup.ts`. El handler por defecto cubre solo `GET /api/notifications`. Cualquier POST, `GET /api/notifications/{id}` u otra ruta sin handler sigue dando error.
- Todas las pruebas del inbox sobrescriben ese GET con `server.use`, así que el handler por defecto nunca las atiende. Lo comprobé cambiando su respuesta por una notificación centinela (`DEFAULT/hit`): las 66 pruebas pasan igual. Ninguna prueba del inbox depende del default, y un polling real caería en el handler de la prueba, que lo registra.
- Efecto residual: en las pruebas de sesión y login, un GET extra a `/api/notifications` ya no deja rastro. Esas pruebas no afirman nada sobre esa ruta. Es un coste pequeño a cambio de quitar las 11 líneas de ruido.
- Lo que no hizo: no añadió ninguna comprobación de `request:unhandled`. Un POST sin handler se mostraría como `ApiErrorNotice` y no como fallo. Es teórico, porque las pruebas del inbox registran los POST en su handler. AMARILLO.

## Cierre parcial de F-06 (candidata)
Aceptado.
- Retiró `aria-selected` de `<tr>`, que era el único punto inválido y barato de la lista.
- La trampa de foco, `inert` del fondo, navegación con flechas y `tabpanel` quedan declarados como tarea candidata, fuera de los criterios.
- `aria-modal="true"` sigue sin trampa de foco, pero eso ya estaba en la ronda 1 y no rompe ningún criterio.

## Revisión adversarial del diff de la ronda
Sin defectos en el código. El diff toca 4 archivos: la página, su prueba, `server.ts` y la bitácora. No hay desborde de alcance.

## Hallazgos que quedan
### Y-01 · AMARILLO · `InboxPage.tsx:49-52` · el descarte de respuestas viejas en la rama `catch` no tiene prueba
Quité `if (id !== peticion.current) return;` del `catch` y las 21 pruebas pasan. La rama de éxito sí está cubierta. Un error viejo podría pisar la lista de la pestaña vigente.

### Y-02 · AMARILLO · prueba «201 con run_id no válido» · solo ejercita el caso `".."`
Cambié la regex por `/^run-/` y las 21 pruebas pasan. Tampoco hay un caso con id hostil que justifique `encodeURIComponent`: quitarlo no rompe ninguna prueba. Una validación más débil que la del README de ui-api seguiría pasando.

### Y-03 · AMARILLO · `estadoRef` y la guarda de pestañas son defensa redundante
Con las guardas de pestaña activas, `estadoRef.current` y `estado` son siempre iguales. Las pruebas no pueden distinguirlos: con las guardas intactas, cambiar `estadoRef.current` por `estado` pasa las 21 pruebas. La bitácora ya lo reconoce (M2 y G). Es defensa en capas, sin daño.

### Y-04 · AMARILLO · pestañas y «Ver detalle» deshabilitados durante el POST
Un botón que se deshabilita pierde el foco de teclado mientras el POST está en vuelo. Es una molestia menor de accesibilidad. «Actualizar» sigue habilitado durante el POST: es inocuo, porque solo recarga la lista.

## Tareas candidatas (fuera de alcance)
- Accesibilidad del panel y las pestañas: trampa de foco, `inert` del fondo, navegación con flechas, `tabpanel`, y revisar `aria-modal`. Viene de F-06.
- Validación en tiempo de ejecución de las respuestas de la API en `apiFetch`, de la tarea candidata de U6-T03. Evitaría F-04 de raíz.
- Un `onUnhandledRequest` propio que haga fallar la prueba en lugar de mostrar el error de red en la página.

## Rutas de transcripciones largas
- `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/34f01c05-e8aa-4201-b219-51a4905daf5f/scratchpad/full2.txt`
- Copia de mutaciones: `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/34f01c05-e8aa-4201-b219-51a4905daf5f/scratchpad/rv-t04/`
- El informe de la ronda 1 está en `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U6-T04/revisiones/U6-T04/ronda-1.md`.

VEREDICTO: VERDE
AMARILLO|InboxPage.tsx:49-52|el descarte de respuestas viejas en el catch no tiene prueba
AMARILLO|InboxPage.test.tsx (201 con run_id no válido)|solo se prueba run_id ".."; una regex más débil pasa y no hay caso hostil para encodeURIComponent
AMARILLO|InboxPage.tsx estadoRef|defensa redundante con la guarda de pestañas, no distinguible por pruebas
AMARILLO|InboxPage.tsx pestañas disabled|pierden el foco de teclado durante el POST
