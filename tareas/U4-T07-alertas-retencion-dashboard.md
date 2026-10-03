# U4-T07 — Alertas de seguridad, retención de auditoría ≥ 90 d y dashboard (con la instrumentación que lo sostiene)

**Unidad:** U4 — Gobernanza & Identidad
**Historias que implementa:** US-M8.3 (límite de autonomía verificable: «incidentes de límite de autonomía = 0» y auditoría que no se pierde)
**Depende de:** U4-T02, U4-T03 y U4-T04 (los servicios deben existir para instrumentarlos). U4-T05 y U4-T06 no son requisito. Puede ir en paralelo con U1-T06 (conflicto trivial esperado en `observability/kustomization.yaml`; quien fusione segundo, rebasa).

---

## Alcance

**Dentro** (una línea, concreta):

> Instrumentar `go-identity` y `go-governance` con el contrato de observabilidad de U5 (log JSON, `/healthz`, `/readyz`, `/metrics`) y con métricas de seguridad; definir como manifiestos revisables las **alertas de seguridad** (`PrometheusRule` con pruebas `promtool`), el **dashboard** de U4 y la **retención ≥ 90 días de la auditoría** (volumen persistente de `go-governance`, retención mínima visible y verificada).

Detalle:

1. **Contrato de observabilidad en ambos servicios.** Igual que en `tareas/U1-T06-observabilidad-salud.md` (se reimplementa en cada módulo: no hay código compartido): log JSON con `timestamp`/`request_id`/`trace_id`/`level` (minúsculas)/`message`, `request_id` aceptado solo con `^[A-Za-z0-9._-]{8,64}$`, `traceparent` W3C propagado; **jamás** contraseñas, hashes, tokens, OTP, secretos TOTP, `GOVERNANCE_SERVICE_TOKEN` ni cuerpos de petición; `/healthz` (shallow, siempre 200), `/readyz` (deep: `go-identity` → archivo de usuarios cargado y sesiones operativas; `go-governance` → `GOVERNANCE_DATA_DIR` escribible, política legible, **cadena de auditoría íntegra**, `IDENTITY_URL` alcanzable con timeout de 2 s); `/metrics` en Prometheus en el puerto `http` (8080) con `aqs_http_requests_total{service,route,method,code}`, `aqs_http_request_duration_seconds` y `aqs_http_in_flight`, con `route` como **patrón** (nunca la ruta cruda). Los tres endpoints operativos no requieren token y no cuentan para ningún límite.
2. **Métricas de seguridad** (todas las series con contador se **inicializan en 0 al arrancar**, requisito de `docs/observability.md`):
   - `go-identity`: `aqs_auth_login_total{result}` con `result` ∈ `success|invalid_credentials|mfa_required|locked|bad_request`; `aqs_auth_lockouts_total`; `aqs_auth_active_sessions` (gauge); `aqs_authz_denied_total{reason}` con `reason` ∈ `unauthorized|forbidden`; `aqs_privilege_escalation_attempts_total{endpoint}` (un `user` autenticado golpeando una ruta de `admin`, o pidiendo una sesión ajena: la etiqueta es el **patrón** de la ruta).
   - `go-governance`: `aqs_gate_decisions_total{decision,to}` con `decision` ∈ `allow|deny|error`; `aqs_gate_denied_total{reason}` con `reason` ∈ `not_confirmed|reset_not_verified|ensayo_not_passed|namespace_not_test|workflow_not_allowed|illegal_transition|audit_failed|internal_error`; `aqs_policy_changes_total{name,result}` con `result` ∈ `accepted|rejected`; `aqs_audit_entries_total`; `aqs_audit_append_failures_total`; `aqs_audit_last_append_timestamp_seconds`; `aqs_audit_chain_ok` (1/0, verificado al arrancar y cada `GOVERNANCE_VERIFY_INTERVAL`, por defecto 15 m); `aqs_audit_retention_days` (gauge con el mínimo configurado).
   - Etiquetas **acotadas**: ninguna lleva `run_id`, `username`, IP, token ni texto libre.
