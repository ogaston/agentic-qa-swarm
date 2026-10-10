# Ronda 3 — U6-T03

VEREDICTO: VERDE

Revisé el sha 9b120e0 en `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U6-T03`. F-03 queda cerrado con evidencia mía. No queda ningún ROJO ni NARANJA. Hice las mutaciones sobre una copia en el scratchpad. Tras mis ejecuciones, `git status --short | wc -l` en el worktree da `0` (`dist` está ignorado).

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Lo anterior sigue en verde | `npm run -s lint && npm run -s typecheck && npm run -s check:api && npx vitest run \| grep -c -E 'failed\|FAIL'` | pasa. lint, typecheck y check:api dan `rc=0`. El conteo es `0`. Son 5 archivos y 45 pruebas en verde. |
| 2 | Flujo de login | `npx vitest run src/session src/pages/Login` | pasa. 23 pruebas (se exigen ≥8), sin `failed`. |
| 3 | Expiración y 401 | `npx vitest run src/session -t 'expira\|401'` | pasa. 5 pasan y 7 quedan filtradas (se exigen ≥3). |
| 4 | El token no persiste ni se filtra | los 3 comandos de CA-4 | pasa. El grep en `src` da `0`, la prueba `no persiste` pasa (1 passed) y el grep en `dist` da `0`. |
| 5 | Alcance | los 2 comandos de CA-5 | pasa. Da `0` y `0`. El merge-base es be21859. |

No hay `.skip`, `.only` ni `.todo` en `src` (el conteo da `0`). La suite dura unos 1,9 s con jsdom y MSW, que es un tiempo coherente con 45 pruebas reales.

## Cierre de F-03
- **Mutación repetida.** Quité la línea `borrarLocal()` del inicio de `login` en una copia del dashboard.
  - Resultado: `Tests 1 failed | 22 passed (23)`.
  - Falla `login descarta la sesión previa (A-2 / F-03) > dos logins seguidos: el segundo POST no lleva el Bearer del primero`.
  - La salida muestra `- Expected null` y `+ Received "Bearer T1"`.
  - En la ronda 2 la misma mutación daba 21 de 21 en verde, así que ahora la regresión se detecta.
- **Lectura de vuelta.** La prueba lee la cabecera `Authorization` real que llega al handler de MSW de `POST /api/auth/login`. No mira códigos de salida ni estado interno.
- **Aislamiento.** Con `-t 'descarta'` ambas pruebas pasan solas (`2 passed | 10 skipped`). `setup.ts` limpia con `cleanup` y `resetHandlers` en cada prueba.
- **Frase falsa de la bitácora.** La sección «Ronda 3» la declara falsa de forma explícita («era falsa. Sí era testeable…»). La línea 127 original de la ronda 2 sigue ahí, pero la bitácora es solo de añadir y la corrección es inequívoca. Lo doy por cerrado.

## Respuestas a A-6 y A-7
- **A-6, sin cambio.** El `ApiError` sintético con `status: 200` y `requestId: 'sin-id'` es un valor interno, y el codificador lo declara. Es aceptable y lo dejo como amarillo.
- **A-7, verificado.** En `safeNext`, la regex de controles, espacios y `\` se aplica antes de `new URL`. `new URL` solo recibe entradas que empiezan por `/` sin `//`, y no encontré una que lance. No hace falta cambio.
- **`api!.login` → `sesion.login`.** El cambio es correcto. La función `login` viene de `useCallback([borrarLocal])`, y `borrarLocal` tiene dependencias vacías, así que la referencia es estable. Se puede usar la capturada al montar. La mutación la ejecuté con la versión final de la prueba, y la prueba detecta la regresión.

## Revisión adversarial del diff de la ronda
El diff de la ronda toca solo `session.test.tsx` y la bitácora (6cc37f3..HEAD). No hay código de producción nuevo ni desborde de alcance.

## Hallazgos
No hay ROJO ni NARANJA.

### A-8 · AMARILLO · `session.test.tsx`, prueba `un login fallido con sesión previa deja sin sesión`
Esa prueba no discrimina la mutación. Con `borrarLocal()` quitado sigue pasando, porque el segundo login sí lleva el Bearer, el 401 dispara el manejador global y este borra la sesión. Por eso la mutación da `1 failed | 22 passed`, y el fallo lo aporta solo la prueba de cabeceras. Lo que no queda fijado con prueba es el efecto «un login fallido por `500` o red también deja sin sesión». No bloquea, porque la regresión central sí está cubierta. Un caso con `500` en el segundo login lo haría discriminante.

### A-6 · AMARILLO · `SessionContext.tsx`
Literal `'sin-id'` y `status: 200` en el error sintético. Es inocuo (ver arriba).

## Tareas candidatas (fuera de alcance)
- Validación en runtime de las respuestas de la API (fechas y esquema) en el cliente. Sigue pendiente desde la ronda 1; es candidata a refuerzo transversal.

## Rutas de transcripciones largas
- Ninguna.

VEREDICTO: VERDE
AMARILLO|session.test.tsx (prueba «un login fallido con sesión previa deja sin sesión»)|No discrimina la mutación de F-03 (el manejador global de 401 también limpia la sesión); falta un caso con 500 o red
