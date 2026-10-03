#!/usr/bin/env bash
# E2E de secrets (U5-T14, CA-2, CA-3, CA-4 y CA-7): clave age desechable, sobre una copia en mktemp -d.
# Imprime las salidas de los criterios y las COMPARA con las esperadas; sale 1 si alguna difiere.
set -uo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 1
SOPS=ghcr.io/getsops/sops:v3.9.1-alpine
KUSTOMIZE=registry.k8s.io/kustomize/kustomize:v5.4.3
YQ='docker run --rm -i --security-opt label=disable mikefarah/yq:4.44.3 -N'
dk() { docker run --rm --security-opt label=disable "$@"; }

fallos=0
check() { # <nombre> <obtenido> <esperado>
  if [ "$2" = "$3" ]; then echo "  [ok] $1"; else echo "  [DIFIERE] $1: obtenido='$2' esperado='$3'"; fallos=1; fi
}

umask 077
t=$(mktemp -d)
t2=$(mktemp -d)
trap 'rm -rf "$t" "$t2"' EXIT
# Copia del arbol de trabajo (incluye cambios sin commitear), no sensible: se extrae con umask 022 porque
# yq/kustomize corren con un uid no root dentro del contenedor. Lo unico sensible es clave.txt (600, umask 077).
chmod 755 "$t"
(umask 022; { git ls-files; git ls-files --others --exclude-standard; } | sort -u | while IFS= read -r f; do [ -e "$f" ] && printf '%s\n' "$f"; done | tar -c -T - | tar -x -C "$t")
dk -v "$t":/k alpine:3.20 sh -c 'apk add -q age >/dev/null 2>&1 && age-keygen -o /k/clave.txt 2>/dev/null'
R=$(grep -o 'age1[0-9a-z]*' "$t/clave.txt")

