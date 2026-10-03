#!/usr/bin/env bash
# E2E de secrets (U5-T14, CA-2 y CA-3): clave age desechable, sobre una copia de HEAD en mktemp -d.
# Imprime: gen rc, nº de .sops.yaml, lista de Secrets/claves, y las comprobaciones de CA-3 y la guardia.
set -uo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 1
SOPS=ghcr.io/getsops/sops:v3.9.1-alpine
KUSTOMIZE=registry.k8s.io/kustomize/kustomize:v5.4.3
YQ='docker run --rm -i --security-opt label=disable mikefarah/yq:4.44.3 -N'
dk() { docker run --rm --security-opt label=disable "$@"; }

t=$(mktemp -d)
trap 'rm -rf "$t"' EXIT
# Copia del arbol de trabajo (incluye cambios sin commitear de los archivos versionados).
{ git ls-files; git ls-files --others --exclude-standard; } | sort -u | while IFS= read -r f; do [ -e "$f" ] && printf '%s\n' "$f"; done | tar -c -T - | tar -x -C "$t"
chmod -R a+rwX "$t"
dk -v "$t":/k alpine:3.20 sh -c 'apk add -q age >/dev/null 2>&1 && age-keygen -o /k/clave.txt 2>/dev/null && chmod 644 /k/clave.txt'
R=$(grep -o 'age1[0-9a-z]*' "$t/clave.txt")

echo "== CA-2"
(cd "$t" && BACKUP_ENDPOINT=https://s3.example.invalid BACKUP_BUCKET=b BACKUP_ACCESS_KEY_ID=a BACKUP_SECRET_ACCESS_KEY=s bash scripts/secrets/generate.sh prod "$R"); echo "gen rc=$?"
ls "$t"/deploy/flux/prod/secrets/*.sops.yaml | wc -l
dk -v "$t":/w -w /w -e SOPS_AGE_KEY_FILE=/w/clave.txt --entrypoint sh "$SOPS" -c 'for f in deploy/flux/prod/secrets/*.sops.yaml; do sops decrypt "$f"; echo ---; done' \
  | $YQ 'select(.kind == "Secret") | .metadata.namespace + "/" + .metadata.name + ":" + (((.data // {}) + (.stringData // {})) | keys | sort | join(","))' | sort

echo "== CA-3"
grep -L 'ENC\[' "$t"/deploy/flux/prod/secrets/*.sops.yaml | wc -l
grep -l -E '^\s+(root-password|password|admin-password|kms-secret-key|secret-access-key|tls.key):\s+[^E]' "$t"/deploy/flux/prod/secrets/*.sops.yaml | wc -l
dk -v "$t":/w -w /w "$KUSTOMIZE" build deploy/flux/prod | grep -c '^kind: Secret'
dk -v "$t":/w -w /w -e SOPS_AGE_KEY_FILE=/w/clave.txt "$SOPS" decrypt deploy/flux/prod/secrets/minio-kms.sops.yaml | grep -c -E 'aqs-kms:[A-Za-z0-9+/]{43}='
dk -v "$t":/w -w /w -e SOPS_AGE_KEY_FILE=/w/clave.txt "$SOPS" decrypt --extract '["stringData"]["tls.crt"]' deploy/flux/prod/secrets/minio-tls.sops.yaml \
  | dk -i alpine:3.20 sh -c 'apk add -q openssl >/dev/null 2>&1; openssl x509 -noout -ext subjectAltName' | grep -c 'minio.aqs-system.svc'

echo "== CA-4 (guardia sobre los 6 cifrados)"
(cd "$t" && bash scripts/ci/check-secrets.sh); echo "cifrados rc=$?"

echo "== negativas"
t2=$(mktemp -d)
trap 'rm -rf "$t" "$t2"' EXIT
cp -r "$t/." "$t2/"
rm -f "$t2"/deploy/flux/prod/secrets/*.sops.yaml
git -C "$t2" status >/dev/null 2>&1
(cd "$t2" && env -u BACKUP_ENDPOINT BACKUP_BUCKET=b BACKUP_ACCESS_KEY_ID=a BACKUP_SECRET_ACCESS_KEY=s bash scripts/secrets/generate.sh prod "$R" 2>/dev/null); echo "sin BACKUP_ENDPOINT rc=$?"
echo "archivos tras fallo: $(ls "$t2"/deploy/flux/prod/secrets/ | tr '\n' ' ')"
(cd "$t2" && BACKUP_ENDPOINT=e BACKUP_BUCKET=b BACKUP_ACCESS_KEY_ID=a BACKUP_SECRET_ACCESS_KEY=s bash scripts/secrets/generate.sh prod 2>/dev/null); echo "sin destinatario rc=$?"
# Secret con sops: pero un valor sin ENC[
f=$t/deploy/flux/prod/secrets/grafana-admin.sops.yaml
sed -i -E 's/^([[:space:]]+admin-user:).*/\1 admin/' "$f"; grep -c -E '^[[:space:]]+admin-user: admin$' "$f"
(cd "$t" && bash scripts/ci/check-secrets.sh >/dev/null 2>&1); echo "valor en claro con sops: rc=$?"
