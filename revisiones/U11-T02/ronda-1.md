# Ronda 1 — U11-T02 (informe del revisor, persistido por el orquestador; resumen fiel del hand-back)

VEREDICTO: NO-VERDE (sin ROJO; dos NARANJA)

Hash de la tarea 1d3bcb41…, HEAD 18356dc, árbol limpio. No levantó kind. Copias del scratchpad. `shellcheck`, `conftest`, `kubeconform` no están en el host; `policies.sh` corrió por su vía alternativa (rc=0).

## Criterios
CA-1 pasa (SIN_APP, 0). CA-2 pasa (BASELINE_IGUAL, 2, PROCEDENCIA, OPENAPI_IGUAL; par falla-pasa). CA-3 pasa (go-warm-manager ok; runner SKIP sin variable, PASS con TARGET_APP_DIR + AQS_PLANNER_PYTHON, FAIL de compilación con directorio vacío).
CA-4 pasa (imagen fijada en el warm-app del overlay kind, no-latest rc=0, policies rc=0). CA-5 pasa (6 SINTAXIS_OK, 0, 4, u11-scripts-test.sh OK). CA-6 no exigido (evidencia creíble, ver abajo). CA-7 pasa. CA-8: la segunda cuenta da 3 (F-01).

## Hallazgos
### F-01 · NARANJA · CA-8, segundo comando (defecto de la especificación; lo corrige el orquestador)
El filtro de alcance deja pasar `deploy/flux/base/warm.yaml` (excepción declarada en el propio criterio) y `tareas/U11-T04-…`/`U11-T05-…` (documentos del orquestador). Sustancialmente el alcance está bien: warm.yaml solo cambia un comentario.

### F-02 · NARANJA · docs/demo-real.md
Presenta como hecho lo que nunca se observó y no es ejecutable tal cual: el Paso 0 pide `grep -c '^FALLA'` → 0 pero hoy da 1 (llm-egress-acotado, U11-T05); los pasos 1–3 afirman resultados («veredicto sin hallazgos», «los runners fallan en el flujo de compra», «El reporte declara bug») cuando el único e2e real cae en `runners` (U11-T04); no hay sección de estado conocido;
la tabla «Dónde» usa la API interna de go-warm-manager (token de servicio y port-forward) sin dar comandos y dice «recibo con run_id» sin decir dónde se ve; el Paso 2 manda `git commit -am` sin decir que hay que editar antes el Dockerfile.
Verificado cierto: el defecto es `ENV TARGET_APP_BUG=oversell` en el Dockerfile del commit (b97a719 no lo trae); la CI del repo externo solo publica en push a main (los PR no tienen imagen); los tiempos están marcados «Pendiente de medir»; sin valores sensibles; el comando de cloudflared parsea; «10 imágenes de la plataforma» es correcto.
Pedido: bloque «Estado conocido» con U11-T04 y U11-T05 como bloqueos pendientes; marcar los resultados como esperados, no observados; dar comandos de lectura de vuelta que existan (`kubectl get deploy warm-app -o jsonpath`, inbox) y quitar/cualificar las filas que exigen token interno; añadir «edita el Dockerfile» antes del `commit -am`.

### F-03 · AMARILLO · scripts/kind
`lib.sh:265-267` define `TARGET_REPO_FULL` pero `kind-e2e.sh:339`, `kind-smoke.sh:290` y `dashboard-smoke.sh:166` repiten el literal en el `jq`; `github-webhook-check.sh:48` lo repite como valor por defecto. El SHA fijado vive en kustomization.yaml y u8-target-app-kind.sh:23. Tomarlo de lib.sh.

### F-04 · AMARILLO · scripts/test/u11-scripts-test.sh
Detecta 8 mutaciones del script real (no es tautológico). Huecos: el caso (d) usa el mismo SHA que el `ls-remote` simulado, así que no distingue TARGET_SHA de HEAD remoto; no cubre kind-smoke, dashboard-smoke ni github-webhook-check; no cubre fallo del token anónimo ni 429 de GHCR (se tratan como «no hay imagen»); no está en CI.

### F-05 · AMARILLO · scripts/kind/lib.sh:276-285 y kind-e2e.sh
`TARGET_BRANCH` se lee pero el webhook firma `refs/heads/main`; si el repo cambia de rama por defecto el error dice «sin red o repo inaccesible» (engañoso, falla seguro); `git ls-remote` sin `GIT_TERMINAL_PROMPT=0`.

## Adversarial (resumen)
Sin referencias colgantes a demo/target-app (build-images con podman simulado y sin demo/: rc=0, 10 builds). Baseline y OpenAPI idénticos al origen con par falla-pasa; `ensure_baseline_real` simulado en 4 casos (ausente, kind-stub, real preexistente, fuente inexistente) correcto; main.go, store*.go, openapi.json, Dockerfile y go.mod del repo externo iguales a la app borrada.
Scripts: webhook firmado con el repo permitido y el SHA elegido; SHA mal formado o sin imagen (404) → rc=2 antes de tocar el clúster; sin red falla claro; sin secretos. Warm: imagen del SHA fijado, usuario 65532 con setcap, puertos 80/9090. Compatibilidad con U11-T01/T03: la imagen entra en el primer prefijo; nada que coordinar.

## Evidencia de CA-6, evaluada: creíble y coherente
e2e.log: repo objetivo @ b97a719…, OK en port-forwards, login, webhook-202, inbox, confirm, deploy, superficie, ensayo y ensayo-job; FALLA runners (run-20d9efb7096b73d9; «run_id/flow_id no aptos para un nombre de Job»); checks OK=14 rc=1. smoke.log: único FALLA llm-egress-acotado. Fechas posteriores al commit 568bd21. Los dos defectos están bien diagnosticados y no ocultan otros fallos.
Sigue sin demostrarse (y la tarea lo difiere): humo en 0, e2e en verde y la segunda lectura con un SHA distinto del fijado.

## Candidatas
- CI: ya no hay job que haga checkout del repo objetivo en el SHA fijado y corra `TestOrdersCuerpoFlujoSanoContraTargetApp` con TARGET_APP_DIR (en CI esa prueba siempre se salta: pérdida de cobertura); tampoco corre u11-scripts-test.sh.
- RUN_ARTIFACT_REF sigue en …/agentic-qa-swarm/target-app:0.0.0.
- go-intake podría deduplicar el reenvío del mismo SHA (no verificado).

VEREDICTO: NO-VERDE