3. **Retención de la auditoría ≥ 90 días.**
   - `GOVERNANCE_AUDIT_RETENTION_DAYS` (por defecto `90`; **el servicio no arranca con un valor menor**) se expone como `aqs_audit_retention_days`. El servicio **no borra, rota ni compacta** el log (sigue siendo append-only); la retención es una garantía de no-borrado, no un proceso.
   - Cada entrada de auditoría se emite además como una línea de log estructurada (`message: "audit"`, `action`, `run_id`, `actor`, sin el detalle de hechos sensibles), de modo que el pipeline de logs de U5-T07 (Loki, 2160 h) conserva una **segunda copia** durante 90 días.
   - Manifiesto: en `deploy/flux/base/control-plane.yaml`, el Deployment `go-governance` monta un `PersistentVolumeClaim` (`go-governance-data`, 5Gi, `ReadWriteOnce`, **sin `storageClassName`**, como U5-T07, pendiente C-17) en `/data`, con `GOVERNANCE_DATA_DIR=/data`, `GOVERNANCE_AUDIT_RETENTION_DAYS=90`, y la anotación `aqs.io/audit-retention-days: "90"` en el PVC y el Deployment. Se añaden también `livenessProbe` (`/healthz`) y `readinessProbe` (`/readyz`) a `go-governance` y `go-identity`. Es el **único** cambio de esos dos Deployments.
4. **Alertas** (nuevo `deploy/flux/base/observability/prometheusrule-security.yaml`, `PrometheusRule` en `aqs-observability`, grupo `aqs.security`, cada alerta con `severity` y `summary`):
   - `AqsAutonomyBoundaryViolation` — `increase(aqs_gate_denied_total{reason="namespace_not_test"}[15m]) > 0`, `critical` (un intento de operar fuera del test namespace: KPI «incidentes de límite de autonomía = 0»).
   - `AqsAuditAppendFailing` — `increase(aqs_audit_append_failures_total[5m]) > 0`, `critical`.
   - `AqsAuditChainBroken` — `aqs_audit_chain_ok == 0`, `for: 1m`, `critical`.
   - `AqsAuthFailuresHigh` — `sum(rate(aqs_auth_login_total{result=~"invalid_credentials|locked"}[5m])) > 0.5`, `for: 5m`, `warning`.
   - `AqsAuthLockout` — `increase(aqs_auth_lockouts_total[15m]) > 0`, `warning`.
   - `AqsPrivilegeEscalationAttempt` — `increase(aqs_privilege_escalation_attempts_total[15m]) > 0`, `warning`.
   - `AqsGateDeniedSpike` — `sum(increase(aqs_gate_denied_total[15m])) > 10`, `warning`.
   - Pruebas `promtool test rules` en `deploy/flux/base/observability/rules_security_test.yaml`: por cada alerta, un caso que **dispara** y uno que **no**, con series sintéticas. Las pruebas citan las métricas por los **mismos nombres** que publican los servicios.
5. **Dashboard** (`deploy/flux/base/observability/dashboard-u4.yaml`, ConfigMap con `grafana_dashboard: "1"`, título «AQS U4 Gobernanza e Identidad», ≥ 6 paneles): resultados de login por `result`, bloqueos, sesiones activas, decisiones de gate por `reason` de denegación, fallos y estado de la cadena de auditoría (con `aqs_audit_retention_days`), intentos de escalada. Se añade `prometheusrule-security.yaml` y `dashboard-u4.yaml` a `resources` de `deploy/flux/base/observability/kustomization.yaml`.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Instrumentar `ui-api`, `go-intake` o los servicios de U2/U3: U1-T06 y las tareas de U2.
- Exportador OTLP/trazas (C-48), enrutamiento de alertas (Alertmanager, Slack, correo) y runbooks de respuesta: operación.
- Modificar `docs/observability.md`, `prometheusrule.yaml`, `servicemonitor.yaml`, `helmreleases.yaml`, `dashboard.yaml` o `deploy/flux/base/backup/**` (los backups del volumen de auditoría son la candidata C-52).
- `scripts/ci/policies.sh` y los workflows: el cableado de `promtool` en CI es la candidata C-33; aquí las pruebas se ejecutan con el comando de los criterios (y U4-T05 ya edita `policies.sh`: evitar conflicto).
- Cambiar la lógica de autenticación, autorización, gates o auditoría más allá de **emitir** métricas, logs y los chequeos de `/readyz`.
- Rotación, compactación, archivado o borrado del log de auditoría.
- Cambios en `control-plane.yaml` distintos de los de `go-governance` y `go-identity` descritos arriba; env, PVC y probes de `ui-api` y `go-intake` son la candidata C-53.
- Aplicar nada a un clúster.

---

## Archivos de contexto