echo "== CA-2"
(cd "$t" && BACKUP_ENDPOINT=https://s3.example.invalid BACKUP_BUCKET=b BACKUP_ACCESS_KEY_ID=a BACKUP_SECRET_ACCESS_KEY=s bash scripts/secrets/generate.sh prod "$R"); rc=$?
echo "gen rc=$rc"
n=$(find "$t"/deploy/flux/prod/secrets -name '*.sops.yaml' | wc -l)
echo "$n"
# shellcheck disable=SC2016 # el for se expande dentro del contenedor
lista=$(dk -v "$t":/w -w /w -e SOPS_AGE_KEY_FILE=/w/clave.txt --entrypoint sh "$SOPS" -c 'for f in deploy/flux/prod/secrets/*.sops.yaml; do sops decrypt "$f"; echo ---; done' \
  | $YQ 'select(.kind == "Secret") | .metadata.namespace + "/" + .metadata.name + ":" + (((.data // {}) + (.stringData // {})) | keys | sort | join(","))' | sort)
echo "$lista"
check "gen rc" "$rc" 0
check "6 archivos" "$n" 6
check "contrato de Secrets" "$lista" "aqs-observability/grafana-admin:admin-password,admin-user
aqs-system/backup-target:access-key-id,bucket,endpoint,secret-access-key
aqs-system/minio-kms:kms-secret-key
aqs-system/minio-root:root-password,root-user
aqs-system/minio-tls:ca.crt,tls.crt,tls.key
aqs-test/warm-db-credentials:password"

echo "== CA-3"
a=$(grep -L 'ENC\[' "$t"/deploy/flux/prod/secrets/*.sops.yaml | wc -l)
b=$(grep -l -E '^\s+(root-password|password|admin-password|kms-secret-key|secret-access-key|tls.key):\s+[^E]' "$t"/deploy/flux/prod/secrets/*.sops.yaml | wc -l)
c=$(dk -v "$t":/w -w /w "$KUSTOMIZE" build deploy/flux/prod | grep -c '^kind: Secret')
d=$(dk -v "$t":/w -w /w -e SOPS_AGE_KEY_FILE=/w/clave.txt "$SOPS" decrypt deploy/flux/prod/secrets/minio-kms.sops.yaml | grep -c -E 'aqs-kms:[A-Za-z0-9+/]{43}=')
e=$(dk -v "$t":/w -w /w -e SOPS_AGE_KEY_FILE=/w/clave.txt "$SOPS" decrypt --extract '["stringData"]["tls.crt"]' deploy/flux/prod/secrets/minio-tls.sops.yaml \
  | dk -i alpine:3.20 sh -c 'apk add -q openssl >/dev/null 2>&1; openssl x509 -noout -ext subjectAltName' | grep -c 'minio.aqs-system.svc')
printf '%s\n%s\n%s\n%s\n%s\n' "$a" "$b" "$c" "$d" "$e"
check "sin ENC[ (esperado 0)" "$a" 0
check "valores en claro (esperado 0)" "$b" 0
check "Secrets en build (esperado 6)" "$c" 6
check "formato KMS (esperado 1)" "$d" 1
check "SAN del certificado (esperado 1)" "$e" 1

echo "== CA-4 (guardia sobre los 6 cifrados)"
(cd "$t" && bash scripts/ci/check-secrets.sh); rc=$?
echo "cifrados rc=$rc"
check "guardia sobre cifrados" "$rc" 0

echo "== CA-7 (policies.sh completo sobre dev y prod generados)"
(cd "$t" && BACKUP_ENDPOINT=https://s3.example.invalid BACKUP_BUCKET=b BACKUP_ACCESS_KEY_ID=a BACKUP_SECRET_ACCESS_KEY=s bash scripts/secrets/generate.sh dev "$R") > /dev/null
(cd "$t" && bash scripts/ci/policies.sh > "$t2/pol.out" 2> "$t2/pol.err"); rc=$?
echo "policies-generado rc=$rc"
nvalid=$(sed -n 's/.*Valid: \([0-9][0-9]*\).*/\1/p' "$t2/pol.err" | tail -1)
echo "kubeconform-valid-generado ${nvalid:-0}"
check "policies sobre generado" "$rc" 0
if [ "${nvalid:-0}" -gt 0 ]; then check "kubeconform valida recursos (>0)" ok ok; else check "kubeconform valida recursos (>0)" "${nvalid:-0}" ">0"; fi
grep -E '^(OK|FALLA) ' "$t2/pol.out" | sort | sed 's/^/  /'

echo "== CA-4 negativa a traves de policies.sh (Secret en claro)"
printf -- '---\napiVersion: v1\nkind: Secret\nmetadata: {name: fuga, namespace: aqs-system}\nstringData: {password: hunter2}\n' >> "$t/deploy/flux/base/minio/service.yaml"
(cd "$t" && bash scripts/ci/policies.sh > "$t2/pol2.out" 2> /dev/null); rc=$?
echo "claro via policies rc=$rc"
check "policies rechaza Secret en claro" "$rc" 1
grep -E '^FALLA check-secrets' "$t2/pol2.out" | sed 's/^/  /'

echo "== negativas de generate.sh"
cp -r "$t/." "$t2/"
rm -f "$t2"/deploy/flux/prod/secrets/*.sops.yaml
printf 'apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources: []\n' > "$t2/deploy/flux/prod/secrets/kustomization.yaml"
(cd "$t2" && env -u BACKUP_ENDPOINT BACKUP_BUCKET=b BACKUP_ACCESS_KEY_ID=a BACKUP_SECRET_ACCESS_KEY=s bash scripts/secrets/generate.sh prod "$R" 2> /dev/null); rc=$?
echo "sin BACKUP_ENDPOINT rc=$rc"
restos=$(find "$t2"/deploy/flux/prod/secrets -type f -printf '%f ')
echo "archivos tras fallo: $restos"
check "sin BACKUP_ENDPOINT" "$rc" 2
check "sin archivos tras fallo" "$restos" "kustomization.yaml "
(cd "$t2" && BACKUP_ENDPOINT=e BACKUP_BUCKET=b BACKUP_ACCESS_KEY_ID=a BACKUP_SECRET_ACCESS_KEY=s bash scripts/secrets/generate.sh prod 2> /dev/null); rc=$?
echo "sin destinatario rc=$rc"
check "sin destinatario" "$rc" 2

echo "== negativa: Secret con sops: y un valor sin ENC["
f=$t/deploy/flux/prod/secrets/grafana-admin.sops.yaml
sed -i -E 's/^([[:space:]]+admin-user:).*/\1 admin/' "$f"
grep -c -E '^[[:space:]]+admin-user: admin$' "$f"
(cd "$t" && bash scripts/ci/check-secrets.sh > /dev/null 2>&1); rc=$?
echo "valor en claro con sops: rc=$rc"
check "guardia: valor sin ENC[" "$rc" 1

echo "== resumen"
if [ "$fallos" -eq 0 ]; then echo "E2E OK"; else echo "E2E FALLA"; fi
exit "$fallos"
