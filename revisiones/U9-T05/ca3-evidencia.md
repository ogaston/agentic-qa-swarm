# U9-T05 — Evidencia de CA-3 (corrida en el clúster vivo `aqs`, 2026-10-21)

La rotación la corrió **el humano** desde este worktree (sha af398dd), porque el respaldo previo
que intentó el codificador fue denegado por el clasificador de permisos. Salida pegada por el humano:

```
$ flock ${XDG_RUNTIME_DIR:-/tmp}/aqs-kind.lock bash -c 'bash scripts/kind/secrets.sh --rotate-demo; echo "rc=$?"; bash scripts/kind/kind-smoke.sh 2>&1 | grep -c "^FALLA"'
aqs-kind: demo.txt migrado de /run/user/1000/aqs-kind a /home/omarjayg/.local/state/aqs-kind (una sola vez)
aqs-kind: admin.txt migrado de /run/user/1000/aqs-kind a /home/omarjayg/.local/state/aqs-kind (una sola vez)
aqs-kind: admin-mfa-secret.txt migrado de /run/user/1000/aqs-kind a /home/omarjayg/.local/state/aqs-kind (una sola vez)
Warning: resource secrets/go-identity-users is missing the kubectl.kubernetes.io/last-applied-configuration annotation which is required by kubectl apply. [...] The missing annotation will be patched automatically.
secrets: credenciales demo y admin escritas en /home/omarjayg/.local/state/aqs-kind (600)
secrets: OK (--rotate-demo: go-identity-users rotado y go-identity reiniciado; ningún otro Secret tocado)
rc=0
```

La línea del conteo de `FALLA` no apareció en lo pegado. El orquestador volvió a correr el smoke
bajo flock justo después (solo lectura sobre el clúster):

```
$ stat -c "%a %n" ~/.local/state/aqs-kind ~/.local/state/aqs-kind/*
700 /home/omarjayg/.local/state/aqs-kind
600 /home/omarjayg/.local/state/aqs-kind/admin-mfa-secret.txt
600 /home/omarjayg/.local/state/aqs-kind/admin.txt
600 /home/omarjayg/.local/state/aqs-kind/demo.txt
$ flock ... bash scripts/kind/kind-smoke.sh ; echo rc ; grep -c ^FALLA
smoke rc=0
0
OK healthz-go-identity
OK healthz-ui-api
OK healthz-go-intake
OK healthz-go-run-controller
OK healthz-go-warm-manager
OK healthz-go-reset
OK healthz-go-governance
OK readyz-go-identity
OK readyz-ui-api
OK readyz-go-intake
OK readyz-go-run-controller
OK readyz-go-warm-manager
OK readyz-go-reset
OK readyz-go-governance
OK login-demo
OK inbox-con-token
OK inbox-sin-token-401
OK warm-estado
OK rbac-test-ns-only
OK netpol-control-positivo
OK netpol-sin-egress-test
OK secrets-no-en-logs
OK ciclo-webhook-confirm-corrida
OK llm-real-configurado
OK llm-egress-acotado
```
