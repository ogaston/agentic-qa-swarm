# Respuesta a la ronda 1 — U1-T05

F-01: Corregido en la ronda 2 (SHA al final de este archivo en el commit): firmas alteradas en cada posición, truncadas y extendidas (`TestPBT_SignatureTampered`), SHA largos/con basura, registros y repos inválidos, tag de 129 y con `/` final (`gen.UnresolvableEvent`), `refs/headsX` y afines (`gen.GitHubRejected`); batería de 69 mutantes, 0 supervivientes no equivalentes (tabla en la bitácora).
F-02: Corregido: `variants` incluye los siete valores de H-1 y `TestPBT_ParserAgreesOnEveryFieldVariant` los recorre; H-1 sigue sin arreglo en producción (fuera de alcance, `TestPBT_Limit_*` y tarea candidata).
F-03: Corregido: `check` itera claves ordenadas.
F-04: Corregido: `Notification` y `ConfirmationReceipt` tienen propiedades propias y `Free` incluye la cadena vacía (con `NonEmpty` donde el esquema exige minLength 1).
F-05: Corregido: cada propiedad tiene su `TestPBT_Fixed_*` y los `PBT.md` lo reflejan.
