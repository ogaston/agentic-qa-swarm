# Ronda 2 — U4-T07

VEREDICTO: NO-VERDE

Queda un NARANJA (F-06): tras el merge con U4-T06, dos de los tres mutantes `.patch` de `go-governance` ya no aplican sobre HEAD. Los 14 criterios dan lo esperado y F-01, F-03, F-04 y F-05 están resueltos. Worktree limpio. HEAD `d2a186e` (merge) sobre `880bb4e` (código de la ronda 2).

## Criterios de aceptación, verificados por el revisor sobre HEAD (post-merge, `GOTOOLCHAIN=go1.26.8`, `-count=1`)
| # | Resultado |
|---|---|
| 1 | `-race`: identity 205 PASS / 0 FAIL; governance 346 / 0 (base `origin/main` con T06 medida con `git archive`: 184 y 302; diferencia +21 y +44 ≥ 14) |
| 2 | `200 200 200`, `200 200 503`, `unavailable` |
| 3 | 3 PASS (`TestAuditChainOKGaugeFollowsMonitor`, `TestReadyzAuditChainIntact`, `TestReadyzAuditChainBroken`) |
| 4 | `6`; `lockouts 1`, `invalid_credentials 5`, `locked 1`; `authz_denied unauthorized 1` |
| 5 | con `workflow_allowed:"true"`: `2`; `not_confirmed` 1, `namespace_not_test` 1, allow `warm_ready` 1, accepted 1, rejected 1, `audit_entries` 6, `chain_ok` 1, `retention_days` 90 |
| 6 | `0`, `15`, `0` |
| 7 | `true`, `0`; governance `gate.allow` y `0` |
| 8 | días 0, 30 y 89: `rc=1`; el `grep` de borrado da `0` |
| 9 | PVC `90 5Gi ReadWriteOnce sin-clase`; Deployment `90 /healthz /readyz 90 go-governance-data`; identity `/healthz /readyz`; kubeconform dev y prod `Valid: 62, Invalid: 0, Errors: 0, Skipped: 0` |
| 10 | siete alertas con severidad, `0`, `SUCCESS: 7 rules found`, `test rc=0` |
| 11 | `7`, `7`, sin `NO PUBLICADA` |
| 12 | título correcto, 12 paneles, sin `NO PUBLICADA` |
| 13 | `policies.sh` con shim de red: 0 `FALLA`, 12 `OK` |
| 14 | `65532:65532` en ambas imágenes, 0 dependencias k8s, `vet` y `gofmt` ok, `0`, `0`, `0` |

`go 1.26.8`, `client_golang v1.24.1` y `rapid v1.3.0` en ambos módulos; `go mod tidy -diff` limpio y `go mod verify` = `all modules verified`. `git diff origin/main..HEAD --name-only` solo toca los servicios, `deploy/flux/base/{control-plane, observability/*}`, la bitácora y `revisiones/U4-T07/`. CI del PR #25 sobre `d2a186e`: `ci`, `policies` y `contracts` en success (incluidos `vuln`, `test`, `build`, `sbom` de ambos servicios). El codificador solo corrió vet, gofmt y test tras el merge (la bitácora lo admite); el revisor corrió los 14 criterios completos sobre HEAD.

## Hallazgos
### F-06 · NARANJA · `services/go-governance/testdata/mutants/01-no-confirm.patch` y `02-namespace-prefix.patch` · el merge dejó dos mutantes de T06 sin poder aplicarse
Los dos parches modifican `authz/rules.go`, que T07 reescribió para añadir `Decision.Code`. `git apply --check` sobre HEAD da `error: patch failed: services/go-governance/authz/rules.go:120` (01) y `:97` (02); sobre `origin/main` aplican limpio, así que la rotura viene de este PR. El 03 (`unknown-as-true`) y el de identity (`01-totp-window`) siguen aplicando y rompiendo propiedades. Portados a mano a HEAD, el 01 hace fallar `TestPBT_Invariant_NoConfirmNeverAllow` y el 02 `NamespaceNotTestNeverAllow` y `TestPBT_Examples_Canonical`: las propiedades de T06 siguen vivas; solo los parches están obsoletos. `services/go-governance/PBT.md` ya dice que si cambia `authz/rules.go` hay que regenerarlos. La bitácora no menciona los mutantes. Exigido: regenerar ambos parches contra el `rules.go` actual, comprobar `git apply --check` y que rompen propiedades, y anotarlo (tocar esos dos archivos de T06 es consecuencia directa de este diff, no desborde).

