# U1-T01 — Stubs: validador fake de firma GitHub + fixtures `Notification`/`ConfirmationReceipt`

**Unidad:** U1 — Ingesta & Inbox
**Historias que implementa:** US-M1
**Depende de:** U5 completa (contratos en `contracts/`, CI en `.github/workflows/`). Ninguna otra tarea de U1. Puede correr en paralelo con U4-T01.

---

## Alcance

**Dentro** (una línea, concreta):

> Crear los módulos Go `services/go-intake/` y `services/ui-api/` (solo `go.mod`, `go.sum` y paquetes de stubs, **sin `Dockerfile` ni `main`**), el paquete `services/go-intake/internal/githubsig` (firma HMAC-SHA256 de GitHub + `FakeVerifier` programable), los payloads de ejemplo de GitHub en `services/go-intake/testdata/github/` y los ejemplos REST válidos e inválidos de `Notification` y `ConfirmationReceipt` en `contracts/openapi/examples/`, validados contra el OpenAPI de U5 por una prueba Go y por `ajv`.

Detalle:

- **Módulos.** `module github.com/ogaston/agentic-qa-swarm/services/go-intake` y `.../services/ui-api`, ambos con `go 1.26.8` (ver Notas). Cada uno con su `go.mod` y su `go.sum` (SEC-10). **Prohibido** `go.work` y `replace` (C-05: el CI exige `go.mod` propio por servicio). `ui-api` se fija en **Go** (el plan dice «Go o TS»; U1-T05 fija `rapid` como framework PBT, que es Go).
- **`githubsig`.** `Sign(secret []byte, body []byte) string` devuelve `sha256=<hex>` (formato de `X-Hub-Signature-256`). Interfaz `Verifier { Verify(secret, body []byte, header string) error }`. `FakeVerifier` programable: aceptar todo, rechazar todo, o aceptar solo un `header` concreto; por defecto **rechaza** (fail-closed). La verificación real (HMAC + `hmac.Equal`) es de U1-T02, no de esta tarea.
- **Payloads de GitHub** (formas mínimas con los campos que usan U1-T02/T03/T05): `push-branch.json` (`ref: refs/heads/main`), `push-tag.json` (`ref: refs/tags/v1.2.0`), `pull-request-opened.json`, `pull-request-synchronize.json`, `pull-request-fork.json` (`head.repo.full_name != base.repo.full_name`), `release-published.json`, `ping.json`. Cada uno con `repository.full_name` y el SHA de 40 hex que corresponda.
- **Ejemplos REST** en `contracts/openapi/examples/valid/` y `.../invalid/`, con nombre `<Esquema>.<caso>.json`, para `Notification` (estados `pending`, `confirmed`, `rejected`; con y sin `artifact`) y `ConfirmationReceipt`. Inválidos: `state` fuera del enum, `artifact.kind` desconocido, `artifact` con campo extra, `confirmed_at` que no es `date-time`, falta `run_id`.
- **Prueba Go** `TestRESTExamplesAgainstOpenAPI` en `services/go-intake`: extrae `components.schemas.<Esquema>` de `contracts/openapi/control-plane.yaml`, lo compila como JSON Schema 2020-12 con `format` activo y comprueba que cada `valid/` pasa y cada `invalid/` falla. Sugerido: `github.com/santhosh-tekuri/jsonschema/v6` y `gopkg.in/yaml.v3`; si el codificador elige otras, las justifica en la bitácora.
- **Prueba Go** `TestNotifyCreatedExampleValid`: el ejemplo `contracts/events/examples/valid/notify.created.json` valida contra `contracts/events/notify.created.schema.json`.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- `Dockerfile`, `cmd/`, servidor HTTP, `/healthz`, métricas o logging: son U1-T02 y U1-T06. Sin `Dockerfile`, `scripts/ci/list-services.sh` no lista estos servicios, y eso es lo esperado.
- Verificación HMAC real y cualquier handler de `/webhooks/github`: **U1-T02**.
- Clasificación de eventos y resolución de artefacto: **U1-T02/U1-T03**.
- Modificar `contracts/openapi/control-plane.yaml`, `contracts/events/*.schema.json`, `contracts/validate.sh` o cualquier workflow. Si un ejemplo no cabe en el esquema, es un defecto del contrato: se reporta como bloqueo (C-11 lo reabre cuando U1 implemente).
- Cliente HTTP, base de datos, broker o cualquier dependencia de red.
- Importar `k8s.io/*`: U1 **nunca** crea Jobs.

