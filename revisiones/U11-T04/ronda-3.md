# Ronda 3 — U11-T04 (informe del revisor, persistido por el orquestador; resumen fiel del hand-back)

VEREDICTO: VERDE

HEAD 66f232f, hash de la tarea 29ca608c… (coincide), base tarea/U11-T01 = 07c8bd8. GOTOOLCHAIN=auto. Mutaciones en una copia; nada en segundo plano; worktree limpio.
CA-1 IGUALES, 5 valid OK, 6 invalid OK(rechaza). CA-2 37 passed. CA-3 todos ok. CA-4 4 passed y FIXTURES_AL_DIA. CA-5 VET_OK, FAIL=0, planner 244 passed, estado 0, alcance 0. (El planner necesita PYTHONPATH=agents/agent-planner/src o `pip install -e`; es de entorno, no del diff.)

## Hallazgos de la ronda 2, reproducidos
- F-07 (ROJO, de especificación) cerrado: con el hash nuevo el alcance de CA-5 da 0 (34 rutas dentro de la lista ampliada); el README del controlador (líneas 109-121) es veraz.
- F-08 cerrado: con `surrogatepass` la PBT pasa 8/8 semillas (20261025 y 1..7); con `strict` falla 8/8; `--hypothesis-show-statistics`: 500 válidos, 196 descartados por assume() (23 %), la rama del hash se ejerce.
- F-09 cerrado: las líneas 154 y 167 de la bitácora conservan el texto y llevan «[CORREGIDO en la ronda 2: …]».

## Regresiones
La mutación a `strict` falla 5 de 17 pruebas de test_flow_id.py (las 3 explícitas, la PBT y test_fixtures_planner_kind_pasan_el_validador). Diff de la ronda 3: solo tests/test_flow_id.py, la bitácora y ronda-2-respuesta.md; ningún código de producción. Planner 244, eval 102, reporter 325 (venv limpio), controlador sin FAIL.

## Hallazgo nuevo (no bloquea)
- F-10 · AMARILLO · tests/test_flow_id.py: `assume(len(_clean(raw)) > 35)` importa `_clean`, nombre privado de flow_id.py.

## Candidatas
- Alinear flow_id en openapi/control-plane.yaml:392 y rehearsal.schema.json; unificar el ID_RE de k6/store.py; que el servidor del planner cierre en fallo ante excepciones inesperadas de plan(); el nombre del Job de ensayo no comprueba el patrón (el revisor de la ronda 1 demostró que no puede reventar).

VEREDICTO: VERDE
