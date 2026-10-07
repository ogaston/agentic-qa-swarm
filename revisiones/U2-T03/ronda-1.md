# Ronda 1 — U2-T03

VEREDICTO: NO-VERDE

Worktree `agentic-qa-swarm-wt-U2-T03`, sha 6893ddc. Los 9 criterios de aceptación pasan corridos por mí. Quedan 5 NARANJA y ningún ROJO; por eso el veredicto es NO-VERDE. El worktree quedó limpio (`git status --short | wc -l` = 0). Mis pruebas exploratorias y las mutaciones las hice en una copia en el scratchpad, no en el worktree.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | ≥25 PASS con -race, 0 FAIL | `go test -race -count=1 -v ./...` (el literal de la tarea, sin `-count=1`, devolvió cacheado) | pasa: 80 PASS (subtests incluidos), 0 FAIL. `TestEventsDump` se salta salvo con `WARM_DUMP_EVENTS_DIR`; lo corrí con la variable puesta |
| 2 | Exactamente 3 Jobs, handoff 1, `deploy.failed` con reason | `go test -run 'Deploy(Retry\|Exhausted\|Failed)' -count=1 -v ./...` | pasa; el test de kube lee la lista de Jobs del clientset falso (3) |
| 3 | kubeconform + conftest + latest/registro con rc≠0 | build de `wm` + los cuatro comandos de la tarea | pasa: `Valid: 1, Invalid: 0`; `23 tests, 23 passed, 0 failures`; `latest rc=1`; `registro rc=1` |
| 4 | Eventos válidos con ajv | `go test -run 'Outbox\|Events'` y volcado con `WARM_DUMP_EVENTS_DIR`, luego `npx -p ajv-cli@5.0.0 -p ajv-formats@3.0.1 ajv validate --spec=draft2020 -c ajv-formats` | pasa: `warm.ready`, `deploy.done`, `deploy.failed` y `surface.ready` dan `valid` |
| 5 | `401 401 400 409`, luego `0` y `0` | función `up` de la bitácora, adaptada al puerto 18311 | pasa: `401 401 400 409`, `0`, `0` |
| 6 | MinIO real | `go test -count=1 -tags minio -run ObjectStoreS3 -v ./...` | pasa: `--- PASS: TestObjectStoreS3 (0.51s)`; el digest de la imagen es el de `scripts/test/minio-local.sh`; el test lee de vuelta byte a byte |
| 7 | Arranque fail-closed | los tres `env ... wm` de la tarea | pasa: `rc=1`, `rc=1`, `rc=1` |
| 8 | client-go, usuario no root, list-services | `go list -deps`, `docker build`, `list-services.sh` | pasa: `192`, `65532:65532`, `1` |
| 9 | Higiene y alcance | `go vet` (con y sin `-tags minio`), `gofmt -l`, `git status`, diff contra el merge-base `4bcac0f` | pasa: `ok`, `0`, `0`; el `doc_test.go` de relleno fue eliminado y `go mod tidy -diff` está limpio |

Dos comprobaciones extra pasan. `go list -f '{{.Imports}}'` sobre el dominio no muestra client-go, minio ni net/http. `scripts/ci/check-secrets.sh` da OK.

## Hallazgos

### F-01 · NARANJA · `cmd/go-warm-manager/main.go:104` (semilla), `adapters/kube/kube.go:33` (Get), `main.go:99` (`WARM_KUBE=fake`) · El warm arranca fail-open: `ready` + `reset_verified=true` sin verificación
- `StateStore.Get` devuelve la semilla cuando el ConfigMap `warm-state` no existe (`IsNotFound`).
- `main` construye esa semilla siempre como `ready` + `reset_verified=true`, incluso con `WARM_KUBE=incluster`.
- Un primer arranque, o alguien que borre el ConfigMap, deja al warm declarando `reset_verified=true` sin que `go-reset` lo haya verificado. Es justo el KPI «reset verificado 100%».
- `TestStateStoreRoundTripAndSeed` consagra ese comportamiento como correcto.
- `WARM_KUBE=fake` sin `WARM_FAKE_STATE` también devuelve `ready` + `reset_verified=true` con la misma semilla. Corrí el binario con `WARM_NAMESPACE=aqs-test WARM_SERVICE_TOKEN=x WARM_OUTBOX_FILE=... WARM_KUBE=fake`: arranca sin queja, y no hay otra barrera (build tag, loopback ni log de advertencia).
- Un `env` mal puesto en el Deployment real hace que el servicio reporte listo y que los deploys «funcionen» contra un clientset en memoria.
- Arreglo esperado:
  - La semilla en modo real debe ser `dirty` (o el arranque debe fallar si falta el ConfigMap).
  - `fake` debe quedar acotado, por ejemplo con `//go:build`, o exigir además `LISTEN_ADDR` en loopback y un log de advertencia.
  - Un test debe fijar la semilla segura.

