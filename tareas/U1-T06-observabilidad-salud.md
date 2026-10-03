# U1-T06 — Logging estructurado, trazas, health shallow/deep y métricas de U1

**Unidad:** U1 — Ingesta & Inbox
**Historias que implementa:** US-M1 (operabilidad de la ingesta; contrato de `docs/observability.md` de U5-T07)
**Depende de:** U1-T02 y U1-T04 (los servicios deben existir para instrumentarlos). Puede ir en paralelo con U1-T03 y U1-T05 si el codificador rebasa sobre ellas antes del PR.

---

## Alcance

**Dentro** (una línea, concreta):

> Instrumentar `go-intake` y `ui-api` según `docs/observability.md`: log JSON con `timestamp`/`request_id`/`trace_id`/`level`/`message`, propagación de `traceparent`, `GET /healthz` (shallow), `GET /readyz` (deep), `GET /metrics` en Prometheus (latencia, errores, throughput y métricas de dominio de U1) y un panel de dashboard de U1 como ConfigMap revisable en `deploy/flux/base/observability/`.

Detalle:

- **Log estructurado** con `log/slog` y `JSONHandler`: una línea JSON por evento con `timestamp` (RFC 3339, UTC; se renombra `time`), `level` (**minúsculas**: `debug|info|warn|error`), `message` (se renombra `msg`), `request_id`, `trace_id`, más `service`, `route`, `status`, `duration_ms` en el log de acceso. `LOG_LEVEL` configurable (por defecto `info`).
- **Identificadores.** `request_id`: se acepta `X-Request-Id` solo si cumple `^[A-Za-z0-9._-]{8,64}$`; si no, se genera uno; se devuelve siempre en la respuesta (`X-Request-Id`). `trace_id`: se toma de `traceparent` (W3C, versión `00`, 32 hex no nulos); si falta o es inválido, se genera; se propaga hacia fuera en `traceparent` de la respuesta y, en `go-intake`, es el `trace_id` del evento `notify.created` (RESILIENCY-05). Sin exportador OTLP ni colector (no hay backend de trazas en U5): «trazas» en esta tarea es **propagación + correlación en logs**; el exportador es la candidata C-48.
- **Sin secretos ni PII.** Nunca se loguean: cuerpo de la petición, `Authorization`, `X-Hub-Signature-256`, `GITHUB_WEBHOOK_SECRET`, tokens, ni el contenido de los payloads de GitHub (que traen nombres y correos). Se loguean identificadores (`notification_id`, `delivery_id`, `repo`, `sha`) y códigos de error.
- **`/healthz` (shallow).** Siempre `200 {"status":"ok"}` si el proceso responde; no toca disco ni red.
- **`/readyz` (deep).** `200` solo si pasan todas las comprobaciones; si no, `503` con `{"status":"unavailable","checks":{...}}` y el nombre de lo que falla. `go-intake`: directorio de datos escribible (archivo temporal creado y borrado), archivo de eventos (`outbox`) escribible, secreto configurado. `ui-api`: directorio de datos escribible, archivo de eventos legible, `TokenVerifier` configurado. Timeout de 2 s por comprobación.
- **Aislamiento de los endpoints operativos.** `/healthz`, `/readyz` y `/metrics` no requieren token, **no cuentan para el rate limit** y no exponen datos de negocio. No se añaden al OpenAPI de U5 (son operativos, no contrato).
- **`/metrics`** (mismo puerto `http` 8080, que es el que raspa el `ServiceMonitor` de U5-T07), formato de texto de Prometheus con `github.com/prometheus/client_golang`:
  - `aqs_http_requests_total{service,route,method,code}`, `aqs_http_request_duration_seconds{service,route,method}` (histograma) y `aqs_http_in_flight{service}`. `route` es el **patrón** (`/notifications/{id}/confirm`), nunca la ruta cruda, para acotar la cardinalidad; las rutas desconocidas se agrupan en `unmatched`. Errores = `code=~"5.."`; throughput = `rate(aqs_http_requests_total[5m])`.
  - `go-intake`: `aqs_intake_notifications_created_total`, `aqs_intake_webhook_rejected_total{reason}` con `reason` en `invalid_signature|unsupported_event|unresolvable_artifact|bad_request|too_large`, `aqs_intake_publish_failures_total`.
  - `ui-api`: `aqs_inbox_confirmations_total`, `aqs_inbox_notifications{state}` (gauge).
  - Todas las series con contador se **inicializan en 0 al arrancar** (requisito de `docs/observability.md`: si no, `increase()` no detecta el primer incremento).
