# Ronda 2 — U9-T03

VEREDICTO: VERDE

F-01 y F-02 de la ronda 1 están cerrados. Corrí yo mismo los 6 criterios de aceptación y todos pasan. No queda ningún ROJO ni NARANJA.

## Criterios de aceptación, verificados por mí (sha ad5fe98)
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Lo anterior sigue en verde | `npm run -s lint && npm run -s typecheck && npm run -s check:api`, y `npx vitest run 2>&1 \| grep -c -E 'failed\|FAIL'` | pasa: lint, typecheck y check:api con rc=0; el conteo da `0` |
| 2 | Ruta raíz | `npx vitest run src/App 2>&1 \| tail -n 3` | pasa: 3 de 3 |
| 3 | Reporte y plan | `npx vitest run src/pages/Run 2>&1 \| tail -n 3` | pasa: 37 pruebas de 37; 21 están en `RunPage.resultados.test.tsx` (≥12 nuevas) |
| 4 | CSP intacta | `npm run -s build >/dev/null && grep -c -E 'unsafe-inline\|unsafe-eval' dist/index.html` | pasa: `0` |
| 5 | Contra kind | `flock ${XDG_RUNTIME_DIR:-/tmp}/aqs-kind.lock bash -c 'bash scripts/kind/dashboard-smoke.sh ...'` | pasa: `rc=0`, 8 `OK` y ningún `FALLA`. Incluye `OK reporte-visible`. Log en `/tmp/rev2-u9t03-smoke.log` |
| 6 | Alcance | `git status --short \| wc -l` y `git diff --name-only $b \| grep -v <patrón> \| wc -l` | pasa: `0` y `0` |

## Cierre de hallazgos de la ronda 1

### F-01 (NARANJA) · cerrado
- **Cambio.** `RunPage.tsx:67-70` ya no depende de `ultimaFase` para decidir si pedir el plan. `alcanzoEnsayo` es verdadero si `state === 'failed'` o si el estado ya llegó a `rehearsing`. Si no hay plan, la API responde `plan_not_ready` y la vista muestra «Plan aún no disponible». `ultimaFase` se sigue usando solo en `Linea`.
- **Pruebas nuevas.** Hay tres: `failed` abierto directamente (plan pedido 1 vez y «Flujos ejecutados» visible), `failed` llegando desde `rehearsing`, y `failed` sin plan.
- **Mutación.**
  - Qué hice: en una copia fuera del worktree, que ya borré, devolví la condición a la dependencia de `ultimaFase`.
  - Resultado: `Tests 2 failed | 35 passed`. Fallan «failed abierto directamente…» y «failed sin plan…».
  - Conclusión: las pruebas detectan la regresión. La de «llegando desde rehearsing» pasa con ambas versiones, y es lo esperado.

### F-02 (AMARILLO) · cubierto
Pruebas añadidas para las cuatro ramas que faltaban:
- 404 de `reports` con otro código, que muestra «Sin acceso al informe».
- Error de red.
- Error genérico con el aviso común.
- 502 en el plan, con «Reintentar» que hace una segunda petición.

## Hallazgos nuevos
Ninguno.

El diff de la ronda 2 toca solo `RunPage.tsx`, su prueba, la bitácora y la respuesta del codificador. No empeora ninguna de las tres candidatas arbitradas.

## Tareas candidatas (fuera de alcance, ya registradas en la ronda 1)
- Cerrar la ventana de 404 tras la confirmación en el control plane.
- Limpiar o marcar la notificación `pending` de prueba que deja el smoke.

## Estado del worktree
Limpio después de la revisión: `git status --short | wc -l` da `0`. `shellcheck` no está instalado, así que no lo corrí.

## Rutas de transcripciones largas
- `/tmp/rev2-u9t03-smoke.log`

VEREDICTO: VERDE
INFORME: revisiones/U9-T03/ronda-2.md
