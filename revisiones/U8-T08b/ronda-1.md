# Ronda 1 — U8-T08b

VEREDICTO: NO-VERDE

CA-1, CA-2, CA-3, CA-5 y CA-6 pasan en las ejecuciones del revisor. CA-4 no se cumple en lo que importa.

## Hallazgos

### F-01 · ROJO · CA-4 · `services/aqs-runner/cmd/aqs-runner/main_test.go:250-283` (`ordersHandler`)
La réplica del handler `POST /orders` no es literal: hardcodea `if !oversell && in.Qty > 2 { 409 }` (stock 2 para el producto 1). El baseline real (`demo/target-app/store.go:24-31`, `seedProducts`) es `{"cafe", 10}, {"te", 5}`; `PlaceOrder` rechaza con 409 solo si `stock < qty`. Con el `qty:5` del ejemplo `contracts/plans/examples/valid/flow-plan.body-y-sondeo.json` y de `TestOversellDefectoAlcanzableConCuerpo`, la app real sin defecto devuelve 201 (falso positivo) y con `oversell` también 201: el plan no distingue defecto de no defecto.
Arreglo: réplica con stock real (producto 1, stock 10; 409 cuando `qty > stock`); `qty` > 10 (p. ej. 11) en la prueba y en el ejemplo `valid`; citar las líneas reales incluido `store.go:24-31`. Barrido de clase: toda aparición de `qty:5` y «stock 2» (ejemplo valid, `TestOversellDefectoAlcanzableConCuerpo`, `TestBodyEnviaJSONConContentType` solo como transporte, prompt, bitácora).

### F-02 · AMARILLO · `agents/agent-planner/src/agent_planner/k6/render.py:26-40`
El fallo cerrado de k6 lanza `ValueError` en vez de `FlowValidationError(fid, ...)`. No bloquea (`publish_flows` no está cableado), pero conviene el error tipado.

### F-03 · AMARILLO · `bitacoras/U8-T08b.md` (CA-6 y CA-4)
CA-6 pega el primer comando sin salida; afirma «citada por archivo:línea» sobre una réplica que no corresponde. Corregir al arreglar F-01.

## Candidatas (fuera de alcance)
- C-1: soporte de `body`/sondeo en `k6/template.js`, o decisión de que k6 no se use para esos flujos.
- C-2: la fixture de U8-T08 debe saber que el sondeo solo relaja el ensayo (los runners siguen estrictos).
- `demo/target-app` sin pruebas de handler con store falso, para importar el handler real en vez de replicarlo.
