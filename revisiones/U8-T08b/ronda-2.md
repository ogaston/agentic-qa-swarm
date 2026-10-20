# Ronda 2 — U8-T08b

VEREDICTO: VERDE

El revisor corrió todos los CA con `-count=1` sobre 6098df6: CA-1 (IGUALES, 191 passed, 4 paquetes Go ok), CA-2, CA-3 (5 PASS), CA-4 (sin defecto sale 0; con oversell sale 1), CA-5 (110 passed) y CA-6 (0, 0, 0). Worktree limpio.

## Hallazgos de la ronda 1
- F-01 (ROJO) resuelto: réplica con stock 10 (`store.go:24-31`), 409 si stock < qty (`store.go:244`), `qty` 11 en pruebas y ejemplo valid. Barrido: solo queda `"qty":5` en `go-run-controller/plan/validate_test.go:75`, caso de validez de esquema sin relación con el defecto.
- F-02 resuelto: `k6/render.py` lanza `FlowValidationError`.
- F-03 resuelto: bitácora con salida real de CA-6 y cita correcta.

Sin hallazgos ROJO ni NARANJA. Observación: la prueba de oversell ejecuta solo la primera venta (la réplica no descuenta stock); está documentado.

## Candidatas (fuera de alcance)
- C-1: `body`/sondeo en `k6/template.js`, o decidir que k6 no se use para esos flujos.
- C-2: la fixture de U8-T08 debe saber que el sondeo solo relaja el ensayo.
- Pruebas de handler en `demo/target-app` con store falso.
