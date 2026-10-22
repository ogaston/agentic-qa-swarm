#!/usr/bin/env bash
# Funciones comunes de scripts/kind/ (U7-T01). Se carga con `source`; no ejecuta nada al cargarse.
# Nombre del clúster efímero y contexto de kubectl que le corresponde.
KIND_CLUSTER_NAME=aqs
KIND_CONTEXT=kind-aqs
export KIND_EXPERIMENTAL_PROVIDER=podman

# Guarda: todo script que hable con el clúster la llama antes de cualquier kubectl.
# Sale con 3 si el contexto actual de kubectl no es exactamente kind-aqs.
require_kind_context() {
  local ctx
  ctx=$(kubectl config current-context 2>/dev/null) || ctx=""
  if [ "$ctx" != "$KIND_CONTEXT" ]; then
    echo "contexto ${ctx:-(ninguno)} no es ${KIND_CONTEXT}" >&2
    exit 3
  fi
}

# Indica si el clúster efímero `aqs` existe (no toca otros clústeres).
kind_cluster_exists() {
  kind get clusters 2>/dev/null | grep -qx "$KIND_CLUSTER_NAME"
}

# Imágenes de la plataforma (U7-T02): prefijo y tag que esperan los manifiestos de deploy/flux/base.
# shellcheck disable=SC2034  # variables usadas por build-images.sh y load-images.sh al hacer source
IMAGE_PREFIX=ghcr.io/ogaston/agentic-qa-swarm
# shellcheck disable=SC2034  # ídem
IMAGE_TAG=0.0.0

# Imprime una línea «<nombre> <dir>» por imagen, tomando la lista de scripts/ci/list-services.sh
# (no se escribe a mano). Su número debe coincidir con los Dockerfile que hay bajo services/ y agents/,
# contados por separado (U8-T03: ya no hay un número fijo). Si AQS_IMAGES está definida (nombres separados
# por espacio), filtra a ese subconjunto y rechaza nombres desconocidos. Sale 1 con mensaje si algo falla.
# Uso: images=$(platform_images "$root") || exit 1
platform_images() {
  local root=$1 list dir name n out="" count=0 expected=0
  local -a dirs=()
  list=$(cd "$root" && bash scripts/ci/list-services.sh) || {
    echo "no se pudo ejecutar scripts/ci/list-services.sh" >&2
    return 1
  }
  while IFS= read -r dir; do
    dirs+=("$dir")
  done < <(jq -r '.[]' <<<"$list")
  count=${#dirs[@]}
  expected=$(cd "$root" && ls -d services/*/Dockerfile agents/*/Dockerfile 2>/dev/null | wc -l)
  if [ "$count" -eq 0 ] || [ "$count" -ne "$expected" ]; then
    echo "la lista de imágenes da $count entradas; hay $expected Dockerfile bajo services/ y agents/" >&2
    return 1
  fi
  for dir in "${dirs[@]}"; do
    name=${dir##*/}
    if [ -n "${AQS_IMAGES:-}" ]; then
      case " $AQS_IMAGES " in *" $name "*) ;; *) continue ;; esac
    fi
    out+="$name $dir"$'\n'
  done
  for n in ${AQS_IMAGES:-}; do
    case "$out" in *"$n "*) ;; *)
      echo "AQS_IMAGES: '$n' no es una imagen de la plataforma" >&2
      return 1 ;;
    esac
  done
  printf '%s' "$out"
}

# Imágenes de demo/*/ (U8-T04): cada directorio con Dockerfile, derivado del disco (no escrito a mano).
# Misma forma que platform_images: «<nombre> <dir>» por línea.
demo_images() {
  local root=$1 d
  for d in "$root"/demo/*/; do
    d=${d%/}
    [ -f "$d/Dockerfile" ] || continue
    printf '%s demo/%s\n' "${d##*/}" "${d##*/}"
  done
}

