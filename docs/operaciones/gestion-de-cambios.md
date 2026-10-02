# Gestion de cambios

Todo cambio a la plataforma entra por pull request al repositorio GitOps (Flux); no se aplica nada a mano en el cluster.

## Registro de cambio

Cada PR es el registro. Debe incluir: descripcion, motivo, unidad/tarea, impacto (entornos y componentes), evidencia de pruebas (comandos y salidas) y ventana propuesta.

## Aprobacion

- Un revisor distinto del autor verifica la evidencia.
- Un humano fusiona; nadie aprueba su propio cambio.
- Cambios en produccion exigen aprobacion explicita registrada en el PR.

## Nota de rollback

Todo PR declara como revertirse: normalmente `git revert` del merge y reconciliacion de Flux. Si el cambio toca datos (p. ej. `evidence`), indicar el prefijo de backup a restaurar segun el [runbook de restore](runbook-restore.md), y verificar que existe uno reciente antes de aplicar.

## Plantilla

```markdown
- Cambio:
- Motivo:
- Impacto:
- Pruebas (comando y salida):
- Aprobador:
- Rollback:
```
