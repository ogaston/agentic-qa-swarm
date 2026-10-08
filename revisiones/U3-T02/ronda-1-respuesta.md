F-01: Corregido: FLOW_ID_RE y _SAFE_ID ahora con fullmatch y sin ^/$; pruebas "f1\n", "\nf1", "a\n" y run_id "run-1\n"; la mutación al código anterior pone 2 pruebas en rojo (bitácora ronda 2).
F-02: Corregido: constante MAX_WORKFLOW_LEN (128) y HANDLER_TIMEOUT_S; MAX_WORKFLOW_LEN, MAX_BODY y HANDLER_TIMEOUT_S documentadas en la tabla del README; prueba de workflow largo.
F-03: Aceptada, sin cambio.
