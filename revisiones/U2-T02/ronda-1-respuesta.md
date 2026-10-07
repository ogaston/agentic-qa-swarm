# Respuesta a la ronda 1 — U2-T02
F-01: Corregido en 9d8a582: una fase agotada nunca se relanza (con FailReason y reintentos de reset agotados solo se reintenta la salida); prueba TestResetExhaustedWithGateDownNeverRelaunchesNorDuplicatesHandoff (rojo: 28 lanzamientos, 26 handoffs; verde: 3 y 1) y barrido TestEveryPhaseExhaustedWithGateDownLaunchesExactlyThree sobre deploy, infer, rehearse, run, reset y report.
F-02: Corregido en 9d8a582: rehearsal.* solo se aplica en rehearsing; fuera de él se descarta con log y aqs_events_dropped_total; pruebas TestRehearsalEventOutsideRehearsingIsDroppedAndNeverSetsFact y TestDroppedEventIsCounted.
F-03: Corregido en 9d8a582: propiedad TestPropertyAdvanceOnlyAfterAllowAndExactlyThreeLaunchCap (allow previo a cada avance, tope exacto 3) y pruebas de gate caído en resetting/agotamiento y de evento fuera de estado.
F-04: Corregido en 9d8a582: trace_id de la corrida en el contexto, una sola clave por línea; TestLogLinesHaveSingleTraceIDWithRunTrace parsea cada línea y cuenta claves (rojo: 2 veces y valor vacío; verde: 1 y el de la corrida).
F-05: Fuera de alcance: tarea candidata propuesta (HMAC del diario, ancla del último seq y validación de legalidad al reproducir).
F-06: Corregido en 9d8a582: head -c 48 en la función up de la bitácora.
F-07: Corregido en 9d8a582: ErrPersist distingue error de persistencia de denegación (se reintenta, no se detiene ni hace handoff); TestPersistErrorIsNotADenial. Hacer visible halted en GET /runs/{id} cambiaría el esquema Run: fuera de alcance, tarea candidata.
