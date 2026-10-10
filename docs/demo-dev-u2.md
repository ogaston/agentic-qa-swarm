# Demostración en dev de U2 (U2-T08)

Guion para que **un humano** valide en el clúster de **dev** el recorrido completo de U2 con la capa de agentes de U3. Ningún paso lo ejecuta el codificador, el revisor ni CI: cada comando que cambia algo en el clúster lleva el marcador `APROBACIÓN HUMANA REQUERIDA` en la línea inmediatamente anterior y requiere la aprobación explícita de quien opera.

Este documento **no contiene credenciales**: los Secrets se nombran por referencia y los valores se leen en el momento, en la terminal de quien opera, sin pegarlos aquí ni en la bitácora.

## 0. Prerrequisitos (solo lectura local)

- Contexto de `kubectl` apuntando al clúster **dev** (`kubectl config current-context` debe decir dev; si dice otra cosa, detente).
- Desplegados por Flux en dev: `go-run-controller`, `go-warm-manager`, `go-reset`, `go-governance`, `go-identity`, `ui-api` (namespace `aqs-system`) y el warm (`warm-app`, `warm-db`) en `aqs-test`.
- El planner de U3 alcanzable desde el controlador. Variables que el humano añade al despliegue del controlador (cambio de configuración, ver 0.1): `RUN_FLOW_SOURCE=u3`, `U3_URL=<URL http(s) del planner>` y `U3_DEFAULT_WORKFLOW=<workflow>` (`run.confirmed` no lleva workflow; sin él el plan falla cerrado). El LLM vive solo en U3: el controlador no recibe ninguna clave.
- Secrets por nombre: `go-governance-service-token`, `go-warm-manager-service-token`, `go-reset-service-token` (clave `token`), todos en `aqs-system`. Una sesión de persona se obtiene con `POST /auth/login` de go-identity; el token resultante se guarda en la variable de entorno local `SESSION_TOKEN` y no se escribe en ningún archivo.
- Antes de nada, la verificación local sin clúster debe estar en verde:

```bash
$ bash scripts/test/u2-demo-local.sh
# Esperado: OK camino-feliz / OK reset-entre-corridas / OK fail-closed-gobernanza / OK cuarentena-por-db-sucia / OK bloqueo-fuera-de-aqs-test
```

### 0.1 Activar el origen de flujos de U3

```bash
# APROBACIÓN HUMANA REQUERIDA — cambia el Deployment del controlador en dev (preferible vía Flux/GitOps; esto es la vía manual)
$ kubectl -n aqs-system set env deploy/go-run-controller RUN_FLOW_SOURCE=u3 U3_URL=<URL-del-planner-de-U3> U3_DEFAULT_WORKFLOW=<workflow>
# Esperado: deployment.apps/go-run-controller env updated

$ kubectl -n aqs-system rollout status deploy/go-run-controller
# Esperado: deployment "go-run-controller" successfully rolled out
```

Si el controlador no arranca, el mensaje del log dice qué falta (`U3_URL es obligatorio`, `RUN_FLOW_SOURCE=u3 exige RUN_PHASES=real`).

## 1. Camino feliz (journey 7.1)

Recorrido: notify → confirm → warm ready → deploy → ensayo → run → reset → estado final.

```bash
# APROBACIÓN HUMANA REQUERIDA — la confirmación crea una corrida real en dev
$ curl -sf -X POST -H "Authorization: Bearer $SESSION_TOKEN" -H 'Content-Type: application/json' \
    -d '{"flows":["checkout"]}' "$UI_API_URL/notifications/<notification-id>/confirm"
# Esperado: 201 y un run_id (anótalo como RUN)

$ kubectl -n aqs-system port-forward svc/go-run-controller 18080:8080
# (en otra terminal) seguir el estado:
$ curl -s -H "Authorization: Bearer $SESSION_TOKEN" http://127.0.0.1:18080/runs/$RUN
# Esperado, en orden: confirmed → warm_ready → deploying → inferring → rehearsing → running → resetting → reporting → done

$ kubectl -n aqs-test get jobs -l run=$RUN
# Esperado: un Job rehearsal-$RUN-1 completado y un runner-$RUN-<flujo> por flujo, todos en aqs-test

$ curl -s "http://127.0.0.1:18080/metrics" | grep -E 'aqs_(warm|reset)'
# Esperado: sin aqs_warm_quarantined > 0
```

