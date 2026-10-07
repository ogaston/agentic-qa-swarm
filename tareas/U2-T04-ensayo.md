# U2-T04 — Ensayo bloqueante (`rehearsal-{run}`) y adaptadores de fase del controlador

**Unidad:** U2 — Orquestación & Warm Sandbox Lifecycle
**Historias que implementa:** US-M5 (el ensayo es un gate técnico no omitible; 2 reintentos y escalado)
**Depende de:** U2-T02 (controlador y puertos), U2-T03 (API de `go-warm-manager`) y U2-T06 (API de `go-reset`), las tres **fusionadas**. Bloquea U2-T05 (ambas modifican `go-run-controller`).

---

## Alcance

**Dentro** (una línea, concreta):

> Añadir a `services/go-run-controller/` el lanzador del Job `rehearsal-{run}` contra el warm, el gate no omitible `ensayo_passed` con 2 reintentos y escalado, y los **adaptadores HTTP reales** de los puertos `PhaseLauncher` de deploy/superficie y de reset hacia `go-warm-manager` y `go-reset`, de modo que el controlador recorra `warm_ready → deploying → inferring → rehearsing` y vuelva a `resetting` sobre servicios reales.

Detalle:

- **Job `rehearsal-{run}`.** El controlador crea, en `aqs-test`, **un** Job por intento (`rehearsal-<run>-<n>`) que ejecuta **un flujo unitario** del `FlowPlan` (el primero) contra el Service del warm: comprueba que cada paso responde con su `expect_status` y que se cumple la invariante mínima. Constructor de Job puro: **sin credenciales de LLM ni de ningún Secret** (`env` sin `*LLM*`, `*API_KEY*`, `*TOKEN*`; sin `envFrom`), **sin egress** (etiqueta que selecciona la NetworkPolicy de aislamiento de `aqs-test` ya existente), `automountServiceAccountToken: false`, `serviceAccountName` explícito, no root, `readOnlyRootFilesystem`, tag fijado, `resources` completos, `activeDeadlineSeconds` (timeout del ensayo, 120 s por defecto). Subcomando `render-rehearsal-job --run <id> --flow <id>` que lo imprime como YAML sin tocar un clúster.
- **Resultado.** El resultado del Job se lee de vuelta de su estado (`succeeded`/`failed`) vía el puerto `KubeAPI` (adaptador `client-go`; pruebas con `kubernetes/fake`) y se traduce a `rehearsal.passed|failed` (válidos contra `contracts/events/rehearsal.schema.json`; `evidence` es un URI). `passed` fija `ensayo_passed=true` en la corrida; `failed` lo deja `false`.
- **Gate duro y no omitible.** `rehearsing → running` solo ocurre con `ensayo_passed=true` **registrado en el `RunStore`**, y el controlador además lo envía al gate de U4. Ni una configuración, ni una variable de entorno, ni un plan vacío lo saltan: un `FlowPlan` roto (sin flujos, o inválido contra su esquema) hace fallar el ensayo **antes** de crear ningún Job de runner.
- **Reintentos y escalado (V8).** Si el ensayo falla: como máximo 2 reintentos (3 Jobs `rehearsal-*` por corrida, nunca un cuarto); tras agotarlos, la corrida pasa a `resetting` → `failed` y se llama a `Alerter.Handoff`. **Ningún Job `runner-*` se crea mientras `ensayo_passed` no sea `true`.**
- **Adaptadores HTTP.** Clientes de `go-warm-manager` (`POST /warm/ensure`, `POST /deploys` + `GET /deploys/{run_id}`, `POST /surface`) y de `go-reset` (`POST /resets`) con bearer de servicio (`WARM_SERVICE_TOKEN`, `RESET_SERVICE_TOKEN`), timeout 5 s, sin seguir redirecciones, solo URL `http(s)`; cualquier error, `409`, cuerpo inválido o timeout se traduce a **fallo de la fase** (nunca a éxito). Las respuestas se validan contra los esquemas de `contracts/plans/` y de `contracts/events/`. Estos clientes se **escriben aquí** (no se importan de otros módulos).
- **Cableado.** `RUN_PHASES=real` selecciona los adaptadores reales (`WARM_URL`, `RESET_URL` obligatorios); `fake` sigue exigiendo `RUN_ALLOW_FAKE_PHASES=true` y se rechaza con `RUN_ENV=prod` (como en U2-T02).

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Los Jobs `runner-*`, la recolección de evidencia y `run.done`: **U2-T05**.
- Cambiar la máquina de estados, los gates o `GET /runs/{id}` de U2-T02 salvo lo imprescindible para enchufar los adaptadores; cualquier cambio de comportamiento ya cubierto por T02 se reporta, no se hace.
- Modificar `go-warm-manager`, `go-reset`, `contracts/**`, `deploy/**`, `policy/**` o workflows. Si falta algo en sus APIs, es un bloqueo.
- El motor de ejecución del flujo dentro del Job (imagen del ensayo): se fija por configuración (`REHEARSAL_IMAGE`, tag fijado, nunca `latest`); construir esa imagen es de U3/T05 (candidata).
- Aplicar nada a un clúster real.

