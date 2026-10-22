# U9-T05 — respuesta a la ronda 1

F-01: Corregido en (sha de este commit): docs/operaciones/kind-local.md líneas 28, 148 y 149 usan AQS_KIND_STATE_DIR (~/.local/state/aqs-kind). Verificado con: grep -rn 'XDG_RUNTIME_DIR.*aqs-kind' docs/ scripts/ → solo locks, la sección de migración y lib.sh.
F-02: Corregido en (sha de este commit): eliminada la asignación STATE_DIR sin uso en kind-smoke.sh, dashboard-smoke.sh y github-webhook-check.sh. Comparación de shellcheck (stdin, base origin/main vs HEAD): 96 warnings en ambos, mismo recuento por código, cero warnings nuevos (diff vacío). Detalle en bitacoras/U9-T05.md, ronda 2.
F-03: Corregido en (sha de este commit): el fake de podman tenía un exit 0 global que anulaba FAKE_IMAGE_MISSING; corregido y añadidos los casos imagen-ausente-export-falla, imagen-ausente-export-ok y directorio-abierto-falla. scripts/test/u9-kind-state.sh: 9 de 9 OK, rc=0.
Amarillos: no atendidos en esta ronda (no bloquean). Tarea candidata: limpiar /tmp/go-identity.tar del nodo kind (ya listada por el revisor).
