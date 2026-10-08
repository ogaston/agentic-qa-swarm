F-01: Corregido en este commit: _URL_CRED con lookbehind y {0,31}, _JWT con lookbehind, _ASSIGN sin prefijo cuadratico; pruebas de rendimiento 64 KiB (12 formas, cota 1.5 s) con mutacion roja M2/M3. El tope de LECTURA es del adaptador (T07): documentado en README; aqui el costo de redactar es lineal.
F-02: Corregido: se queda una sola pasada (antes de recortar); la prueba barre el desplazamiento por ambas fronteras y falla (mutacion M1) si se redacta despues de recortar.
F-03: Corregido: Authorization JSON/Basic, comilla escapada, url:p@ss@host; test_redact_forms_leave_no_tail con 2-3 variantes por tipo (mutaciones M5, M7 rojas; M6 roja pero sin confirmar el parche exacto).
F-04: Corregido: fullmatch en _URI_RE; paridad con casos "\n" y con FormatChecker (mutacion M4 roja).
F-05: Corregido en parte: type:array junto a minItems/maxItems (ajv real: 4 valid); tope de lectura documentado como de T07. URIs de entrada sin redactar: tarea candidata.
