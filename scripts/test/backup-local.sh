#!/usr/bin/env bash
# Prueba local de backup.sh / restore.sh contra MinIO (digest tomado del StatefulSet) en docker.
# Uso: bash scripts/test/backup-local.sh [ruta/a/backup.sh [ruta/a/restore.sh]]
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BACKUP_SH="${1:-$ROOT/deploy/flux/base/backup/backup.sh}"
RESTORE_SH="${2:-$ROOT/deploy/flux/base/backup/restore.sh}"
BACKUP_SH="$(cd "$(dirname "$BACKUP_SH")" && pwd)/$(basename "$BACKUP_SH")"
RESTORE_SH="$(cd "$(dirname "$RESTORE_SH")" && pwd)/$(basename "$RESTORE_SH")"
MINIO_IMAGE="$(docker run --rm -i --security-opt label=disable -v "$ROOT":/w:ro -w /w mikefarah/yq:4.44.3 -N \
  'select(.kind == "StatefulSet") | .spec.template.spec.containers[0].image' deploy/flux/base/minio/statefulset.yaml)"
AWS_IMAGE='amazon/aws-cli:2.18.0'
NAME="aqs-backup-test-$$"
NET="aqs-backup-test-$$"
AK=testaccesskey
SK=testsecretkey123
EP="http://$NAME:9000"

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

# aws_run <entrypoint> args... ; stdin del harness se reenvia al contenedor.
aws_run() {
  local ep="$1"; shift
  docker run --rm -i --network "$NET" --security-opt label=disable \
    -v "$BACKUP_SH":/scripts/backup.sh:ro -v "$RESTORE_SH":/scripts/restore.sh:ro \
    -e SRC_ENDPOINT="$EP" -e SRC_ACCESS_KEY_ID="$AK" -e SRC_SECRET_ACCESS_KEY="$SK" \
    -e DST_ENDPOINT="$EP" -e DST_BUCKET=backup -e DST_ACCESS_KEY_ID="$AK" -e DST_SECRET_ACCESS_KEY="$SK" \
    -e AWS_ACCESS_KEY_ID="$AK" -e AWS_SECRET_ACCESS_KEY="$SK" -e AWS_DEFAULT_REGION=us-east-1 \
    --entrypoint "$ep" "$AWS_IMAGE" "$@"
}
s3() { aws_run aws --endpoint-url "$EP" "$@" </dev/null; }

for _ in $(seq 1 60); do
  if s3 s3api list-buckets >/dev/null 2>&1; then break; fi
  sleep 1
done

fail() { echo "FALLA: $*" >&2; exit 1; }

s3 s3api create-bucket --bucket evidence >/dev/null
s3 s3api create-bucket --bucket backup >/dev/null

# 1. Siembra 3 objetos en evidence.
KEYS=(a/uno.txt a/dos.txt tres.bin)
for k in "${KEYS[@]}"; do
  printf 'contenido de %s %s\n' "$k" "$RANDOM$RANDOM" | aws_run aws --endpoint-url "$EP" s3 cp - "s3://evidence/$k" >/dev/null
done
sum_of() { aws_run aws --endpoint-url "$EP" s3 cp "s3://evidence/$1" - </dev/null | sha256sum | cut -d' ' -f1; }
declare -A ORIG
for k in "${KEYS[@]}"; do ORIG[$k]="$(sum_of "$k")"; done

# 2. Prefijo viejo (31 dias) y prefijo manual (sin formato de fecha) creados a mano en el destino.
OLD="$(date -u -d '31 days ago' +%Y-%m-%dT%H)"
echo viejo | aws_run aws --endpoint-url "$EP" s3 cp - "s3://backup/$OLD/a/viejo.txt" >/dev/null
echo manual | aws_run aws --endpoint-url "$EP" s3 cp - "s3://backup/manual/nota.txt" >/dev/null

# 3. Ejecuta backup.sh.
aws_run /bin/sh /scripts/backup.sh </dev/null >/dev/null

# 4. Cifrado leido de vuelta con head-object.
NEW="$(s3 s3 ls s3://backup/ | awk '{print $2}' | tr -d / | grep -E '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}$' | grep -v "^$OLD$" | sort | tail -1)"
[ -n "$NEW" ] || fail "no hay prefijo nuevo en el destino"
COUNT="$(s3 s3 ls "s3://backup/$NEW/" --recursive | wc -l)"
echo "backup objetos=$COUNT"
[ "$COUNT" = 3 ] || fail "backup objetos=$COUNT, se esperaban 3"
SSE="$(s3 s3api head-object --bucket backup --key "$NEW/tres.bin" --query ServerSideEncryption --output text)"
echo "sse=$SSE"
[ "$SSE" = AES256 ] || fail "sse=$SSE, se esperaba AES256"

# 5. Poda: viejo desaparece, nuevo y manual siguen.
count() { { s3 s3 ls "s3://backup/$1/" --recursive || true; } | wc -l; }
N_OLD="$(count "$OLD")"
N_NEW="$(count "$NEW")"
N_MAN="$(count manual)"
if [ "$N_NEW" -gt 0 ]; then N_NEW=1; fi
echo "poda viejo=$N_OLD nuevo=$N_NEW"
if [ "$N_OLD" != 0 ] || [ "$N_NEW" != 1 ]; then fail "poda inesperada"; fi
[ "$N_MAN" = 1 ] || fail "la poda toco el prefijo manual/"

# 6. Vacia evidence y restaura.
s3 s3 rm s3://evidence/ --recursive >/dev/null
aws_run /bin/sh /scripts/restore.sh "$NEW" </dev/null >/dev/null

# 7. Checksums.
EQ=0
for k in "${KEYS[@]}"; do
  [ "$(sum_of "$k" 2>/dev/null || true)" = "${ORIG[$k]}" ] && EQ=$((EQ + 1))
done
echo "restore iguales=$EQ"
[ "$EQ" = 3 ] || fail "restore iguales=$EQ, se esperaban 3"