### F-02 · NARANJA · `service.go:171-190` (`Deploy`), `adapters/kube/kube.go:54-75` (`Put`) · `Deploy` no es atómico y el warm admite deploys concurrentes
- `Deploy` hace Get, comprueba `ready`+`reset_verified`, y luego Put `dirty`. Entre el Get y el Put no hay exclusión ni compare-and-swap.
- El `Put` del adaptador kube hace Get y Update con el objeto recién leído, sin `resourceVersion` heredado del Get previo, así que gana el último en escribir.
- Lo demostré con un `StateStore` que añade 20 ms de latencia al Get. Cinco `Deploy` concurrentes de runs distintos crearon 5 Jobs sobre el mismo `warm-app`.
- Ese escenario es alcanzable por la API con cinco `POST /deploys` seguidos, porque `StartDeploy` solo serializa por `run_id`.
- Rompe el invariante «`Deploy` exige ready+reset_verified» y la idea de una corrida por warm.
- Relacionado: si `Jobs.Status` devuelve un error transitorio (apiserver 500), `waitJob` devuelve el error. `Deploy` lo cuenta como intento fallido y crea el Job siguiente, aunque el anterior siga corriendo. Con un `Status` que falla una vez, obtuve 2 Jobs y `deploy.done`.
- Arreglo esperado: serializar la transición ready→dirty (mutex de servicio más `resourceVersion`/Update condicional en el adaptador) y no consumir un reintento por un error de lectura (reintentar la lectura o tratar el error como indeterminado).

### F-03 · NARANJA · `service.go:128` (`go func(){ _ = s.Deploy(...) }`), `internal/api/api.go:~100` · Errores tragados y trace_id inconsistente
- Con el warm `dirty`, `POST /deploys` devuelve `202 pending`. Un instante después `GET /deploys/r-1` da `failed` con `attempts:0`.
  - No hay evento `deploy.failed`, ni handoff, ni línea de log (lo comprobé en `wm.log`).
  - El único rastro es el estado en memoria.
  - El controlador (T02) solo se entera si sondea.
- Los errores de `Put(dirty)`, `Publish` y `Handoff` también se descartan con `_ =`. Si falla `Put(dirty)`, el estado se queda `pending` para siempre. Si falla `Handoff`, `track` marca `failed`, pero no se publica `deploy.failed`.
- `api.go` llama a `trace(r)` dos veces, una para el evento y otra para el log «deploy aceptado». Sin cabecera `X-Trace-Id` genera dos trace IDs aleatorios distintos, así que el log no correlaciona con el evento (la tarea pide logs con `trace_id`).
- Arreglo esperado: loguear los errores del goroutine, calcular `trace` una sola vez, y decidir qué hace el servicio cuando el warm no está listo (409 síncrono o `deploy.failed`).

### F-04 · NARANJA · `internal/api/api.go` (`WriteTimeout: 60*time.Second`), `service.go:84-118` · `POST /warm/ensure` desde `idle-escalado` no puede terminar con el timeout por defecto
- El timeout por defecto de espera a Ready es 120 s, pero el servidor HTTP corta la escritura a los 60 s.
- Verifiqué la semántica con un servidor mínimo en el scratchpad: `WriteTimeout=1s` con handler de 2 s da `client err: EOF`.
- La respuesta se pierde aunque el warm llegue a Ready entre el segundo 60 y el 120.
- La tarea pide el timeout «configurable», pero `main.go` nunca lo cablea: no hay variable de entorno para `WarmReadyTimeout`, `JobTimeout` ni `PollInterval`. El valor 120 s solo existe como literal en `service.go`.
- Hay más literales en puntos de decisión:
  - `activeDeadlineSeconds` 600 y `ttlSecondsAfterFinished` 3600 en `jobspec.go`;
  - `JobTimeout` de 10 minutos;
  - los límites de recursos del Job;
  - el usuario 65532.
