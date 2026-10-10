# Ronda 1 — U6-T03

VEREDICTO: NO-VERDE

Los cinco criterios pasan con mis propias ejecuciones. Quedan dos hallazgos NARANJA y no hay ROJO.

## Criterios de aceptación, verificados por mí
Todo se corrió desde cero en `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U6-T03/web/dashboard`, sha ab0887d, con `npm ci` previo.

| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Lo anterior sigue en verde | `npm run -s lint && npm run -s typecheck && npm run -s check:api && npx vitest run 2>&1 \| grep -c -E 'failed\|FAIL'` | pasa. lint, typecheck y check:api dan rc=0, el conteo es `0`, 5 archivos y 37 pruebas en verde |
| 2 | Flujo de login | `npx vitest run src/session src/pages/Login --reporter=verbose` | pasa. 15 pruebas (se exigen ≥8) y cubren todos los casos listados |
| 3 | Expiración y 401 | `npx vitest run src/session -t 'expira\|401'` | pasa. 3 pasan y 2 quedan filtradas por `-t`. No hay `.skip`, `.only` ni `.todo` en `src` |
| 4 | El token no persiste ni se filtra | los 3 comandos de CA-4 | pasa. grep da `0`, la prueba `no persiste` pasa y el grep sobre `dist` da `0` |
| 5 | Alcance | los 2 comandos de CA-5 | pasa. `git status --short \| wc -l` da `0` y los archivos fuera de alcance dan `0`. Merge-base `be21859`. El worktree quedó limpio tras mis ejecuciones (`dist` está ignorado) |

## Puntos declarados por el codificador
1. **`schema.gen.ts` (fdfdf92)**: correcto y dentro de alcance.
   - Solo toca `web/dashboard/src/api/schema.gen.ts` y la bitácora.
   - Tiene 96 líneas añadidas y 0 borradas, y son `/warm` y `/confirmations` de U6-T02.
   - `check:api` regenera y compara, y da rc=0.
   - No pude comprobar que `check:api` fallara antes de la regeneración. Es coherente con el diff, que solo añade.
2. **`git checkout -- .` accidental**: el estado final es correcto.
   - Mis ejecuciones sobre ab0887d dan 37 de 37 en verde.
   - El diff de los 4 archivos reaplicados es coherente con lo descrito en la bitácora.
   - La declaración es honesta. No es un defecto.
3. **Decisiones no especificadas**: razonables.
   - `safeNext` con `/\` es más estricto que la tarea y es correcto.
   - Un 429 sin `Retry-After` deja el botón habilitado y muestra el mensaje. Es aceptable, porque no hay tiempo de espera conocido.
   - El 401 global solo actúa si la petición llevaba token. Es lo correcto: un 401 de credenciales no es una sesión caducada.
   - Redirigir con `next` al expirar es coherente con la tarea.

Mutaciones que hice sobre una copia en el scratchpad (el worktree no se tocó):

| Mutación | Resultado |
|---|---|
| Quitar el chequeo `//` de `safeNext` | falla la prueba de `evil.example`, bien |
| Quitar ambas guardas de doble envío | fallan 2 pruebas, bien |
| Enviar siempre `otp` | falla 1 prueba, bien |
| Logout sin borrado local si el servidor falla | falla 1 prueba, bien |
| Quitar solo el `ref` `enCurso` | pasa, porque `disabled` basta. Es redundancia, no defecto |
| Quitar `setAuthToken(null)` de `borrarLocal` | **pasa 37 de 37**. Ver F-01 |

## Hallazgos

### F-01 · NARANJA · `src/session/session.test.tsx` (logout y expira) · el borrado del token del módulo no tiene prueba
Quité `setAuthToken(null)` de `borrarLocal` en una copia y las 37 pruebas siguen pasando. Las pruebas de logout, expiración y 401 solo comprueban que la ruta cambia y que `ana` desaparece del DOM. Ninguna lee de vuelta el estado que importa en seguridad: que tras cerrar la sesión no se siga enviando `Authorization: Bearer <token>`.

Sin esa lectura, un token viejo podría quedar vivo en memoria tras el logout o la expiración sin que nada falle. Eso va contra la promesa central de la tarea: sesión solo en memoria y borrada al salir.