- `unidades-y-tareas.md` (sección U4)
- `aidlc-docs/inception/application-design/unit-task-plans/U4.md`
- `aidlc-docs/inception/requirements/requirements.md` (NF-SEG-02/NF-SEG-14 alertas y logging, retención ≥ 90 d)
- `docs/observability.md` (contrato de log y de métricas; retención de Loki y Prometheus)
- `deploy/flux/base/observability/` (`prometheusrule.yaml` y `dashboard.yaml` como modelo, `kustomization.yaml`), `deploy/flux/base/backup/rules_test.yaml` (modelo de prueba de reglas)
- `deploy/flux/base/control-plane.yaml` (Deployments `go-governance` y `go-identity`)
- `tareas/U1-T06-observabilidad-salud.md`, `tareas/U4-T02-go-identity-autenticacion.md`, `tareas/U4-T03-autorizacion.md`, `tareas/U4-T04-go-governance.md`
- `tareas/candidatas.md` (C-17, C-33, C-48)

---

## Criterios de aceptación

Desde la raíz del worktree. Alias y arranque:

```bash
K='docker run --rm --security-opt label=disable -v '"$PWD"':/w -w /w registry.k8s.io/kustomize/kustomize:v5.4.3'
Y='docker run --rm -i --security-opt label=disable mikefarah/yq:4.44.3 -N'
PT='docker run --rm --security-opt label=disable --entrypoint promtool -v '"$PWD"':/w -w /w prom/prometheus:v2.55.1'
upi() { t=$(mktemp -d); (cd services/go-identity && go build -o "$t/gi" ./cmd/go-identity) || return 1
  pw=$(openssl rand -base64 18); jq -n --arg h "$(printf '%s' "$pw" | "$t/gi" hash-password)" '[{username:"marta",password_hash:$h,role:"user"}]' > "$t/users.json"
  IDENTITY_USERS_FILE="$t/users.json" LISTEN_ADDR=127.0.0.1:$1 "$t/gi" > "$t/log" 2>&1 & pid=$!; sleep 1; B=http://127.0.0.1:$1; }
upg() { t=$(mktemp -d); (cd services/go-governance && go build -o "$t/gg" ./cmd/go-governance) || return 1
  GOVERNANCE_AUTH=fake GOVERNANCE_ALLOW_FAKE_AUTH=true GOVERNANCE_FAKE_TOKENS='tok-admin=a1:admin,tok-user=u1:user' GOVERNANCE_SERVICE_TOKEN="$(openssl rand -hex 32)" GOVERNANCE_DATA_DIR="$t/data" IDENTITY_URL=http://127.0.0.1:9 LISTEN_ADDR=127.0.0.1:$1 "$t/gg" > "$t/log" 2>&1 & pid=$!; sleep 1; B=http://127.0.0.1:$1
  svc=$(tr '\0' '\n' < /proc/$pid/environ | sed -n 's/^GOVERNANCE_SERVICE_TOKEN=//p'); }
```

- [ ] **CA-1** — Pruebas unitarias en verde (≥ 14 casos nuevos entre los dos servicios), con `-race`.
  ```bash
  for m in go-identity go-governance; do (cd services/$m && go test -race -v ./... | grep -c -E '^\s*--- PASS'; go test -race ./... 2>&1 | grep -c FAIL); done
  ```
  Esperado: dos pares con `FAIL` en `0` y el recuento de `PASS` crece en ≥ 14 respecto de la base (el codificador anota ambos números en su bitácora). Cubren: formato y campos del log, `level` en minúsculas, redacción de secretos, readiness con cada fallo, métricas inicializadas, cardinalidad de `route`, cada `reason`, retención mínima.

- [ ] **CA-2** — `/healthz`, `/readyz` y `/metrics` responden sin token; `/readyz` pasa a `503` cuando un chequeo falla.
  ```bash
  upi 18240; for p in healthz readyz metrics; do curl -s -o /dev/null -w "identity $p %{http_code}\n" $B/$p; done; kill $pid; rm -rf "$t"
  upg 18241; for p in healthz metrics; do curl -s -o /dev/null -w "governance $p %{http_code}\n" $B/$p; done
  curl -s -o /dev/null -w 'governance readyz(identidad inalcanzable) %{http_code}\n' $B/readyz
  curl -s $B/readyz | jq -r '.status'
  kill $pid; rm -rf "$t"
  ```
  Esperado: `identity healthz 200`, `identity readyz 200`, `identity metrics 200`; `governance healthz 200`, `governance metrics 200`, `governance readyz(identidad inalcanzable) 503` y `unavailable` (el `IDENTITY_URL` apunta a un puerto cerrado). Antes de la tarea: `404` en todos (rojo inicial).

