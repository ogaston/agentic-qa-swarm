# U2-T02 — `go-run-controller`: máquina de estados persistida, gates y `GET /runs/{id}`

**Unidad:** U2 — Orquestación & Warm Sandbox Lifecycle
**Historias que implementa:** US-M2, US-M5 (todas las transiciones pasan por los gates de U4; fail-closed global)
**Depende de:** U2-T01 (módulo y stubs), U1-T07 (`run.confirmed` en el outbox), U4-T03/T04 (`GET /auth/session`, `POST /gates/authorize`). Puede correr en paralelo con U2-T03 y U2-T06.

---

## Alcance

**Dentro** (una línea, concreta):

> Implementar en `services/go-run-controller/` la máquina de estados persistida de una corrida (`confirmed → warm_ready → deploying → inferring → rehearsing → running → resetting → reporting → done`, y `failed`), con **un gate de U4 en cada transición** y puertos para todo lo que toca el mundo exterior, más `GET /runs/{id}`, `/healthz`, `/readyz`, `/metrics` y el `Dockerfile`.

Detalle:

- **Estados y transiciones.** Los estados son exactamente el enum `Run.state` del OpenAPI. Transiciones legales: la cadena lineal anterior, más `X → failed` desde cualquier estado no terminal, más `X → resetting` desde `deploying`, `inferring`, `rehearsing` y `running` (fin o fallo de corrida: reset verificado antes de cerrar). `done` y `failed` son terminales. Una transición no listada se rechaza sin tocar el estado.
- **Gate en cada transición.** Antes de aplicar `from → to`, el controlador llama a `GateClient.Authorize(GateRequest)` (puerto) con `run_id`, `from`, `to`, `target_namespace` (fijo por `RUN_TEST_NAMESPACE`, por defecto `aqs-test`; el controlador **nunca** emite otro) y los hechos que **él** conoce: `confirmed` (de `run.confirmed`), `reset_verified` (del puerto `WarmStateReader`), `ensayo_passed` (de `rehearsal.passed`), `workflow_allowed` y `workflow`. Un hecho que no conoce se envía `unknown` (nunca `true`). `workflow_allowed` en `warm_ready` lo reporta el controlador, porque `go-governance` no lo contrasta en ese estado (ver `services/go-governance/authz/doc.go`). **Error del gate, timeout, respuesta inválida o `allow=false` → la corrida no avanza y pasa a `failed`/`resetting` según el estado** (fail-closed; nunca se reintenta un gate denegado).
- **Puertos** (interfaz + fake en memoria + adaptador real donde se indica): `GateClient` (adaptador HTTP contra `go-governance` `POST /gates/authorize` con `Authorization: Bearer $GOVERNANCE_SERVICE_TOKEN`, timeout 2 s), `RunStore` (adaptador: diario JSONL **append-only** en `RUN_DATA_DIR`, que se reproduce al arrancar; cada línea lleva `seq` y hash de la anterior), `EventSource` (adaptador: lee `RUN_EVENTS_FILE`, el JSONL de eventos; marcador de transición C-45), `EventPublisher` (adaptador: escribe `RUN_OUTBOX_FILE`, `event_id` determinista UUIDv5), `WarmStateReader` (solo fake hasta U2-T03/T06), `Alerter` (solo fake; registra el handoff). Las fases que crean Jobs (deploy, ensayo, runners, reset) son **puertos** (`PhaseLauncher`) con fake; sus adaptadores reales son T03, T04, T05 y T06.
- **Reintentos (V8).** Una fase que falla se reintenta como máximo 2 veces; a la tercera falla, `failed` + `Alerter.Handoff`. Nunca hay reintento infinito ni un tercer lanzamiento.
- **`GET /runs/{id}`.** Respuesta válida contra `components.schemas.Run`; `404` si no existe. Autenticación: `Authorization: Bearer` validado contra `go-identity` `GET {IDENTITY_URL}/auth/session` (mismo contrato que U1-T07), timeout 2 s; `401` sin token o inválido, `503` con `Retry-After` si identidad no responde. Sin caché. (El circuito de U1-T07 **no** se replica aquí: candidata.)
- **Idempotencia y reanudación.** Procesar dos veces el mismo `run.confirmed` crea una sola corrida. Al reiniciar, las corridas no terminales se reanudan desde su último estado persistido; una corrida a medias nunca vuelve a `confirmed`.
- **Observabilidad.** `/healthz` (siempre 200), `/readyz` (200 solo con el diario reproducido y los puertos configurados), `/metrics` con `aqs_run_transitions_total{from,to,result}`, `aqs_gate_calls_total{result}`, `aqs_handoff_total{phase}`, `aqs_runs_active`. Logs JSON a stdout con `trace_id`; sin tokens ni secretos.
- **`Dockerfile`** siguiendo `services/go-identity/Dockerfile` (`golang:1.26.8-alpine3.24`, runtime `alpine:3.24`, usuario 65532). Con él, `list-services.sh` lo incluye en la CI.
- **Arranque.** Variables: `RUN_TEST_NAMESPACE`, `RUN_DATA_DIR`, `RUN_EVENTS_FILE`, `RUN_OUTBOX_FILE`, `GOVERNANCE_URL`, `GOVERNANCE_SERVICE_TOKEN`, `IDENTITY_URL`, `LISTEN_ADDR`. Falta cualquiera → no arranca (rc ≠ 0). `GOVERNANCE_URL` e `IDENTITY_URL` solo `http(s)`.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Adaptadores reales de Kubernetes/Jobs (**U2-T03 a T06**) y cualquier import de `k8s.io/*` aquí.
- Implementar o modificar `go-governance` o `go-identity` (U4). Si falta algo del OpenAPI, es un bloqueo.
- Modificar `contracts/**` (incluidos los esquemas de U2-T01), `deploy/**`, `ci.yml` o cualquier otro workflow. El cableado de manifiestos es **U2-T07**.
- Un broker real, persistencia en base de datos o réplicas (C-45, C-49): los puertos se dejan listos.
- Autorización por propietario sobre `GET /runs/{id}` (C-47); el `Role` se valida pero no se aplica.
- Un esquema de evento de handoff o de `warm.quarantined` (no existe en `contracts/`): el `Alerter` registra y mide, nada más (candidata).

