# Ronda 1 — U6-T05

VEREDICTO: NO-VERDE

Los cinco criterios pasan en mi ejecución y no hay desborde de alcance. El veredicto es NO-VERDE por un hallazgo NARANJA: tres piezas del alcance, o de lo que el código promete, no tienen prueba que falle si se rompen. Lo demostré con mutaciones en una copia fuera del worktree, que sigue limpio (0 líneas de `git status`, HEAD f333d57).

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Lo anterior en verde | `npm ci` y luego `npm run -s lint && npm run -s typecheck && npm run -s check:api && npx vitest run 2>&1 \| grep -c -E 'failed\|FAIL'` | `0`. 9 archivos, 71 pruebas pasan, 2.29 s. |
| 2 | Corrida y sondeo | `npx vitest run src/pages/Run` | Pasa: 12 pruebas, 9 de RunPage más 3 de RunsPage por coincidencia de prefijo. La lista verbose cubre los 8 escenarios que pide el criterio, más `trace_id`. |
| 3 | Mis corridas | `npx vitest run src/pages/Runs` | Pasa: 3 pruebas. |
| 4 | Warm | `npx vitest run src/pages/Warm` | Pasa: 7 pruebas. |
| 5 | Alcance | `git status --short \| wc -l` y el `git diff --name-only` contra el merge-base | `0` y `0`. |

Los tiempos de la suite no son sospechosos: las pruebas usan reloj falso y MSW, y duran entre 0.2 y 1.3 s con aserciones sobre el conteo de peticiones.

## Veredicto sobre los 5 puntos declarados
1. **«running = 6 fases hechas»**. La lectura de 5 `hecha` más `running` como `actual` es la única coherente con una línea de tiempo (`running` es la sexta fase). La prueba cuenta «hecha o actual» y además exige `aria-current` en `running`. Es aceptable. Queda como ambigüedad de la especificación para el orquestador, no como defecto.
2. **«Mis corridas» en `/runs`**. Lo permite la tarea («a elección; documentarlo») y está en la bitácora y el README. Sin objeción.
3. **`usePolling` sin AbortSignal**. No quedan temporizadores vivos. Con una prueba mía y una petición en vuelo al desmontar, hubo 1 petición en total y `vi.getTimerCount()` dio 0 tras 20 s. Tras `done` o `failed` no se programa nada. Sí queda viva la petición que ya estaba en vuelo hasta que termine; su resultado se descarta con la bandera `vivo`. Lo que sí falla es el comentario del hook (ver F-03). El problema de jsdom con AbortController es plausible y la bitácora lo reproduce.
4. **`nav` en ProtectedLayout**. Correcto y dentro de `web/dashboard`, pero sin prueba (ver F-01).
5. **Pruebas de corrida y warm sin pasar por la guarda de sesión**. Es aceptable para estas vistas. El 401 lo resuelve el manejador global de `apiFetch`, que borra la sesión, y la guarda desmonta la vista y con ella el sondeo. Esa ruta no queda cubierta de punta a punta por esta tarea (ver tarea candidata).

Lo que sí comprobé como correcto con pruebas mías en la copia:
- Cambiar de `:id` no deja datos de la corrida anterior y no vuelve a pedir la vieja. Funciona gracias a `key={id}`.
- Un `503` tras una respuesta buena conserva la línea de tiempo y muestra «Servicio no disponible» con la referencia de soporte.
- El token no se muestra en ningún punto: solo se usa `ApiErrorNotice`.
- Las vistas no cambian estado: todo son `GET`.