- Arreglo esperado: que `WriteTimeout` supere el máximo de espera, o que ensure responda de forma asíncrona. Exponer los timeouts como configuración.

### F-05 · NARANJA · pruebas · Cobertura con huecos y rojo-primero insuficiente
Mutaciones que corrí sobre una copia y que **sobreviven** (la suite sigue en verde):
- En `Deploy`, cambiar `w.State != StateReady || !w.ResetVerified` por `w.State != StateReady`. `TestDeployRefusals` solo prueba `dirty`+`false`; no hay prueba de `ready` con `reset_verified=false`. La tarea pide expresamente «`Deploy` exija ready+reset_verified».
- En `ValidateArtifact`, cambiar `EqualFold(r, host)` por `HasSuffix(host, r)`. No hay ningún caso negativo con registro parecido (`evilghcr.io`), con puerto, con userinfo ni con mayúsculas.
- Quitar `s.Token != ""` en `authorized`. No hay prueba de la API con token vacío (el guard está solo en `main`).
- Quitar la comprobación de namespace en `kube.Jobs.Create`. El clientset falso la cubre de rebote, así que probablemente sea redundante.

Las otras 16 mutaciones que probé (hostNetwork, automount, readOnlyRootFilesystem, envFrom, `latest`, MaxRetries=3, falta de handoff, no marcar dirty, `DisallowUnknownFields`, Redis en las sondas, entre otras) sí las matan los tests.

Sobre el rojo-primero:
- La bitácora admite que las pruebas «nacieron verdes» y que el commit con tests de servicio llegó junto al código. El rojo del build y de `render-deploy-job` sí está registrado, como pedía la tarea.
- La evidencia por mutación cubre CA-2 y CA-6, y el rojo de CA-5 (`401 405 400 409`) es real.
- Para el resto, la suficiencia la determinan las supervivientes de arriba. Que dos invariantes que la tarea nombra por escrito no tengan prueba es la medida de lo que falta.

Arreglo esperado: añadir esos casos negativos y repetir las mutaciones, pegando el rojo en la bitácora.

### F-06 · AMARILLO · varios · Mejoras que no bloquean
- `ports.go:42`: el puerto `ContainerRuntime` que pedía la tarea está declarado pero ni el `Service` ni los fakes lo usan, y la bitácora no declara la omisión. El artefacto viaja por argumentos del Job.
- `probe.HTTP` y `InferSurface` solo sondean rutas comunes sobre el Service; no hay «sondeo de puertos declarados». La bitácora lo registra como candidata 4.
- `ValidateArtifact` es más laxo que la gramática OCI. Acepta `ghcr.io//x:1`, `ghcr.io/ogaston/../../evil/x:1`, `ghcr.io/ogaston/x:1:2`, `ghcr.io/ogaston/x:latest@sha256:<64hex>` (esta última es válida porque el digest manda) y repos en mayúsculas. No es evasión de registro: probé mayúsculas, `@evil.com`, `.evil.com`, puerto y `docker.io/library`, y todos los hosts fuera de la lista se rechazan. Pero `EqualFold` aplica plegado Unicode (sin impacto con `ghcr.io`).
- `Authorization: bearer <tok>` en minúsculas devuelve 401 (RFC 7235 dice que el esquema no distingue mayúsculas).
- `Dockerfile` fija `WARM_OUTBOX_FILE=/tmp/outbox.jsonl`. Con rootfs de solo lectura necesita un `emptyDir`, y en cada reinicio del pod se pierde la idempotencia del outbox.
- `fakes.go` está en el paquete de producción y se compila en el binario.
- Con una misma `run_id` reutilizada tras un reset, el presupuesto de reintentos no está persistido. Con `FakeJobs` sin AlreadyExists obtuve 6 Jobs para el mismo run. En un clúster real los nombres coinciden durante el TTL de 1 h y la creación daría AlreadyExists, así que el riesgo real es bajo. La protección práctica es la compuerta `dirty`, que sí persiste en el ConfigMap.