# all_images <root>: platform_images + demo_images. AQS_IMAGES filtra y valida contra el conjunto completo.
all_images() {
  local root=$1 plat dem all sel="" line name n
  plat=$(AQS_IMAGES= platform_images "$root") || return 1
  dem=$(demo_images "$root") || return 1
  all="$plat"$'\n'"$dem"
  if [ -n "${AQS_IMAGES:-}" ]; then
    while IFS= read -r line; do
      [ -n "$line" ] || continue
      name=${line%% *}
      case " $AQS_IMAGES " in *" $name "*) sel+="$line"$'\n' ;; esac
    done <<<"$all"
    for n in $AQS_IMAGES; do
      printf '%s\n' "$sel" | awk -v n="$n" '$1 == n { f = 1 } END { exit !f }' || {
        echo "AQS_IMAGES: '$n' no es una imagen de la plataforma ni de demo/" >&2
        return 1
      }
    done
    all=$sel
  fi
  printf '%s' "$all"
}

# --- Pods de prueba de U8-T05 (scripts/test/u8-*-kind.sh). Los crea y borra cada script (trap). ---
# Todos corren como 65532, sin token de ServiceAccount, sin privilegios. Ninguna credencial sale por
# la línea de comandos ni se imprime: el mc del pod las toma de variables de entorno desde los Secrets.

# pod_sleep_up <ns> <pod> <imagen> [etiqueta app.kubernetes.io/name]: pod que duerme; la etiqueta decide
# qué pasa el ingreso de NetworkPolicy (go-run-controller es el único cliente de los agentes).
pod_sleep_up() {
  local ns=$1 pod=$2 img=$3 label=${4:-} lbl=""
  [ -n "$label" ] && lbl="    app.kubernetes.io/name: $label"
  kubectl apply -n "$ns" -f - >/dev/null <<YAML || return 1
apiVersion: v1
kind: Pod
metadata:
  name: $pod
  namespace: $ns
  labels:
    aqs.io/test: u8-t05
$lbl
spec:
  restartPolicy: Never
  automountServiceAccountToken: false
  securityContext:
    runAsNonRoot: true
    runAsUser: 65532
    seccompProfile:
      type: RuntimeDefault
  containers:
    - name: probe
      image: $img
      command: ["sleep", "900"]
      securityContext:
        allowPrivilegeEscalation: false
        capabilities:
          drop: ["ALL"]
YAML
  kubectl wait -n "$ns" --for=condition=Ready "pod/$pod" --timeout=180s >/dev/null
}

# mc_pod_up <ns> <pod>: pod con la imagen de MinIO fijada (la del StatefulSet minio, mismo digest) y mc.
# Credenciales por env desde minio-root (raíz, solo para preparar y limpiar) y aqs-evidence-s3 (el usuario
# de los agentes, lo que se prueba). CA de MinIO montada: SSL_CERT_FILE. TLS verificado, sin --insecure.
mc_pod_up() {
  local ns=$1 pod=$2 img
  img=$(kubectl get statefulset minio -n "$ns" -o jsonpath='{.spec.template.spec.containers[0].image}') || return 1
  [ -n "$img" ] || return 1
  kubectl apply -n "$ns" -f - >/dev/null <<YAML || return 1
apiVersion: v1
kind: Pod
metadata:
  name: $pod
  namespace: $ns
  labels:
    aqs.io/test: u8-t05
spec:
  restartPolicy: Never
  automountServiceAccountToken: false
  securityContext:
    runAsNonRoot: true
    runAsUser: 65532
    seccompProfile:
      type: RuntimeDefault
  containers:
    - name: mc
      image: $img
      command: ["sleep", "900"]
      env:
        - name: MC_CONFIG_DIR
          value: /tmp/mc
        - name: SSL_CERT_FILE
          value: /etc/minio-ca/ca.crt
        - name: MINIO_ROOT_USER
          valueFrom: {secretKeyRef: {name: minio-root, key: root-user}}
        - name: MINIO_ROOT_PASSWORD
          valueFrom: {secretKeyRef: {name: minio-root, key: root-password}}
        - name: EVIDENCE_ACCESS_KEY
          valueFrom: {secretKeyRef: {name: aqs-evidence-s3, key: access-key}}
        - name: EVIDENCE_SECRET_KEY
          valueFrom: {secretKeyRef: {name: aqs-evidence-s3, key: secret-key}}
      securityContext:
        allowPrivilegeEscalation: false
        capabilities:
          drop: ["ALL"]
      volumeMounts:
        - {name: tmp, mountPath: /tmp}
        - {name: ca, mountPath: /etc/minio-ca, readOnly: true}
  volumes:
    - name: tmp
      emptyDir: {}
    - name: ca
      secret:
        secretName: minio-tls
        items:
          - {key: ca.crt, path: ca.crt}
YAML
  kubectl wait -n "$ns" --for=condition=Ready "pod/$pod" --timeout=180s >/dev/null
}

