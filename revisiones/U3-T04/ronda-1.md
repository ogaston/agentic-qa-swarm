# Ronda 1 — U3-T04

VEREDICTO: NO-VERDE

Los 8 criterios de aceptación pasan con mis propias ejecuciones. El veredicto es NO-VERDE por 4 hallazgos NARANJA, todos de fondo en `redactSecrets` y en la validación manual. No hay ROJOS.

## Criterios de aceptación, verificados por mí
Sha revisado: b4e05f2be9517f997f594165454d8b25f17a2926.

| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Suite verde y dataset | `pytest -q` (88 passed en 4.5 s); `-k dataset -v \| grep -c PASSED` | pasa (88 ≥ 40; 20 ≥ 12) |
| 2 | Esquema y ejemplos con ajv real | el bucle de CA-2 con `ajv-cli@5.0.0` y `ajv-formats@3.0.1`; `jq -e` | pasa (4 `valid rc=0`, 5 `invalid rejected=0`, `true`) |
| 3 | Secretos sembrados redactados | `demo_redaction --out`; grep de patrones antes/prompt/reporte; grep del repo | pasa (antes 7, prompt 0, reporte 0, REDACTED 2 y 1; repo 0) |
| 4 | `redact_secrets` | `-k redact -v` | pasa (10 tipos, idempotencia x10, binario, logs del dataset intactos; sin SKIPPED) |
| 5 | Logs como dato | `-k 'injection or verdict_guard or cites_only'` | pasa (10 PASSED) |
| 6 | Límites, almacén, evento | `-k 'budget or timeout or ...'`; grep README | pasa (17 PASSED; grep README = 3) |
| 7 | Precisión | `python -m agent_reporter.precision` | pasa: `{"evaluated": 7, "correct": 7, "precision": 1.0}`, rc 0 |
| 8 | Imagen, higiene, alcance | build, inspect, list-services, compileall, git status, diffs | pasa: `65532:65532`, 1, 0, ok, 0, 0, 0 |

- `pyproject.toml` no aparece en el diff, así que `addopts` está intacto.
- El worktree quedó limpio después de la revisión (`git status --short | wc -l` da 0). Mis mutaciones corrieron sobre una copia en el scratchpad, con `PYTHONPATH` apuntando a la copia.
- Mutaciones que probé, todas ponen una prueba en rojo:

| Mutación | Resultado |
|---|---|
| no redactar al publicar | 1 failed |
| quitar la regla de `bug` | 1 failed |
| quitar la regla de `sin-hallazgos` | 1 failed |
| quitar URIs ⊆ entrada | 2 failed |
| quitar `finding_id` único | 1 failed |
| quitar la comparación de hash | 1 failed |
| quitar el tipo JWT | 1 failed |
| quitar el escape de `<` | 1 failed |
| quitar la redacción de asignaciones | 1 failed |
| relajar `any(s != "passed")` | 1 failed |

- Dos mutaciones sobreviven con 88 passed: quitar solo la primera pasada de redacción y quitar solo la segunda. Ver F-02.

## Hallazgos
### F-01 · NARANJA · `redact.py:_URL_CRED` · Backtracking cuadrático, sin prueba de rendimiento
- El prefijo de esquema `[A-Za-z][A-Za-z0-9+.-]*://` no tiene cota. Cualquier tramo largo alfanumérico sin `://` cuesta O(n²).
- Medición: `redact_secrets(b"a"*n)` tarda 0.17 s con n=10 000, 0.70 s con 20 000, 2.43 s con 40 000 y **6.53 s con 65 536** (un solo objeto del tope por defecto, por ejemplo un blob base64).
- Escala igual con `token*N`, `password*N` y `eyJ*N`. `_JWT` y `_ASSIGN` también muestran cuadrático, aunque menor.
- La redacción corre sobre el objeto **completo antes de recortar** y `reader.get` no tiene tope de lectura. Con un objeto de 1 MiB el tiempo se vuelve inviable.
- Es una denegación de servicio sobre un `ThreadingHTTPServer`.
- La bitácora dice haber bajado 22 s a 0.9 s en 10 KB. Eso no es lineal, y `grep` de `tests/` no encuentra ninguna prueba de rendimiento o de tiempo. Hace falta una regresión.
- Acotar el prefijo de esquema (por ejemplo `{0,32}`) y, o bien un tope de lectura/redacción, o bien recortar antes de redactar por tramos con solape.

### F-02 · NARANJA · `tests/test_redact.py::test_redact_secret_split_by_truncation_never_survives` · Mutación superviviente real, y la prueba es vacua
- Pregunta del orquestador: la defensa en profundidad **no es aceptable** aquí. La segunda pasada (post-recorte) no cubre el caso que justifica la primera: un secreto cortado por el recorte.
- Reproduje la mutación quitando solo la redacción previa al recorte: `88 passed`.
- Con esa mutación, `prepare("r", [...], Limits(max_object_bytes=520))` sobre `"x"*225 + AKIA… + "y"*2000` deja `AKIA` y `ABCDEFGH` **en el prompt** (fuga real pre-LLM).
- La prueba nunca parte un secreto. Con `max_object_bytes=520`, cabeza y cola son de unos 245 bytes y el secreto, desplazado 500 bytes o más, cae entero en el trozo descartado.
- Hace falta una prueba que recorra el desplazamiento hasta que el secreto cruce la frontera de la cabeza y de la cola, y que falle sin la primera pasada.
- La segunda pasada es redundante: quitarla también da 88 passed. Decídase explícitamente cuál se queda, con una prueba que la distinga.

