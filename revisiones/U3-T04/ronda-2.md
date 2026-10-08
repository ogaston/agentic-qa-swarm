# Ronda 2 — U3-T04

VEREDICTO: NO-VERDE

Los 8 criterios de aceptación pasan con mis propias ejecuciones, imagen incluida. F-01, F-02, F-03 y F-05 están resueltos. Queda en pie un hallazgo NARANJA: F-04, el validador manual de URIs. Esto es la ronda 2 de 3.

## Criterios de aceptación, verificados por mí (sha ead9804)
| # | Criterio | Resultado |
|---|---|---|
| 1 | `pytest -q`; `-k dataset \| grep -c PASSED` | pasa: 122 passed en 3.7 s (≥ 40); 20 (≥ 12) |
| 2 | ajv real (`ajv-cli@5.0.0`, `ajv-formats@3.0.1`) | pasa: 4 `valid rc=0`, 5 `invalid rejected=0`, `jq` = `true`. Ya no salen avisos `strictTypes` |
| 3 | `demo_redaction` y grep de patrones | pasa: antes 7, prompt 0, reporte 0, REDACTED 1 y 2, repo 0 |
| 4 | `-k redact -v` | pasa: sin FAILED ni SKIPPED. Incluye tipos, formas, idempotencia, binario, recorte y rendimiento |
| 5 | `-k 'injection or verdict_guard or cites_only'` | pasa: 10 PASSED |
| 6 | límites, almacén, evento y servidor | pasa: 17 PASSED, grep README = 3 |
| 7 | `python -m agent_reporter.precision` | pasa: `{"evaluated": 7, "correct": 7, "precision": 1.0}`, rc 0 |
| 8 | imagen, higiene, alcance | pasa: `65532:65532`, 1, 0, ok, 0, 0, 0. `pyproject.toml` fuera del diff, así que `addopts` está intacto. El contenedor ejecuta `redact_secrets` correctamente |

El worktree quedó limpio (`git status --short | wc -l` = 0). Mis mutaciones corrieron sobre una copia en el scratchpad.

## Verificación de cada hallazgo anterior

**F-01 (cuadrático), RESUELTO.**
- Barrí 40 formas a 64 KiB con un temporizador de 20 s: `a`, `a.`, `token`, `password`, `eyJ`, `eyJa.`, `password="`, `password="\\`, `authorization=`, `a://`, `x://a:`, `bearer`, `-----BEGIN`, entre otras. El máximo fue 0.04 s; ninguna llegó a 1 s.
- Mutaciones con `-k performance`:
  - `_URL_CRED` sin lookbehind ni cota, 6 failed.
  - `_JWT` sin lookbehind, 1 failed.
  - Quitar solo el lookbehind de `_URL_CRED`, o solo la cota `{0,31}`, deja 12 passed. Cada protección por sí sola basta; es redundancia, no un agujero.
  - Con el prefijo viejo `[\w.-]{0,32}` en `_ASSIGN` quedan 12 passed. Era lineal con constante alta, no cuadrático, así que es coherente.
- El tope de lectura queda documentado en el README como asunto de T07. Lo acepto.

**F-02 (una pasada, antes de recortar), RESUELTO.**
- La prueba barre el desplazamiento en 0..1299 sobre el final de la cabeza y en 0..699 sobre el inicio de la cola, con `max_object_bytes=520`. Comprobé la aritmética: cabeza y cola de unos 245 bytes, por lo que el cruce real cae dentro del rango.
- Mutación redactar después de recortar: `test_redact_secret_split_by_truncation_never_survives` falla (1 failed), y `test_redact_secret_in_result_json_is_redacted` también en la variante sin redactar el objeto.
- Sin fuga en ningún desplazamiento con una sola pasada, salvo en las mutaciones.

**F-03 (formas), RESUELTO.** Probé más de 45 variantes a mano.
- Authorization: JSON con `Basic`, comillas simples, `Authorization=` y `"Authorization" : "Token …"` salen redactadas.
- Comilla escapada, doble y barras finales: sin cola.
- `postgres://user:p@ss:w@rd@host` queda limpio.
- Mutaciones con `-k forms`:
  - Quitar comilla en Authorization, 1 failed.
  - Comilla doble escapada, 1 failed. Queda atribuida con certeza a `test_redact_forms_leave_no_tail[password]`.
  - Password de URL con `@`, 1 failed.
