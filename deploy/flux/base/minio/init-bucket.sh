#!/bin/sh
# Crea el bucket "evidence" con cifrado por defecto SSE AES256. Idempotente.
# Entorno: AQS_S3_ENDPOINT, AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_CA_BUNDLE (opcional).
set -eu

: "${AQS_S3_ENDPOINT:?AQS_S3_ENDPOINT es obligatorio}"
BUCKET="${AQS_BUCKET:-evidence}"
export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-us-east-1}"

if aws --endpoint-url "$AQS_S3_ENDPOINT" s3api head-bucket --bucket "$BUCKET" 2>/dev/null; then
  echo "bucket $BUCKET ya existe"
else
  aws --endpoint-url "$AQS_S3_ENDPOINT" s3api create-bucket --bucket "$BUCKET"
fi

aws --endpoint-url "$AQS_S3_ENDPOINT" s3api put-bucket-encryption --bucket "$BUCKET" \
  --server-side-encryption-configuration \
  '{"Rules":[{"ApplyServerSideEncryptionByDefault":{"SSEAlgorithm":"AES256"}}]}'
echo "bucket $BUCKET listo con cifrado AES256"