## Punto (1), fail-closed: lo que comprobé
- Un `deploy` roto da exactamente 3 Jobs con `deploy.failed` y handoff 1.
- Un segundo `POST /deploys` con el mismo run, o con otro run, no crea más Jobs. El segundo run falla en `ErrNotReady` porque el warm quedó `dirty`.
- Tras un reinicio (mapa en memoria vacío) pasa lo mismo, gracias al ConfigMap `warm-state`. Con F-01 resuelto esto se sostiene; con la semilla fail-open, la protección depende de que el ConfigMap exista.

## Decisiones del codificador, juzgadas
- **Imagen `ghcr.io/ogaston/aqs-warm-deployer:0.1.0`**: no existe en el repo. No hay Dockerfile, workflow, referencia en `deploy/` ni en `scripts/` (`grep` limpio). Es un default de marcador, configurable con `WARM_DEPLOYER_IMAGE` y pasa las políticas. **Aceptable pero no cierra el flujo**: el servicio publica `deploy.done` sobre un Job que, tal como está, no puede ejecutar nada útil. Debe constar como placeholder en el código y en el README, y su construcción debe quedar asignada a T07.
- **Artefacto por argumentos y sin token de SA**: aceptable, coherente con el alcance de la tarea (`automountServiceAccountToken: false`). Señala una contradicción de especificación que el orquestador debe resolver, no el codificador. Un Job sin token no puede parchear `warm-app`, y `isolation.rego` prohíbe RoleBinding a las SA de `aqs-test`. Y `warm-deployer` tampoco existe en `deploy/` (candidata 3 de la bitácora, ya registrada).
- **Sondeo solo de rutas comunes**: aceptable como AMARILLO, ver F-06.
- **Rojo-primero por mutación**: suficiente solo para CA-2, CA-6 y CA-5; ver F-05.

## Tareas candidatas (fuera de alcance)
- T07: definir quién aplica el artefacto sin token de SA en el Job. Alternativa: que `go-warm-manager`, con su propia SA y RBAC sobre `aqs-test`, haga el patch. Construir la imagen del deployer. Crear la SA `warm-deployer` y los permisos de `go-warm-manager` sobre el ConfigMap, los pods, los Jobs y el escalado. Montar `/tmp` o un PVC para el outbox.
- Política Rego para Jobs generados (`runAsNonRoot`, `readOnlyRootFilesystem`, `resources`, tag no `latest`, sin `envFrom`). Hoy `policy/` solo cubre serviceAccountName, automount y hostNetwork en Jobs (candidata 2 de la bitácora, ya registrada).
- Llevar la API REST al OpenAPI de `contracts/` y a `contracts/validate.sh`; también la cobertura de `contracts/plans/`.
- Presupuesto de reintentos persistido por `run_id` (ConfigMap o anotación) y limpieza del Job anterior antes de reintentar.

VEREDICTO: NO-VERDE
NARANJA|cmd/go-warm-manager/main.go:104 + adapters/kube/kube.go:33|Arranque fail-open: semilla ready+reset_verified=true si falta el ConfigMap, y WARM_KUBE=fake sin acotar
NARANJA|service.go:171-190 + adapters/kube/kube.go:54-75|Deploy no atómico: 5 deploys concurrentes crean 5 Jobs; un error de Status consume un reintento con el Job previo vivo
NARANJA|service.go:128 + internal/api/api.go|Errores tragados (202 y fallo silencioso sin evento ni log) y trace_id distinto entre log y evento
NARANJA|internal/api/api.go WriteTimeout 60s + service.go timeout 120s|ensure desde idle-escalado no puede terminar; timeouts sin cablear a configuración
NARANJA|service_test.go / types_test.go|Mutaciones sobrevivientes: Deploy sin reset_verified, registro por sufijo, token vacío; rojo-primero insuficiente
INFORME: revisiones/U2-T03/ronda-1.md