---

## Archivos de contexto

- `unidades-y-tareas.md` (U2) y `aidlc-docs/inception/application-design/unit-task-plans/U2.md`
- `aidlc-docs/inception/application-design/components.md` (C9), `services.md` (orquestación y eventos), `component-methods.md`
- `contracts/openapi/control-plane.yaml` (`Run`, `GateRequest`, `GateDecision`, `/runs/{id}`, `/auth/session`) y `contracts/events/*.schema.json`
- `services/go-governance/authz/doc.go` y `types.go` (hechos, legalidad de transiciones, reglas por estado)
- `tareas/U4-T04-go-governance.md` (autenticación servicio a servicio) y `tareas/U1-T07-integracion-u4-run-confirmed.md` (patrón de cliente HTTP a identidad, outbox, `event_id`)
- `services/ui-api/` (patrón de `cmd/`, `internal/obs`, `Dockerfile`) y `services/go-identity/Dockerfile`
- `tareas/candidatas.md` (C-45, C-47, C-49, C-50)

---

## Criterios de aceptación

Desde la raíz del worktree. Arranque común (el controlador contra `go-identity` y `go-governance` reales; los puertos de fase usan sus fakes en memoria, activados con `RUN_PHASES=fake`, variable que **solo** se acepta con `RUN_ALLOW_FAKE_PHASES=true` y que el servicio rechaza si `RUN_ENV=prod`). El codificador toma los nombres de variable de `go-identity` y `go-governance` de `tareas/U4-T02-*.md` y `tareas/U4-T04-*.md`, y documenta la función `up` en su bitácora. `up` siembra `RUN_EVENTS_FILE` con `jq -c . contracts/events/examples/valid/run.confirmed.json` (su `run_id` es `r-1`, el que usan los criterios), arranca `go-identity` (puerto 18200), `go-governance` (18300) y el controlador (18301), y deja en `$tok`, `$gtok`, `$idpid`, `$t` el token de persona, el token de servicio, el PID de identidad y el directorio temporal.

