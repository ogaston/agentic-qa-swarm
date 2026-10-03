#!/usr/bin/env bash
# Genera y cifra con SOPS + age los 6 Secrets de la plataforma (U5-T14).
# Uso: BACKUP_ENDPOINT=.. BACKUP_BUCKET=.. BACKUP_ACCESS_KEY_ID=.. BACKUP_SECRET_ACCESS_KEY=.. \
#        scripts/secrets/generate.sh <dev|prod> <age-recipient>
# Lo ejecuta un humano. Los temporales viven en mktemp -d y se borran con trap.
# Solo escribe deploy/flux/<env>/secrets/*.sops.yaml (cifrados) y secrets/kustomization.yaml.
set -euo pipefail

ALPINE=alpine:3.20
SOPS=ghcr.io/getsops/sops:v3.9.1-alpine

env_name=${1:-}
recipient=${2:-}
case "$env_name" in dev | prod) ;; *)
  echo "uso: generate.sh <dev|prod> <age-recipient>" >&2
  exit 2
  ;;
esac
[[ "$recipient" =~ ^age1[0-9a-z]+$ ]] || {
  echo "falta el destinatario age (age1...)" >&2
  exit 2
}
for v in BACKUP_ENDPOINT BACKUP_BUCKET BACKUP_ACCESS_KEY_ID BACKUP_SECRET_ACCESS_KEY; do
  [ -n "${!v:-}" ] || {
    echo "falta la variable $v" >&2
    exit 2
  }
done

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
out="$root/deploy/flux/$env_name/secrets"
[ -d "$out" ] || {
  echo "no existe $out" >&2
  exit 1
}

work=$(mktemp -d)
chmod 755 "$work"
trap 'rm -rf "$work"' EXIT
mkdir "$work/plain" "$work/enc"

# Valores aleatorios, CA y certificado de MinIO (dentro de un contenedor; se devuelven al usuario actual).
docker run --rm --security-opt label=disable -v "$work/plain":/o "$ALPINE" sh -c '
  set -e
  apk add -q openssl >/dev/null 2>&1
  cd /o
  for n in minio-root-password warm-db-password grafana-admin-password; do
    openssl rand -hex 24 | tr -d "\n" > $n
  done
  printf "aqs-kms:%s" "$(openssl rand -base64 32 | tr -d "\n")" > kms
  openssl req -x509 -newkey rsa:2048 -nodes -days 3650 -subj "/CN=aqs-minio-ca" -keyout ca.key -out ca.crt 2>/dev/null
  openssl req -newkey rsa:2048 -nodes -subj "/CN=minio.aqs-system.svc" -keyout tls.key -out tls.csr 2>/dev/null
  printf "subjectAltName=DNS:minio.aqs-system.svc,DNS:minio.aqs-system.svc.cluster.local\n" > san.ext
  openssl x509 -req -in tls.csr -CA ca.crt -CAkey ca.key -CAcreateserial -days 825 -extfile san.ext -out tls.crt 2>/dev/null
  rm -f ca.key tls.csr san.ext ca.srl
'

block() { sed 's/^/      /' "$1"; } # indenta un PEM bajo un escalar de bloque
scalar() { printf '%s' "$(cat "$1")"; }

emit() { # <ns> <nombre> ; el cuerpo de stringData llega por stdin
  {
    printf 'apiVersion: v1\nkind: Secret\nmetadata:\n  name: %s\n  namespace: %s\ntype: Opaque\nstringData:\n' "$2" "$1"
    cat
  } > "$work/plain/$2.yaml"
}

emit aqs-system minio-root <<Y
  root-user: aqs-minio
  root-password: "$(scalar "$work/plain/minio-root-password")"
Y
emit aqs-system minio-kms <<Y
  kms-secret-key: "$(scalar "$work/plain/kms")"
Y
emit aqs-system minio-tls <<Y
  tls.crt: |
$(block "$work/plain/tls.crt")
  tls.key: |
$(block "$work/plain/tls.key")
  ca.crt: |
$(block "$work/plain/ca.crt")
Y
yq_quote() { printf '%s' "$1" | sed "s/'/''/g"; }
emit aqs-system backup-target <<Y
  endpoint: '$(yq_quote "$BACKUP_ENDPOINT")'
  bucket: '$(yq_quote "$BACKUP_BUCKET")'
  access-key-id: '$(yq_quote "$BACKUP_ACCESS_KEY_ID")'
  secret-access-key: '$(yq_quote "$BACKUP_SECRET_ACCESS_KEY")'
Y
emit aqs-test warm-db-credentials <<Y
  password: "$(scalar "$work/plain/warm-db-password")"
Y
emit aqs-observability grafana-admin <<Y
  admin-user: admin
  admin-password: "$(scalar "$work/plain/grafana-admin-password")"
Y

names=(minio-root minio-kms minio-tls backup-target warm-db-credentials grafana-admin)
for n in "${names[@]}"; do
  docker run --rm --security-opt label=disable -v "$work/plain":/in:ro "$SOPS" \
    encrypt --age "$recipient" --encrypted-regex '^(data|stringData)$' "/in/$n.yaml" > "$work/enc/$n.sops.yaml"
  grep -q 'ENC\[' "$work/enc/$n.sops.yaml" || {
    echo "cifrado fallido: $n" >&2
    exit 1
  }
done

# Todo cifrado: recien ahora se toca el arbol.
rm -f "$out"/*.sops.yaml
cp "$work"/enc/*.sops.yaml "$out/"
{
  printf 'apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n'
  for n in "${names[@]}"; do printf '  - %s.sops.yaml\n' "$n"; done
} > "$out/kustomization.yaml"
echo "OK: ${#names[@]} Secrets cifrados en deploy/flux/$env_name/secrets/"