---

## Archivos de contexto

- `unidades-y-tareas.md` (sección U1)
- `aidlc-docs/inception/application-design/unit-task-plans/U1.md`
- `aidlc-docs/inception/application-design/components.md` (C1)
- `aidlc-docs/inception/application-design/services.md` (`ui-api`, `go-intake`)
- `contracts/openapi/control-plane.yaml` (`Notification`, `ConfirmationReceipt`)
- `contracts/events/notify.created.schema.json` y `contracts/events/examples/valid/notify.created.json`
- `contracts/validate.sh` (invocación de `ajv` que se reutiliza en los criterios)
- `scripts/ci/list-services.sh`, `scripts/ci/detect-lang.sh`

---

## Criterios de aceptación

Desde la raíz del worktree. Alias:

```bash
Y='docker run --rm -i --security-opt label=disable mikefarah/yq:4.44.3 -N'
AJV=(npx --yes -p ajv-cli@5.0.0 -p ajv-formats@3.0.1 ajv)
```

- [ ] **CA-1** — Las pruebas de ambos módulos pasan.
  ```bash
  for m in go-intake ui-api; do (cd services/$m && go test ./... 2>&1 | tail -n 3); done
  ```
  Esperado: ninguna línea `FAIL`; en `go-intake` aparece `ok` con el paquete `githubsig` y el de las pruebas de contrato. Antes de la tarea: `cd: services/go-intake: No such file or directory` (rojo inicial).

- [ ] **CA-2** — Cada ejemplo REST válido pasa `ajv` contra el esquema extraído del OpenAPI, y cada inválido es rechazado por el esquema.
  ```bash
  t=$(mktemp -d); for s in Notification ConfirmationReceipt; do $Y -o=json ".components.schemas.$s" < contracts/openapi/control-plane.yaml > "$t/$s.json"; done
  for f in contracts/openapi/examples/valid/*.json; do s=$(basename "$f" | cut -d. -f1); "${AJV[@]}" validate --spec=draft2020 -c ajv-formats -s "$t/$s.json" -d "$f" >/dev/null 2>&1; echo "valid $(basename $f) rc=$?"; done
  for f in contracts/openapi/examples/invalid/*.json; do s=$(basename "$f" | cut -d. -f1); "${AJV[@]}" validate --spec=draft2020 -c ajv-formats -s "$t/$s.json" -d "$f" 2>&1 | grep -q 'invalid$'; echo "invalid $(basename $f) rejected=$?"; done; rm -rf "$t"
  ```
  Esperado: todas las líneas `valid ... rc=0` y todas las `invalid ... rejected=0`; al menos 5 `valid` y 5 `invalid`.

- [ ] **CA-3** — El `FakeVerifier` rechaza por defecto y la firma de `Sign` coincide con la de `openssl`.
  ```bash
  cd services/go-intake && go test -run 'FakeVerifier|Sign' -v ./internal/githubsig/ | grep -E '^(--- |ok|FAIL)'
  printf '{"a":1}' | openssl dgst -sha256 -hmac 's3cret' | sed 's/^.* /sha256=/'
  ```
  Esperado: todas las líneas `--- PASS`; hay una prueba cuyo nombre contiene `DefaultRejects` (o equivalente) y otra que fija el vector `Sign([]byte("s3cret"), []byte(`{"a":1}`))` al valor que imprime `openssl` en la segunda línea (el codificador lo copia a la prueba como constante).

