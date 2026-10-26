# Ronda 2 — U11-T02 (informe del revisor, persistido por el orquestador; resumen fiel del hand-back)

VEREDICTO: VERDE

HEAD 777c9bd (código de la ronda 2 en d084ec9), hash de la tarea fc487fcb…, worktree limpio. Scratchpad único. GOTOOLCHAIN=auto. CA-6 diferido a U11-T04/T05 (no exigido).

## Criterios (ejecución propia)
CA-1 SIN_APP/0. CA-2 BASELINE_IGUAL/2/PROCEDENCIA. CA-3 go-warm-manager ok + OPENAPI_IGUAL; runner SKIP sin variable, PASS con TARGET_APP_DIR=../demo/target-app (b97a719) y AQS_PLANNER_PYTHON, FAIL de compilación con directorio vacío.
CA-4 imagen fijada en el warm-app del overlay kind, no-latest rc=0, policies rc=0. CA-5 seis SINTAXIS_OK, 0, 4 (TARGET_REPO_FULL), u11-scripts-test.sh OK (casos a, b, b2, b3, c, d, d2, e, f, f', m, g). CA-7 DEMO_REAL, 2, cuentas 5/4/8, 0. CA-8 0/0/0/0.

## Hallazgos de la ronda 1, reproducidos
- F-02 resuelto: bloque «Estado conocido» (U11-T04/U11-T05), lo observado separado de lo esperado, tabla «Dónde» sin token interno con comandos que existen, Paso 2 edita el Dockerfile antes del commit (coincide con el Dockerfile real de b97a719), Paso 2b del segundo commit, tiempos «Pendiente de medir», sin valores sensibles.
- F-03 resuelto: los cuatro scripts toman TARGET_REPO_FULL de lib.sh; u8-target-app-kind.sh lee el SHA de kustomization.yaml. F-05 resuelto (probado contra la red real: GIT_TERMINAL_PROMPT=0, rama inexistente, red caída, SHA sin imagen → 404 en GHCR).
- F-04 resuelto salvo F-06: 15 mutaciones sobre copias de los scripts reales, 13 detectadas.
- app-objetivo.md §4 documenta que hay que añadir el repo también a WARM_ALLOWED_IMAGE_PREFIXES (U11-T03).

## Compatibilidad
`git merge-tree` T02+T03 limpio; en el árbol fusionado: go test de go-warm-manager ok, kustomize con WARM_ALLOWED_IMAGE_PREFIXES=ghcr.io/ogaston/demo-target-app-qa-swarm,ghcr.io/ogaston/agentic-qa-swarm/target-app y el warm-app dentro del primer prefijo, policies OK.
La fusión con tarea/U11-T04 da conflicto add/add solo en los archivos de tarea (no código); lo resuelve el orquestador al fusionar.

## Hallazgos nuevos (no bloquean)
- F-06 · AMARILLO · u11-scripts-test.sh: M13/M14 (cambiar repo o ref firmados en kind-smoke.sh y dashboard-smoke.sh) sobreviven; usan las mismas variables de lib.sh que sí cubre kind-e2e.
- F-07 · AMARILLO · docs/demo-real.md:91-101, 86: el «bloque completo» del Dockerfile está abreviado; el recibo del dashboard navega a /runs/<run_id>.
- F-08 · AMARILLO · bitacoras/U11-T02.md: evidencia de CA-5 desactualizada (la tarea ya cuenta TARGET_REPO_FULL; medido 4).

## Candidatas
- CI: job que clone el repo objetivo en el SHA fijado y corra TestOrdersCuerpoFlujoSanoContraTargetApp con TARGET_APP_DIR (hoy siempre se salta); u11-scripts-test.sh no corre en CI.
- RUN_ARTIFACT_REF sigue en …/agentic-qa-swarm/target-app:0.0.0.
- kind-local.md conserva filas «Funciona (U8-T08)» sin re-verificar con la imagen del repo externo.
- Posible dedupe de go-intake del reenvío del mismo SHA.

VEREDICTO: VERDE
