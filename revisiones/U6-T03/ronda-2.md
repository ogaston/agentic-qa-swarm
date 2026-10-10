# Ronda 2 — U6-T03

VEREDICTO: NO-VERDE

Los dos NARANJA de la ronda 1 (F-01, F-02) quedan cerrados con evidencia mía. Queda un hallazgo NARANJA nuevo (F-03): el cambio de A-2 se aplicó sin prueba, y la razón que da la bitácora es falsa. No hay ROJO.

Revisé sha 65f40e8 en `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U6-T03/web/dashboard`. Las mutaciones las hice sobre una copia en el scratchpad. El worktree está limpio tras mis ejecuciones (`git status --short | wc -l` da `0`; `dist` está ignorado).

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Lo anterior sigue en verde | `npm run -s lint && npm run -s typecheck && npm run -s check:api && npx vitest run 2>&1 \| grep -c -E 'failed\|FAIL'` | pasa. lint, typecheck y check:api sin error, el conteo es `0`, 5 archivos y 43 pruebas en verde. El `rc=1` del `grep -c` es el código de salida de grep cuando cuenta 0, no un fallo. |
| 2 | Flujo de login | `npx vitest run src/session src/pages/Login` | pasa. 21 pruebas (se exigen ≥8), sin `failed` |
| 3 | Expiración y 401 | `npx vitest run src/session -t 'expira\|401'` | pasa. 5 pasan y 5 quedan filtradas por `-t` (se exigen ≥3). No hay `.skip`, `.only` ni `.todo` en `src`. |
| 4 | El token no persiste ni se filtra | los 3 comandos de CA-4 | pasa. El grep en `src` da `0`, la prueba `no persiste` pasa y el grep en `dist` da `0`. |
| 5 | Alcance | los 2 comandos de CA-5 | pasa. Da `0` y `0`. |

## Cierre de la ronda 1, con mutaciones repetidas
Salida de las mutaciones sobre `src/session` y `src/pages/Login` (21 pruebas en la base):

| Mutación | Resultado |
|---|---|
| Quitar `setAuthToken(null)` de `borrarLocal` | **6 fallan**: `expira`, las 4 de «el token del módulo se borra en todo cierre» y `expires_at no parseable`. En la ronda 1 daba 37 de 37 en verde. |
| Quitar el chequeo `Date.parse` de `expires_at` en `login` | falla 1 prueba (`expires_at no parseable rechaza el login`) |
| Quitar `!Number.isFinite(restante)` del temporizador | pasa 21 de 21 |
| Quitar `borrarLocal()` del inicio de `login` (A-2) | pasa 21 de 21. Ver F-03. |
| Quitar solo la regex de `safeNext` | pasa 21 de 21 (el check de origen la cubre) |
| Quitar solo el check `new URL().origin` de `safeNext` | pasa 21 de 21 (la regex lo cubre) |
| Quitar regex y check de origen a la vez | falla la prueba del tabulador |

- **F-01 está cerrado.** Las pruebas leen de vuelta la cabecera `Authorization` con un `probe` de MSW. Cada una confirma primero que el `Bearer` sí llega y comprueba después que ya no llega. Cubren logout con `500`, logout con `204` (camino feliz), expiración y `401`. La prueba `expira` ahora exige `next=%2Finbox` exacto (cierra A-5).
- **F-02 está cerrado.** `expires_at` inválido rechaza el login antes de fijar el token, y la prueba lee de vuelta que no hay sesión ni `Authorization`.
  - El `isFinite` del temporizador no tiene prueba propia, pero es redundante: tras el check de `login` no se puede llegar a él. No es defecto.
  - Un `expires_at` parseable pero ya pasado termina en `/login?next=%2Finbox`. Lo probé en una copia y es coherente.
- **A-1 está cerrado** con defensa doble (regex de controles, espacios y `\`, más el check de origen). Cada capa tapa a la otra; la prueba del tabulador se rompe si faltan las dos. Que ninguna mutación individual falle es redundancia deliberada y no la penalizo.
- **A-3 y A-4 están cerrados.** El README ya documenta el reloj adelantado y el `expires_at` inválido, y la bitácora tiene CA-5 con salida real.
- **A-2 queda abierto**: ver F-03.

## Hallazgos

### F-03 · NARANJA · `src/session/SessionContext.tsx` (`borrarLocal()` al inicio de `login`) · el cambio de A-2 se aplicó sin prueba, y la justificación es falsa
La bitácora dice «no pude construir un escenario de prueba limpio» y que la garantía es «estructural». Pero sí se puede probar, y la mutación pasa en verde.
- Mutación: quitar `borrarLocal()` de `login` deja 21 de 21 en verde. Es el mismo patrón que F-01.
- Prueba posible en unos 15 líneas: un componente mínimo con `useSession()` dentro de `SessionProvider`, que llama a `login` dos veces seguidas.
  - Con un handler MSW de `/auth/login` que registra `request.headers.get('authorization')`, el resultado esperado es `[null, null]`.
  - Lo escribí en una copia y pasa con el código actual. Con la mutación da `Received "Bearer T1"`, o sea que detecta la regresión.
  - Esa copia la borré; el worktree no se tocó.
- El cambio también tiene un efecto de comportamiento sin cubrir: cualquier `login`, aunque falle, ya deja sin sesión previa. Eso hay que fijarlo con una prueba, no dejarlo implícito.

Pedido: añadir esa prueba (cabecera `Authorization` ausente en el segundo `POST /api/auth/login` con sesión activa) y corregir la frase de la bitácora.

## Amarillos (no bloquean)
- **A-6 · `SessionContext.tsx`**: el `ApiError` sintético de `expires_at` usa `status: 200` y `requestId: 'sin-id'` (literal incrustado, valor interno). Es inocuo.
- **A-7**: `safeNext` es correcto, y también hace falta que `new URL` no lance para entradas que empiezan por `/`. Con la regex previa no encontré entradas que lancen.

## Revisión adversarial del diff nuevo
- `safeNext`: rechazo de `/\t/evil`, `//`, `/\`, espacios y controles, más check de origen. Sin eludir encontrado. `/..//evil.example` queda en el mismo origen.
- El rechazo de `expires_at`: el `throw` ocurre antes de `setAuthToken`, así que no hay token vivo. El `ApiError` llega a `ApiErrorNotice` y muestra el mensaje. Sin fuga de token.
- `ApiErrorNotice` no cambió en esta ronda.
- Alcance: sin desborde. Todo está en `web/dashboard/**`, la bitácora y `revisiones/U6-T03/`.

## Tareas candidatas
- Validación en runtime de las respuestas de la API (fechas y esquema) en el cliente. Sigue pendiente desde la ronda 1; es candidata a refuerzo transversal.

## Rutas de transcripciones largas
- Ninguna. Todas las salidas relevantes están citadas arriba.

VEREDICTO: NO-VERDE
NARANJA|src/session/SessionContext.tsx (borrarLocal() al inicio de login)|El cambio de A-2 no tiene prueba (mutación pasa 21/21) y la bitácora afirma, sin fundamento, que no era testeable
