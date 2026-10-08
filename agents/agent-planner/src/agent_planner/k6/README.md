# agent_planner.k6 — flujos inspectables

Cada flujo de un `FlowPlan` se convierte en un script k6 (JavaScript) estándar:
`flows/<run_id>/<flow_id>.k6.js`. Es una plantilla fija (`template.js`, `// aqs-k6-template: v1`)
más una constante `const FLOW = {...};` con el flujo como JSON. Nada del plan se interpola como código.

Es la copia auditable del plan: el runner de U2 ejecuta los mismos pasos con `HTTPStepsExecutor`.

## Ejecutar un script a mano

    k6 run -e BASE_URL=http://127.0.0.1:8080 flows/<run>/<flow>.k6.js

Sin k6 instalado (imagen fijada):

    podman run --rm --network host -v "$PWD:/w:z" -w /w docker.io/grafana/k6:0.55.0 \
      run -e BASE_URL=http://127.0.0.1:8080 /w/flows/<run>/<flow>.k6.js

El script falla (umbral `checks`) si algún `expect_status` no coincide, y aborta con `fail()`
si `BASE_URL` falta o no es http(s).

## Validación (antes de publicar, todo o nada)

1. el plan valida contra `contracts/plans/flow-plan.schema.json`;
2. el script es exactamente la plantilla v1 (hash fijado) y `extract_flow(script) == flow`;
3. `k6 inspect` (binario `K6_BIN` o podman) termina con 0.

Generar ejemplos: `python -m agent_planner.k6 render-examples --out DIR`.