# mc_as <aqs|root> <pod> <args de mc...>: ejecuta mc en el pod con el usuario indicado. Alias «m».
# aqs = usuario de evidencia (aqs-evidence-s3); root = minio-root. El alias se monta dentro del pod.
mc_as() {
  local who=$1 pod=$2 ns=${MC_NS:-aqs-system} envs
  shift 2
  if [ "$who" = root ]; then
    envs='export MC_HOST_m="https://$MINIO_ROOT_USER:$MINIO_ROOT_PASSWORD@minio.aqs-system.svc:9000"'
  else
    envs='export MC_HOST_m="https://$EVIDENCE_ACCESS_KEY:$EVIDENCE_SECRET_KEY@minio.aqs-system.svc:9000"'
  fi
  kubectl exec -i -n "$ns" "$pod" -- sh -c "$envs; exec mc \"\$@\"" mc "$@"
}

# pod_down <ns> <pod...>: borra pods de prueba y ESPERA a que desaparezcan (sin error si no existen).
# Esperar evita que una corrida siguiente aplique sobre un pod aún en terminación (carrera vista en U8-T05).
pod_down() {
  local ns=$1
  shift
  kubectl delete pod "$@" -n "$ns" --ignore-not-found --wait=true --timeout=120s >/dev/null 2>&1 || true
}

# --- U9-T05 (C-127): insumos persistentes fuera de tmpfs -----------------------------------------
# Insumos del humano y credenciales demo: AQS_KIND_STATE_DIR (fuera del repo, sobrevive al reinicio).
# Efímeros (pids, trabajo temporal, venvs) y los locks se quedan en AQS_KIND_RUNTIME_DIR / XDG_RUNTIME_DIR.
AQS_KIND_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/tmp}/aqs-kind"
AQS_KIND_STATE_DIR="${AQS_KIND_STATE_DIR:-${XDG_STATE_HOME:-$HOME/.local/state}/aqs-kind}"
AQS_KIND_INSUMOS="litellm.env deepseek.key demo.txt admin.txt admin-mfa-secret.txt"

# Crea el directorio de insumos con modo 700 (idempotente).
kind_state_init() {
  mkdir -p "$AQS_KIND_STATE_DIR" && chmod 700 "$AQS_KIND_STATE_DIR"
}

# Migración única: si un insumo falta en el directorio nuevo y existe en el antiguo (tmpfs), lo mueve y avisa.
kind_state_migrate() {
  local f
  for f in $AQS_KIND_INSUMOS; do
    if [ ! -e "$AQS_KIND_STATE_DIR/$f" ] && [ -f "$AQS_KIND_RUNTIME_DIR/$f" ]; then
      kind_state_init || return 1
      mv "$AQS_KIND_RUNTIME_DIR/$f" "$AQS_KIND_STATE_DIR/$f" && chmod 600 "$AQS_KIND_STATE_DIR/$f" || return 1
      echo "aqs-kind: $f migrado de $AQS_KIND_RUNTIME_DIR a $AQS_KIND_STATE_DIR (una sola vez)" >&2
    fi
  done
  return 0
}

# Comprueba permisos: directorio 700 y cada fichero 600. Nombra el archivo infractor, nunca su contenido.
kind_state_check() {
  local f mode rc=0
  [ -d "$AQS_KIND_STATE_DIR" ] || return 0
  mode=$(stat -c %a "$AQS_KIND_STATE_DIR")
  if [ "$mode" != 700 ]; then
    echo "aqs-kind: el directorio $AQS_KIND_STATE_DIR tiene modo $mode (se espera 700)" >&2
    rc=1
  fi
  for f in "$AQS_KIND_STATE_DIR"/*; do
    [ -f "$f" ] || continue
    mode=$(stat -c %a "$f")
    if [ "$mode" != 600 ]; then
      echo "aqs-kind: $(basename "$f") tiene modo $mode (se espera 600)" >&2
      rc=1
    fi
  done
  return $rc
}