---

## Archivos de contexto

- `unidades-y-tareas.md` (U2) y `aidlc-docs/inception/application-design/unit-task-plans/U2.md`
- `aidlc-docs/inception/application-design/components.md` (C2, C9), `component-methods.md` (`runRehearsal`)
- `contracts/events/rehearsal.schema.json` y ejemplos; `contracts/plans/flow-plan.schema.json`
- `services/go-run-controller/` (U2-T02), `services/go-warm-manager/README.md` y `services/go-reset/` (APIs de U2-T03/T06)
- `tareas/U2-T02-run-controller.md`, `tareas/U2-T03-warm-manager.md`, `tareas/U2-T06-reset-verificado.md`
- `policy/*.rego` (aislamiento de `aqs-test`, `security`, `workloads`) y `deploy/flux/base/security/` (NetworkPolicy del test ns)

---

## Criterios de aceptación

Desde la raíz del worktree.

- [ ] **CA-1** — Pruebas unitarias en verde (≥ 20 casos nuevos), con `-race`.
  ```bash
  cd services/go-run-controller && go test -race -v ./... | grep -c -E '^\s*--- PASS'; go test -race ./... 2>&1 | grep -c FAIL
  ```
  Esperado: ≥ `45` acumulado con U2-T02 y `0`. Cubren: constructor del Job; resultado `succeeded`/`failed`; plan roto o vacío; reintentos 0, 1, 2 y tercera falla; cada adaptador HTTP ante `200`, `409`, `5xx`, cuerpo inválido, timeout y URL no `http(s)`.

- [ ] **CA-2** — **Un plan roto nunca crea Jobs de runners**; los Jobs vistos son solo ensayos fallidos y un handoff.
  ```bash
  cd services/go-run-controller && go test -run 'Rehearsal(BrokenPlan|Exhausted|NeverSkipped)' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS`; las pruebas **leen de vuelta** del clientset falso la lista de Jobs: con un plan roto o un ensayo que siempre falla hay **3** Jobs `rehearsal-<run>-*`, **0** Jobs `runner-*`, la corrida termina en `failed` pasando por `resetting`, y `Alerter` recibió 1 handoff.

- [ ] **CA-3** — El gate no se puede omitir ni siquiera configurándolo.
  ```bash
  cd services/go-run-controller && go test -run 'EnsayoGate|NoBypass' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS` en al menos tres pruebas: `rehearsing→running` sin `ensayo_passed` registrado se rechaza; una variable de entorno `RUN_SKIP_REHEARSAL=true` (u otra que el codificador intente) **no existe** ni tiene efecto; con `ensayo_passed=true` en el almacén pero el gate de U4 respondiendo `allow=false`, no avanza.

- [ ] **CA-4** — El Job de ensayo no lleva credenciales, no tiene egress y pasa las políticas.
  ```bash
  t=$(mktemp -d); (cd services/go-run-controller && go build -o "$t/rc" ./cmd/go-run-controller)
  "$t/rc" render-rehearsal-job --run r-1 --flow checkout > "$t/job.yaml"
  grep -c -i -E 'LLM|API_KEY|TOKEN|secretKeyRef|envFrom' "$t/job.yaml"
  docker run --rm -i --security-opt label=disable ghcr.io/yannh/kubeconform:v0.6.7 -strict -summary - < "$t/job.yaml"
  docker run --rm --security-opt label=disable -v "$PWD":/project:z -v "$t":/in:z -w /project openpolicyagent/conftest:v0.56.0 test /in/job.yaml --policy policy --all-namespaces
  rm -rf "$t"
  ```
  Esperado: `0`, `Valid: 1, Invalid: 0` y `0 failures`.

