# Respuesta a la ronda 2 — U2-T02
F-01: Corregido en f191eb8: la intención de lanzar (Started) se persiste antes de lanzar, con tope absoluto de 3 por fase; el handoff se persiste antes de alertar; barrido en la bitácora (seis fases, handoff, publisher) con una prueba por camino. Rojo: disco caído = 20 lanzamientos por fase; verde: 0 (y exactamente 3 si solo falla el Save posterior).
F-02: Corregido en f191eb8: ErrPersist en el avance normal no mata la corrida ni hace handoff (se reintenta); aqs_persist_errors_total y /readyz no-listo mientras falle el Save. Las pruebas TestPersistErrorOnPendingExitIsNotADenial y TestPersistErrorOnAdvanceDoesNotFailRun fallan con la mutación M5 y con la equivalente en el avance (ejecutadas, rojo y verde en la bitácora).
F-03: Corregido en f191eb8: propiedad con handoffs sin duplicados (uno por fase), tope 3 incluido rehearse, fallos de rehearse y del almacén aleatorios.
F-04: Corregido en f191eb8: rehearsal.* exige Launched[rehearse]; TestRehearsalEventBeforeLaunchIsDropped.
F-05: Corregido en f191eb8: una llamada al gate por paso con el gate caído en resetting; TestResettingGateDownOneGateCallPerTick.
(b) rehearsal.passed fuera de secuencia: aceptado como candidata (timeout de espera, U2-T04); no se tocó.
