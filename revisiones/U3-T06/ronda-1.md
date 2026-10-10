# Ronda 1 — U3-T06

VEREDICTO: NO-VERDE

Worktree `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U3-T06`, sha 7542764. Terminé con el worktree limpio (`git status --short` da 0). Corrí con `PYTHONDONTWRITEBYTECODE=1` y `__pycache__` borrado. Las mutaciones las apliqué en copias bajo el scratchpad, con `PYTHONPATH` apuntando a la copia.

## Criterios de aceptación, verificados por mí
Los 7 comandos pasan con el seed aleatorio de una corrida. Varían con el seed: CA-1, CA-3, CA-4 y CA-5 en agent-reporter fallan de forma intermitente (ver F-01 y F-02).

| # | Criterio | Resultado de mi corrida |
|---|---|---|
| 1 | CA-1 | `agent-planner passed=6 seeds=1 failed=0`; `agent-reporter passed=6 seeds=1 failed=0`. **Intermitente en reporter, F-01/F-02.** |
| 2 | CA-2 | Planner: `Falsifying example … flow_id='00000'`, y las dos últimas líneas son idénticas. Reporter: `run_id='00000'`. Pasa (con la salvedad de F-07). |
| 3 | CA-3 | 157 passed / 0 en planner; 140 passed / 0 en reporter, en unos 24 s cada uno. Pasa. |
| 4 | CA-4 | `test_pbt_generator_coverage PASSED` en ambos con el seed de esa corrida. **Falla con ~30 % de los seeds en reporter, F-01.** |
| 5 | CA-5 | Las 6 mutaciones (a-f) mueren. Detalle abajo. |
| 6 | CA-6 | `1` y `1`; PBT.md de 39 y 40 líneas; coincidencias 3 y 2. Pasa. |
| 7 | CA-7 | Secretos `0`; los dos `compileall` dan ok; `git status` da 0. Fuera del filtro quedan solo `redact.py` y `reporter.py`, que la tarea permite ampliar con regresión fija. |

**Mutaciones (seed 1, `timeout 120`).** Las mató la propiedad que aparece entre paréntesis.
- Las seis de CA-5:
  - (a) mató `validator_accept_implies_invariants`.
  - (b) mató `k6_render_extract_roundtrip`, solo por `s1.isascii()`.
  - (c) mató `redact_removes_seeded_secrets…` ("sobrevive un fragmento de jwt").
  - (d) mató `redact_removes…` (idempotencia).
  - (e) mató `report_validator_accept_implies_invariants`.
  - (f) mató `evidence_truncation…`.
- Mutaciones propias que mueren:
  - planner: sin tope `max_flows`, sin chequeo de `flow_id` duplicado, sin escape de `<`, `>=` en el tope de tokens, `sort_keys=False`, `match` en vez de `fullmatch`, sin tope de pasos.
  - reporter: sin lista blanca de URIs, `sin-hallazgos` aceptado con flujos no pasados, sin redacción de private_key / url / github, tope total ignorado, sin cabeza, bandera `truncated` falsa, `_AWS` demasiado amplio (lo mata la propiedad de texto limpio).
- Control sin mutar (no-op): 6 passed.
- **Superviviente.** Sin el chequeo `run_id` en `validate.py`, la mutación sobrevive con `PBT_SEED=1` y muere con los seeds 2 a 5. Ver F-06.

**Arreglos de producción.**
- Cada regresión falla sin su arreglo, probado por separado. Con `redact.py` de `main`: 4 failed de las 7. Con `reporter.py` de `main`: 3 failed.
- El arreglo de `redact.py` no reintroduce costo cuadrático. Medí 64 KiB con las 12 formas de T04 más 22 formas nuevas (`token\"`, `Authorization\": \"`, `\nBearer `, `eyJ.` y otras). Todas dan unos 10 a 40 ms, y al duplicar la entrada el tiempo se duplica (ratio ~2).
- La idempotencia se mantiene en las pruebas de T04 (la suite pasa) y en la propiedad 7, salvo F-02.
- Valoración del alcance: ampliar a `redact.py` y `reporter.py` es legítimo, porque la tarea lo prevé para defectos reales con regresión fija.
- `_truncate` está bien arreglado: no supera el tope y no parte caracteres UTF-8.

## Hallazgos

