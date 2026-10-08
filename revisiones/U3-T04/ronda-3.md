# Ronda 3 — U3-T04

VEREDICTO: VERDE

Los 8 criterios de aceptación pasan con mis propias ejecuciones, imagen incluida. F-04 está cerrado, F-06 corregido y no queda ningún ROJO ni NARANJA. Vale la pena fusionarlo, y el humano fusiona.

## Criterios de aceptación, verificados por mí (sha acf6447)
| # | Criterio | Resultado |
|---|---|---|
| 1 | `pytest -q`; `-k dataset \| grep -c PASSED` | pasa: 127 passed en 4.1 s (≥ 40); 20 (≥ 12) |
| 2 | ajv real (`ajv-cli@5.0.0`, `ajv-formats@3.0.1`) y `jq` | pasa: 4 `valid rc=0`, 5 `invalid rejected=0`, `true` |
| 3 | `demo_redaction`, grep de patrones antes/prompt/reporte, grep del repo | pasa: antes 7, prompt 0, reporte 0, REDACTED 1 y 2, repo 0 |
| 4 | `-k redact -v` | pasa: 48 PASSED, 0 FAILED, 0 SKIPPED |
| 5 | `-k 'injection or verdict_guard or cites_only'` | pasa: 10 PASSED |
| 6 | límites, almacén, evento, servidor | pasa: 19 PASSED, 0 SKIPPED; grep README = 3 |
| 7 | `python -m agent_reporter.precision` | pasa: `{"evaluated": 7, "correct": 7, "precision": 1.0}`, rc 0 |
| 8 | imagen, higiene, alcance | pasa: `65532:65532`, 1, 0, ok, 0, 0, 0; `pyproject.toml` no tocado, así que `addopts` está intacto |

Pruebas de runtime sobre la imagen:
- `LLM_PROVIDER=fake` con `REPORTER_ENV=prod` no arranca: `LLM_PROVIDER=fake prohibido con REPORTER_ENV=prod`.
- `s3://[/x` se rechaza y una URI válida de la plataforma se acepta.

El worktree quedó limpio (`git status --short | wc -l` = 0). Mis mutaciones corrieron sobre una copia en el scratchpad.

## F-04 · validador de URIs: CERRADO

**Reconocimiento.** El codificador tiene razón. `s3://x:y` y `https://x:y` los **acepta** ajv-formats; en el corpus congelado figuran como `True`. Mi lista de la ronda 2 los puso como ejemplos de «más laxo que ajv» por error. Solo eran más laxos que el checker de Python (rfc3986-validator), que sí los rechaza. El resto de mi conclusión se sostiene. Los demás ejemplos de esa lista sí los rechaza ajv: `s3://[`, `s3://x%zz`, `s3://%`, `s3://%4`. El núcleo del hallazgo (154 URIs más laxas que ajv, y el 500 con `s3://[`) fue real. Lo que no se sostiene es la atribución de esos dos ejemplos a ajv.

**El corpus congelado coincide con ajv real.** Regeneré las 2948 URIs de `tests/fixtures/uri_ajv_corpus.json` con mi `fuzz.js` y ajv 8.20.0 + ajv-formats 3.0.1. El mismo generador y la misma semilla dan `fixture == ajv real: True`, con 0 diferencias.

**Fuzz nuevo, con semillas y átomos distintos.** Son 6006 URIs sobre autoridad, puerto, `%`, corchetes, `?`, `#`, unicode, `..`, userinfo y similares, contrastadas con ajv real y con `jsonschema + FormatChecker`.

| Comparación | Resultado |
|---|---|
| validador manual más laxo que ajv | 0 |
| validador manual más laxo que jsonschema + FormatChecker | 0 |
| validador manual más estricto que ajv | 304 |

- Los 304 más estrictos son userinfo, puerto no numérico y hosts raros como `https://u:p@h/` o `s3://@`. Es una decisión consciente: la plataforma no los emite.

**Sin rechazos de URIs reales.** Acepta todas estas formas:
- `s3://aqs-evidence/runs/<run_id>/flow/result.json`
- run_id UUID y ULID
- `https` con puerto
- `https` con query presignada (`X-Amz-Credential=…%2F…`, `#frag`)
- `s3://b/a%20b/c.json`
- todas las URIs de los ejemplos y del dataset

**POST con `s3://[/x/y/result.json`.**
- Por HTTP real (`make_server` + `urllib`) devuelve `422 {"error":"no_evidence"}`.
- Con el validador desactivado, el lector lanza `InvalidEvidenceUri` y se mapea a `NoEvidence`, también 422.
- `s3://b/%2e%2e/x` da 502 `evidence_unavailable`, sin traversal, porque el lector no decodifica.

**Mutaciones, todas en rojo:**

| Mutación | Resultado |
|---|---|
| permitir `%` suelto | 2 failed |
| permitir `[` y `]` | 2 failed |
| varios `#` | 2 failed |
| puerto no numérico | 1 failed |
| `match` en vez de `fullmatch` | 8 failed |
| sin mapeo a `NoEvidence` | 1 failed |
| sin captura de `ValueError` en `_rel` | 1 failed |

## F-06: CORREGIDO
- `password='it\'s-tail'` sale redactada. La mutación que quita el escape de la comilla simple da 1 failed en `test_redact_forms_leave_no_tail[password]`. La de la comilla doble también da 1 failed, así que las dos quedan atribuidas por separado. En la ronda 2 la simple sobrevivía.
- Comilla sin cerrar (`password="unterminated…`, `'…`, `{"password":"x` al final de línea, `Authorization="…`, `api_key: '…`, `secret: "a b c`) se redacta hasta el fin de línea.
  - Una mutación que deshabilita `uq` rompe la prueba (1 failed).
  - `token="` solo y `password=""` quedan intactos, correctamente, porque no hay valor.
- Idempotencia mantenida en todas las variantes que probé.
- Sin regresión de rendimiento. Corrí 40 formas a 64 KiB con temporizador (`redos2.py`) y cero superan 0.3 s. Añadí `password="`, `password='`, `secret="\`, comillas alternas, una línea larga de 65 000 bytes con comilla abierta y 6000 saltos con `password="`: todas ≤ 0.012 s.

## Barrido de clase
- No quedan regex `^…$` con `.match` en `src/`.
- Las pruebas del servidor y del lector cubren el mapeo de errores.
- Los patrones de redacción mantienen costo lineal, tanto por cotas como por lookbehind.

## AMARILLO (no bloquea)
- Un JWT pegado a un carácter base64url (`xeyJ…`, `session-eyJ…`) no se redacta por el lookbehind. La contraseña de URL con `/` (`https://u:pa/ss@host/`) queda en claro. Ambos están declarados por el codificador como candidatas.
- El validador es deliberadamente más estricto que ajv con userinfo, hosts IPv6 y puerto no numérico. Está documentado en el código.

## Tareas candidatas
- Ampliar patrones fuera de la lista de la tarea: `bearer:`, `Bearer` con menos de 4 caracteres, `Cookie`/`Set-Cookie`, `ASIA…`, `github_pat_…`, `sk-…`, `pwd=`.
- Redactar credenciales y query de las URIs de entrada antes del prompt.
- Escáner dedicado de secretos.
- Incluir `report.schema.json` en `contracts/validate.sh`.
- Tope de lectura del adaptador de evidencia (T07).

VEREDICTO: VERDE