- Comilla simple escapada (`password='it\'s-tail'`): el código la maneja, pero esa mutación sobrevive (9 passed). Ver F-06.

**F-05, RESUELTO en lo comprometido.** `type: array` quitó los avisos de ajv. El tope de lectura queda documentado. Las URIs de entrada sin redactar en el prompt quedan declaradas como candidata.

## Hallazgos

### F-04 · NARANJA · `report_model.py:_URI_RE` y `tests/test_reporter.py::URI_CASES` · El validador manual de URIs sigue siendo más laxo que el contrato, y la prueba lo afirma al revés
- El `fullmatch` cierra el caso `"\n"`: ajv y la prueba están de acuerdo en los 12 casos fijos.
- La afirmación de la prueba es falsa. El comentario dice «nunca es más laxo que jsonschema+FormatChecker» y `assert not (hand_ok and not py_ok)` solo se evalúa sobre las 12 URIs de `URI_CASES`.
- Fuzz que corrí contra ajv-formats real (Ajv2020 8.20.0 + ajv-formats 3.0.1), 2948 URIs:

| Comparación | Resultado |
|---|---|
| manual más laxo que ajv | 154 casos |
| manual más estricto que ajv | 0 casos |
| manual más laxo que jsonschema + FormatChecker | 156 casos |

- Ejemplos aceptados por el manual y rechazados por el contrato: `s3://x%zz`, `s3://%`, `s3://%4`, `s3://[`, `s3://]`, `s3://x[y`, `https://x:y`.
- La clase de caracteres `[A-Za-z0-9\-._~:/?#\[\]@!$&'()*+,;=%]*` no valida porcentajes ni corchetes.
- Efecto real y reproducible: `POST /v1/report` con `evidence_uris: ["s3://[/x/y/result.json"]` devuelve **500 `internal`**, no el 422 `no_evidence` del contrato.
  - Causa: `validate_evidence_uris` lo acepta, y `DirEvidenceReader._rel` falla en `urlsplit` con `ValueError: Invalid IPv6 URL`.
  - Lo comprobé con `handle_report`: `(500, {'error': 'internal'})`.
  - La tarea fija `422 | 502 | 504`.
- Corrección sugerida: validar por RFC 3986 de verdad (percent-encoding `%[0-9A-Fa-f]{2}`, corchetes solo en la autoridad, etc.). Y la prueba de paridad debe correr el fuzz contra `FormatChecker` y contra casos fijos de ajv-formats, no 12 URIs.
- Además, `_rel` debería capturar `ValueError` como `EvidenceUnavailable`, y `reporter.py` mapear a `NoEvidence` lo que no cumple el contrato.

### F-06 · AMARILLO · varios, no bloquea
- Una comilla simple escapada no está probada (M6b sobrevive con 9 passed). Añadir `password='it\'s-tail'` a `FORMS`.
- Valor con comilla sin cerrar (`password="unterminated-secret` o con `'`) sale intacto. Ya ocurría en la ronda 1. Es plausible en logs truncados por el ejecutor.
- Regresión menor del lookbehind de `_JWT`: `xeyJ…` o `session-eyJ…` pegado a un carácter base64url ya no se redacta. `token=eyJ…` y `Bearer eyJ…` sí se redactan.
- `https://u:pa/ss@host/`, con `/` en la contraseña, queda en claro.

## Tareas candidatas
- Ampliar patrones fuera de la lista de la tarea: `bearer:`, `Bearer abc` (menos de 4 caracteres), `Cookie`/`Set-Cookie`, `ASIA…`, `github_pat_…`, `sk-…`, `pwd=`.
- Redactar credenciales y query en las URIs de entrada antes del prompt (declarada por el codificador).
- Escáner dedicado de secretos.
- Incluir `report.schema.json` en `contracts/validate.sh`.

VEREDICTO: NO-VERDE
NARANJA|agents/agent-reporter/src/agent_reporter/report_model.py:_URI_RE|Validador manual más laxo que el contrato en 154 URIs (`s3://x%zz`, `s3://[`); `POST` con `s3://[…` da 500 en vez de 422; la prueba afirma lo contrario con solo 12 casos
