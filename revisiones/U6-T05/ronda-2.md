# Ronda 2 — U6-T05

VEREDICTO: NO-VERDE

F-01, F-02 y F-03 quedan cerrados. F-04 solo está cerrado a medias. El aviso «Mostrando la última lectura correcta.» tiene prueba en `/warm`, pero la línea equivalente que se añadió en `RunPage.tsx` no tiene ninguna. Es la misma clase de defecto de la ronda 1 (comportamiento nuevo sin prueba que falle al mutarlo), así que lo clasifico NARANJA. El worktree quedó limpio tras mi revisión (0 líneas de `git status`, HEAD e85f4e5). No hay desborde de alcance.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Lo anterior en verde | `npm ci` y luego `npm run -s lint && npm run -s typecheck && npm run -s check:api && npx vitest run 2>&1 \| grep -c -E 'failed\|FAIL'` | `0`. 77 pruebas pasan. |
| 2 | Corrida y sondeo | `npx vitest run src/pages/Run` | Pasa: 13 pruebas, 10 de RunPage más 3 de RunsPage por prefijo. |
| 3 | Mis corridas | `npx vitest run src/pages/Runs` | Pasa: 3 pruebas. |
| 4 | Warm | `npx vitest run src/pages/Warm` | Pasa: 10 pruebas. |
| 5 | Alcance | `git status --short \| wc -l` y el `git diff --name-only` contra el merge-base | `0` y `0`. |

Las duraciones (1 a 1,3 s) se explican por reloj falso y MSW, con aserciones sobre el conteo de peticiones.

## Cierre de los hallazgos de la ronda 1
Hice las mutaciones en una copia fuera del worktree. La base da 77/77, y cada mutación hace fallar exactamente una prueba, la prevista:

| Mutación | Prueba que falla |
|---|---|
| Quitar «Mis corridas» del nav | `nav.test.tsx > enlaza a Inbox, Mis corridas y Warm con sus href` |
| Quitar `key={id}` en `RunPage` | `RunPage.test.tsx > cambiar de :id no muestra datos de la corrida anterior ni vuelve a pedirla` |
| Quitar `state === 'ready' &&` del aviso del warm | `WarmPage.test.tsx > sin aviso si state=dirty aunque reset_verified=false` |
| `usePolling` sin `Retry-After` (`Math.max(...)` por `intervalMs`) | `usePolling.test.tsx > respeta Retry-After en 429` |
| Quitar el aviso «Mostrando…» en `WarmPage` | `WarmPage.test.tsx > tras una lectura buena, un fallo marca el dato como desactualizado` |
| Quitar el aviso «Mostrando…» en `RunPage` | **ninguna: 77/77 pasan** (ver F-01) |

- **F-01 (NARANJA anterior): cerrado.** Las tres pruebas son sólidas. La de `key` deja la corrida B colgada para que lo visible sea lo que quedó de A, y comprueba que A no se vuelve a pedir. La del nav exige exactamente 3 enlaces con su `href`. Los casos negativos del aviso cubren `dirty` con `reset_verified=false` y `ready` con `true`.
- **F-02: cerrado.** Probé el hook con pruebas propias en la copia y se comporta bien:
  - Con `Retry-After: 30` hay 1 petición a los 12 s y la segunda a los 30 s.
  - Dos `429` seguidos y luego éxito: `fallosSeguidos` vuelve a 0 y la cadencia vuelve a 3 s.
  - Cinco `429` seguidos: exactamente 5 peticiones, `agotado=true` y `vi.getTimerCount()=0`.
  - Desmontar durante la espera: 1 temporizador antes, 0 después, y ninguna petición más en 60 s.
  - Con `Retry-After` menor que el intervalo se usa el intervalo.
- **F-03: cerrado.** El JSDoc dice ahora que la petición en curso termina y su resultado se descarta.
- **F-04: parcial.** `/warm` queda cubierto. `/runs/:id` no (F-01 de esta ronda). El aviso aparece en `/warm` también con `503 warm_unavailable`. En `RunPage` se excluye el 404, lo cual es correcto.

## Hallazgos
### F-01 · NARANJA · `web/dashboard/src/pages/Run/RunPage.tsx` (línea añadida, ~76) · el aviso de dato desactualizado de la corrida no tiene prueba
El diff añade `{!noEncontrada && error !== null && data !== null && <p>Mostrando la última lectura correcta.</p>}`. Sustituí esa condición por `false &&` y la suite completa siguió en 77/77. La respuesta del codificador y la bitácora dicen «F-04 corregido» para `WarmPage` y `RunPage`, pero solo hay prueba (roja y luego verde) en `WarmPage.test.tsx`. Hace falta una prueba en `RunPage.test.tsx`: una lectura `running` buena, luego un `503`, y se espera el texto «Mostrando la última lectura correcta» junto a la línea de tiempo. Conviene añadir también el caso contrario: un 404 no muestra el texto. Al corregirlo, la bitácora debe pegar la mutación en rojo.

### F-02 · AMARILLO · `usePolling.ts`, `alCambiarVisibilidad` · ocultar y volver a la pestaña salta la espera de `Retry-After`
Con un `429` y `Retry-After: 30`, un ciclo ocultar/mostrar la pestaña dentro de esos 30 s dispara de inmediato una segunda petición (mi sonda: 2 peticiones). Es un caso marginal, porque al volver a la pestaña se pide de inmediato por diseño. No bloquea, pero conviene anotar el límite. Una espera mínima pendiente evitaría el salto.

### F-03 · AMARILLO · `usePolling.ts:129` · `Retry-After` sin tope superior
`setTimeout` con un retraso mayor que 2^31-1 ms (unos 24,8 días) se dispara de inmediato. Un `Retry-After` absurdo, o una fecha HTTP muy lejana, daría reintentos sin espera. Queda acotado por `maxFailures` (5 peticiones y para), así que es inofensivo. Aun así, `Math.min(…, tope)` sería más limpio. No lo ejecuté.

### F-04 · AMARILLO · `RunPage.test.tsx` · doble línea en blanco
Hay dos líneas en blanco consecutivas tras la prueba nueva. El lint pasa. Es solo estética.

## Tareas candidatas (fuera de alcance, heredadas de la ronda 1)
- Prueba de integración del 401 con una vista que sondea: `apiFetch`, borrado de sesión, redirección de la guarda y parada del sondeo, de punta a punta.
- Si se quiere abortar de verdad las peticiones en curso, unificar `fetch`/`AbortController` en el entorno de pruebas y pasar `AbortSignal` desde `usePolling`.

## Rutas de transcripciones largas
- Copia con mutaciones y sondas, fuera del worktree: `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/34f01c05-e8aa-4201-b219-51a4905daf5f/scratchpad/m2`
- Informe previo: `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U6-T05/revisiones/U6-T05/ronda-1.md`

VEREDICTO: NO-VERDE
NARANJA|web/dashboard/src/pages/Run/RunPage.tsx (aviso «Mostrando la última lectura correcta»)|El aviso de dato desactualizado de la corrida no tiene prueba que falle al mutarlo