### F-03 · NARANJA · `redact.py` · Formas dentro de la lista de la tarea que dejan pasar el valor
Las dos primeras son las más serias.
- `{"Authorization": "Basic dXNlcjpwYXNzd29yZA=="}` sale intacto. `_AUTH_HEADER` exige `authorization[ \t]*:` sin comilla intermedia y `Authorization` no está en `_KEYWORDS`. La forma de cabecera JSON es la habitual en volcados de requests de una corrida de QA.
- `{"password":"pa\"ss-tail-secret"}` sale como `{"password":"[REDACTED:secret_assignment]"ss-tail-secret"}`. La comilla escapada corta el valor y la cola queda en claro, aunque el criterio nombra explícitamente «JSON `"password": "…"` incluido».
- `postgres://user:p@ss@host/db` sale como `...[REDACTED:url_credentials]ss@host/db`, con fuga de la cola de la contraseña. Menor.
- Ninguno está probado: `test_redact_each_type_leaves_no_value` solo cubre una forma por tipo.

### F-04 · NARANJA · `report_model.py:9` · Patrón `^…$` + `.match` repetido (el de T02/T03), con prueba de paridad débil
- `_URI = re.compile(r"^(s3|https)://[^\s]+$")` con `.match`: `$` admite un `\n` final.
- `validate_report` acepta `evidence_uris: ["s3://a/b\n"]` y `validate_evidence_uris` acepta `["s3://a/b\n"]`.
- ajv real con `ajv-formats` **lo rechaza** (`format: uri` en `/findings/0/evidence_uris/0`). El validador manual y el esquema divergen.
- La prueba de paridad (`test_report_schema_parity_hand_validator_vs_jsonschema`) solo compara los 9 ejemplos fijos y además usa `Draft202012Validator` sin `FormatChecker`, así que ni siquiera ve el `format: uri`.
- Juicio sobre el validador manual: es aceptable como decisión (la imagen no ve `contracts/`), pero la paridad hay que probarla con casos de borde y con formatos activados, no solo con los ejemplos.
- Corrección: `fullmatch` o `\Z`, más una prueba de paridad con `FormatChecker` y casos `"\n"`.
- `path` con `\n` final es aceptado por ambos, y es coherente porque el esquema usa `^/` sin ancla final.

### F-05 · AMARILLO · varios
- Las URIs de entrada van sin redactar al prompt, campo `"uri"`. Con `https://user:pw@host/...?token=…` el prompt las lleva en claro. Esas URIs vienen de la plataforma (U2), así que el riesgo es bajo.
- `ajv` avisa por `strictTypes`: falta `"type":"array"` junto a `minItems`/`maxItems` en `allOf/0/then|else`. Hoy es solo un `log` (CA-2 oculta stderr), pero rompe si `validate.sh` se endurece (`-strict=true`) al incluir el esquema.
- `reader.get` lee el objeto completo en memoria antes de recortar, aunque el criterio dice «lee cada objeto con tope». Es un puerto, así que el tope real es de T07. Conviene documentarlo.
- `ReportStore.get` y `store_unavailable` (502) no están en la tarea. Son necesarios para la lectura de vuelta, están documentados en README y bitácora, y quedan dentro de `agents/agent-reporter/`. Lo acepto.
- La tarea decía «`finding_id` con el patrón de `flow_id` de T02», pero `flow_id` en T02 solo tiene `minLength: 1`. `finding_id` hace lo mismo, así que es coherente.

## Tareas candidatas
- Ampliar patrones (fuera de la lista de la tarea):
  - `Cookie`/`Set-Cookie`
  - `ASIA…` (STS)
  - `github_pat_…`
  - `sk-…`
  - `pwd=`
  - `bearer:`
  - `secret =` con el valor en la línea siguiente
- Escanear con una herramienta dedicada de secretos.
- Incluir `report.schema.json` en `contracts/validate.sh`, ya en la bitácora.
- Redactar las URIs con credenciales o tokens de consulta en el prompt.

VEREDICTO: NO-VERDE
NARANJA|agents/agent-reporter/src/agent_reporter/redact.py:_URL_CRED|Backtracking cuadrático (64 KiB de letras = 6.5 s), sin prueba de rendimiento y redacción antes de recortar sin tope
NARANJA|tests/test_redact.py::test_redact_secret_split_by_truncation|Mutación superviviente: quitar la primera pasada da 88 passed y fuga AKIA al prompt; la prueba nunca parte el secreto
NARANJA|agents/agent-reporter/src/agent_reporter/redact.py|"Authorization" JSON con Basic, comilla escapada en "password" y url:p@ss@host dejan el valor
NARANJA|agents/agent-reporter/src/agent_reporter/report_model.py:9|^...$ con .match acepta "s3://a/b\n" (ajv lo rechaza); paridad solo con 9 ejemplos y sin FormatChecker
