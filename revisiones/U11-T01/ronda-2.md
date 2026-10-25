# Ronda 2 — U11-T01 (informe del revisor, persistido por el orquestador; resumen fiel del hand-back)

VEREDICTO: NO-VERDE

CA-1..CA-6 pasan con ejecución propia (hash de la tarea 3ba38acc…; GOTOOLCHAIN=auto; policies.sh en docker RC=0; alcance 0).

## Hallazgos de la ronda 1, reproducidos
- F-01 (ROJO) resuelto: prueba propia de reinicio (Apply, ConfirmTraced, reabrir con `items` vacío, RepublishPending): el artefacto sale en el
  evento y queda marcado; con registro antiguo sin artefacto no publica ni marca (3 WARN), sobrevive a reinicios y publica cuando Apply
  re-proyecta la notificación. Mutaciones (quitar `Artifact` del `record`; quitar la guarda `c.Artifact == nil`) hacen fallar las pruebas.
  Archivo mixto (línea antigua, nueva y basura): Skipped=1, 3 recibos.
- F-02 (NARANJA) resuelto: `.fail` eliminado; `TestPBT_Fixed_RunConfirmedArtifactRoundTrip` con semilla 20261025.
- F-03 (AMARILLO) resuelto en lo sustancial; el fixture no es tautológico (idéntico al generado por NewRunConfirmed). Ver F-09.
- F-04 (AMARILLO) resuelto. F-05 (AMARILLO) resuelto en ports.go (mismo defecto en otro sitio: F-08).

## Hallazgos nuevos
### F-06 · NARANJA · deploy/flux/base/control-plane.yaml:132 + scripts/kind/*
La Nota de la tarea afirma que el respaldo `RUN_ARTIFACT_REF` mantiene `kind-e2e.sh` funcionando. Es falso: kind-e2e.sh:332, kind-smoke.sh:290 y
dashboard-smoke.sh:166 firman el webhook con `ogaston/agentic-qa-swarm`; la lista permitida solo trae `ogaston/demo-target-app-qa-swarm`
(reproducido con el binario real: HTTP 422 repo_not_allowed). Aunque se añadiera, go-intake produce siempre un artefacto para un commit, y
el controlador desplegaría `ghcr.io/ogaston/agentic-qa-swarm:<sha>`, que no existe; el respaldo ya no se usa desde un push. Defecto de
especificación: arreglarlo exige tocar scripts/kind o el overlay kind, fuera de alcance. Mínimo: declarar la rotura y corregir la Nota.

### F-07 · AMARILLO · services/ui-api/inbox/publish.go
Un recibo antiguo cuya notificación nunca se re-proyecta queda pendiente para siempre (WARN cada 5 s por recibo, sin estado terminal).
Sugerencia: registrar una vez, o métrica, cuando `Replayed()` es verdadero y sigue sin artefacto.

### F-08 · AMARILLO · services/ui-api/inbox/publish.go:37-41
Comentario de `eventNamespace` huérfano encima de `ErrArtifactUnavailable` (misma clase que F-05).

### F-09 · AMARILLO · cobertura y robustez
e2e_artifact_test.go guarda la Run a mano (no ejercita evento→Run real; eso lo cubre TestRunConfirmedArtifactQuedaEnLaCorrida). El fixture es
una copia congelada. Un registro editado a mano con `"artifact":{}` se carga como no nulo y se publica con kind y ref vacíos: falta validar
kind y ref al cargar.

## Candidatas
- Parche del overlay kind y de los scripts de humo/e2e para usar el repo permitido y una imagen existente (F-06).
- Prefijo de repositorio de imagen en go-warm-manager; métrica de uso del respaldo; los PR no tienen imagen publicada (U11-T02).

VEREDICTO: NO-VERDE
