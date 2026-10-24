#!/usr/bin/env bash
# Crea en el clúster kind `aqs` los Secrets que referencia deploy/flux/kind (U7-T03).
# Valores aleatorios generados aquí: nada se escribe en el repo ni se imprime en la salida.
# Descubre los Secrets con `kubectl kustomize deploy/flux/kind` (secretName, secretKeyRef, secretRef).
# Si un Secret referenciado no está en la lista que este script sabe crear, falla con su nombre
# antes de crear nada. Idempotente: si el Secret ya existe, no se regenera.
# Las contraseñas de los usuarios demo (y el mfa_secret del admin) quedan en
# $AQS_KIND_STATE_DIR (por defecto ${XDG_STATE_HOME:-$HOME/.local/state}/aqs-kind, permisos 700/600, U9-T05);
# el resto solo vive en el clúster.
# --rotate-demo (U9-T05): regenera solo go-identity-users y los ficheros demo/admin/mfa, sin exigir
# litellm.env ni deepseek.key, y reinicia go-identity. Nunca usa un hash vacío ni borra el Secret viejo antes de tener el nuevo.
# Requiere la guarda de contexto kind-aqs (sale con 3 en otro contexto).
set -euo pipefail
umask 077

rotate_demo=0
case "${1:-}" in
  --rotate-demo) rotate_demo=1 ;;
  "") ;;
  *) echo "uso: secrets.sh [--rotate-demo]" >&2; exit 2 ;;
esac

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
# shellcheck disable=SC1091  # lib.sh se carga por ruta dinámica (relativa a root)
. "$root/scripts/kind/lib.sh"

fail() { echo "secrets: $*" >&2; exit 1; }

require_kind_context

GO_IDENTITY_IMAGE="${IMAGE_PREFIX}/go-identity:${IMAGE_TAG}"
state_dir="$AQS_KIND_STATE_DIR"
work=$(mktemp -d "${AQS_KIND_RUNTIME_DIR}-work.XXXXXX")
chmod 700 "$work"
trap 'rm -rf "$work"' EXIT
# Insumos persistentes (U9-T05): crear el directorio, migrar una vez desde tmpfs y validar permisos antes de usarlos.
kind_state_init || fail "no se pudo crear $AQS_KIND_STATE_DIR"
kind_state_migrate || fail "migración de insumos desde $AQS_KIND_RUNTIME_DIR falló"
kind_state_check || fail "permisos de insumos incorrectos (ver arriba); corrígelos con chmod y vuelve a ejecutar"

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
  [target-app-baseline-token]=aqs-test
  [agents-llm-key]=aqs-system
  [llm-upstream]=aqs-system
)

# --- U8-T09: insumos del humano (fuera del repo). Se validan ANTES de tocar el clúster y sus valores
# nunca se imprimen: los errores nombran la variable, no su contenido.
litellm_env="$AQS_KIND_STATE_DIR/litellm.env"
deepseek_key="$AQS_KIND_STATE_DIR/deepseek.key"
LITELLM_UPSTREAM_IP="" LITELLM_UPSTREAM_PORT="" LITELLM_UPSTREAM_MODEL="" LITELLM_UPSTREAM_URL="" LITELLM_UPSTREAM_IPS=""

# Lee una variable de litellm.env sin evaluar el archivo (no se ejecuta como shell).
litellm_var() {
  { grep -E "^$1=" "$litellm_env" || true; } | tail -n1 | cut -d= -f2- | tr -d '\r'
}