Anota en la bitácora la salida real de cada comando. Esperado además: `reset.verified` publicado entre corridas.

## 2. Bloqueo de escape

Un flujo que intenta salir de `aqs-test` es bloqueado y queda en el audit de `go-governance`.

```bash
# APROBACIÓN HUMANA REQUERIDA — solicita al gate una transición hacia otro namespace (se audita; no despliega nada)
$ curl -s -X POST -H "Authorization: Bearer $(kubectl -n aqs-system get secret go-governance-service-token -o jsonpath='{.data.token}' | base64 -d)" \
    -H 'Content-Type: application/json' http://127.0.0.1:18081/gates/authorize \
    -d '{"run_id":"demo-escape","from":"confirmed","to":"warm_ready","target_namespace":"staging","confirmed":"true","reset_verified":"true","ensayo_passed":"unknown","workflow_allowed":"true"}'
# Esperado: {"allow":false,...} con audit_ref

$ curl -s -H "Authorization: Bearer $SESSION_TOKEN" "http://127.0.0.1:18081/audit?run=demo-escape"
# Esperado: la entrada de la denegación para demo-escape (allow=false, namespace staging)
```

(Requiere `kubectl -n aqs-system port-forward svc/go-governance 18081:8080` en otra terminal.) Un `FlowPlan` de U3 con un paso fuera del warm (`http://...` o `//host/...`) nunca llega a crear Jobs: el controlador lo rechaza con `paso fuera de la raíz del warm` en el log y la fase falla.

## 3. Fail-closed de gobernanza

Con `go-governance` detenido ninguna transición avanza.

```bash
# APROBACIÓN HUMANA REQUERIDA — escala go-governance a 0 réplicas en dev
$ kubectl -n aqs-system scale deploy/go-governance --replicas=0
# Esperado: deployment.apps/go-governance scaled

# (confirmar una corrida nueva como en la sección 1)
$ curl -s -H "Authorization: Bearer $SESSION_TOKEN" http://127.0.0.1:18080/runs/$RUN
# Esperado: la corrida se queda en confirmed (no avanza); /readyz del controlador y el log indican el gate caído

# APROBACIÓN HUMANA REQUERIDA — restaura go-governance
$ kubectl -n aqs-system scale deploy/go-governance --replicas=1
# Esperado: deployment.apps/go-governance scaled; la corrida retoma o cierra en failed tras el reset
```

## 4. Reset: DB sucia a mano → cuarentena y aviso

```bash
# APROBACIÓN HUMANA REQUERIDA — ensucia a mano la base del warm (solo en aqs-test de dev)
$ kubectl -n aqs-test exec statefulset/warm-db -- <comando-sql-que-inserta-una-fila-fuera-del-baseline>
# Esperado: la fila queda en la DB del warm

$ kubectl -n aqs-system port-forward svc/go-warm-manager 18082:8080
$ curl -s -H "Authorization: Bearer $(kubectl -n aqs-system get secret go-warm-manager-service-token -o jsonpath='{.data.token}' | base64 -d)" http://127.0.0.1:18082/warm
# Esperado tras la siguiente verificación de higiene/reset: "state":"cuarentena","reset_verified":false y el aviso (aqs_warm_quarantined > 0 / alerta de U5)
```

Con el warm en cuarentena, una corrida nueva no arranca (el gate niega por `reset_verified=false`).

## 5. 3 corridas seguidas

```bash
# APROBACIÓN HUMANA REQUERIDA — tres confirmaciones consecutivas, esperando done entre una y otra
$ curl -sf -X POST -H "Authorization: Bearer $SESSION_TOKEN" -H 'Content-Type: application/json' \
    -d '{"flows":["checkout"]}' "$UI_API_URL/notifications/<notification-id-N>/confirm"
# Esperado por cada corrida N=1..3: estado final done y reset.verified publicado
```

Esperado: la 2.ª y la 3.ª solo arrancan con `reset_verified=true` (las llamadas al gate hacia `warm_ready` de esas corridas llevan `reset_verified=true` en el audit de la sección 2) y el reset queda verificado al 100 %.

## 6. Registro de resultados (CA-8)

El humano anota en `bitacoras/U2-T08.md`, por sección, la salida real. CA-8 solo lo cierra el humano.
