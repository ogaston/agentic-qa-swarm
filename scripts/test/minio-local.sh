#!/usr/bin/env bash
# Prueba local de init-bucket.sh contra MinIO (imagen fijada por digest) en docker.
# Uso: bash scripts/test/minio-local.sh [ruta/a/init-bucket.sh]
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="${1:-$ROOT/deploy/flux/base/minio/init-bucket.sh}"
SCRIPT="$(cd "$(dirname "$SCRIPT")" && pwd)/$(basename "$SCRIPT")"
MINIO_IMAGE='cgr.dev/chainguard/minio@sha256:9dcc028b309030afa86fc1fc8d93907ae373ea3fb75277cca3fc77e4645932b7'
AWS_IMAGE='amazon/aws-cli:2.18.0'
NAME="aqs-minio-test-$$"
NET="aqs-minio-test-$$"
AK=testaccesskey
SK=testsecretkey123

cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker network create "$NET" >/dev/null
docker run -d --name "$NAME" --network "$NET" --security-opt label=disable \
  -e MINIO_ROOT_USER="$AK" -e MINIO_ROOT_PASSWORD="$SK" \
  -e MINIO_KMS_SECRET_KEY="aqs-test-key:$(head -c 32 /dev/zero | base64)" \
  "$MINIO_IMAGE" server /data --address :9000 >/dev/null

aws_run() { # $1 = entrypoint, resto = argumentos
  local ep="$1"; shift
  docker run --rm --network "$NET" --security-opt label=disable \
    -v "$SCRIPT":/scripts/init-bucket.sh:ro \
    -e AQS_S3_ENDPOINT="http://$NAME:9000" -e AWS_ACCESS_KEY_ID="$AK" \
    -e AWS_SECRET_ACCESS_KEY="$SK" -e AWS_DEFAULT_REGION=us-east-1 \
    --entrypoint "$ep" "$AWS_IMAGE" "$@"
}

for _ in $(seq 1 60); do
  if aws_run aws --endpoint-url "http://$NAME:9000" s3api list-buckets >/dev/null 2>&1; then break; fi
  sleep 1
done

readback() {
  aws_run aws --endpoint-url "http://$NAME:9000" s3api head-bucket --bucket evidence
  local alg
  alg="$(aws_run aws --endpoint-url "http://$NAME:9000" s3api get-bucket-encryption --bucket evidence \
    --query 'ServerSideEncryptionConfiguration.Rules[0].ApplyServerSideEncryptionByDefault.SSEAlgorithm' --output text)"
  if [ "$alg" != "AES256" ]; then
    echo "FALLA: cifrado leido '$alg', se esperaba AES256" >&2
    exit 1
  fi
  echo "evidence $alg"
}

for _ in 1 2; do
  aws_run /bin/sh /scripts/init-bucket.sh >/dev/null
  readback
done
