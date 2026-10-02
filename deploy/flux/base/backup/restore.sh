#!/bin/sh
# Restaura un prefijo de backup (YYYY-MM-DDTHH) al bucket evidence. No borra lo existente.
# Uso: restore.sh <YYYY-MM-DDTHH>   (o RESTORE_PREFIX en el entorno)
# Mismas variables SRC_* (bucket evidence, destino de la restauracion) y DST_* (bucket de backups) que backup.sh.
set -eu

PREFIX="${1:-${RESTORE_PREFIX:-}}"
: "${PREFIX:?indique el prefijo YYYY-MM-DDTHH a restaurar}"
case "$PREFIX" in
  [0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]) ;;
  *) echo "prefijo invalido: $PREFIX (se espera YYYY-MM-DDTHH)" >&2; exit 2 ;;
esac
: "${SRC_ENDPOINT:?SRC_ENDPOINT es obligatorio}"
: "${SRC_ACCESS_KEY_ID:?SRC_ACCESS_KEY_ID es obligatorio}"
: "${SRC_SECRET_ACCESS_KEY:?SRC_SECRET_ACCESS_KEY es obligatorio}"
: "${DST_ENDPOINT:?DST_ENDPOINT es obligatorio}"
: "${DST_BUCKET:?DST_BUCKET es obligatorio}"
: "${DST_ACCESS_KEY_ID:?DST_ACCESS_KEY_ID es obligatorio}"
: "${DST_SECRET_ACCESS_KEY:?DST_SECRET_ACCESS_KEY es obligatorio}"
SRC_BUCKET="${SRC_BUCKET:-evidence}"
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

STAGING="$(mktemp -d "${STAGING_DIR:-/tmp}/evidence-restore.XXXXXX")"
trap 'rm -rf "$STAGING"' EXIT

LISTING="$(dst s3 ls "s3://$DST_BUCKET/$PREFIX/" || true)"
if [ -z "$LISTING" ]; then
  echo "el prefijo $PREFIX no existe o esta vacio en $DST_BUCKET" >&2
  exit 1
fi
dst s3 sync "s3://$DST_BUCKET/$PREFIX/" "$STAGING" --only-show-errors
src s3 sync "$STAGING" "s3://$SRC_BUCKET/" --only-show-errors
echo "restore $PREFIX listo"