- **Dashboard.** Un ConfigMap nuevo `deploy/flux/base/observability/dashboard-u1.yaml` (etiqueta `grafana_dashboard: "1"`, dashboard «AQS U1 Ingesta») con al menos 4 paneles: peticiones/s por servicio, tasa de errores 5xx, p95 de latencia, rechazos del webhook por motivo. Se añade a `resources` de `deploy/flux/base/observability/kustomization.yaml`. Se espera un conflicto trivial en esa línea con U4-T07 (quien fusione segundo, rebasa).

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Exportador OTLP/Jaeger/Tempo y cualquier agente de trazas: C-48.
- Alertas (`PrometheusRule`) de U1, SLOs y retención: U5-T07 ya fijó la retención; las alertas de seguridad son de **U4-T07**.
- Modificar `docs/observability.md`, `prometheusrule.yaml`, `servicemonitor.yaml`, `helmreleases.yaml` o el dashboard existente `dashboard.yaml`.
- Logging de acceso en un intermediario (API Gateway/LB, NF-SEG-02): es infraestructura, no de estos servicios.
- Cambiar la lógica de negocio de las rutas, o los códigos y cuerpos de respuesta existentes.
- Aplicar nada a un clúster: el dashboard es un manifiesto revisable.

---

## Archivos de contexto

- `unidades-y-tareas.md` (sección U1)
- `aidlc-docs/inception/application-design/unit-task-plans/U1.md`
- `docs/observability.md` (contrato de log y métricas, retención)
- `deploy/flux/base/observability/` (`dashboard.yaml` como modelo, `servicemonitor.yaml`, `kustomization.yaml`)
- `scripts/ci/policies.sh` (validación de manifiestos que debe seguir en verde)
- `services/go-intake/`, `services/ui-api/`
- `tareas/candidatas.md`

---

## Criterios de aceptación

Desde la raíz del worktree. Arranque común (los nombres de variables de U1-T02/T04 se reutilizan):

```bash
run_intake() { t=$(mktemp -d); (cd services/go-intake && go build -o "$t/go-intake" ./cmd/go-intake)
  GITHUB_WEBHOOK_SECRET=s3cret INTAKE_DATA_DIR="$t/d" INTAKE_EVENTS_FILE="$t/e.jsonl" LISTEN_ADDR=127.0.0.1:$1 "$t/go-intake" > "$t/log" 2>&1 & pid=$!; sleep 1; }
```

- [ ] **CA-1** — Pruebas unitarias en verde (≥ 10 casos entre los dos servicios) y el log cumple el contrato.
  ```bash
  for m in go-intake ui-api; do (cd services/$m && go test -v ./... | grep -c -E '^\s*--- PASS'; go test ./... 2>&1 | grep -c FAIL); done
  ```
  Esperado: números ≥ `5` por servicio (suma ≥ 10) y `0`. Cubren: formato JSON y campos, `level` en minúsculas, request_id aceptado/ignorado/generado, `traceparent` válido/inválido, redacción de secretos, readiness con cada fallo, métricas inicializadas, cardinalidad de `route`.

