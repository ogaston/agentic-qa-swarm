# Resumen de Build and Test (2026-11-03)

| Verificación | Resultado |
|---|---|
| `go vet` + `go test -count=1` en los 8 servicios Go | 0 fallos |
| `contracts/validate.sh` | ok (ejemplos válidos e inválidos) |
| `agents/eval` pytest | 113 passed |
| Agentes con venv (pytest) | 336 passed, 3 deselected |
| Dashboard: lint, typecheck, build | ok |
| Dashboard: vitest | 296 passed, 9 skipped, 24 archivos |

## No ejecutado en esta pasada
- Humo en kind (`kind-smoke.sh`), matriz RBAC y e2e de autoservicio: requieren el clúster local; su última ejecución en verde está en `docs/entrega/evidencia-2026-11-02.md`.
- Pruebas de carga y de seguridad: fuera del alcance de la entrega (en el modal figuran como «Próximamente»).
- Egress de `aqs-system` (C-51): abierto, declarado en el README.

**Estado:** build y pruebas automatizadas en verde. Falta la aprobación del humano para cerrar la etapa.
