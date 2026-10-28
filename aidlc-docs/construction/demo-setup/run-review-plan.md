# Revisión de la corrida de demo

Alcance autorizado el 2026-10-28: comprobar el resultado de la corrida terminada por el usuario. Consultas de lectura, con autenticación local; sin imprimir credenciales ni lanzar corridas o resets.

- [x] 1. Identificar la confirmación y el SHA de la corrida más reciente del usuario. `run-d3fdad3c9118b08a`, confirmado a las 15:07:07Z, SHA `460b66b36e0db2ad12e7a7437fbfa43039541692`.
- [x] 2. Comprobar estado terminal, razones de fallo, reporte, resultados y evidencia disponible. Run done/sin-hallazgos, reporte sin findings; siete logs de runner con ok:true y estados esperados.
- [x] 3. Contrastar SHA con la imagen del warm y comprobar ready/reset_verified, Jobs y pods. SHA coincide, ocho Jobs completos sin fallos, warm ready/reset_verified=true, modo de defecto off, pods sin fases problemáticas ni contenedores en espera.
- [x] 4. Guardar evidencia resumida, actualizar el estado y comunicar el resultado con sus límites. Evidencia: `code/run-d3fdad3c9118b08a-verification.md`. Se valida esta corrida sana; no el caso oversell ni toda la aplicación.
