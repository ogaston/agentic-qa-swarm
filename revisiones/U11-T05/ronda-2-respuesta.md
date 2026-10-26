# Ronda 2 — respuesta del codificador (U11-T05, ronda 3)

F-01: Corregido en el commit de la ronda 3 (ver bitácora «Ronda 3»): se añadieron `https://h:1`, `https://h:65535` (OK) y `https://h:0`, `https://h:65536` (error). Mutaciones del revisor: `>= 0` → la prueba FALLA con `https://h:0 salió h 0`; `<= 65536` → FALLA con `https://h:65536 salió h 65536`. Ambas restauradas con `cmp`.

F-02: Corregido en el commit de la ronda 3: el esquema se compara sin distinguir mayúsculas (`HTTPS://H` → `H 443`); el host solo admite `[A-Za-z0-9._-]+` o IPv6 entre corchetes. `https://h\nevil`, `https://h evil`, `https://h;x`, `https://$(x)` y `https://h(x)` devuelven rc=1 sin salida. Los casos existentes siguen en verde.

F-03: Corregido: la sensibilidad exige que la regla vieja dé EXACTAMENTE `h 80`; si falta python3, la prueba FALLA explícitamente. Pegada la prueba completa contra la regla vieja dentro de la función: FALLA (bitácora, «Prueba COMPLETA contra la regla VIEJA»); restaurada, verde.

F-04: Corregido: la bitácora tiene fechas `date -Is` en CA-3 y los comandos completos de la ronda; la salida de CA-4 se pega al final tras el commit.

Hash de la tarea actualizado en la bitácora: `20aa0625ba3435983f507c978bbe5122f6d445a0`.
