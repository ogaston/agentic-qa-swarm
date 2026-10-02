#!/bin/sh
# Copia s3://evidence a s3://<bucket-destino>/<YYYY-MM-DDTHH>/ (UTC) con --sse AES256
# y borra los prefijos de fecha con mas de RETENTION_DAYS dias (30 por defecto).
# Solo toca prefijos con formato YYYY-MM-DDTHH; cualquier otro (p. ej. manual/) se respeta.
# Origen:  SRC_ENDPOINT, SRC_ACCESS_KEY_ID, SRC_SECRET_ACCESS_KEY, SRC_CA_BUNDLE (opcional), SRC_BUCKET (evidence)
# Destino: DST_ENDPOINT, DST_BUCKET, DST_ACCESS_KEY_ID, DST_SECRET_ACCESS_KEY, DST_CA_BUNDLE (opcional)
set -eu

: "${SRC_ENDPOINT:?SRC_ENDPOINT es obligatorio}"
: "${SRC_ACCESS_KEY_ID:?SRC_ACCESS_KEY_ID es obligatorio}"
: "${SRC_SECRET_ACCESS_KEY:?SRC_SECRET_ACCESS_KEY es obligatorio}"
: "${DST_ENDPOINT:?DST_ENDPOINT es obligatorio}"
: "${DST_BUCKET:?DST_BUCKET es obligatorio}"
: "${DST_ACCESS_KEY_ID:?DST_ACCESS_KEY_ID es obligatorio}"
: "${DST_SECRET_ACCESS_KEY:?DST_SECRET_ACCESS_KEY es obligatorio}"
SRC_BUCKET="${SRC_BUCKET:-evidence}"
RETENTION_DAYS="${RETENTION_DAYS:-30}"
export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-us-east-1}"

src() {
  if [ -n "${SRC_CA_BUNDLE:-}" ]; then
    AWS_ACCESS_KEY_ID="$SRC_ACCESS_KEY_ID" AWS_SECRET_ACCESS_KEY="$SRC_SECRET_ACCESS_KEY" \
      AWS_CA_BUNDLE="$SRC_CA_BUNDLE" aws --endpoint-url "$SRC_ENDPOINT" "$@"
  else
    AWS_ACCESS_KEY_ID="$SRC_ACCESS_KEY_ID" AWS_SECRET_ACCESS_KEY="$SRC_SECRET_ACCESS_KEY" \
      aws --endpoint-url "$SRC_ENDPOINT" "$@"
  fi
}
dst() {
  if [ -n "${DST_CA_BUNDLE:-}" ]; then
    AWS_ACCESS_KEY_ID="$DST_ACCESS_KEY_ID" AWS_SECRET_ACCESS_KEY="$DST_SECRET_ACCESS_KEY" \
      AWS_CA_BUNDLE="$DST_CA_BUNDLE" aws --endpoint-url "$DST_ENDPOINT" "$@"
  else
    AWS_ACCESS_KEY_ID="$DST_ACCESS_KEY_ID" AWS_SECRET_ACCESS_KEY="$DST_SECRET_ACCESS_KEY" \
      aws --endpoint-url "$DST_ENDPOINT" "$@"
  fi
}

STAGING="$(mktemp -d "${STAGING_DIR:-/tmp}/evidence-backup.XXXXXX")"
trap 'rm -rf "$STAGING"' EXIT

PREFIX="$(date -u +%Y-%m-%dT%H)"
src s3 sync "s3://$SRC_BUCKET" "$STAGING" --only-show-errors
dst s3 sync "$STAGING" "s3://$DST_BUCKET/$PREFIX/" --sse AES256 --only-show-errors
echo "backup $PREFIX listo"

CUTOFF="$(date -u -d "$RETENTION_DAYS days ago" +%Y-%m-%dT%H)"
CUTOFF_N="$(printf %s "$CUTOFF" | tr -d "T-")"
dst s3 ls "s3://$DST_BUCKET/" | while read -r _ name; do
  p="${name%/}"
  case "$p" in
    [0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9])
      if [ "$(printf %s "$p" | tr -d "T-")" -lt "$CUTOFF_N" ]; then
        dst s3 rm "s3://$DST_BUCKET/$p/" --recursive --only-show-errors
        echo "podado $p"
      fi
      ;;
  esac
done
echo "poda completada (corte $CUTOFF)"
