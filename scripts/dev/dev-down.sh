#!/usr/bin/env bash
# Para el entorno local de scripts/dev/dev-up.sh y borra su estado. Idempotente.
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)/scripts/test/u6-down.sh"
