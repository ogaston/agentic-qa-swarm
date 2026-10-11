#!/usr/bin/env bash
# Crea en el clúster kind `aqs` los Secrets que referencia deploy/flux/kind (U7-T03).
# Valores aleatorios generados aquí: nada se escribe en el repo ni se imprime en la salida.
# Descubre los Secrets con `kubectl kustomize deploy/flux/kind` (secretName, secretKeyRef, secretRef).
# Si un Secret referenciado no está en la lista que este script sabe crear, falla con su nombre
# antes de crear nada. Idempotente: si el Secret ya existe, no se regenera.
# Las contraseñas de los usuarios demo (y el mfa_secret del admin) quedan en
# ${XDG_RUNTIME_DIR:-/tmp}/aqs-kind/ con permisos 600; el resto solo vive en el clúster.
# Requiere la guarda de contexto kind-aqs (sale con 3 en otro contexto).
set -euo pipefail
umask 077

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
# shellcheck disable=SC1091  # lib.sh se carga por ruta dinámica (relativa a root)
. "$root/scripts/kind/lib.sh"

fail() { echo "secrets: $*" >&2; exit 1; }

require_kind_context

GO_IDENTITY_IMAGE="${IMAGE_PREFIX}/go-identity:${IMAGE_TAG}"
state_dir="${XDG_RUNTIME_DIR:-/tmp}/aqs-kind"
work=$(mktemp -d "${XDG_RUNTIME_DIR:-/tmp}/aqs-kind-work.XXXXXX")
chmod 700 "$work"
trap 'rm -rf "$work"' EXIT

# Namespace de cada Secret conocido (según los objetos que lo consumen en deploy/flux/base).
declare -A SECRET_NS=(
  [minio-root]=aqs-system
  [minio-kms]=aqs-system
  [minio-tls]=aqs-system
  [aqs-evidence-s3]=aqs-system
  [warm-db-credentials]=aqs-test
  [go-governance-service-token]=aqs-system
  [go-reset-service-token]=aqs-system
  [go-warm-manager-service-token]=aqs-system
  [go-identity-users]=aqs-system
  [go-intake-webhook]=aqs-system
)

rand_hex() { openssl rand -hex "$1" | tr -d '\n'; }