check_litellm_inputs() {
  [ -f "$litellm_env" ] || fail "falta $litellm_env (insumo del humano, modo 600); ponlo en esa ruta. Los Secrets existentes no se tocan"
  [ -f "$deepseek_key" ] || fail "falta $deepseek_key (insumo del humano, modo 600); ponlo en esa ruta. Los Secrets existentes no se tocan"
  [ -s "$deepseek_key" ] || fail "$deepseek_key está vacío. Los Secrets existentes no se tocan"
  LITELLM_UPSTREAM_IP=$(litellm_var LITELLM_UPSTREAM_IP)
  LITELLM_UPSTREAM_PORT=$(litellm_var LITELLM_UPSTREAM_PORT)
  LITELLM_UPSTREAM_MODEL=$(litellm_var LITELLM_UPSTREAM_MODEL)
  # Proveedor directo (p. ej. DeepSeek): LITELLM_UPSTREAM_URL=https://host[/v1]. El egreso se abre a las IPv4 que
  # resuelve el host en este momento (IP/32 cada una); LITELLM_UPSTREAM_IP es opcional y se suma si existe.
  LITELLM_UPSTREAM_URL=$(litellm_var LITELLM_UPSTREAM_URL)
  if [ -n "$LITELLM_UPSTREAM_URL" ]; then
    [[ "$LITELLM_UPSTREAM_URL" =~ ^https://([A-Za-z0-9.-]+)(:[0-9]{1,5})?(/[A-Za-z0-9._/-]*)?$ ]] || fail "LITELLM_UPSTREAM_URL debe ser https://host[:puerto][/ruta] en litellm.env"
    local host=${BASH_REMATCH[1]}
    [ -n "$LITELLM_UPSTREAM_PORT" ] || LITELLM_UPSTREAM_PORT=443
    LITELLM_UPSTREAM_IPS=$(getent ahostsv4 "$host" | awk '{print $1}' | sort -u | tr '\n' ' ')
    if [ -n "$LITELLM_UPSTREAM_IP" ]; then LITELLM_UPSTREAM_IPS="$LITELLM_UPSTREAM_IPS$LITELLM_UPSTREAM_IP"; fi
    [ -n "${LITELLM_UPSTREAM_IPS// /}" ] || fail "no se pudo resolver $host (¿sin red?)"
  else
    [[ "$LITELLM_UPSTREAM_IP" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] || fail "LITELLM_UPSTREAM_IP ausente o no es IPv4 literal en litellm.env"
    LITELLM_UPSTREAM_IPS=$LITELLM_UPSTREAM_IP
  fi
  [[ "$LITELLM_UPSTREAM_PORT" =~ ^[0-9]{1,5}$ ]] || fail "LITELLM_UPSTREAM_PORT ausente o no numérico en litellm.env"
  [[ "$LITELLM_UPSTREAM_MODEL" =~ ^[A-Za-z0-9._/-]+$ ]] || fail "LITELLM_UPSTREAM_MODEL ausente o con caracteres no válidos en litellm.env"
}

# Namespace con apply (no create): así lleva last-applied-configuration y apply -k no avisa (F-04).
ensure_namespace() {
  kubectl create namespace "$1" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
}

# ConfigMap go-reset-baseline con el baseline REAL de la app de referencia (U8-T04, C-104), leido de
# demo/target-app/baseline/baseline.sh. Sustituye al relleno kind-stub de U7-T03 (etiqueta aqs.io/kind-stub).
# Si el ConfigMap existe sin esa etiqueta (un baseline real aplicado a mano), no se toca.
ensure_baseline_real() {
  local ns=aqs-system name=go-reset-baseline src="$root/demo/target-app/baseline/baseline.sh" stub
  [ -f "$src" ] || fail "falta el baseline de la app de referencia: $src"
  if kubectl get configmap "$name" -n "$ns" >/dev/null 2>&1; then
    stub=$(kubectl get configmap "$name" -n "$ns" -o jsonpath='{.metadata.labels.aqs\.io/kind-stub}')
    if [ "$stub" != "true" ]; then
      echo "secrets: $ns/configmap $name ya existe y no es relleno de kind; no se toca"
      return 0
    fi
    kubectl delete configmap "$name" -n "$ns" >/dev/null || fail "no se pudo quitar el relleno kind-stub"
    echo "secrets: $ns/configmap $name: relleno kind-stub sustituido por el baseline real"
  fi
  ensure_namespace "$ns"
  kubectl create configmap "$name" -n "$ns" --from-file=baseline.sh="$src" --dry-run=client -o yaml \
    | kubectl apply -f - >/dev/null || fail "no se pudo crear el ConfigMap $name"
  echo "secrets: $ns/configmap $name creado (baseline real de demo/target-app)"
}

# Secret target-app-baseline-token: el token del listener de baseline de la app (aqs-test, warm-app) y
# el que lee go-reset (aqs-system). Mismo valor en los dos namespaces; se copia sin imprimirlo.
copy_target_app_token() {
  local name=target-app-baseline-token src=aqs-test dst=aqs-system
  if kubectl get secret "$name" -n "$dst" >/dev/null 2>&1; then
    # Si ya existe, debe llevar el mismo valor que el de aqs-test: si no, warm-app y go-reset
    # discreparian y el baseline fallaria en silencio con 401 (U8-T04, F-06). No se regenera ni se imprime.
    if [ "$(kubectl get secret "$name" -n "$dst" -o jsonpath='{.data.token}')" = \
         "$(kubectl get secret "$name" -n "$src" -o jsonpath='{.data.token}')" ]; then
      echo "secrets: $dst/$name ya existe con el mismo valor que $src; no se regenera"
      return 0
    fi
    fail "$dst/$name y $src/$name tienen valores distintos; borra uno y vuelve a ejecutar"
  fi
  ensure_namespace "$dst"
  kubectl get secret "$name" -n "$src" -o json \
    | jq --arg ns "$dst" '{apiVersion: "v1", kind: "Secret", type: .type, metadata: {name: .metadata.name, namespace: $ns}, data: .data}' \
    | kubectl apply -f - >/dev/null || fail "no se pudo copiar $name a $dst"
  echo "secrets: $dst/$name copiado desde $src"
}

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

# Insumos del humano (U8-T09): validados antes de crear nada. --rotate-demo no los necesita (U9-T05).
if [ "$rotate_demo" -eq 0 ]; then check_litellm_inputs; fi

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
# Los ficheros demo/admin/mfa se escriben en $work/aux (fuera de $1, así no entran como claves del Secret)
# y solo se publican en $state_dir DESPUÉS de que el Secret nuevo se haya aplicado (publish_go_identity_aux).
gen_go_identity_users() { # key: users.json
  local pw_user pw_admin mfa hash_user hash_admin
  pw_user=$(openssl rand -base64 18 | tr -d '\n')
  pw_admin=$(openssl rand -base64 18 | tr -d '\n')
  mfa=$(head -c 20 /dev/urandom | base32 | tr -d '\n=')
  ensure_go_identity_image
  hash_user=$(printf '%s' "$pw_user" | podman run --rm -i "$GO_IDENTITY_IMAGE" hash-password) \
    || fail "go-identity hash-password falló (¿existe la imagen $GO_IDENTITY_IMAGE? ver build-images.sh)"
  hash_admin=$(printf '%s' "$pw_admin" | podman run --rm -i "$GO_IDENTITY_IMAGE" hash-password) \
    || fail "go-identity hash-password falló (admin)"
  [ -n "$hash_user" ] && [ -n "$hash_admin" ] \
    || fail "go-identity hash-password devolvió un hash vacío; no se reemplaza el Secret go-identity-users"
  jq -n --arg hu "$hash_user" --arg ha "$hash_admin" --arg mfa "$mfa" '[
    {username: "demo", password_hash: $hu, role: "user"},
    {username: "admin", password_hash: $ha, role: "admin", mfa_secret: $mfa}
  ]' > "$1/users.json"
  mkdir -p "$work/aux" && chmod 700 "$work/aux"
  printf 'demo\n%s\n' "$pw_user" > "$work/aux/demo.txt"
  printf 'admin\n%s\n' "$pw_admin" > "$work/aux/admin.txt"
  printf '%s\n' "$mfa" > "$work/aux/admin-mfa-secret.txt"
}

# Publica los ficheros demo/admin/mfa en el directorio de insumos (600), tras aplicar el Secret.
publish_go_identity_aux() {
  [ -d "$work/aux" ] || return 0
  kind_state_init
  install -m 600 "$work/aux/demo.txt" "$work/aux/admin.txt" "$work/aux/admin-mfa-secret.txt" "$state_dir/"
  echo "secrets: credenciales demo y admin escritas en $state_dir (600)"
}

# Imagen go-identity para hash-password: si no está en podman local, se exporta del nodo kind (U9-T05).
ensure_go_identity_image() {
  podman image exists "$GO_IDENTITY_IMAGE" && return 0
  local node="${KIND_CLUSTER_NAME}-control-plane" tar="$work/go-identity.tar"
  if podman exec "$node" ctr -n k8s.io images export /tmp/go-identity.tar "$GO_IDENTITY_IMAGE" >/dev/null 2>&1 \
     && podman cp "$node:/tmp/go-identity.tar" "$tar" >/dev/null 2>&1 \
     && podman load -i "$tar" >/dev/null 2>&1 \
     && podman image exists "$GO_IDENTITY_IMAGE"; then
    echo "secrets: imagen $GO_IDENTITY_IMAGE exportada del nodo kind $node"
    return 0
  fi
  fail "la imagen $GO_IDENTITY_IMAGE no está en podman local ni en el nodo $node; ejecuta scripts/kind/build-images.sh y load-images.sh"
}

# --rotate-demo (U9-T05): regenera go-identity-users, valida los hashes ANTES de tocar el clúster,
# aplica el Secret nuevo, publica los ficheros demo/admin/mfa y reinicia go-identity esperando el rollout.
rotate_demo() {
  local dir="$work/go-identity-users" ns=aqs-system
  mkdir -p "$dir" && chmod 700 "$dir"
  gen_go_identity_users "$dir"
  kubectl create secret generic go-identity-users -n "$ns" --from-file=users.json="$dir/users.json" \
    --dry-run=client -o yaml | kubectl apply -f - >/dev/null \
    || fail "no se pudo aplicar el Secret go-identity-users (el anterior sigue vigente)"
  publish_go_identity_aux
  kubectl rollout restart deployment/go-identity -n "$ns" >/dev/null || fail "no se pudo reiniciar go-identity"
  kubectl rollout status deployment/go-identity -n "$ns" --timeout=180s >/dev/null || fail "go-identity no llegó a Ready tras la rotación"
  echo "secrets: OK (--rotate-demo: go-identity-users rotado y go-identity reiniciado; ningún otro Secret tocado)"
  exit 0
}

if [ "$rotate_demo" -eq 1 ]; then
  rotate_demo
fi

gen_go_intake_webhook() { # key: secret (HMAC del webhook de GitHub, go-intake)
  rand_hex 32 > "$1/secret"
}
# Clave del gateway del humano (U8-T09, ronda 3): planner y reporter la presentan como Bearer al endpoint
# compatible con OpenAI. Sale de deepseek.key (fuera del repo); se reaplica en cada ejecución.
gen_agents_llm_key() { # key: api-key
  tr -d ' \r\n' < "$deepseek_key" > "$1/api-key"
}
# Endpoint y modelo del gateway (U8-T09, ronda 3): salen de litellm.env. Se reaplican en cada ejecución.
gen_llm_upstream() { # keys: base-url (http://IP:PUERTO/v1), host (IP literal del gateway), model (modelo del gateway)
  if [ -n "$LITELLM_UPSTREAM_URL" ]; then
    printf '%s' "${LITELLM_UPSTREAM_URL%/}" > "$1/base-url"
    printf '%s' "$(sed -E 's#^https://([^:/]+).*#\1#' <<<"$LITELLM_UPSTREAM_URL")" > "$1/host"
  else
    printf 'http://%s:%s/v1' "$LITELLM_UPSTREAM_IP" "$LITELLM_UPSTREAM_PORT" > "$1/base-url"
    printf '%s' "$LITELLM_UPSTREAM_IP" > "$1/host"
  fi
  printf '%s' "$LITELLM_UPSTREAM_MODEL" > "$1/model"
}

# NetworkPolicy de egreso de planner y reporter al gateway: IP/32 y puerto de litellm.env. Se aplica al clúster
# y NO se escribe en el repo (kubectl apply -k no la poda). Es aditiva a las de base/ (DNS, MinIO del reporter).
render_agents_egress() {
  local blocks="" ip
  for ip in $LITELLM_UPSTREAM_IPS; do blocks+="        - ipBlock:
            cidr: ${ip}/32
"; done
  kubectl apply -f - >/dev/null <<EOF || fail "no se pudo aplicar la NetworkPolicy agents-egress-upstream"
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: agents-egress-upstream
  namespace: aqs-system
spec:
  podSelector:
    matchExpressions:
      - key: app.kubernetes.io/name
        operator: In
        values: [agent-planner, agent-reporter]
  policyTypes: [Egress]
  egress:
    - to:
${blocks%$'\n'}
      ports:
        - {protocol: TCP, port: ${LITELLM_UPSTREAM_PORT}}
EOF
  echo "secrets: aqs-system/networkpolicy agents-egress-upstream renderizada desde litellm.env (IP/32 y puerto locales)"
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
  [target-app-baseline-token]=gen_service_token
  [agents-llm-key]=gen_agents_llm_key
  [llm-upstream]=gen_llm_upstream
)

# --- 4. Creación ------------------------------------------------------------------------
for name in "${wanted[@]}"; do
  ns=${SECRET_NS[$name]}
  # agents-llm-key y llm-upstream salen de los insumos del humano: se reaplican siempre para que un cambio llegue.
  if [[ "$name" != agents-llm-key && "$name" != llm-upstream ]] && kubectl get secret "$name" -n "$ns" >/dev/null 2>&1; then
    echo "secrets: $ns/$name ya existe; no se regenera"
    continue
  fi
  ensure_namespace "$ns"
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
  if [ "$name" = go-identity-users ]; then publish_go_identity_aux; fi
  echo "secrets: $ns/$name creado"
done

render_agents_egress

for name in "${wanted[@]}"; do
  if [ "$name" = target-app-baseline-token ]; then copy_target_app_token; fi
done
ensure_baseline_real

echo "secrets: OK (${#wanted[@]} Secrets referenciados por deploy/flux/kind; ConfigMap go-reset-baseline real)"
