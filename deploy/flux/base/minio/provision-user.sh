#!/bin/sh
# Crea el usuario de evidencia de los agentes (U8-T05, C-107). Idempotente: se puede correr N veces.
# Política mínima sobre el bucket evidence: ListBucket, GetObject y PutObject. Nada de admin.
# Entorno: MC_HOST_aqs (raíz, de minio-root; mc lo lee sin ponerlo en la línea de comandos),
#          EVIDENCE_ACCESS_KEY y EVIDENCE_SECRET_KEY (del Secret aqs-evidence-s3), AQS_BUCKET (opcional, evidence),
#          SSL_CERT_FILE (CA de MinIO), MC_CONFIG_DIR (escribible).
set -eu

: "${MC_HOST_aqs:?MC_HOST_aqs es obligatorio}"
: "${EVIDENCE_ACCESS_KEY:?EVIDENCE_ACCESS_KEY es obligatorio}"
: "${EVIDENCE_SECRET_KEY:?EVIDENCE_SECRET_KEY es obligatorio}"
BUCKET="${AQS_BUCKET:-evidence}"
POLICY="aqs-evidence"
export MC_CONFIG_DIR="${MC_CONFIG_DIR:-/tmp/mc}"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cat > "$tmp/policy.json" <<JSON
{"Version":"2012-10-17","Statement":[
  {"Effect":"Allow","Action":["s3:ListBucket"],"Resource":["arn:aws:s3:::${BUCKET}"]},
  {"Effect":"Allow","Action":["s3:GetObject","s3:PutObject"],"Resource":["arn:aws:s3:::${BUCKET}/*"]}
]}
JSON

mc admin policy create aqs "$POLICY" "$tmp/policy.json" >/dev/null
# Actualiza la clave si el usuario ya existe (rotación del Secret) y lo deja con la política.
mc admin user add aqs "$EVIDENCE_ACCESS_KEY" "$EVIDENCE_SECRET_KEY" >/dev/null
# Si el attach falla (p. ej. ya adjunta en una corrida previa) no se traga el error: se exige que el usuario
# tenga la política. Cualquier otro fallo tumba el Job.
if ! mc admin policy attach aqs "$POLICY" --user "$EVIDENCE_ACCESS_KEY" >/dev/null 2>&1; then
  mc admin user info aqs "$EVIDENCE_ACCESS_KEY" 2>/dev/null | grep -q "$POLICY" \
    || { echo "no se pudo adjuntar la politica $POLICY al usuario de evidencia" >&2; exit 1; }
fi
echo "usuario de evidencia listo: politica $POLICY sobre bucket $BUCKET"
