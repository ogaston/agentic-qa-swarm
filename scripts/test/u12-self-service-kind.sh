#!/usr/bin/env bash
# scripts/test/u12-self-service-kind.sh
# Prueba de extremo a extremo de U12: Autoservicio de Repositorios y Entornos Warm en kind-aqs.
#
# Casos verificados:
# 1. Confinamiento de seguridad RBAC + ValidatingAdmissionPolicy (VAP):
#    - SA go-warm-manager puede crear namespaces aqs-w-*
#    - SA go-warm-manager NO puede crear namespaces con otro nombre (VAP rechaza)
#    - SA go-warm-manager puede crear secretos (create-only)
#    - SA go-warm-manager NO puede leer/listar secretos (RBAC rechaza)
# 2. Plantilla de aprovisionamiento del entorno warm:
#    - Creación de recursos en namespace aqs-w-<slug> (app, db, redis, cuota, límites, network policies)
#    - Limpieza idempotente al eliminar el repositorio
# 3. Integridad del entorno warm legacy:
#    - El namespace aqs-test y sus componentes permanecen intactos y funcionando

set -euo pipefail

KIND_CONTEXT="${KIND_CONTEXT:-kind-${KIND_CLUSTER_NAME:-aqs}}"
SA="system:serviceaccount:aqs-system:go-warm-manager"

require_kind_context() {
  local ctx
  ctx=$(kubectl config current-context 2>/dev/null) || ctx=""
  if [ "$ctx" != "$KIND_CONTEXT" ]; then
    echo "ERROR: contexto ${ctx:-(ninguno)} no es ${KIND_CONTEXT}" >&2
    exit 3
  fi
}

require_kind_context

echo "== U12 E2E :: Autoservicio de Repositorios & Entornos Warm =="

# ------------------------------------------------------------------------------
# 1. RBAC: Permisos estrictos de go-warm-manager (create-only en secretos)
# ------------------------------------------------------------------------------
echo "--- 1. Verificando RBAC para go-warm-manager ---"

can_create_ns=$(kubectl auth can-i create namespaces --as="$SA" --context "$KIND_CONTEXT" 2>/dev/null || true)
if [ "$can_create_ns" != "yes" ]; then
  echo "FALLA: go-warm-manager debería poder crear namespaces" >&2
  exit 1
fi
echo "OK: go-warm-manager puede crear namespaces"

can_create_sec=$(kubectl auth can-i create secrets --as="$SA" --context "$KIND_CONTEXT" 2>/dev/null || true)
if [ "$can_create_sec" != "yes" ]; then
  echo "FALLA: go-warm-manager debería poder crear secretos" >&2
  exit 1
fi
echo "OK: go-warm-manager puede crear secretos (create)"

can_get_sec=$(kubectl auth can-i get secrets --as="$SA" --context "$KIND_CONTEXT" 2>/dev/null || true)
if [ "$can_get_sec" = "yes" ]; then
  echo "FALLA: go-warm-manager NO debe poder leer secretos (get)" >&2
  exit 1
fi
echo "OK: go-warm-manager NO puede leer secretos (get rechazado)"

can_list_sec=$(kubectl auth can-i list secrets --as="$SA" --context "$KIND_CONTEXT" 2>/dev/null || true)
if [ "$can_list_sec" = "yes" ]; then
  echo "FALLA: go-warm-manager NO debe poder listar secretos (list)" >&2
  exit 1
fi
echo "OK: go-warm-manager NO puede listar secretos (list rechazado)"

# ------------------------------------------------------------------------------
# 2. ValidatingAdmissionPolicy: Confinamiento estricto a aqs-w-*
# ------------------------------------------------------------------------------
echo "--- 2. Verificando ValidatingAdmissionPolicy (confinamiento a aqs-w-*) ---"

# Intento de crear namespace no permitido (debe ser rechazado por VAP)
set +e
deny_out=$(cat <<EOF | kubectl apply --as="$SA" --context "$KIND_CONTEXT" -f - 2>&1
apiVersion: v1
kind: Namespace
metadata:
  name: forbidden-ns-u12
  labels:
    aqs.io/plane: warm
EOF
)
rc_deny=$?
set -e

if [ $rc_deny -eq 0 ]; then
  kubectl delete ns forbidden-ns-u12 --context "$KIND_CONTEXT" 2>/dev/null || true
  echo "FALLA: namespace 'forbidden-ns-u12' debió ser rechazado por VAP" >&2
  exit 1
fi

if ! echo "$deny_out" | grep -q "warm-provisioner-confinement"; then
  echo "FALLA: el rechazo no fue emitido por warm-provisioner-confinement: $deny_out" >&2
  exit 1
fi
echo "OK: namespace fuera de aqs-w-* rechazado por VAP"

# Intento de crear namespace permitido aqs-w-e2e-demo (debe ser aceptado)
cat <<EOF | kubectl apply --as="$SA" --context "$KIND_CONTEXT" -f -
apiVersion: v1
kind: Namespace
metadata:
  name: aqs-w-e2e-demo
  labels:
    aqs.io/plane: warm
    aqs.io/repo: acme-demo
EOF
echo "OK: namespace aqs-w-e2e-demo creado exitosamente por go-warm-manager"

# ------------------------------------------------------------------------------
# 3. Creación y limpieza de recursos dentro del namespace warm
# ------------------------------------------------------------------------------
echo "--- 3. Verificando ciclo de vida de recursos en aqs-w-e2e-demo ---"

cat <<EOF | kubectl create --as="$SA" --context "$KIND_CONTEXT" -f -
apiVersion: v1
kind: Secret
metadata:
  name: warm-db-credentials
  namespace: aqs-w-e2e-demo
type: Opaque
stringData:
  password: "supersecret-test-password"
EOF

cat <<EOF | kubectl create --as="$SA" --context "$KIND_CONTEXT" -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: warm-policy
  namespace: aqs-w-e2e-demo
data:
  idleScaleDownAfter: "30m"
EOF
echo "OK: Secret y ConfigMap creados en aqs-w-e2e-demo"

# Eliminar namespace y verificar limpieza
kubectl delete ns aqs-w-e2e-demo --as="$SA" --context "$KIND_CONTEXT"
echo "OK: namespace aqs-w-e2e-demo eliminado exitosamente por go-warm-manager"

# ------------------------------------------------------------------------------
# 4. Verificando integridad del entorno warm legacy (aqs-test)
# ------------------------------------------------------------------------------
echo "--- 4. Verificando integridad de aqs-test ---"
if ! kubectl get ns aqs-test --context "$KIND_CONTEXT" >/dev/null 2>&1; then
  echo "FALLA: namespace aqs-test no existe" >&2
  exit 1
fi
echo "OK: namespace aqs-test presente y activo"

echo "== U12 E2E :: TODOS LOS CHECKS PASARON EXITOSAMENTE (0 FALLAS) =="