- [ ] **CA-4** — Los siete payloads de GitHub existen, son JSON válido y llevan los campos que usará U1-T02/T03.
  ```bash
  cd services/go-intake/testdata/github && ls | wc -l && for f in *.json; do jq -e . "$f" >/dev/null || echo "ROTO $f"; done
  jq -r '.repository.full_name' push-branch.json push-tag.json pull-request-opened.json release-published.json | sort -u
  jq -e '.pull_request.head.repo.full_name != .pull_request.base.repo.full_name' pull-request-fork.json
  jq -r '.ref' push-branch.json push-tag.json
  ```
  Esperado: `7`; ningún `ROTO`; una sola línea con `owner/repo`; `true`; `refs/heads/main` y `refs/tags/v1.2.0`.

- [ ] **CA-5** — Higiene del módulo y U1 sigue sin ser desplegable.
  ```bash
  for m in go-intake ui-api; do (cd services/$m && go vet ./... && test -z "$(gofmt -l .)" && test -s go.sum && ! grep -q '^replace' go.mod && echo "$m ok"); done
  test ! -e go.work && echo "sin go.work"
  go list -C services/go-intake -deps ./... | grep -c -E 'k8s.io|client-go'
  bash scripts/ci/list-services.sh | grep -c -E 'go-intake|ui-api'
  ```
  Esperado: `go-intake ok`, `ui-api ok`, `sin go.work`, `0` y `0`.

- [ ] **CA-6** — Árbol limpio tras el commit y nada fuera de alcance tocado.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/(go-intake|ui-api)/|contracts/openapi/examples/|bitacoras/U1-T01.md)' | wc -l
  ```
  Esperado: `0` y `0`.

---

## Plan de pruebas

- Contrato: `TestRESTExamplesAgainstOpenAPI` (valid pasan, invalid fallan, ≥5 y ≥5) y `TestNotifyCreatedExampleValid`.
- `githubsig`: vector de firma fijo contra `openssl`; `FakeVerifier` en sus tres modos y su valor por defecto (rechaza); encabezado vacío, sin prefijo `sha256=`, con hex impar.
- Negativa: una copia temporal de un ejemplo `valid` con un campo extra en `artifact` debe fallar la prueba de contrato (se ejecuta y se registra, no se commitea).

**Rojo primero:** el codificador pega en su bitácora la salida literal de `ls services/go-intake` (no existe) y del primer comando de CA-1 antes de crear nada.

---

## Notas

- **Go (aprendido en U4).** El módulo va en `go 1.26.8` (no `go 1.24`: con 1.24 el job `vuln` de la CI de GitHub falla por avisos de la biblioteca estándar), con las dependencias más recientes compatibles con esa versión. Patrón de referencia: `services/go-identity` y `services/go-governance`. `govulncheck` no corre en el entorno del loop (`vuln.go.dev` da 403): la confirmación es el job `vuln` de `ci` en el PR de GitHub (el orquestador lo abre como borrador para que corra).
- **Comandos con `yq` (aprendido en U4).** `keys` no ordena (usa `keys | sort`); `x // "y"` trata `false` como ausente; `if/then` de jq no parsea en yq; `yq -N` sobre un build imprime líneas en blanco (filtra con `grep -v '^$'`). Si un criterio no puede dar el esperado por esa causa, el codificador lo reporta con comando y salida; no rellena a ciegas.
- **Informes del loop.** El diff de `revisiones/<tarea>/` (informes del revisor) no cuenta como desborde en los criterios de alcance.
- Archivos que se **modifican en su sitio**: ninguno. Todo es nuevo.
- Los ejemplos REST viven en `contracts/` (no duplicados por servicio) para que U1-T04 y U1-T07 los reutilicen. Las rutas relativas desde los tests son `../../contracts/...`.
- Sin `Dockerfile` los tests de estos módulos **no corren en `ci.yml`** hasta U1-T02; la verificación hasta entonces es local. Es una limitación conocida, no un defecto.
- Los payloads no necesitan ser réplicas completas de GitHub: solo los campos documentados arriba. Ningún secreto real ni nombre de repositorio real.
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
