# Ronda 3 — U4-T07

VEREDICTO: VERDE

Los cuatro mutantes aplican, rompen propiedades PBT con seed y revierten. Los dos casos nuevos de `Healthy` matan sus mutantes. El README de go-identity dice la verdad. Sin regresiones; el diff de la ronda no toca producción. Worktree limpio. Todo lo destructivo se hizo en una copia (`git archive HEAD | tar -x`). `GOTOOLCHAIN=go1.26.8`, `-count=1`, HEAD `49743e5` (código en `5241e16`).

## Verificaciones corridas por el revisor
| # | Resultado |
|---|---|
| F-06 | los 4 mutantes aplican con `git apply --check`, rompen PBT y revierten; los 3 de governance fallan por PBT y el de identity falla `TestPBT_TOTP` |
| F-07 | los 3 mutantes de `Healthy` mueren: longitud (`política_extra_válida`), versión (`versión_adelantada,_mismo_valor`), valor (`alterado`) |
| 1 | identity 205 PASS / 0 FAIL; governance 348 / 0 (ronda 2: 346; los 2 nuevos son los casos de F-07) |
| 4 | `go vet`, `gofmt -l`, `go test -race -count=1 ./...` limpios en ambos módulos |
| 9 | PVC `90 5Gi ReadWriteOnce sin-clase`; Deployment `90 /healthz /readyz 90 go-governance-data`; identity `/healthz /readyz`; kubeconform dev y prod `Valid: 62, Invalid: 0, Errors: 0, Skipped: 0` (con `--network host` y el CA del proxy) |
| 13 | `policies.sh`: 2 `FALLA` de kubeconform por el proxy de la sesión inalcanzable desde el contenedor (entorno); el resto `OK`; kubeconform manual con `--network host` da 62 válidos; la CI de `policies` en HEAD está en success. PrometheusRule: `aqs-backup-rules`, `aqs-rules`, `aqs-security-rules` |
| 14 | imágenes (con `--network host`) `65532:65532`; sin dependencias k8s; `vet` y `gofmt` ok; solo los 4 de `revisiones/U4-T07/` fuera de la lista (no cuentan); `git status` 0 |
| diff | `git diff d2a186e..HEAD --stat`: solo bitácora, `revisiones/`, `store_health_test.go`, dos `.patch` y el README de identity; ningún archivo de producción |

## Hallazgos
### F-06 · resuelto
Los dos parches regenerados son fieles al significado de T06: `01-no-confirm` quita `need("confirmed", DenyNotConfirmed, in.Confirmed)` en `StateRehearsing` (el original de `origin/main` quitaba `need("confirmed", in.Confirmed)` en el mismo `case`; la diferencia es solo la firma nueva por `Decision.Code`); `02-namespace-prefix` sigue siendo `strings.HasPrefix`. Seeds y contraejemplos: 01 `-rapid.seed=1791065833341030446` (Allow sin `confirmed=true` en `inferring->rehearsing`; falla `NoConfirmNeverAllow`); 02 `-rapid.seed=1791065838239072058` (Allow con namespace `"aqs-test "`; fallan `NamespaceNotTestNeverAllow` y `TestPBT_Examples_Canonical`); 03 `-rapid.seed=1791065843023308436` (`unknown` pasa como Allow; fallan `NoConfirmNeverAllow` y `NoResetVerifiedNeverAllow`); identity 01-totp-window `-rapid.seed=1791065848492280843` (código aceptado con desplazamiento -1m30s). Ninguno rompe la compilación. `PBT.md` sigue correcto.

### F-07 · resuelto
### F-09 · resuelto
El README separa las cabeceras de la API (`nosniff`, `no-store`, CSP, HSTS) de la excepción de `/healthz`, `/readyz` y `/metrics`; coincide con el `curl -i` de la ronda 2.
### F-08 · sin cambios
Candidata fuera de alcance.

### Nota de CI (informativa)
El PR #25 no tiene runs para `5241e16` (se subió junto con `49743e5`, y solo este disparó CI); `49743e5` es descendiente directo y solo añade documentación, así que sus runs cubren el código.

## CI de GitHub, PR #25
Sobre `49743e5`: `ci` (run 37157884990), `policies` y `contracts` en success. Jobs de `ci`: `no-latest`, `discover`, `build`, `vuln` (govulncheck), `test` y `sbom` de go-identity y go-governance en success; `publish` skipped. Runs anteriores (`aa700d4`, `d2a186e`) también en success.

## Tareas candidatas
- Que la CI haga `git apply --check` de los mutantes `.patch`.
- TTL para `Healthy` en `/readyz` (F-08); `fsGroup: 65532` y `strategy: Recreate` en C-53; cableado de `promtool` en C-33.

VEREDICTO: VERDE
INFORME: revisiones/U4-T07/ronda-3.md