- [ ] **CA-1** — Pruebas unitarias en verde (≥ 25 casos) y las de contrato se omiten sin `GOVERNANCE_URL`.
  ```bash
  cd services/go-run-controller && go test -v ./... | grep -c -E '^\s*--- PASS'; go test ./... 2>&1 | grep -c FAIL; go test -tags contract -run Contract -v ./... 2>&1 | grep -c -E 'SKIP|skipp'
  ```
  Esperado: ≥ `25`, `0` y ≥ `1`. Cubren: cada transición legal; cada una de las ilegales (tabla exhaustiva estado × estado: todas las parejas que no son legales; las legales son 19: 8 de la cadena, 8 hacia `failed` y 3 hacia `resetting`); gate `allow=false`; error y timeout del gate; hecho `unknown`; reintentos 1, 2 y tercera falla → handoff (sin tercer lanzamiento); idempotencia de `run.confirmed`; reanudación tras reinicio; diario con hash roto → no arranca.

- [ ] **CA-2** — Los gates de la tarea: sin confirmación, sin `reset_verified` y sin `ensayo_passed` la corrida no avanza.
  ```bash
  cd services/go-run-controller && go test -run 'Gate(Deny|Unknown|Error)|Without(Confirm|Reset|Ensayo)' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS` en al menos cuatro pruebas: `confirmed→warm_ready` sin confirmación; `warm_ready→deploying` con `reset_verified` falso o desconocido; `rehearsing→running` sin `ensayo_passed`; y el gate caído → no avanza. En todas, el estado leído de vuelta del `RunStore` **no cambió**.

- [ ] **CA-3** — Contra `go-governance` real el controlador respeta lo que decide (no lo que él cree).
  ```bash
  up
  cd services/go-run-controller && GOVERNANCE_URL=http://127.0.0.1:18300 GOVERNANCE_SERVICE_TOKEN="$gtok" go test -tags contract -run Contract -v ./... | grep -E '^\s*--- (PASS|FAIL|SKIP)|^(ok|FAIL)'
  down
  ```
  Esperado: al menos 4 `--- PASS` (transición legal con hechos verdaderos → `allow`; sin confirmación → deny; `target_namespace` distinto de `aqs-test` → deny; gate detenido → la corrida no avanza) y ningún `FAIL`/`SKIP`.

- [ ] **CA-4** — `GET /runs/{id}` de extremo a extremo: válido contra el esquema, `404`, `401` y `503`.
  ```bash
  up
  curl -s -H "Authorization: Bearer $tok" http://127.0.0.1:18301/runs/r-1 | jq -c .
  curl -s -o /dev/null -w '%{http_code} ' -H "Authorization: Bearer $tok" http://127.0.0.1:18301/runs/no-existe
  curl -s -o /dev/null -w '%{http_code} ' http://127.0.0.1:18301/runs/r-1
  kill $idpid; sleep 1
  curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer $tok" http://127.0.0.1:18301/runs/r-1
  down
  ```
  Esperado: un objeto con `id` y `state` (valida contra `components.schemas.Run`: el codificador lo comprueba con `ajv` extrayendo el esquema con `yq`), luego `404 401 503`.

- [ ] **CA-5** — Reanudación y diario: matar el proceso a mitad de corrida y volver a arrancar no pierde ni repite estado.
  ```bash
  cd services/go-run-controller && go test -run 'Resume|Journal|Idempotent' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS` en al menos tres pruebas (reanuda desde el último estado; no duplica una corrida por `run.confirmed` repetido; rechaza arrancar si el hash del diario no encadena) y ningún `FAIL`.

- [ ] **CA-6** — Fail-closed al arrancar y vallas del fake.
  ```bash
  t=$(mktemp -d); (cd services/go-run-controller && go build -o "$t/rc" ./cmd/go-run-controller)
  env -i PATH="$PATH" "$t/rc" >/dev/null 2>&1; echo "sin config rc=$?"
  env RUN_PHASES=fake RUN_ENV=prod RUN_ALLOW_FAKE_PHASES=true RUN_DATA_DIR="$t/d" "$t/rc" >/dev/null 2>&1; echo "prod+fake rc=$?"
  env RUN_PHASES=fake RUN_DATA_DIR="$t/d" "$t/rc" >/dev/null 2>&1; echo "fake sin permiso rc=$?"
  env GOVERNANCE_URL=ftp://x RUN_DATA_DIR="$t/d" "$t/rc" >/dev/null 2>&1; echo "url no http rc=$?"; rm -rf "$t"
  ```
  Esperado: cuatro `rc=` distintos de `0`.