# --- 1. Descubrimiento de los Secrets que referencia el overlay ---------------------------
refs=$(kubectl kustomize "$root/deploy/flux/kind") || fail "kubectl kustomize deploy/flux/kind falló"
mapfile -t wanted < <(awk '
  /secretName:/ { print $2 }
  /secretKeyRef:|secretRef:/ { f = 3; next }
  f > 0 && /^[[:space:]]*name:/ { print $2; f = 0; next }
  f > 0 { f-- }
' <<<"$refs" | sort -u)
[ "${#wanted[@]}" -gt 0 ] || fail "el overlay no referencia ningún Secret (salida vacía o inesperada)"

# --- 2. Validación: todo lo referenciado debe ser conocido, antes de tocar el clúster --------
for name in "${wanted[@]}"; do
  [ -n "${SECRET_NS[$name]:-}" ] || fail "el Secret '$name' lo referencia deploy/flux/kind y no está en la lista que sabe crear"
done

# --- 3. Generadores (escriben en $1, un directorio privado) --------------------------------
gen_minio_root() { # keys: root-user, root-password
  printf 'aqs-minio' > "$1/root-user"
  rand_hex 24 > "$1/root-password"
}
gen_minio_kms() { # key: kms-secret-key
  printf 'aqs-kms:%s' "$(openssl rand -base64 32 | tr -d '\n')" > "$1/kms-secret-key"
}
gen_minio_tls() { # keys: ca.crt, tls.crt, tls.key (SAN del servicio minio en aqs-system)
  openssl req -x509 -newkey rsa:2048 -nodes -days 3650 -subj "/CN=aqs-kind-minio-ca" \
    -keyout "$1/ca.key" -out "$1/ca.crt" >/dev/null 2>&1
  openssl req -newkey rsa:2048 -nodes -subj "/CN=minio.aqs-system.svc" \
    -keyout "$1/tls.key" -out "$1/tls.csr" >/dev/null 2>&1
  printf 'subjectAltName=DNS:minio.aqs-system.svc,DNS:minio.aqs-system.svc.cluster.local\n' > "$1/san.ext"
  openssl x509 -req -in "$1/tls.csr" -CA "$1/ca.crt" -CAkey "$1/ca.key" -CAcreateserial \
    -days 825 -extfile "$1/san.ext" -out "$1/tls.crt" >/dev/null 2>&1
  rm -f "$1/ca.key" "$1/tls.csr" "$1/san.ext" "$1/ca.srl"
}
gen_aqs_evidence_s3() { # keys: access-key, secret-key
  rand_hex 10 > "$1/access-key"
  rand_hex 24 > "$1/secret-key"
}
gen_warm_db_credentials() { # key: password
  rand_hex 24 > "$1/password"
}
gen_service_token() { # key: token (go-governance, go-reset, go-warm-manager)
  rand_hex 32 > "$1/token"
}
gen_go_identity_users() { # key: users.json; contraseñas y mfa_secret a $state_dir (600)
  local pw_user pw_admin mfa hash_user hash_admin
  pw_user=$(openssl rand -base64 18 | tr -d '\n')
  pw_admin=$(openssl rand -base64 18 | tr -d '\n')
  mfa=$(head -c 20 /dev/urandom | base32 | tr -d '\n=')
  hash_user=$(printf '%s' "$pw_user" | podman run --rm -i "$GO_IDENTITY_IMAGE" hash-password) \
    || fail "go-identity hash-password falló (¿existe la imagen $GO_IDENTITY_IMAGE? ver build-images.sh)"
  hash_admin=$(printf '%s' "$pw_admin" | podman run --rm -i "$GO_IDENTITY_IMAGE" hash-password) \
    || fail "go-identity hash-password falló (admin)"
  jq -n --arg hu "$hash_user" --arg ha "$hash_admin" --arg mfa "$mfa" '[
    {username: "demo", password_hash: $hu, role: "user"},
    {username: "admin", password_hash: $ha, role: "admin", mfa_secret: $mfa}
  ]' > "$1/users.json"
  mkdir -p "$state_dir"
  chmod 700 "$state_dir"
  printf 'demo\n%s\n' "$pw_user" > "$state_dir/demo.txt"
  printf 'admin\n%s\n' "$pw_admin" > "$state_dir/admin.txt"
  printf '%s\n' "$mfa" > "$state_dir/admin-mfa-secret.txt"
  chmod 600 "$state_dir/demo.txt" "$state_dir/admin.txt" "$state_dir/admin-mfa-secret.txt"
}

gen_go_intake_webhook() { # key: secret (HMAC del webhook de GitHub, go-intake)
  rand_hex 32 > "$1/secret"
}

# Archivos de cada Secret: nombre del Secret -> función generadora.
declare -A GENERATOR=(
  [minio-root]=gen_minio_root
  [minio-kms]=gen_minio_kms
  [minio-tls]=gen_minio_tls
  [aqs-evidence-s3]=gen_aqs_evidence_s3
  [warm-db-credentials]=gen_warm_db_credentials
  [go-governance-service-token]=gen_service_token
  [go-reset-service-token]=gen_service_token
  [go-warm-manager-service-token]=gen_service_token
  [go-identity-users]=gen_go_identity_users
  [go-intake-webhook]=gen_go_intake_webhook
)

# --- 4. Creación ------------------------------------------------------------------------
for name in "${wanted[@]}"; do
  ns=${SECRET_NS[$name]}
  if kubectl get secret "$name" -n "$ns" >/dev/null 2>&1; then
    echo "secrets: $ns/$name ya existe; no se regenera"
    continue
  fi
  kubectl get namespace "$ns" >/dev/null 2>&1 || kubectl create namespace "$ns" >/dev/null
  dir="$work/$name"
  mkdir "$dir"
  chmod 700 "$dir"
  "${GENERATOR[$name]}" "$dir"
  # Todos los archivos de $dir son claves del Secret (salvo los auxiliares ya borrados por el generador).
  from_file=()
  for f in "$dir"/*; do
    from_file+=("--from-file=$(basename "$f")=$f")
  done
  kubectl create secret generic "$name" -n "$ns" "${from_file[@]}" --dry-run=client -o yaml \
    | kubectl apply -f - >/dev/null
  echo "secrets: $ns/$name creado"
done

echo "secrets: OK (${#wanted[@]} Secrets referenciados por deploy/flux/kind)"