### F-07 · AMARILLO · `internal/policy/store_health_test.go` · dos mutantes de `Healthy` sobreviven
Sustituir `if len(disk) != len(s.latest)` por `if false`, o quitar `d.Version != v.Version`, no rompe nada (política extra válida y consecutiva; versión adelantada con el mismo valor). Casos de borde de manipulación externa; un caso por mutante basta.

### F-08 · AMARILLO · `Healthy()` en `/readyz` sin TTL
Cada `/readyz` (sin token) relee `policies.jsonl` con `s.mu` tomado: 5,2 ms con 2000 versiones; con 8 escritores y 4 lectores bajo `-race` un `Get` llegó a esperar 140 ms. En operación real no molesta; la cadena de auditoría sí tiene TTL de 10 s. Candidata: reutilizar el resultado unos segundos.

### F-09 · AMARILLO · `services/go-identity/README.md` · redacción confusa de la excepción de cabeceras
La lista `nosniff, Cache-Control, CSP, HSTS` aparece tras los dos puntos de la excepción y parece describirla, cuando describe las cabeceras de la API. Separar las dos ideas.

## Lo que sí aguanta
- F-01 con el binario real: tras el arranque, `policies.jsonl` con basura añadida, vaciado, borrado o alterado de valor da `policies:"fail"`; al restaurar vuelve a `"ok"` (con `chmod 000` sigue `"ok"` porque corre como root: no es fallo del código). 8 PUT concurrentes de 150 versiones con 4 lectores de `Healthy` bajo `-race`: 0 falsos positivos; reabrir el almacén sale sano; sin efecto en las decisiones del gate.
- Mutantes propios sobre `Healthy`/`policyCheck`: detectados (devuelve nil, sin `poison`, sin comparar el valor, ignorar el error de `parseFile`, `policyCheck` ignora `Healthy`); sobreviven los dos de F-07.
- F-03: `for: 2m`, `4m` o `6m` hacen fallar `promtool test rules`; `TestPolicyChangesStartAtZero` existe y pasa; el mutante de `usersCheck` sobrevive porque `users.Parse` ya rechaza el archivo vacío (`users.go:66`): la prueba fija el contrato «sin usuarios, no listo» y la rama de producción es redundante; el codificador lo reconoce en la bitácora.
- F-04: el README de go-governance es verdad (403 como `user` deja `policy.rejected` en `/audit`, `aqs_policy_changes_total{name="events"}` en 0/0 y `aqs_http_requests_total{code="403"}` en 1); bitácora rectificada.
- F-05: con `curl -i`, `/healthz` y `/readyz` llevan solo `Cache-Control: no-store`; `/metrics` ninguna cabecera de seguridad; `/auth/users` sí lleva CSP, HSTS y `nosniff`. El README dice la verdad.
- F-02: `control-plane.yaml` sin cambios; sigue fuera de alcance (candidata: ampliar C-53 con `fsGroup: 65532` y `strategy: Recreate`).

## Tareas candidatas (fuera de alcance)
- Ampliar C-53 con `fsGroup: 65532` y `strategy: Recreate` para go-governance.
- Verificación incremental de la cadena sin retener el cerrojo; `request_id` en la línea `audit`.
- Que la CI aplique los mutantes `.patch` de T06 (`git apply --check`) para que esta rotura por merge no pase inadvertida.
- Cableado de `promtool` de seguridad en CI (ya es C-33).

VEREDICTO: NO-VERDE
NARANJA|services/go-governance/testdata/mutants/01-no-confirm.patch y 02-namespace-prefix.patch (merge con U4-T06)|Tras el merge, dos mutantes .patch de T06 ya no aplican sobre HEAD porque T07 reescribió authz/rules.go; hay que regenerarlos
INFORME: revisiones/U4-T07/ronda-2.md