- [ ] **CA-7** — Métricas y logs sin secretos.
  ```bash
  up
  curl -s http://127.0.0.1:18301/metrics | grep -E '^(aqs_run_transitions_total|aqs_gate_calls_total|aqs_handoff_total|aqs_runs_active)' | cut -d'{' -f1 | cut -d' ' -f1 | sort -u
  grep -c -F "$tok" "$t/rc.log"; grep -c -F "$gtok" "$t/rc.log"; curl -s http://127.0.0.1:18301/metrics | grep -c -F "$tok"
  down
  ```
  Esperado: los cuatro nombres de métrica (al menos `aqs_gate_calls_total` y `aqs_runs_active` con valor tras las pruebas), y `0 0 0`.

- [ ] **CA-8** — La imagen cumple las reglas de U5 y este servicio aún no crea Jobs.
  ```bash
  go list -C services/go-run-controller -deps ./... | grep -c -E 'k8s.io|client-go'
  docker build -q -t aqs-go-run-controller:ci services/go-run-controller >/dev/null && docker inspect aqs-go-run-controller:ci --format '{{.Config.User}}'
  bash scripts/ci/list-services.sh | grep -c 'go-run-controller'
  ```
  Esperado: `0`, un usuario no vacío distinto de `root`/`0`, y `1`.

- [ ] **CA-9** — Higiene y alcance.
  ```bash
  (cd services/go-run-controller && go vet ./... && go vet -tags contract ./... && go test -race ./... >/dev/null && test -z "$(gofmt -l .)" && echo ok); git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/go-run-controller/|bitacoras/U2-T02.md|revisiones/U2-T02/)' | wc -l
  ```
  Esperado: `ok`, `0` y `0`.

---

## Plan de pruebas

- Unitarias con fakes en memoria de todos los puertos; reloj inyectable; tabla de transiciones exhaustiva generada del enum `Run.state` (todas las parejas, legales e ilegales).
- Propiedad (`pgregory.net/rapid`, como en U1-T05/U4-T06): para toda secuencia aleatoria de eventos y respuestas de gate, el estado nunca avanza si el gate denegó, nunca sale de un estado terminal y nunca hay más de 2 reintentos por fase.
- Contrato: `-tags contract` contra `go-governance` y `go-identity` reales (CA-3, CA-4).
- Negativa: el token reenviado a identidad y `GOVERNANCE_SERVICE_TOKEN` no aparecen en logs ni métricas (CA-7).

**Rojo primero:** el codificador registra que `go build ./cmd/go-run-controller` no existe y que `GET /runs/r-1` no responde antes de empezar.

---

## Notas

- **Go y Dockerfile (aprendido en U4/U1).** `go 1.26.8`; `golang:1.26.8-alpine3.24` + `alpine:3.24` con tags fijados y usuario 65532. Si el `docker build` falla por la CA del proxy, se verifica con un contexto temporal fuera del repo (ver `tareas/U1-T07-*.md`). En máquina local, la CLI `docker` es podman: montajes con `:z`.
- **Un gate por transición, sin atajos.** El controlador no decide permisos: pregunta. Una prueba programada con la verdad de cada fila no demuestra nada: la matriz `services/go-governance/authz/testdata/authorize_matrix.json` se usa solo para elegir casos de contrato (C-54), y el evaluador que responde es el real.
- **Arquitectura.** Hexagonal: la máquina de estados no importa HTTP ni archivos; todo entra por puertos. Los adaptadores de archivo y HTTP son los únicos que tocan el exterior.
- **Candidatas a registrar** (no hacer aquí): circuito para el cliente de identidad; esquema de evento de handoff / `warm.quarantined`; `deploy/**` con las variables del controlador (U2-T07); transporte real de eventos (C-45).
- **Informes del loop.** El diff de `revisiones/<tarea>/` no cuenta como desborde.
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