## Hallazgos
### F-01 · NARANJA · cobertura ausente en tres puntos del alcance (clase: partes cargadas de comportamiento sin prueba que las ancle)
Hice mutaciones en una copia fuera del worktree y la suite completa siguió con 71/71 pasando en estos tres casos:
- **Nav de cabecera** (`src/session/ProtectedLayout.tsx`). Es un entregable explícito («Enlaces en la cabecera: Inbox, Mis corridas, Warm»). Quité el enlace «Mis corridas» y todo pasó. No hay ninguna prueba del nav.
- **`key={id}` en `RunPage.tsx`** (`<VistaCorrida key={id} …>`). El hook documenta que depende de él: «el efecto solo depende de `intervalMs` y `enabled`». Sin `key`, el sondeo de la corrida A sigue vivo al navegar a B y muestra datos de A. Quité `key={id}` y todo pasó. Escribí una prueba de cambio de `:id` y esa sí falla sin `key` (`expected <code></code> to be null`), así que la carrera existe y nadie la protege.
- **Aviso `ready` + `reset_verified=false`** (`WarmPage.tsx`). Quité la condición `data.state === 'ready' &&` y todo pasó. La prueba solo usa `ready`, así que el aviso aparecería también en `dirty` o `cuarentena`, donde el criterio no lo pide. Falta el caso `dirty` con `reset_verified=false` sin aviso y el caso `ready` con `true` sin aviso.

Para cerrarlo hacen falta:
- una prueba del nav (por ejemplo, renderizar `ProtectedLayout` con sesión y comprobar los tres enlaces con su `href`);
- una prueba de navegación A→B en `RunPage`;
- los dos casos negativos del aviso del warm.

Las mutaciones que sí fueron detectadas por la suite: eliminar el estado terminal, quitar el listener de `visibilitychange`, subir `maxFailures`, cambiar el intervalo del warm, cambiar `limit=20`, quitar la última fase alcanzada y aflojar la regex del id.

### F-02 · AMARILLO · `usePolling.ts` · el sondeo ignora `Retry-After` en 429 y 503
Con un `429` y `Retry-After: 30`, la vista sigue sondeando cada 3 s y hace 5 peticiones en unos 12 s antes de parar (mi prueba: `PETICIONES 429: 5`). Contra una API con limitación de tasa, el cliente insiste. La tarea no lo especifica, así que no bloquea. Sugiero usar `max(intervalo, retryAfter)` cuando el error lo trae. La cuenta atrás de `ApiErrorNotice` se reinicia en cada fallo.

### F-03 · AMARILLO · `usePolling.ts` (comentario JSDoc) · el comentario contradice la implementación
Dice «Desmontar aborta la petición en curso y no pide más». No se aborta nada: la petición termina y su resultado se descarta. Debe decir «descarta». Es la misma discrepancia que la bitácora reconoce en su decisión técnica, pero el código la afirma al revés.

### F-04 · AMARILLO · `WarmPage.tsx` y `RunPage.tsx` · datos viejos junto a un error
Tras éxitos previos, un fallo deja la tarjeta o línea de tiempo anterior visible junto al aviso. En `/warm` eso puede mostrar «Listo» mientras el aviso dice «Estado del warm no disponible». Es una decisión de diseño defendible, y por eso no es naranja. Valdría marcarlo como dato desactualizado.

## Tareas candidatas (fuera de alcance)
- Prueba de integración que cubra el 401 en una vista que sondea: `apiFetch` llama al manejador, la sesión se borra, la guarda redirige y el sondeo se detiene. Hoy cada tramo se prueba por separado (client, session y polling), nunca junto.
- Si se quiere abortar de verdad las peticiones, unificar el `fetch`/`AbortController` en el entorno de pruebas (jsdom/undici) y pasar `AbortSignal` desde `usePolling`.

## Rutas de transcripciones largas
- Suite completa: `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/34f01c05-e8aa-4201-b219-51a4905daf5f/scratchpad/full.txt`
- Copia con mutaciones y pruebas extra (fuera del worktree): `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/34f01c05-e8aa-4201-b219-51a4905daf5f/scratchpad/m`

VEREDICTO: NO-VERDE
NARANJA|web/dashboard/src/session/ProtectedLayout.tsx; src/pages/Run/RunPage.tsx (key={id}); src/pages/Warm/WarmPage.tsx (aviso ready)|Nav, cambio de :id y condición del aviso del warm sin prueba que falle al mutarlos