- [ ] **CA-3** — `readyz` de `go-governance` falla si la cadena de auditoría está rota (y `aqs_audit_chain_ok` baja a 0).
  ```bash
  cd services/go-governance && go test -run 'Readyz.*Chain|AuditChainOK' -v ./... | grep -E '^\s*--- (PASS|FAIL)'
  ```
  Esperado: al menos 2 `PASS` (cadena íntegra → `ready` y `aqs_audit_chain_ok 1`; línea alterada → `503` y `aqs_audit_chain_ok 0`), sin `FAIL`.

- [ ] **CA-4** — Métricas de seguridad de `go-identity`: inicializadas en 0, y cuentan lo que ocurre.
  ```bash
  upi 18242
  curl -s $B/metrics | grep -c -E '^aqs_auth_(lockouts_total|login_total\{result="(success|invalid_credentials|mfa_required|locked|bad_request)"\}) 0$'
  for i in 1 2 3 4 5; do curl -s -o /dev/null -X POST $B/auth/login -H 'Content-Type: application/json' -d '{"username":"marta","password":"incorrecta-123"}'; done
  curl -s -o /dev/null -X POST $B/auth/login -H 'Content-Type: application/json' -d "$(jq -n --arg p "$pw" '{username:"marta",password:$p}')"
  curl -s $B/metrics | grep -E '^aqs_auth_(login_total\{result="(invalid_credentials|locked)"\}|lockouts_total) '
  curl -s -o /dev/null -H 'Authorization: Bearer x' $B/auth/users
  curl -s $B/metrics | grep -E '^aqs_authz_denied_total\{reason="unauthorized"\} '
  kill $pid; rm -rf "$t"
  ```
  Esperado: `6` (las seis series arrancan en 0); `aqs_auth_login_total{result="invalid_credentials"} 5`, `aqs_auth_login_total{result="locked"} 1`, `aqs_auth_lockouts_total 1`; `aqs_authz_denied_total{reason="unauthorized"} 1`.

- [ ] **CA-5** — Métricas de `go-governance`: decisiones por motivo, cambios de política y auditoría.
  ```bash
  upg 18243
  curl -s $B/metrics | grep -c -E '^aqs_(audit_append_failures_total|audit_entries_total) 0$'
  g() { curl -s -o /dev/null -X POST $B/gates/authorize -H "Authorization: Bearer $svc" -H 'Content-Type: application/json' -d "$1"; }
  g '{"run_id":"r1","from":"confirmed","to":"warm_ready","target_namespace":"aqs-test","confirmed":"false","reset_verified":"true"}'
  g '{"run_id":"r2","from":"inferring","to":"rehearsing","target_namespace":"prod","confirmed":"true","workflow_allowed":"true"}'
  g '{"run_id":"r3","from":"confirmed","to":"warm_ready","target_namespace":"aqs-test","confirmed":"true","reset_verified":"true","workflow_allowed":"true"}'
  curl -s -o /dev/null -X PUT $B/policies/events -H 'Authorization: Bearer tok-admin' -H 'Content-Type: application/json' -d '{"value":{"enabled_events":["tag"]}}'
  curl -s -o /dev/null -X PUT $B/policies/events -H 'Authorization: Bearer tok-admin' -H 'Content-Type: application/json' -d '{"value":{"enabled_events":["push"]}}'
  curl -s -o /dev/null -X PUT $B/policies/events -H 'Authorization: Bearer tok-user' -H 'Content-Type: application/json' -d '{"value":{}}'
  curl -s $B/metrics | grep -E '^aqs_(gate_denied_total\{reason="(not_confirmed|namespace_not_test)"\}|gate_decisions_total\{decision="allow",to="warm_ready"\}|policy_changes_total\{name="events",result="(accepted|rejected)"\}|audit_entries_total|audit_chain_ok|audit_retention_days) '
  kill $pid; rm -rf "$t"
  ```
  Esperado: `2`; y las líneas `aqs_gate_denied_total{reason="not_confirmed"} 1`, `aqs_gate_denied_total{reason="namespace_not_test"} 1`, `aqs_gate_decisions_total{decision="allow",to="warm_ready"} 1`, `aqs_policy_changes_total{name="events",result="accepted"} 1`, `…result="rejected"} 1`, `aqs_audit_entries_total` ≥ `5`, `aqs_audit_chain_ok 1` y `aqs_audit_retention_days 90`.