- [ ] **CA-2** — `/healthz` y `/readyz` responden 200 sin token, y `/readyz` pasa a 503 cuando el almacén no es escribible.
  ```bash
  run_intake 18100
  for p in healthz readyz; do curl -s -o /dev/null -w "$p %{http_code}\n" http://127.0.0.1:18100/$p; done
  chmod 500 "$t/d"; curl -s -w ' %{http_code}\n' http://127.0.0.1:18100/readyz | tr -d '\n'; echo; chmod 700 "$t/d"
  kill $pid; rm -rf "$t"
  ```
  Esperado: `healthz 200`, `readyz 200` y una línea con `"status":"unavailable"` y un nombre de comprobación fallida (p. ej. `data_dir`) terminada en `503`. (Si el CI corre como root y `chmod` no bloquea la escritura, el codificador usa un directorio inexistente en el arranque y lo documenta.) Antes de la tarea: `404` en ambos (rojo inicial).

- [ ] **CA-3** — Cada línea del log es JSON con los cinco campos del contrato y `level` en minúsculas.
  ```bash
  run_intake 18101
  curl -s -o /dev/null -H 'X-Request-Id: req-12345678' -H 'traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01' http://127.0.0.1:18101/healthz
  curl -s -o /dev/null -X POST -H 'X-Hub-Signature-256: sha256=00' -d '{}' http://127.0.0.1:18101/webhooks/github
  kill $pid; sleep 0.3; jq -e -s 'length > 0 and all(.[]; has("timestamp") and has("request_id") and has("trace_id") and has("level") and has("message") and (.level|test("^(debug|info|warn|error)$")))' "$t/log"
  jq -r 'select(.request_id=="req-12345678") | .trace_id' "$t/log" | sort -u; rm -rf "$t"
  ```
  Esperado: `true` y `4bf92f3577b34da6a3ce929d0e0e4736`.

- [ ] **CA-4** — El log no contiene secretos, firmas, cuerpos ni tokens.
  ```bash
  run_intake 18102
  b='{"repository":{"full_name":"acme/shop"},"marca":"CUERPO-SECRETO-XYZ"}'
  curl -s -o /dev/null -X POST -H 'X-GitHub-Event: push' -H 'X-Hub-Signature-256: sha256=deadbeefcafe' -H 'Authorization: Bearer tok-super-secreto' -d "$b" http://127.0.0.1:18102/webhooks/github
  kill $pid; sleep 0.3; grep -c -E 's3cret|deadbeefcafe|tok-super-secreto|CUERPO-SECRETO-XYZ' "$t/log"; rm -rf "$t"
  ```
  Esperado: `0`.

- [ ] **CA-5** — `/metrics` expone las series acordadas, inicializadas en 0 al arrancar, con cardinalidad acotada.
  ```bash
  run_intake 18103
  curl -s http://127.0.0.1:18103/metrics > "$t/m1"
  grep -c -E '^aqs_intake_(notifications_created_total|publish_failures_total) 0$' "$t/m1"
  for p in /a /b /c /d; do curl -s -o /dev/null http://127.0.0.1:18103$p; done
  curl -s -o /dev/null -X POST -H 'X-Hub-Signature-256: sha256=00' -d '{}' http://127.0.0.1:18103/webhooks/github
  curl -s http://127.0.0.1:18103/metrics | grep -E '^aqs_(http_requests_total|intake_webhook_rejected_total)' | grep -c -E 'route="(/a|/b|/c|/d)"'
  curl -s http://127.0.0.1:18103/metrics | grep -c -E 'aqs_intake_webhook_rejected_total\{reason="invalid_signature"\} 1'
  curl -s http://127.0.0.1:18103/metrics | grep -c -E '^aqs_http_request_duration_seconds_bucket'
  kill $pid; rm -rf "$t"
  ```
  Esperado: `2`, `0` (las rutas desconocidas se agrupan en `unmatched`), `1` y un número ≥ `5`.

- [ ] **CA-6** — Los endpoints operativos no se ven afectados por el rate limit de `ui-api` y no filtran datos de negocio.
  ```bash
  cd services/ui-api && go test -run 'OpsEndpoints|RateLimitExempt' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS` en al menos una prueba que dispara más peticiones de las permitidas contra `/healthz`, `/readyz` y `/metrics` sin recibir `429`, y otra que comprueba que `/metrics` no contiene `notification_id` ni `repo` como etiqueta.

