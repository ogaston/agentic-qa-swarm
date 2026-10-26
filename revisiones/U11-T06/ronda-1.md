# Ronda 1 — U11-T06

VEREDICTO: NO-VERDE

Los tres criterios de aceptación pasan con mi propia ejecución. Queda un hallazgo NARANJA en la bitácora de U11-T05 (F-01), porque el codificador afirma algo falso sobre el repo. Queda también un AMARILLO sobre los equivalentes. No hay ROJO.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | La prueba cubre los casos listados y pasa | `bash scripts/test/u11-egress-port-test.sh; echo rc=$?` | pasa: rc=0, termina en `OK`. 55 líneas OK, 0 FALLA. 30 casos `caso_ok/caso_err` nuevos y 24 previos (25 líneas OK previas en origin/main, medidas con la prueba de origin/main) |
| 2a | Los 19 mutantes y los extras mueren, con guion re-ejecutable | `bash scripts/test/u11-egress-port-mutants.sh; echo rc=$?` | pasa: 24 MUERTO, 0 VIVO, 0 NO-APLICADO, `OK todos los mutantes mueren`, rc=0, 3 s |
| 2b | Desbordamiento cerrado | `bash -c '. scripts/kind/lib.sh; llm_endpoint_host_port https://h:18446744073709551696; echo "rc=$?"'` | pasa: `rc=1` y sin salida |
| 3 | Nada más cambió | los tres comandos de CA-3, con base `origin/main` (173c245) | pasa: `SINTAXIS_OK`, 0, 0, 0 |

Comprobaciones adicionales:
- El worktree quedó limpio tras mis corridas (`git status --short | wc -l` = 0).
- `scripts/kind/lib.sh` no cambia al correr el guion. El diff de `lib.sh` es solo el tope de dígitos. La regla (https 443, http 80, explícito) no se toca.
- Mis mutantes propios dieron este resultado:
  - Mueren: `-le 4`, `-le 20`, rango `>= 2`, rango `<= 65534`, tope antes de quitar ceros, y quitar `,,` del esquema.
  - Sobreviven y son equivalentes: `-le 6`, `-le 19`, quitar `digits=${digits:-0}` (solo cambia el mensaje de stderr), quitar `10#` (ya sin ceros a la izquierda), y la guarda de host vacío.
  - Los equivalentes que declara el codificador son correctos.
- Ningún mutante muere por causa colateral: 0 errores de sintaxis ni `unbound` en los que probé.

## Arbitrajes
1. CA-3 contra `origin/main`: es razonable. `origin/tarea/U11-T05` ya es ancestro de `origin/main`. Con la base literal (merge-base 33b5854) el filtro da 92, no 0, aunque el diff sea correcto. El codificador lo reportó sin cambiar el criterio, como debía. El criterio literal de la tarea queda mal especificado; eso es del orquestador.
2. Los 3 casos extra de ceros (`:00`, `:000443`, `:0000000001`): cuentan. Están en la misma prueba, dentro de alcance, y no cambian la regla. La lista de la tarea suma 27, no 30; es un defecto de conteo de la especificación. `:0000000001` ejercita el orden «quitar ceros, luego tope». Defiende un mutante («tope antes de quitar ceros») que yo comprobé que muere, pero que el guion no incluye (ver F-02).
3. Mutantes no reconstruibles: no viola ningún criterio. CA-2 se cumple con 19 mutantes reconstruidos por categoría a partir del texto de F-05, que da 4 + 7 + 2 + 1 + 1 + 4 = 19, más los extras. Es un hallazgo parcial: ver F-01 y F-02.

## Hallazgos
### F-01 · NARANJA · bitacoras/U11-T05.md:248-260 y bitacoras/U11-T06.md (resumen) · Cierre de F-07 con una afirmación falsa
El alcance 3 pedía pegar el comando literal de la sonda del puerto 80 y los comandos completos del bring-up. El codificador escribe que la sonda «NO quedó registrado ni en esta bitácora ni en el repo». Es falso:
- El comando está literal en `tareas/U11-T05-egreso-llm-fijado-por-ip.md:62-68`, en CA-3 de esa tarea: el bring-up completo con `bash scripts/kind/...` en cada paso y el `kubectl --context kind-aqs -n aqs-system exec deploy/agent-planner -- python3 -c '…socket.create_connection((u.hostname,80),timeout=5)…PUERTO80_BLOQUEADO'`.
- Verificado con `grep -rn PUERTO80`.
- La reconstrucción del bring-up («los cuatro últimos son los nombres de scripts/kind/…») también debía tomarse de ahí.
- Los informes `revisiones/U11-T05/ronda-2.md` y `ronda-3.md` confirman que la sonda se corrió desde el pod del planner.

Corrección: pegar los comandos de CA-3 de la tarea de U11-T05 como «comando de CA-3 de la tarea, ejecutado por el codificador y los revisores». Quitar la afirmación falsa y la línea «F-07 parcial» del resumen.

### F-02 · AMARILLO · bitacoras/U11-T06.md («Mutantes equivalentes») · Equivalentes incompletos
- Se listan 6 de los 11. `ronda-3.md:16` nombra seis: guarda de host vacío, rest-any, rest-no-anchor, print-newline, port-nodigits-class y port-sign.
- Faltan `rest-any` y `port-sign`, que son nombrables y justificables. Los demás de «etc.» no se pueden reconstruir.
- Los 6 listados incluyen dos propios (`10#` y `digits:-0`) que no son del revisor. Esos están bien justificados.
- La lista de 11 no está en el repo, así que el requisito literal de la tarea («lista los 11 equivalentes del revisor») es imposible tal cual. Es una brecha de especificación, no solo del codificador. Sí debió acotar con un barrido propio de equivalentes en vez de dejarlo declarado.
- Opcional: añadir al guion el mutante «cero antes del tope» (`digits=$port`, ya en la lista como `ceros-sin-quitar`) o un `-le 4`. Hoy el guion no cubre el tope contra el límite bajo.

## Tareas candidatas
- Corregir la especificación de U11-T06: el conteo de 27 vs 30 casos, y la base de CA-3 (`origin/tarea/U11-T05` ya fusionada, debe ser `origin/main`).
- Cuando se arbitre el hallazgo del sondeo, extraer los 11 equivalentes del barrido del revisor (guardarlos en `revisiones/`) para que el siguiente codificador no tenga que reconstruirlos.

VEREDICTO: NO-VERDE
NARANJA|bitacoras/U11-T05.md:248-260|Cierre de F-07 con afirmación falsa: la sonda del puerto 80 y el bring-up sí están literales en tareas/U11-T05-egreso-llm-fijado-por-ip.md CA-3
AMARILLO|bitacoras/U11-T06.md (equivalentes)|Equivalentes incompletos (6 de 11; faltan rest-any y port-sign)
