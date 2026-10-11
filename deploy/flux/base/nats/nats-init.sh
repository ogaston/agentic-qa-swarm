#!/bin/sh
# Crea el stream AQS_EVENTS si no existe. Idempotente: si ya existe, no lo toca.
# Retención por límites, max_age 7 días, deduplicación de 2 min por Nats-Msg-Id (120 s).
# Entorno: NATS_URL (obligatorio).
set -eu

: "${NATS_URL:?NATS_URL es obligatorio}"
STREAM=AQS_EVENTS

if nats --server "$NATS_URL" --timeout 10s stream info "$STREAM" >/dev/null 2>&1; then
  echo "stream $STREAM ya existe"
else
  nats --server "$NATS_URL" --timeout 10s stream add "$STREAM" \
    --subjects 'aqs.events.>' \
    --storage file \
    --retention limits \
    --max-age 168h \
    --dupe-window 2m \
    --replicas 1 \
    --discard old \
    --defaults
  echo "stream $STREAM creado"
fi
