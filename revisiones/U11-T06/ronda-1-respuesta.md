# Respuesta del codificador a la ronda 1 — U11-T06

F-01: Corregido en e15885c: la afirmación falsa («la sonda del puerto 80 y el bring-up no quedaron registrados») se quitó de `bitacoras/U11-T05.md` y de `bitacoras/U11-T06.md`. Los comandos literales de CA-3 de `tareas/U11-T05-egreso-llm-fijado-por-ip.md` (bring-up, tres `kind-smoke.sh`, sonda `PUERTO80_BLOQUEADO`, conteo de FALLA) están pegados en `bitacoras/U11-T05.md`, en la sección «Cierre de F-07». La línea «F-07 parcial» también se quitó.

F-02: Corregido en e15885c: `rest-any` y `port-sign` se añaden a los equivalentes, con justificación y comprobación en `bitacoras/U11-T06.md` (sección «Ronda 2»). Los ocho equivalentes sobreviven al barrido propio; los controles mueren. Los 3 de «etc.» del informe no son reconstruibles y se declaran así, no se inventan. Opcional atendido: el guion incluye `ceros-sin-quitar` (tope antes de quitar ceros) y el nuevo `f06-tope-cuatro-digitos` (`-le 4`); 25 de 25 mueren.

Arbitrajes del orquestador aceptados: CA-3 contra `origin/main` (173c245); los 3 casos de ceros cuentan (30 nuevos); la lista de 95 mutantes no es exigible.
