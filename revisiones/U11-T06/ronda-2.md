# Ronda 2 — U11-T06 (sha 0826214)

VEREDICTO: VERDE

Los tres criterios de aceptación pasan con mi ejecución, F-01 y F-02 están corregidos y no encontré regresiones.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | La prueba cubre los casos y pasa | `bash scripts/test/u11-egress-port-test.sh` | pasa. rc=0, última línea `OK`, 55 líneas OK, 0 FALLA. |
| 2a | Los mutantes mueren con un guion re-ejecutable | `bash scripts/test/u11-egress-port-mutants.sh` | pasa. 25 MUERTO, 0 VIVO, 0 NO-APLICADO, `OK todos los mutantes mueren`, rc=0, 3,1 s. |
| 2b | Desbordamiento cerrado | `bash -c '. scripts/kind/lib.sh; llm_endpoint_host_port https://h:18446744073709551696; echo "rc=$?"'` | pasa. Da `rc=1` y no imprime nada. |
| 3 | Nada más cambió | los tres comandos de CA-3, medidos contra `origin/main` (173c245) | pasa. `SINTAXIS_OK`, 0, 0, 0. |

- **Alcance de CA-3:** el worktree quedó limpio (`git status --short | wc -l` = 0).
- **Diff de `lib.sh`:** contra 173c245 es solo el tope de dígitos. La regla del puerto no cambia.
- **Cambio de la ronda 2 en código:** entre 5561ee4 y 0826214 solo cambia una línea de `u11-egress-port-mutants.sh`, el mutante `f06-tope-cuatro-digitos`. La prueba y `lib.sh` no cambian.

## Verificación de F-01 (NARANJA): corregido
- **Comparación literal:** comparé con `diff` las líneas 63-69 del CA-3 de `tareas/U11-T05-egreso-llm-fijado-por-ip.md` con `bitacoras/U11-T05.md:253-259`. Son idénticas. Solo cambia la valla de código, que está en otro sitio.
- **Bring-up y sonda:** el bring-up lleva `bash scripts/kind/…` en cada paso. La sonda `PUERTO80_ALCANZABLE`/`PUERTO80_BLOQUEADO` está completa, con el `kubectl exec deploy/agent-planner`. También están los tres `kind-smoke.sh` y el conteo de `^FALLA`.
- **Afirmaciones falsas:** `grep` no encuentra ya «NO quedó registrado» ni «F-07 parcial» en ninguna de las dos bitácoras.
- **Reconocimiento del error:** `bitacoras/U11-T06.md` ronda 2 reconoce el error y adjunta el `grep -rn PUERTO80`.
- **Frase imprecisa:** la bitácora dice «ejecutado por el codificador y los revisores». Los informes de revisión lo respaldan para la sonda, y no lo considero un defecto.

## Verificación de F-02 (AMARILLO): corregido
- **Equivalentes:** `rest-any` y `port-sign` están añadidos con justificación, hasta 8 equivalentes nombrados. Los 3 restantes del «etc.» se declaran no reconstruibles y no se inventan, lo cual es honesto.
- **Corridas propias:** apliqué cada mutante a una copia en el scratchpad con `LIB_SH` y corrí la prueba.
  - `rest-any`: sobrevive (rc=0), equivalente como declara el codificador.
  - `port-sign` (`>= 1` por `> 0`): sobrevive (rc=0), equivalente.
  - `-le 4`: muere (rc=1).
  - Control de puerto 0 (`>= 0`): muere (rc=1).
- **Mutante `-le 4`:** ya está en el guion como `f06-tope-cuatro-digitos`. `ceros-sin-quitar` también cubre el tope antes de quitar ceros.

## Hallazgos
Ninguno en pie (sin ROJO ni NARANJA).

### AMARILLO (no bloquea)
- `bitacoras/U11-T06.md`, resumen de la ronda 1: dice «6 equivalentes demostrados… el resto de los 11 no se puede reconstruir». La ronda 2 lo corrige con 8, pero el resumen no se actualizó. Es una incoherencia cosmética dentro del mismo archivo.

## Tareas candidatas
- Corregir la especificación de U11-T06, cosa del orquestador:
  - el conteo de 27 frente a 30 casos nuevos;
  - la base de CA-3 (`origin/tarea/U11-T05` ya está fusionada y debe ser `origin/main`);
  - la exigencia de «los 11 equivalentes del revisor», cuya lista no está en el repo.
- Guardar en `revisiones/` la lista de mutantes y equivalentes de los barridos del revisor.

VEREDICTO: VERDE
AMARILLO|bitacoras/U11-T06.md (resumen ronda 1)|Resumen dice 6 equivalentes; la ronda 2 ya documenta 8 (incoherencia cosmética)