- [ ] **CA-6** — Cardinalidad acotada y sin datos sensibles en las etiquetas.
  ```bash
  upg 18244
  for i in $(seq 20); do curl -s -o /dev/null -X POST $B/gates/authorize -H "Authorization: Bearer $svc" -d "{\"run_id\":\"run-$i\",\"from\":\"confirmed\",\"to\":\"warm_ready\",\"target_namespace\":\"ns-$i\"}"; curl -s -o /dev/null $B/ruta-$i; done
  curl -s $B/metrics | grep -c -E 'run-[0-9]+|ns-[0-9]+|ruta-[0-9]+'
  curl -s $B/metrics | grep -c -E 'route="unmatched"'
  curl -s $B/metrics | grep -c -i -E "$svc|tok-admin|tok-user"
  kill $pid; rm -rf "$t"
  ```
  Esperado: `0`, un número ≥ `1` y `0`.

- [ ] **CA-7** — Los logs cumplen el contrato y no contienen secretos.
  ```bash
  upi 18245
  curl -s -o /dev/null -H 'X-Request-Id: req-12345678' -H 'traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01' $B/healthz
  curl -s -o /dev/null -X POST $B/auth/login -H 'Content-Type: application/json' -d "$(jq -n --arg p "$pw" '{username:"marta",password:$p}')"
  curl -s -o /dev/null -X POST $B/auth/login -H 'Content-Type: application/json' -d '{"username":"marta","password":"CLAVE-INCORRECTA-XYZ"}'
  kill $pid; sleep 0.3
  jq -e -s 'length > 0 and all(.[]; has("timestamp") and has("request_id") and has("trace_id") and has("level") and has("message") and (.level|test("^(debug|info|warn|error)$")))' "$t/log"
  grep -c -F -e "$pw" -e 'CLAVE-INCORRECTA-XYZ' -e "$(jq -r '.[0].password_hash' "$t/users.json")" "$t/log"; rm -rf "$t"
  upg 18246; curl -s -o /dev/null -X POST $B/gates/authorize -H "Authorization: Bearer $svc" -d '{"run_id":"r1","from":"confirmed","to":"warm_ready","target_namespace":"aqs-test","confirmed":"true","reset_verified":"true","workflow_allowed":"true"}'; kill $pid; sleep 0.3
  jq -r 'select(.message=="audit") | .action' "$t/log" | sort -u; grep -c -F "$svc" "$t/log"; rm -rf "$t"
  ```
  Esperado: `true`, `0`; y para `go-governance` una línea `gate.allow` (la copia de auditoría en el log) y `0` (el token de servicio no aparece).

- [ ] **CA-8** — Retención: el servicio no arranca con menos de 90 días y no existe ninguna ruta que borre.
  ```bash
  t=$(mktemp -d); (cd services/go-governance && go build -o "$t/gg" ./cmd/go-governance)
  for d in 0 30 89; do env GOVERNANCE_AUTH=fake GOVERNANCE_ALLOW_FAKE_AUTH=true GOVERNANCE_FAKE_TOKENS='a=a:admin' GOVERNANCE_SERVICE_TOKEN="$(openssl rand -hex 32)" GOVERNANCE_DATA_DIR="$t/d" GOVERNANCE_AUDIT_RETENTION_DAYS=$d "$t/gg" >/dev/null 2>&1; echo "dias=$d rc=$?"; done; rm -rf "$t"
  grep -r -n -E 'os\.(Remove|RemoveAll|Truncate|Rename)|\.Truncate\(' services/go-governance --include='*.go' | grep -v '_test.go' | wc -l
  ```
  Esperado: tres líneas con `rc=` distinto de `0`; y `0` (el código de producción de `go-governance` no borra, trunca ni renombra archivos; si un uso es legítimo —p. ej. un archivo temporal de `/readyz`—, el codificador lo acota a `os.CreateTemp` con su propio directorio y lo justifica, y el `grep` se ajusta de forma explícita).

