# Respuesta a ronda 1 — U8-T08b

F-01: Corregido en 15d43de: réplica de `POST /orders` con stock 10 para el producto 1 (`demo/target-app/store.go:24-31`, `seedProducts`), 409 solo si `stock < qty` (`store.go:244`); `qty` pasa de 5 a 11 en pruebas, ejemplo valid, `rehearsal_test.go` y `test_flow_plan_probe.py`. Barrido de `qty:5`/«stock 2»/`> 2` sin resultados en `services/`, `agents/agent-planner/` y `contracts/plans/`.

F-02: Corregido en 15d43de: `k6/render.py` lanza `FlowValidationError(flow_id, "k6", motivo)`; la prueba espera ese tipo.

F-03: Corregido en 15d43de: bitácora Ronda 2 con la salida real del CA-6 (`0` y `0`) y la cita correcta de la réplica (`main_test.go`, `ordersHandler` → `demo/target-app/main.go:170-186`, `store.go:244`, stock de `store.go:24-31`).

Candidatas: C-1 y C-2 sin cambios; se añade la del reviewer (importar el handler real de `demo/target-app` con store falso) como candidata, no hecha.