- [ ] **CA-7** — El dashboard de U1 es un manifiesto válido, está en el build y pasa las políticas.
  ```bash
  Y='docker run --rm -i --security-opt label=disable mikefarah/yq:4.44.3 -N'; K='docker run --rm --security-opt label=disable -v '"$PWD"':/w -w /w registry.k8s.io/kustomize/kustomize:v5.4.3'
  $K build deploy/flux/prod | $Y 'select(.kind=="ConfigMap" and .metadata.labels.grafana_dashboard=="1") | .metadata.name' | sort
  $K build deploy/flux/prod | $Y 'select(.metadata.name=="aqs-dashboard-u1") | .data | to_entries | .[0].value' | jq -r '.title, (.panels | length)'
  bash scripts/ci/policies.sh 2>&1 | grep -c '^FALLA'
  ```
  Esperado: dos nombres (el existente y `aqs-dashboard-u1`), `AQS U1 Ingesta` y un número ≥ `4`, y `0` (`scripts/ci/policies.sh` sin ninguna `FALLA`).

- [ ] **CA-8** — Cada expresión de los paneles usa solo métricas que los servicios publican.
  ```bash
  Y='docker run --rm -i --security-opt label=disable mikefarah/yq:4.44.3 -N'
  $Y '.data | to_entries | .[0].value' < deploy/flux/base/observability/dashboard-u1.yaml | jq -r '.panels[].targets[].expr' | grep -o -E 'aqs_[a-z_]+' | sort -u
  ```
  Esperado: una lista que contiene solo nombres de las secciones «Métricas» de esta tarea (con sufijos `_bucket`/`_total` según corresponda). El revisor la compara con `curl /metrics` de CA-5.

- [ ] **CA-9** — Higiene y alcance.
  ```bash
  for m in go-intake ui-api; do (cd services/$m && go vet ./... && test -z "$(gofmt -l .)" && echo "$m ok"); done; git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/(go-intake|ui-api)/|deploy/flux/base/observability/(dashboard-u1.yaml|kustomization.yaml)|bitacoras/U1-T06.md)' | wc -l
  git diff -U0 $b -- deploy/flux/base/observability/kustomization.yaml | grep -E '^[+-][^+-]' | grep -v -E '^\+\s*- dashboard-u1.yaml$' | wc -l
  ```
  Esperado: `go-intake ok`, `ui-api ok`, `0`, `0` y `0`.

---

## Plan de pruebas

- Pruebas del formato de log con un `bytes.Buffer` como destino: cada línea decodifica con `encoding/json` y cumple el contrato.
- Negativas de secretos: peticiones con `Authorization`, firma y cuerpo conocidos; el buffer no los contiene.
- Readiness con inyección de fallos (directorio inexistente, archivo de eventos de solo lectura, verificador nulo).
- Métricas con `prometheus/testutil`: valores iniciales y tras N peticiones; etiquetas de `route` limitadas a los patrones.
- Dashboard: el JSON embebido parsea y cada `expr` referencia métricas del conjunto permitido (prueba o `jq`).

**Rojo primero:** el codificador registra en su bitácora el `404` de `/healthz`, `/readyz` y `/metrics` con los binarios de U1-T02/T04 antes de tocar nada.

---

## Notas

- Archivos que se **modifican en su sitio**: el cableado de `cmd/*` y los handlers de U1-T02/T04 (middleware), y `deploy/flux/base/observability/kustomization.yaml` (una línea). Nada de duplicados con sufijo.
- El `request_id` y el `trace_id` son distintos: el primero identifica la petición HTTP; el segundo, la cadena de eventos de una corrida.
- El uso de `golang.org/x/vuln` en CI (`govulncheck`) revisará las dependencias nuevas; si `client_golang` trae una vulnerabilidad, se reporta como bloqueo, no se ignora.
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
