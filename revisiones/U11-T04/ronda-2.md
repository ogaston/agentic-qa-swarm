# Ronda 2 — U11-T04 (informe del revisor, persistido por el orquestador; resumen fiel del hand-back)

VEREDICTO: NO-VERDE (1 ROJO, 0 NARANJA, 2 AMARILLO). Los dos NARANJA de la ronda 1 quedan cerrados.

HEAD 88f2050, hash de la tarea 5a0b833…, base tarea/U11-T01 (07c8bd8). Venv nuevos; mutaciones en un `git archive`; worktree limpio.
CA-1 pasa (IGUALES, 5 OK, 6 OK(rechaza)). CA-2 pasa (37 passed, semilla 20261025). CA-3 pasa (sin FAIL; vet OK). CA-4 pasa (4 passed; FIXTURES_AL_DIA). CA-5 FALLA: alcance = 1 (F-07).

## Hallazgos
### F-07 · ROJO · CA-5 / alcance · services/go-run-controller/README.md
La ronda 2 añadió 14 líneas (sección «flow_id: patrón y despliegue») a un archivo fuera de las rutas permitidas de CA-5 y del «Dentro» de la tarea. Es consecuencia de F-06 de la ronda 1 (se pidió que el aviso «figure en las notas»). El contenido es veraz: `DecodeStoredPlan` devuelve «plan guardado inválido» (runctl/planstore.go:74) y la afirmación de `'a\n'` es cierta.
Salidas: mover/quitar el texto (agents/agent-planner/README.md y la bitácora están permitidos) o que el orquestador amplíe la lista del CA y lo registre.

### F-08 · AMARILLO · agents/agent-planner/tests/test_flow_id.py (`test_flow_id_pbt_alfabeto_con_sustitutos`)
La propiedad con alfabeto de sustitutos casi nunca llega a la rama del hash (con `min_size=36` casi todo se vuelve `-` y el id limpio no pasa de 35). Con `strict` en lugar de `surrogatepass` la PBT falló con 3 de 8 semillas; las 3 pruebas explícitas matan la mutación de forma determinista. Conviene mezclar `[a-z0-9]` con sustitutos en el alfabeto.

### F-09 · AMARILLO · bitacoras/U11-T04.md líneas 154 y 167
Las secciones de la ronda 1 mantienen la afirmación errónea de que `test_uri_parity_with_jsonschema_format_checker` falla por un «fallo preexistente de entorno». La corrección de la línea 174 es cierta (venv limpio: pasa; reporter 325 passed).

## Reproducción de los hallazgos de la ronda 1
- F-01 cerrado: 25 ids a mano (sustitutos `\ud800`/`\udc00` al principio/medio/fin, emoji, solo sustitutos, NUL, U+2028, combinantes largas, surrogateescape, `a\n`, `İ`/`K`/`ß`/`ａ`/`ﬃ`): ninguno lanza, todos en patrón ≤35, idempotentes y estables entre procesos (PYTHONHASHSEED distintos); `\ud800` y `\udc00` dan hashes distintos; `plan()` y `POST /v1/plan` con 11 ids así devuelven 200 normalizado. Propiedad propia (30000 ejemplos) y de planes con colisión (5000) pasan.
- F-02 cerrado: con la constante en `{0,34}` y `{0,36}` falla la prueba de paridad (también `test_pbt_generator_coverage`, derivado de la misma constante); las dos copias del esquema son idénticas.
- F-03 cerrado (`regexp.MustCompile(plan.FlowIDPattern)`, sin ciclo, paridad del runner lee el esquema). F-04 cerrado (un ejemplo inválido nuevo falla nombrándolo en Go y Python). F-05 cerrado (id de 86 caracteres, colisión `Flujo_A`/`flujo a` → `flujo-a`, `flujo-a-2`, lote de 7). F-06 cerrado en contenido (jsonschema acepta `'a\n'`, el regexp de Go y ajv lo rechazan).

## Fuzz diferencial (993 ids, semilla 20261026)
ValidateFlowPlan, BuildJob (run real `run-20d9efb7096b73d9`), flowIDRe, ajv 2020 (993 archivos), validate_plan_dict y FLOW_ID_RE.fullmatch aceptan exactamente los mismos 126 ids: 0 discrepancias. jsonschema solo (sin fullmatch) difiere en 7 ids terminados en `\n` (caso documentado; plan() normaliza antes de validar).

## Regresiones
Planner 244 passed (7 deselected por marcadores); controlador sin FAIL (U8 y U9 incluidos); agents/eval 102; agent-reporter 325 (venv limpio).

## Candidatas
- Alinear flow_id en openapi/control-plane.yaml:392 y rehearsal.schema.json; unificar el ID_RE de k6/store.py; que el servidor del planner cierre en fallo ante excepciones inesperadas de plan().

VEREDICTO: NO-VERDE
