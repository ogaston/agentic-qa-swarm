# Ronda 1 — U9-T03

VEREDICTO: NO-VERDE

Los 6 criterios de aceptación pasan con mi propia ejecución. Queda un defecto NARANJA dentro del alcance. Es una corrida `failed` abierta directamente: el plan nunca se pide.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Lo anterior sigue en verde | `npm run -s lint && npm run -s typecheck && npm run -s check:api && npx vitest run 2>&1 \| grep -c -E 'failed\|FAIL'` | pasa: lint, typecheck y check:api sin error; el conteo da `0` (el `rc=1` es del propio `grep -c` al contar cero) |
| 2 | Ruta raíz | `npx vitest run src/App 2>&1 \| tail -n 3` | pasa: 3 de 3 |
| 3 | Reporte y plan | `npx vitest run src/pages/Run 2>&1 \| tail -n 3` | pasa: 30 pruebas, de ellas 14 nuevas (≥12) |
| 4 | CSP intacta | `npm run -s build >/dev/null && grep -c -E 'unsafe-inline\|unsafe-eval' dist/index.html` | pasa: `0` |
| 5 | Contra kind | `flock ${XDG_RUNTIME_DIR:-/tmp}/aqs-kind.lock bash scripts/kind/dashboard-smoke.sh` | pasa: 8 `OK`, incluido `OK reporte-visible`; sin `FALLA`; `rc=0`. Log en `/tmp/rev-u9t03-smoke.log` |
| 6 | Alcance | `git status --short \| wc -l` y `git diff --name-only $b \| grep -v ... \| wc -l` | pasa: `0` y `0` |

Las pruebas nuevas cubren lo que pide CA-3. Entre ellas: 1 solo `GET /reports` y ninguno más en 30 s, 0 antes de `done`, los 3 veredictos con texto distinto y el `<img onerror>` mostrado como texto. Las fixtures salen de `contracts/plans/examples/valid/`. Las pruebas de la ruta raíz comprueban la ubicación final (`currentLocation`), no un código de salida.

## Hallazgos

### F-01 · NARANJA · `web/dashboard/src/pages/Run/RunPage.tsx:67-69` · Corrida `failed` abierta directamente: el plan nunca se pide
El alcance dice «Corrida `failed`. Si hay plan, se muestra». `alcanzoEnsayo` se calcula con `ultimaFase.current`, un `useRef` que solo se llena con estados no fallidos vistos durante este montaje del sondeo. Cuando la página se abre o se recarga con la corrida ya en `failed`, `ultimaFase` es `null`, `indiceFase(null)` es `-1` y `PlanFlujos` queda con `activo=false`. El usuario no ve el plan de ninguna corrida fallida que abra después. El plan es la evidencia más útil en ese caso. El backend ya responde `404 plan_not_ready` cuando no hay plan, así que pedirlo es seguro.

Lo reproduje en una copia fuera del worktree, que ya borré. Añadí una prueba con `ruta('/api/runs/ID', [corrida('failed')])` y un plan válido. Salida: `× failed directo con plan disponible → expected +0 to be 1`. El plan se pidió 0 veces.

Hay un hueco de cobertura relacionado. Ninguna prueba nueva afirma que un `failed` muestre el plan. `failed sin report_uri` solo mira que no se pida `/reports`. `failed con report_uri` solo cuenta los `GET /reports`. Dos acciones:
1. Pedir el plan también cuando `state === 'failed'` sin depender de `ultimaFase`, o documentar y probar otro criterio.
2. Añadir una prueba que, para `failed` (cargado directo y llegando desde `rehearsing`), afirme que se pide el plan y se renderiza «Flujos ejecutados».

### F-02 · AMARILLO · `Resultados.tsx` (`PostMortem`) · Ramas sin prueba
- `404` con un código distinto de `report_not_ready` (rama «Sin acceso al informe»).
- Estado `0` (error de red).
- Error genérico mostrado con `ApiErrorNotice`.
- `502` en el plan, que muestra «Reintentar».

No bloquea.

## Las tres candidatas del codificador
1. **404 transitorio de `GET /runs/{id}` tras confirmar**: es un defecto de plataforma (go-run-controller), fuera de alcance. Por su efecto en el dashboard conviene registrarla como tarea. `RunPage` marca el 404 como fatal (`isFatal`, comportamiento previo de U7) y detiene el sondeo con «Corrida no encontrada». Ese comportamiento no lo cambió este diff, así que no es defecto de esta tarea. El smoke ya tolera el 404 durante 2 minutos.
2. **Notificación `pending` de prueba que deja el smoke**: es un efecto secundario aceptable. El alcance pedía «confirmar una notificación de prueba». No pude comprobar el estado en el inbox porque el smoke baja la pila al terminar. Es candidata de limpieza o marca de prueba. No es defecto de esta tarea.
3. **«Familia» inexistente en `FlowPlanFlow`**: no es defecto. La vista muestra `name` y `flow_id`, y el esquema no tiene ese campo.

## Tareas candidatas (fuera de alcance)
- Cerrar la ventana de 404 tras la confirmación en el control plane. Que `POST /notifications/{id}/confirm` solo responda cuando `GET /runs/{id}` ya sea visible.
- Que el smoke, o el inbox, limpie o marque las notificaciones de prueba.

## Estado del worktree
Limpio después de la revisión: `git status --short | wc -l` da `0`. `dist/` y `node_modules` están ignorados por git. No hay `shellcheck` instalado, así que no lo corrí.

## Transcripciones largas
- `/tmp/rev-u9t03-smoke.log`

VEREDICTO: NO-VERDE
NARANJA|web/dashboard/src/pages/Run/RunPage.tsx:67-69|Corrida failed abierta directamente: el plan nunca se pide (y sin prueba de que failed muestre el plan)
INFORME: revisiones/U9-T03/ronda-1.md
