# Respuesta a ronda-2 — U3-T06 (ronda 3)
F-01: Corregido en el commit de la ronda 3: `_BEARER` con lookbehind ASCII `(?-i:(?<![A-Za-z0-9_]))`; `GLUE_ST` con `st.characters()`; barrido determinista de todo U+0000..U+FFFF (tests/test_redact_sweep.py) y regresion fija; roja (9 fallos del barrido) con el codigo anterior.
F-02: Corregido en el commit de la ronda 3: `(?-i:[nrtbf])` en `_ASSIGN`; regresion fija y barrido de todo `\X` ASCII; un `\n`/`\t` literal en minuscula dentro de un valor es corte aceptado y documentado en redact.py.
F-03: Corregido en el commit de la ronda 3: centinela en area de uso privado (U+E000.., planos 15/16), sin `next()` que pueda lanzar; si se agota, `ReportRejected("redaccion_sin_centinela")`; pruebas con los 256 U+24xx, U+2400..245F y agotamiento.
F-04: No es un defecto: aceptado por el revisor; la regresion fija protege la mutacion (test_second_raw_assignment_after_json_escaped_newlines_is_redacted y barrido).
F-05: Fuera de alcance: tarea candidata (idempotencia con secretos pegados sin separador, preexistente en main).