- [ ] **CA-9** — Manifiestos válidos: el volumen, las anotaciones y las sondas están en el build, y ningún otro cambio.
  ```bash
  $K build deploy/flux/prod | $Y 'select(.kind=="PersistentVolumeClaim" and .metadata.name=="go-governance-data") | [.metadata.annotations."aqs.io/audit-retention-days", .spec.resources.requests.storage, (.spec.accessModes|join(",")), (.spec.storageClassName // "sin-clase")] | join(" ")'
  $K build deploy/flux/prod | $Y 'select(.kind=="Deployment" and .metadata.name=="go-governance") | [.metadata.annotations."aqs.io/audit-retention-days", .spec.template.spec.containers[0].livenessProbe.httpGet.path, .spec.template.spec.containers[0].readinessProbe.httpGet.path, (.spec.template.spec.containers[0].env[] | select(.name=="GOVERNANCE_AUDIT_RETENTION_DAYS") | .value), .spec.template.spec.volumes[0].persistentVolumeClaim.claimName] | join(" ")'
  $K build deploy/flux/prod | $Y 'select(.kind=="Deployment" and .metadata.name=="go-identity") | [.spec.template.spec.containers[0].livenessProbe.httpGet.path, .spec.template.spec.containers[0].readinessProbe.httpGet.path] | join(" ")'
  for e in dev prod; do $K build deploy/flux/$e | docker run --rm -i ghcr.io/yannh/kubeconform:v0.6.7 -strict -summary -schema-location default -schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json' -; done
  ```
  Esperado: `90 5Gi ReadWriteOnce sin-clase`; `90 /healthz /readyz 90 go-governance-data`; `/healthz /readyz`; y dos líneas `Invalid: 0, Errors: 0, Skipped: 0`.

- [ ] **CA-10** — Las alertas existen, con severidad y resumen, y `promtool` las valida.
  ```bash
  Rb=deploy/flux/base/observability/prometheusrule-security.yaml
  $Y '.spec.groups[].rules[] | .alert + " " + .labels.severity' < $Rb | sort
  $Y '.spec.groups[].rules[] | select(.annotations.summary == null or .labels.severity == null) | .alert' < $Rb | wc -l
  t=$(mktemp -d); $Y '.spec' < $Rb > "$t/rules.yaml"; cp deploy/flux/base/observability/rules_security_test.yaml "$t/"
  docker run --rm --security-opt label=disable --entrypoint promtool -v "$t":/w -w /w prom/prometheus:v2.55.1 check rules rules.yaml
  docker run --rm --security-opt label=disable --entrypoint promtool -v "$t":/w -w /w prom/prometheus:v2.55.1 test rules rules_security_test.yaml; echo "test rc=$?"; rm -rf "$t"
  ```
  Esperado: siete líneas, en este orden alfabético: `AqsAuditAppendFailing critical`, `AqsAuditChainBroken critical`, `AqsAuthFailuresHigh warning`, `AqsAuthLockout warning`, `AqsAutonomyBoundaryViolation critical`, `AqsGateDeniedSpike warning`, `AqsPrivilegeEscalationAttempt warning`; `0`; `SUCCESS: 7 rules found`; y `SUCCESS` con `test rc=0`.

- [ ] **CA-11** — Cada alerta tiene un caso que dispara y otro que no, y las métricas que usan las publican los servicios.
  ```bash
  Tf=deploy/flux/base/observability/rules_security_test.yaml
  $Y '[.tests[].alert_rule_test[] | select(.exp_alerts | length > 0) | .alertname] | unique | length' < $Tf
  $Y '[.tests[].alert_rule_test[] | select((.exp_alerts // []) | length == 0) | .alertname] | unique | length' < $Tf
  for m in $($Y '.spec.groups[].rules[].expr' < deploy/flux/base/observability/prometheusrule-security.yaml | grep -o -E 'aqs_[a-z_]+' | sort -u); do grep -r -q -F "\"$m\"" services/go-identity services/go-governance --include='*.go' || echo "NO PUBLICADA $m"; done; echo fin
  ```
  Esperado: `7`, `7`, ninguna línea `NO PUBLICADA` y `fin`. (Una prueba por cada alerta que dispara y otra que no dispara con series cercanas al umbral.)

