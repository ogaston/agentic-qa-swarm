# U2-T04 — respuesta a la ronda 1
F-01: Corregido en el commit de esta ronda: `InClusterConfig` salió a `run()` (`inClusterClient`); `buildConfig(c, cs kubernetes.Interface, ...)` elige en UNA rama fakes o reales (sin sobrescribir); `TestBuildConfigRealWiresEveryRealPort` (clientset `kubernetes/fake`) afirma `*WarmClient`, `*RealPhases` con Rehearse/Warm/Reset/Artifact, `Results` = mismo lanzador y ningún Fake de fase/warm. W1-W5 mueren (ver bitácora).
F-02: Corregido en el commit de esta ronda: `TestResultFailedCounterWithoutConditionIsFailed` (muere la mutación a passed).
F-03: Corregido en el commit de esta ronda: casos separados State/ResetVerified en Ensure, `TestWarmDeployPostRunIDMismatch`, `TestDefaultTimeoutsAndEnsureOwnDeadline` (afirma 5 s de fase y 130 s de ensure). La presencia de `reset_verified` en `decodeWarm` queda como está (cierra igual): sin caso nuevo, candidata menor.
F-04: Corregido en el commit de esta ronda: `TestLoadConfigRunEnvVariantsRejectFake` (`PROD`, ` Production `, etc.); la mutación sin ToLower muere.
F-05: No es un defecto del código: la reconciliación de CA-2 (0 Jobs con plan roto) y de CA-4 (grep TOKEN vs automountServiceAccountToken) corresponde al archivo de la tarea, que corrige el orquestador; no toqué tareas/.
(b) ensure: Corregido: `WarmClient.EnsureTimeout` (130 s por defecto) con contexto propio; el resto de llamadas siguen en 5 s.
