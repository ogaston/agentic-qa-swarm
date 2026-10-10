# Ronda 1 — U6-T04

VEREDICTO: NO-VERDE

Los 5 criterios de aceptación pasan en mi ejecución. Hay 2 hallazgos NARANJA y ningún ROJO: una prueba que no prueba lo que dice (sin polling) y un defecto real al cambiar de pestaña con un POST en vuelo.

## Criterios de aceptación, verificados por mí
Desde `web/dashboard/`, con `npm ci` hecho. La salida completa de vitest está en `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/34f01c05-e8aa-4201-b219-51a4905daf5f/scratchpad/full.txt`.

| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Lo anterior en verde | `npm run -s lint`, `typecheck`, `check:api`, `npx vitest run \| grep -c -E 'failed\|FAIL'` | pasa. Todos con exit 0; el conteo da `0`; 6 ficheros y 61 pruebas pasan. |
| 2 | Lista y filtros | `npx vitest run src/pages/Inbox -t 'lista'` | pasa, 7 pruebas. Hay 5 genuinas de «Inbox lista» y 2 de «recarga la lista» que el filtro `-t` arrastra. Ver F-01 sobre la prueba de polling. |
| 3 | Confirmación explícita | `... -t 'confirma'` | pasa, 10 pruebas. |
| 4 | Nunca se confirma solo | `... -t 'sin auto'` | pasa, 1 prueba. Con la mutación «confirmar al abrir el panel» falla (7 pruebas), así que detecta el defecto que dice detectar. |
| 5 | Alcance | `git status --short \| wc -l`; `git diff --name-only $(git merge-base HEAD origin/main) \| grep -v -E '^(web/dashboard/\|bitacoras/U6-T04\.md\|revisiones/U6-T04/)' \| wc -l` | pasa. Da `0` y `0`. Sin desborde de alcance. |

El worktree quedó limpio tras mis ejecuciones. Hice todas las mutaciones sobre una copia en `.../scratchpad/rv-t04`, no sobre el worktree.

## Puntos que declaró el codificador

**1. Cambio de sonda 401 de `/api/notifications` a `/api/warm` en `session.test.tsx`: se conserva lo que demostraban esas pruebas.**
- Quité `setAuthToken(null)` de `borrarLocal` con el `session.test.tsx` del PR. Fallan 7 pruebas, incluidas las 3 de F-01 de U6-T03 (logout 500, logout 204, expiración, 401) y la de «login descarta sesión previa». Es la misma mutación que en U6-T03 daba «37 de 37 pasan».
- Con el `session.test.tsx` original de `origin/main` y la página real, fallan 2 pruebas sin mutar: la del 401 y la de «tras un 401 no lleva Authorization». Confirma que el cambio era necesario: `/inbox` pide `/api/notifications` al montar y esa sonda ya no sirve.
- `/api/warm` es un endpoint protegido real del contrato. Las aserciones no cambian. Aceptado.

**2. Avisos de MSW (F-03): ver abajo.**

**3. `App.tsx`: correcto.** Solo el import y el elemento de la ruta `/inbox`; no hay otros cambios.

## Hallazgos

### F-01 · NARANJA · `InboxPage.test.tsx`, prueba «Actualizar... no hay polling (2 s con temporizador falso)» (CA-2) · la prueba de «sin polling» no detecta polling
La prueba llama a `vi.useFakeTimers()` después de montar la página. Un `setInterval` creado al montar es un temporizador real, y `advanceTimersByTime(2000)` no lo dispara.
- Mutación: añadí a la página `setInterval(() => cargar(estado), 1900)`. La suite completa pasa: `Tests 16 passed (16)`.
- Con intervalos de 100 ms y 1500 ms, la prueba de Actualizar también pasa. Solo detecta un intervalo de unos 10 ms, que es lo que dura la prueba en tiempo real.
- Esto incumple lo que CA-2 pide: «no hay peticiones en segundo plano (2 s de temporizador falso sin peticiones)». La prueba pasa pero no demuestra nada.
- Pedido: instalar los temporizadores falsos **antes** de renderizar (con `shouldAdvance` o con el patrón de `session.test.tsx`), avanzar más de 2 s y contar peticiones. Debe fallar con la mutación de arriba.

### F-02 · NARANJA · `InboxPage.tsx:86-119` y `:49-56` · cambiar de pestaña con un POST en vuelo deja la vista incoherente y el error invisible
Escribí pruebas adversariales temporales en la copia (no están en el worktree).
- **Caso A (409 con cambio de pestaña).** Confirmo en Pendientes y, con el POST en vuelo (409 tras 200 ms), pulso «Confirmadas». `confirmar` captura `estado` del render antiguo y, al llegar el 409, hace `cargar(estado)`.
  - Salida: `LISTAS ['pending','confirmed','pending'] tabSel(Confirmadas)=true  ve acme/confirmado=false  ve acme/api=true`.
  - La pestaña seleccionada es «Confirmadas» pero la tabla muestra la notificación **pendiente** `acme/api`. Desde ahí se puede volver a «Ver detalle» y «Revisar y confirmar» una notificación pendiente bajo una pestaña que dice Confirmadas.
- **Caso H (400 con cambio de pestaña).** El efecto de cambio de pestaña cierra el panel (`setPanelAbierto(false)`), y `errorAccion` solo se pinta dentro del panel.
  - Salida: `H error visible false dialog false`.
  - El fallo desaparece sin aviso.