### F-01 · ROJO · `agents/agent-reporter/tests/test_pbt_reporter.py:166-169` (`test_pbt_generator_coverage`) · La prueba de cobertura falla con ~30 % de los seeds
`PBT_SEED=7` falla dos veces seguidas con la misma salida:
```
AssertionError: ['aceptado_sin-hallazgos']
```
- Seeds que fallan: 7, 9, 11, 12, 17 y 21. Pasan: 1 a 6, 8, 10, 13 a 16, 18 a 20. Son 6 de 20.
- Como CA-1, CA-3, CA-4 y CA-5 usan un seed aleatorio, la CI y los criterios fallan una de cada tres corridas.
- Causa: el generador casi nunca produce una respuesta válida con veredicto `sin-hallazgos` aceptada, es decir, evidencia con todos los flujos pasados y una respuesta de clase `valid` con ese veredicto. 500 sorteos no bastan para ver esa clase.
- La coder midió con un solo seed y no vio el defecto.
- Hace falta sesgar el generador hacia esa clase, o construir el caso a propósito.
- Planner: los seeds 7, 9, 11 a 17 pasan (más el aleatorio de la primera corrida), así que no es intermitente.

### F-02 · ROJO · `redact.py` + `gen.py:secret_text` · La propiedad 7 halla un defecto real que no se arregló
Con `PBT_SEED=21`, `test_pbt_redact_removes_seeded_secrets_idempotent_bounded` falla. Ejemplo reducido:
```
{"msg": " password=00000000\n\npassword: 00000000 ", "n": 1}   (con \n escapados en JSON)
-> {"msg": " password=[REDACTED:secret_assignment] 00000000 ", "n": 1}
```
- El valor crudo de la primera asignación (`[^\s"',;&]+`) incluye `\`, así que se come `00000000\n\npassword:` y la segunda clave queda dentro del valor. El segundo secreto sobrevive.
- Es un defecto del mismo tipo que los 3 que sí se registraron. Falta la regresión fija y el arreglo.
- La bitácora dice "reporter con 4000 ejemplos → ok". Es una búsqueda con un solo seed.
- Es otra causa de intermitencia en CA-1, CA-3 y CA-5.

### F-03 · NARANJA · `redact.py:23-28` (`_BEARER`, `_JWT`) · El arreglo del defecto 3 es incompleto y el generador no lo ve
- El arreglo solo cubre los escapes `\n \r \t \b \f`. Un secreto precedido por cualquier otro escape `\uXXXX` o por un carácter no ASCII (json.dumps con `ensure_ascii=True`, por defecto) sigue sin redactarse.
- Lo comprobé con `redact_secrets` sobre `json.dumps({"m": pre + "Bearer 00000000 x"})` y un JWT con `pre` en `\x1b`, `é`, `日`, `\x00`, `\x7f`: filtran el bearer y el JWT. La excepción es `ensure_ascii=False` con `é`, `日` o `\x7f`, que sí se redacta.
- Con `{"m": "éBearer DDDD4444"}` el valor queda en claro.
- También con ANSI crudo: `\x1b[31mBearer CCCC3333` se queda tal cual, porque la `m` pega la palabra.
- Causa del hueco en la prueba: `secret_text` siempre pone un separador (`" ", "\n", "\t", ", ", "; ", " (", "\n\n"`) justo antes de cada secreto. Nunca coloca un secreto detrás de un carácter Unicode o de control, así que el generador no puede encontrar esta clase.
- Es una laguna del generador (PBT-07) y del arreglo.
- Se pide: ampliar el generador (secreto tras Unicode y tras control) y completar el límite izquierdo (`\\u[0-9a-fA-F]{4}`), con su regresión fija.
- El hueco ya existía antes de T06, pero la tarea reclama contextos arbitrarios.

### F-04 · NARANJA · `tests/gen.py:evidence_blob`, `test_pbt_reporter.py:112-125` · La propiedad 9 esquiva un defecto real
- La coder lo declara como "tarea candidata", pero la tarea manda arreglar en la ronda lo que una propiedad halle.
- Evidencia con bytes no UTF-8: `prepare("r", [uri], M(b"\xff"*100), Limits(max_object_bytes=100, max_total_bytes=100))` produce un `content` de **300 bytes**. `decode(errors="replace")` expande cada byte inválido a 3 bytes. El tope `REPORTER_MAX_TOTAL_BYTES` se triplica y el prompt crece (1404 caracteres).
- La propiedad 9 afirma "el prompt nunca supera `REPORTER_MAX_TOTAL_BYTES`". Se salvó restringiendo el generador a UTF-8 válido.
- Quedó anotado en la bitácora (esto es honesto), pero sin ejemplo reducido ni regresión.
- Se pide: arreglar (recortar después de decodificar, o decodificar antes de recortar) con regresión, o dejar la propiedad con evidencia de bytes arbitrarios.

### F-05 · AMARILLO · `test_pbt_planner.py:test_pbt_k6_render_extract_roundtrip` · Solo `isascii()` mata la mutación (b)
- Es un requisito añadido por la coder. El round-trip por sí solo no ve `ensure_ascii=False`, porque `extract_flow` lee el mismo literal.
- La tarea promete "cualquier Unicode"; la aserción fija en la prueba una propiedad que el código ya tenía como decisión (`ensure_ascii=True`). Es aceptable y está documentado en la bitácora.

### F-06 · AMARILLO · `test_pbt_validator_accept_implies_invariants` · Poder de detección dependiente del seed
- Sin la comprobación de `run_id`, la mutación sobrevive con `PBT_SEED=1` y muere con 2 a 5. Los 200 ejemplos tardan unos 1,8 s, y con ese seed el sesgo de generación no llegó a `wrong_run`.
- Además, la propiedad nunca exige que la clase `valid` sea aceptada: un validador que rechace todo pasaría. Solo lo cubre `resp_aceptada` en la cobertura.
- Conviene afirmar `kind == "valid" → acepta`.

### F-07 · AMARILLO · `test_pbt_demo.py` · La demo no prueba la reproducción por seed
El contraejemplo reducido es siempre `'00000'`, con cualquier seed, así que "mismo seed, mismo contraejemplo" se cumple trivialmente. Usar `database=None` está bien. Para demostrar la reproducción por seed bastaría una propiedad cuyo contraejemplo dependa del orden de sorteo.

### F-08 · AMARILLO · Entorno e higiene
- Los `.venv` fabricados copiando site-packages quedan fuera del control de versiones, que es lo correcto. La `.pth` apunta a una ruta absoluta del worktree.
- `test_repo_hygiene.py` recorre `.hypothesis/` (en `agent-reporter/.hypothesis` y `agent-reporter/tests/.hypothesis`). Hoy solo contiene `unicode_data` (gz, binario, se salta con `UnicodeDecodeError`). Una base de ejemplos fallidos sería binaria. Es un riesgo menor, y la coder ya lo anotó.
- El tiempo de suite es de unos 20-24 s para `-k pbt` y 24-30 s completa por servicio (en paralelo sube a 30-40 s), bajo el límite de 60 s.

## Nota de higiene pedida por el coordinador
- En el worktree T06, `grep -rnE 'AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9]{36}' agents contracts` (sin `.venv`, `__pycache__`, `.pytest_cache`) da **0** coincidencias.
- Los literales de `test_redact_regression.py` (`AUTH`, `BEAR`, `JWT`, la clave `pass`+`word`) están partidos por concatenación. No hay ningún `AKIA…` ni `ghp_…` con forma real en `PBT.md`, `gen.py` ni las pruebas.
- Los literales cortos (`"AKIA"`, `"ghp_"` en `test_redact.py`) son de T04 y no encajan en el patrón completo.
- Las pruebas de higiene de ambos servicios pasan con los archivos de T06 presentes: planner 1 passed, reporter 2 passed.
- No reproduje el fallo de CI de T05, porque ese literal está en `agents/eval`, que no existe en este worktree.
- Para el merge combinado: cualquier literal tipo `AKIA` de otra tarea que caiga en `agents/` romperá `test_repo_hygiene.py` del reporter. No es defecto de T06.

## Tareas candidatas (fuera de alcance)
- Convertir la prueba de higiene en un escáner único y compartido que excluya `.hypothesis/` y `agents/eval/` según política, o que parta los literales de prueba.
- `_truncate` y las marcas en bytes no UTF-8: política única de recorte (si F-04 no se acepta en esta ronda).

VEREDICTO: NO-VERDE
ROJO|agents/agent-reporter/tests/test_pbt_reporter.py:166-169 (CA-1/CA-3/CA-4)|test_pbt_generator_coverage del reporter falla con ~30 % de los seeds (clase aceptado_sin-hallazgos casi nunca sale)
ROJO|redact.py + gen.py:secret_text (CA-1/CA-5)|La propiedad 7 halla un defecto sin arreglar (seed 21: password=…\n\npassword: … deja un secreto) y falla de forma intermitente
NARANJA|redact.py:23-28 y gen.py:secret_text|Arreglo del defecto 3 incompleto: bearer/JWT tras \uXXXX, ANSI o Unicode siguen filtrando; el generador no coloca secretos tras esos caracteres
NARANJA|gen.py:evidence_blob / test_pbt_reporter.py:112-125|Propiedad 9 esquiva un defecto real (evidencia no UTF-8 triplica el tope); sin arreglo ni regresión
