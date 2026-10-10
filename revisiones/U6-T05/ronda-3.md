# Ronda 3 — U6-T05

VEREDICTO: VERDE

Corrí todo en el worktree /home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U6-T05 (HEAD bfb8c1b, código en 707497f). Las mutaciones y sondas las hice en una copia en el scratchpad. El worktree sigue limpio (0 líneas en `git status --short`).

No escribí el informe en disco porque soy solo lectura y mis instrucciones prohíben crear archivos de informe. Este mensaje es el informe; si lo quieren en `revisiones/U6-T05/ronda-3.md`, que lo guarde el orquestador.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Lo anterior sigue en verde | `npm run -s lint && npm run -s typecheck && npm run -s check:api && npx vitest run 2>&1 \| grep -c -E 'failed\|FAIL'` | `0`. 10 archivos y 81 pruebas pasan, 2,2 s. |
| 2 | Corrida y sondeo (≥8 pruebas) | `npx vitest run src/pages/Run` | 15 pasan: 12 de RunPage y 3 de RunsPage, por prefijo. Sin `failed`. |
| 3 | Mis corridas (≥3) | `npx vitest run src/pages/Runs` | 3 pasan. |
| 4 | Estado del warm (≥6) | `npx vitest run src/pages/Warm` | 10 pasan. |
| 5 | Alcance | `git status --short \| wc -l` y `git diff --name-only $(git merge-base HEAD origin/main) \| grep -v -E '^(web/dashboard/\|bitacoras/U6-T05\.md\|revisiones/U6-T05/)' \| wc -l` | `0` y `0`. El merge-base es 94fc8e8. |

La suite completa dura 2,2 s con 81 pruebas, que es plausible: usa relojes falsos y MSW, y no depende de estado externo.

## Cierre de F-01 (NARANJA de la ronda 2): cerrado
Hice las mutaciones en la copia aislada, con la base en 81/81 verdes.
- **`false &&` delante del aviso de RunPage.tsx:76.** Falla 1 prueba: «lectura buena y luego 503: conserva la línea de tiempo y avisa del dato desactualizado» (80 pasan, 1 falla). Queda cubierto.
- **Quitar `!noEncontrada` del aviso.** Falla «lectura buena y luego 404: no muestra el aviso de dato desactualizado». Es el caso 404 sin aviso y también está cubierto.
- **Quitar el `Math.min` del tope.** Falla «Retry-After sin tope: la espera se limita a 300 s».
- **Desactivar `if (restaMs > 0) programar(restaMs)`.** Falla «volver a la pestaña no salta una espera de Retry-After pendiente».

Las cuatro mutaciones las mata exactamente una prueba, la esperada. Las pruebas nuevas comprueban el aviso por texto exacto y la línea de tiempo por `aria-current="step"`, así que no solo miran que algo exista.

## Revisión adversarial del diff de la ronda (b68089c..HEAD, 4 archivos en web/)
`usePolling.ts` introduce `ESPERA_MAXIMA_MS=300_000` y `esperaHasta`. Escribí sondas descartables sobre el hook en la copia aislada.
- **Temporizadores colgados:** no hay. Con `Retry-After:1` y `maxFailures` 5, el sondeo se detiene en 5 llamadas y `vi.getTimerCount()` es 0. `parar()` sigue limpiando el temporizador en `programar` y `ejecutar`.
- **Parada tras `maxFailures`:** intacta. `alCambiarVisibilidad` sigue sin reanudar si `detenido` o `agotado`, y `refrescar` los reinicia.
- **Reinicio tras éxito:** intacto. El éxito pone `fallos=0` y reprograma con el intervalo normal.
- **Pausa de `Retry-After`:** con la pestaña oculta y vuelta a visible a mitad de la espera, programa solo la espera restante. No pide de inmediato.
- **Tope de 300 s:** con `Retry-After:3600` pide a los ~300 s, no a las horas.
- **Fallo sin `Retry-After`, ocultar y volver antes del intervalo:** espera lo que falta y no pide al instante (1 llamada, y la 2.ª a los 3 s). Es coherente con la regla y no rompe ninguna prueba.
- **Cambios en README, JSDoc y RunPage.test.tsx:** el README documenta la regla y el JSDoc coincide con el código. La doble línea en blanco (F-04) está eliminada.

## Hallazgos
No hay ROJO ni NARANJA.

### F-05 · AMARILLO · web/dashboard/src/hooks/usePolling.ts · `esperaHasta` no se limpia tras un éxito o un `refrescar()` manual
`esperaHasta` solo se asigna en el `catch`. Escenario que reproduje con una sonda:
1. Un 429 con `Retry-After:60` fija `esperaHasta` a now+60 s.
2. El usuario pulsa «Actualizar» (`refrescar()`), la petición tiene éxito y se retoma el intervalo normal.
3. Si ocultan y vuelven a mostrar la pestaña dentro de esos 60 s, `alCambiarVisibilidad` ve `restaMs>0` y reprograma para dentro de 59 s.
4. Resultado: se pierde el sondeo normal de 3 s hasta que termine la espera antigua.

Es un caso de borde raro, con datos aún válidos y sin pérdida de seguridad. Arreglo trivial: `esperaHasta = 0` en la rama de éxito y en `ejecutarRef.current`. No bloquea.

## Tareas candidatas
Ninguna nueva.

## Rutas de transcripciones largas
Ninguna. Las sondas y mutaciones están en el scratchpad de la sesión: `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/34f01c05-e8aa-4201-b219-51a4905daf5f/scratchpad/m/`.

VEREDICTO: VERDE
AMARILLO|web/dashboard/src/hooks/usePolling.ts|esperaHasta no se limpia tras éxito/refrescar manual (borde menor)