- **Falta de cobertura.** Quité el descarte de respuestas viejas (`if (id !== peticion.current) return;`) y las 16 pruebas pasan. La bitácora lo cita como característica y no tiene prueba.
- Pedido: no usar un `estado` capturado en el camino 409/404 (por ejemplo, un `ref` con la pestaña actual), mostrar el error de acción aunque el panel se haya cerrado o impedir cambiar de pestaña mientras hay un POST en vuelo, y añadir una prueba para el caso A y otra para la respuesta vieja descartada.

### F-03 · AMARILLO · pruebas de sesión y login (11 líneas `[MSW] Error` en la salida completa) · el seguro `onUnhandledRequest: 'error'` queda neutralizado en esas pruebas
- **Por qué son avisos y no fallos.** En MSW 2, `'error'` imprime el error y hace que `fetch` rechace con un fallo de red. `apiFetch` lo convierte en `ApiError(network_error)`, y `InboxPage` lo captura y lo pinta como `ApiErrorNotice`. Ninguna aserción de esas pruebas lo ve, así que no fallan.
- **Qué oculta.** En esas pruebas, cualquier petición no mockeada que hoy haga la página queda silenciada. Hay 11 líneas de ruido que normalizan ignorar el aviso.
- **Mismo mecanismo en las pruebas del inbox.** Un POST sin handler también se tragaría y se mostraría como error, no como fallo de prueba. En las pruebas actuales sí hay handler donde hace falta.
- Sugerencia: un handler por defecto de `GET /api/notifications` en las pruebas de sesión y login, y que las pruebas del inbox comprueben `request:unhandled`.

### F-04 · AMARILLO · `InboxPage.tsx:98` · `run_id` del recibo no se valida antes de navegar
`navigate(`/runs/${encodeURIComponent(recibo.run_id)}`)` confía en el cuerpo. El README de ui-api fija el formato: `run-` + 32 hex. Con un recibo atípico (respuesta del propio backend):
- `run_id` = `".."` → URL `/`
- `run_id` = `""` → `/runs/`
- 201 sin `run_id` → `/runs/undefined`

La confirmación ya se registró y la persona aterriza en una ruta sin sentido, sin aviso. Sugerencia: validar `^run-[0-9a-f]{32}$` y, si no cumple, mostrar un aviso con el id de notificación. El `encodeURIComponent` es correcto, pero quitarlo (id de notificación y de run) no rompe ninguna prueba: no hay un caso con un id hostil.

### F-05 · AMARILLO · `bitacoras/U6-T04.md` · evidencia incompleta
- CA-5 dice «Se ejecuta tras el commit (ver abajo)» y no hay nada abajo. Es el mismo defecto que A-4 de U6-T03. Yo lo corrí: `0` y `0`.
- La salida del «rojo primero» está abreviada con `...`, no es la salida literal.

### F-06 · AMARILLO · accesibilidad del panel y de la tabla
- `aria-modal="true"` sin trampa de foco ni fondo inerte; el Tab se escapa del «modal».
- `aria-selected` sobre `<tr>` no es válido en una tabla normal.
- Las pestañas usan `role="tab"` sin navegación con flechas ni `tabpanel`.
- El Escape solo funciona con el foco dentro del diálogo.

Estas mejoras no están en los criterios de aceptación.

## Revisión adversarial sin hallazgo (con la mutación que lo demuestra)
- **Confirmación solo por acción del usuario:** mutación «confirmar al abrir el panel» → 7 pruebas fallan, incluida la de CA-4.
- **Doble envío:** sin la guarda `enCurso` la prueba sigue pasando, porque `disabled={enVuelo}` basta. Sin ninguna de las dos (guarda y `disabled`) falla. Cada protección es redundante pero la prueba detecta quitar ambas.
- **Cuerpo exacto del POST:** mutación `flows: [...familias,'x']` → falla la prueba del cuerpo.
- **Filtros:** quitar `?state=` → fallan 3 pruebas.
- **409 sin recarga:** falla su prueba.
- **Escape:** quitarlo → fallan 2 pruebas.
- **Confirmar en `confirmed`/`rejected`:** quitar la condición `pending` → falla su prueba.
- **Token:** `InboxPage.tsx` no referencia el token.
- **Estados 401, 413, 415:** el 401 se deja a la sesión sin dejar bloqueado el botón (`finally`). 413 y 415 van por la rama genérica de `ApiErrorNotice`, sin prueba propia; es la misma rama que el 400, que sí tiene prueba.

## Tareas candidatas (fuera de alcance)
- Validación en tiempo de ejecución de las respuestas de la API en `apiFetch` (ya anotada en U6-T03); evita F-04 de raíz.
- Un handler por defecto compartido en `src/test/setup.ts` para rutas protegidas, para que las pruebas de sesión y login no dependan de lo que pide cada página.

## Rutas de transcripciones largas
- `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/34f01c05-e8aa-4201-b219-51a4905daf5f/scratchpad/full.txt`
- Copia de mutaciones: `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/34f01c05-e8aa-4201-b219-51a4905daf5f/scratchpad/rv-t04/`

VEREDICTO: NO-VERDE
NARANJA|InboxPage.test.tsx prueba «Actualizar... no hay polling» (CA-2)|la prueba de «sin polling» no detecta polling (un setInterval de 1,9 s pasa)
NARANJA|InboxPage.tsx:86-119 y :49-56|cambiar de pestaña con un POST en vuelo deja la lista de otra pestaña y el error invisible; el descarte de respuestas viejas no tiene prueba
