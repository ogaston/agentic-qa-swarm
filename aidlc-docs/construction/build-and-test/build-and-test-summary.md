# Resumen de Build and Test (2026-11-03)

| Verificación | Resultado |
|---|---|
| `go vet` + `go test -count=1` en los 8 servicios Go | 0 fallos |
| `contracts/validate.sh` | ok (ejemplos válidos e inválidos) |
| `agents/eval` pytest | 113 passed |
| Agentes con venv (pytest) | 336 passed, 3 deselected |
| Dashboard: lint, typecheck, build | ok |
| Dashboard: vitest | 296 passed, 9 skipped, 24 archivos |

## Verificación en clúster kind (2026-11-04)
Tras `scripts/demo/up.sh` y `up.sh --reset` (`reset_verified=true`):
- `scripts/kind/kind-smoke.sh`: OK, sin FALLA. El caso `ciclo-webhook-confirm-corrida` daba 401 porque el repo objetivo está registrado en el asistente de proyectos y go-intake verifica con el secreto derivado; el humo ahora lo deriva igual que `up.sh`.
- `scripts/ci/rbac-matrix.sh`: OK (dev y prod).
- `scripts/test/u12-self-service-kind.sh`: 0 fallas.

## Fuera de alcance / abierto
- Pruebas de carga y de seguridad: «Próximamente» en el modal.
- Egress de `aqs-system` (C-51): abierto, declarado en el README.
- Entornos warm dinámicos de prueba en CrashLoopBackOff en el clúster local (restos de ensayos de autoservicio).

**Estado:** build, pruebas automatizadas y verificación en kind en verde. Falta la aprobación del humano para cerrar la etapa.