- [ ] **CA-5** — Los adaptadores reales funcionan contra `go-warm-manager` y `go-reset` reales y fallan cerrado.
  ```bash
  up
  cd services/go-run-controller && WARM_URL=http://127.0.0.1:18310 WARM_SERVICE_TOKEN="$wtok" RESET_URL=http://127.0.0.1:18320 RESET_SERVICE_TOKEN="$rtok" go test -tags contract -run 'Contract(Warm|Reset)' -v ./... | grep -E '^\s*--- (PASS|FAIL|SKIP)|^(ok|FAIL)'
  down
  ```
  Esperado: al menos 4 `--- PASS` (`/warm/ensure` listo; `409` → fallo de fase; token incorrecto → fallo de fase; servicio detenido → fallo de fase) y ningún `FAIL`/`SKIP`. `up` compila y arranca ambos servicios con sus fakes de Kubernetes y se documenta en la bitácora. Sin las variables, las pruebas se omiten.

- [ ] **CA-6** — Recorrido hasta el ensayo, de extremo a extremo con los fakes de fase y de Kubernetes.
  ```bash
  cd services/go-run-controller && go test -run 'Flow(ToRehearsal|ResetOnFailure)' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS`; la primera recorre `confirmed→warm_ready→deploying→inferring→rehearsing` (cada paso con su gate); la segunda comprueba que un fallo en `deploying` lleva a `resetting` y la corrida no queda abandonada.

- [ ] **CA-7** — Vallas de configuración.
  ```bash
  t=$(mktemp -d); (cd services/go-run-controller && go build -o "$t/rc" ./cmd/go-run-controller)
  env RUN_PHASES=real RUN_DATA_DIR="$t/d" "$t/rc" >/dev/null 2>&1; echo "real sin URLs rc=$?"
  env RUN_PHASES=real WARM_URL=ftp://x RESET_URL=http://x RUN_DATA_DIR="$t/d" "$t/rc" >/dev/null 2>&1; echo "url no http rc=$?"
  env RUN_PHASES=fake RUN_ENV=prod RUN_ALLOW_FAKE_PHASES=true RUN_DATA_DIR="$t/d" "$t/rc" >/dev/null 2>&1; echo "prod+fake rc=$?"; rm -rf "$t"
  ```
  Esperado: tres `rc=` distintos de `0`.

- [ ] **CA-8** — Imagen, dependencias y alcance.
  ```bash
  go list -C services/go-run-controller -deps ./... | grep -c -E 'k8s.io/client-go'
  docker build -q -t aqs-go-run-controller:ci services/go-run-controller >/dev/null && docker inspect aqs-go-run-controller:ci --format '{{.Config.User}}'
  (cd services/go-run-controller && go vet ./... && go vet -tags contract ./... && test -z "$(gofmt -l .)" && echo ok); git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/go-run-controller/|bitacoras/U2-T04.md|revisiones/U2-T04/)' | wc -l
  ```
  Esperado: ≥ `1`, un usuario no vacío distinto de `root`/`0`, `ok`, `0` y `0`.

---

## Plan de pruebas

- Unitarias con `kubernetes/fake` y fakes de `PhaseLauncher`; reloj inyectable; `httptest` para los clientes (200, 409, 5xx, cuerpo roto, lento, redirección).
- Propiedad (`rapid`): para toda secuencia de resultados de Job, nunca hay más de 3 Jobs `rehearsal-*` por corrida, nunca existe un `runner-*` sin `ensayo_passed=true`, y `passed` solo sale de un Job `succeeded`.
- Contrato contra los servicios reales (CA-5) y política (CA-4).
- Negativa: el codificador intenta añadir `RUN_SKIP_REHEARSAL` y debe ver su propia prueba de CA-3 fallar (rojo → eliminar).

**Rojo primero:** registrar que `render-rehearsal-job` no existe y que el controlador sale de `warm_ready` sin llamar a ningún adaptador real.

---

## Notas

- **Go y Dockerfile.** Igual que U2-T02; el `Dockerfile` ya existe y se modifica en su sitio solo si hace falta (p. ej. el subcomando de render no lo requiere). Podman: montajes con `:z`.
- **Orden de fusión.** T02, T03 y T06 deben estar fusionadas. Si no lo están, el orquestador reporta el hueco y no despacha.
- **Candidatas a registrar:** imagen del ensayo y del runner; circuito en los clientes HTTP del controlador; esquema de evento de handoff.
- **Informes del loop.** El diff de `revisiones/<tarea>/` no cuenta como desborde.
- Ningún comando contra un clúster ni la nube; la prueba real de `kubectl get jobs` la hace el humano en dev (U2-T08).

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