- [ ] **CA-12** — Dashboard válido, en el build, y con expresiones que usan solo métricas publicadas.
  ```bash
  $K build deploy/flux/prod | $Y 'select(.kind=="ConfigMap" and .metadata.name=="aqs-dashboard-u4") | .data | to_entries | .[0].value' | jq -r '.title, (.panels | length)'
  $K build deploy/flux/prod | $Y 'select(.kind=="ConfigMap" and .metadata.name=="aqs-dashboard-u4") | .data | to_entries | .[0].value' | jq -r '.panels[].targets[].expr' | grep -o -E 'aqs_[a-z_]+' | sort -u | sed -E 's/_(bucket|sum|count)$//' | while read -r m; do grep -r -q -F "\"$m\"" services/go-identity services/go-governance --include='*.go' || echo "NO PUBLICADA $m"; done; echo fin
  ```
  Esperado: `AQS U4 Gobernanza e Identidad`, un número ≥ `6`; ninguna línea `NO PUBLICADA` y `fin`.

- [ ] **CA-13** — Las políticas de manifiestos y la validación común siguen en verde, y los archivos nuevos entran en el build.
  ```bash
  bash scripts/ci/policies.sh 2>&1 | grep -c '^FALLA'
  $K build deploy/flux/prod | $Y 'select(.kind=="PrometheusRule") | .metadata.name' | sort
  ```
  Esperado: `0` y una lista que incluye el `PrometheusRule` de U5-T07, el de backups y el nuevo de seguridad.

- [ ] **CA-14** — Imágenes conformes y servicios sin Kubernetes; higiene y alcance.
  ```bash
  for m in go-identity go-governance; do docker build -q -t aqs-$m:ci services/$m >/dev/null && docker inspect aqs-$m:ci --format '{{.Config.User}}'; go list -C services/$m -deps ./... | grep -c -E 'k8s.io|client-go'; (cd services/$m && go vet ./... && test -z "$(gofmt -l .)" && echo "$m ok"); done
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/(go-identity|go-governance)/|deploy/flux/base/(control-plane\.yaml|observability/(prometheusrule-security|rules_security_test|dashboard-u4|kustomization)\.yaml)|bitacoras/U4-T07\.md)' | wc -l
  git diff -U0 $b -- deploy/flux/base/observability/kustomization.yaml | grep -E '^[+-][^+-]' | grep -v -E '^\+\s*- (prometheusrule-security|dashboard-u4)\.yaml$' | wc -l
  ```
  Esperado: por cada servicio, un usuario no vacío distinto de `root`/`0`, `0` y `<servicio> ok`; luego `0`, `0` y `0`.

---

## Plan de pruebas

- Unitarias de log, métricas y `/readyz` con inyección de fallos (archivo de usuarios ilegible, directorio de datos de solo lectura, cadena alterada, identidad inalcanzable); `prometheus/testutil` para valores iniciales y tras N eventos; cardinalidad de `route`.
- Negativas de secretos: peticiones con contraseñas, tokens, OTP y el token de servicio conocidos; ni el log ni `/metrics` los contienen.
- `promtool test rules`: por alerta, un caso que dispara y uno que no (series cercanas al umbral; la de cadena rota con `for: 1m`).
- Retención: arranque con 0, 30 y 89 días falla; con 90 arranca; ninguna llamada de borrado en el código de producción.
- Sin llamadas a un clúster ni a la nube; los servidores se levantan en `127.0.0.1`.

**Rojo primero:** el codificador registra en su bitácora el `404` de `/healthz`, `/readyz` y `/metrics` con los binarios de U4-T02/T04 y la ausencia de `prometheusrule-security.yaml` y `go-governance-data` en el build, antes de tocar nada.

---

## Notas

- Archivos que se **modifican en su sitio**: `deploy/flux/base/control-plane.yaml` (solo `go-governance` y `go-identity`), `deploy/flux/base/observability/kustomization.yaml` (dos líneas) y el cableado de `cmd/*` de ambos servicios. Nada de duplicados con sufijo.
- El PVC de auditoría no está cubierto por los backups de U5-T08 (C-52) y los `env`/PVC/sondas de `ui-api` y `go-intake` siguen sin cablear (C-53): se reportan como candidatas, no se resuelven aquí.
- Con una sola réplica y un PVC `ReadWriteOnce`, `go-governance` no escala horizontalmente mientras la auditoría sea un archivo local: es una limitación declarada, igual que las sesiones en memoria de `go-identity` (C-49).
- Los umbrales de las alertas (0,5/s, 10 en 15 min, `for`) son valores razonables, no requisitos del PRD; el humano los confirma (como en C-08).
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
