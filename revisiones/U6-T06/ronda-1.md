# Ronda 1 — U6-T06

VEREDICTO: VERDE

Hice la revisión en el worktree `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U6-T06`, en el sha 5d410b6. Los puertos 18500–18504 estaban libres al empezar. Corrí los cinco CA yo mismo, con los puertos por defecto y desde la raíz del worktree. El arbitraje H-1 (CSP solo en `preview`) lo apliqué como decisión ya tomada, no como defecto.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Lo anterior sigue en verde | `npm ci`, `lint`, `typecheck` y `vitest` en `web/dashboard`; `go test -count=1 ./...` en `services/ui-api` | pasa: `0` y `0`. Vitest: 11 ficheros y 102 pruebas, todas pasan. |
| 2 | Arranque y parada | `u6-up.sh`, `/healthz` en 18500–18503 y `/` en 18504, luego `u6-down.sh` y el mismo sondeo | pasa: `200 200 200 200 200`, y tras parar `000 000 000 000 000` |
| 3 | Recorrido completo | `u6-demo-local.sh \| grep -E '^(OK\|FALLA)'` y `ss` | pasa: 11 `OK`, ninguna `FALLA`, `rc=0` y `0` puertos escuchando. Tardó unos 5 s, con servicios reales y build. |
| 4 | CSP y build sin código en línea | `npm run build` y `grep` sobre `dist/index.html`; luego `u6-up.sh` y `curl -D -` a :18504 | pasa: `0` y `1`. `dist/index.html` solo tiene `<script type="module" src=...>` y `<link rel=stylesheet>` externos. |
| 5 | Ningún secreto en el repo ni en la salida | `grep` sobre `/tmp/u6.out`, `git status --short \| wc -l` y `git diff --name-only` fuera de lista | pasa: `0`, `0` y `0` |

La cabecera CSP servida por `preview` es exactamente la de la tarea, y también salen `X-Content-Type-Options: nosniff` y `Referrer-Policy: no-referrer`. Con `vite dev` en el puerto 18790 comprobé que sale `nosniff` y `no-referrer`, y que no sale CSP. Eso es lo que decidió el arbitraje.

## Hallazgos
No hay ROJO ni NARANJA.

### F-01 · AMARILLO · `bitacoras/U6-T06.md` (sección CA-5 del arbitraje) · evidencia incompleta
La bitácora deja `git status --short | wc -l` en "(ver cierre tras el commit)" y escribe "Verde tras el commit" sin pegar la salida. El resultado sí es cierto: mi ejecución dio `0`. Falta solo la salida pegada.

### F-02 · AMARILLO · `scripts/test/u6-up.sh`, `u6-down.sh` y `u6-demo-local.sh` · perilla `U6_PORT_BASE`
La tarea no la pide. El valor por defecto es 18500 y la perilla está documentada como solo para pruebas. No bloquea y no desborda el alcance. Si se mantiene, conviene que las pruebas de la tarea sigan usando los puertos por defecto.

### F-03 · AMARILLO · `web/dashboard/src/pages/Run/RunPage.tsx:99` · `style={{...}}` bajo `style-src 'self'`
La línea 99 usa un atributo `style` de React y es anterior a esta tarea. React lo aplica por CSSOM, que la CSP no bloquea, así que lo esperable es que funcione. Con `curl` no se puede probar que la app funcione bajo la CSP en un navegador real. La tarea ya lo reconoce como candidata C-95.

## Verificaciones adicionales (sin hallazgo)
- **Acotado al alcance:** el diff contra la base solo toca `scripts/test/u6-*.sh`, `u6_stubs.py`, `web/dashboard/vite.config.ts` y la bitácora. No hay cambios en servicios Go ni en agentes.
- **Estado leído de vuelta:** las comprobaciones leen el estado de vuelta contra los servicios reales. `confirmar-otra-vez-409` comprueba que la segunda confirmación no se acepta. `mis-corridas` busca el `run_id` en `/api/confirmations`. `logout-invalida-token` comprueba que `/api/auth/session` da 401 tras el logout.
- **Cableado y negativos:** `corrida-hasta-done` exige la secuencia exacta `deploying running done`. `sin-token-401` cubre dos casos: sin token y token inventado, que debe rechazar el stub del controlador.
- **Secretos:** la contraseña y los tokens quedan en ficheros 600 dentro de un `mktemp -d` 700. La contraseña no aparece en argv: `hash-password` la lee por stdin. Ni la salida del recorrido ni el log llevan secretos.
- **Limpieza:** `u6-down` mata por PID, borra el directorio de estado y el puntero, y es idempotente. El `trap` lo invoca siempre.
- **Rojo primero:** la bitácora documenta el rojo del script inexistente y el rojo de `csp-estricta` antes de tocar `vite.config.ts`.

## Estado final
Worktree limpio: `git status --short` da `0` líneas. Puertos 18500–18504 y 18790 libres, sin procesos huérfanos.

## Tareas candidatas
- `vite preview` y `vite dev` envían `Access-Control-Allow-Origin: *` por defecto (`cors: true`). No es un riesgo para este entorno loopback. Si el dashboard llega a servirse de verdad, conviene decidir `preview.cors` explícitamente. Queda como candidata para la tarea de servidor estático de producción, que está fuera de alcance aquí.
- C-95 (e2e con navegador) sigue siendo la única prueba real de la UI bajo la CSP.

## Rutas de transcripciones largas
Ninguna. Las salidas completas están en el cuerpo del informe.

VEREDICTO: VERDE
AMARILLO|bitacoras/U6-T06.md (CA-5 del arbitraje)|Salida de git status sin pegar en la bitácora
INFORME: revisiones/U6-T06/ronda-1.md