Pedido: una prueba que, tras logout (con 500 del servidor), expiración y 401, haga una petición con `apiFetch` y compruebe con MSW que no lleva cabecera `Authorization`. Conviene añadir también el logout con `204`, que es el camino feliz del contrato y hoy no tiene prueba.

### F-02 · NARANJA · `src/session/SessionContext.tsx:94-105` · `expires_at` no parseable provoca un bucle de temporizadores
En `armar()`, `Date.parse(session.expiresAt) - Date.now()` da `NaN` si `expires_at` no es una fecha válida.
- `NaN <= 0` es falso, así que no expulsa.
- `Math.min(NaN, MAX)` es `NaN`, así que `setTimeout(armar, NaN)` equivale a 1 ms.
- Reproducido con el mismo algoritmo en Node: 457 re-armados en 500 ms, más el aviso `TimeoutNaNWarning`.

El contrato declara `date-time`, pero el dato viene de la red y el `LoginBody` del cliente no valida nada. El resultado es un bucle caliente, sin expulsar y sin avisar. Un valor que no sea una fecha debe tratarse como sesión no válida: expulsar o rechazar el login. Falta además una prueba con `expires_at` inválido.

## Amarillos (no bloquean)
- **A-1 · `src/session/next.ts`**: `safeNext('/\t/evil.example')` pasa el filtro. WHATWG elimina el tab y resuelve a `//evil.example`. No es un open redirect real, porque `navigate` usa `pushState` y este lanzaría `SecurityError` por origen distinto. Pero como defensa en profundidad conviene rechazar caracteres de control y comprobar `new URL(raw, origin).origin === origin`.
- **A-2 · `LoginPage.tsx` y `client.ts`**: `/login` no redirige si ya hay sesión, y un login desde ahí envía el `Bearer` viejo a `/auth/login`. Un 401 por contraseña errónea dispararía entonces el manejador global y cerraría la sesión previa. Es poco alcanzable (el login hace `replace`), pero conviene no enviar `Authorization` a `/auth/login`.
- **A-3 · `SessionContext.tsx`**: si el reloj del cliente va adelantado respecto a `expires_at`, la sesión se expulsa apenas se crea. Es una limitación conocida. No hay `expires_in` en el contrato, así que basta anotarlo en el README.
- **A-4 · `bitacoras/U6-T03.md`**: CA-5 figura como «pendiente». Debe quedar con salida real en la ronda 2.
- **A-5 · `session.test.tsx`**: la prueba `expira` solo exige `startsWith('/login')`. No comprueba el parámetro `next` ni que se hayan dejado de enviar peticiones. Se resuelve con F-01.

## Revisión adversarial sin hallazgo
- **Persistencia del token**: el token solo vive en el estado de React y en el módulo del cliente. Los greps de `src` y `dist` dan `0`. La prueba `no persiste` mira `localStorage`, `sessionStorage`, `document.cookie` y el DOM con un marcador único.
- **`ApiErrorNotice`**: solo muestra `code`, `message`, `retryAfter` y `requestId`, nunca el token.
- **Doble envío**: `disabled` más el `ref` más el chequeo `bloqueado` cubren el caso. La mutación sin ambas guardas lo demuestra.
- **Temporizadores**: el de expiración se limpia en el cleanup del `useEffect`, se re-arma por encima del máximo de `setTimeout` y no hay fugas de `setInterval`. El de cuenta atrás también se limpia.
- **Durante el logout**: se suprime el 401 con `cerrando`. Es correcto y se restablece en `finally`.
- **Rutas sin sesión**: redirigen sin hacer ninguna petición. La prueba usa `request:start` y es genuina.
- **Alcance**: sin desborde. Todo está en `web/dashboard/**` más la bitácora.

## Tareas candidatas (fuera de alcance)
- Validación en runtime de las respuestas de la API (fechas y esquema) en el cliente. Hoy `apiFetch<T>` confía en el tipo. Es candidata a U6-T01/T02 o a un refuerzo transversal.

## Rutas de transcripciones largas
- `/tmp/claude-1000/full.log` (salida completa de `vitest run`).

VEREDICTO: NO-VERDE
NARANJA|src/session/session.test.tsx (logout/expira/401)|El borrado del token del módulo no tiene prueba: quitar `setAuthToken(null)` deja 37/37 en verde
NARANJA|src/session/SessionContext.tsx:94-105|`expires_at` no parseable (NaN) provoca un bucle de temporizadores de 1 ms sin expulsar
