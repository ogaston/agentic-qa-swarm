# Ronda 1 — respuesta del codificador a revisiones/U11-T01/ronda-1.md

F-01: Corregido en e9826ad: el artefacto se persiste en el registro de confirmación (`record.Artifact`, `ConfirmTraced`); tras reinicio se lee de ahí y el replay ya no es necesario. `Publish` sin artefacto disponible devuelve `ErrArtifactUnavailable` sin publicar ni marcar, y `RepublishPending` lo reintenta. Pruebas reales en `services/ui-api/inbox/publish_test.go`: `TestRunConfirmedArtifactSobreviveReinicio` y `TestPublishSinArtefactoDisponibleNoMarcaPublicado` (rojo y verde en la bitácora).
F-02: Corregido en e9826ad: eliminado el `.fail` de `adapters/testdata/rapid/`; añadida `TestPBT_Fixed_RunConfirmedArtifactRoundTrip` (variante Fixed_); semilla explícita `-rapid.seed=20261025` registrada en la bitácora.
F-03: Corregido en e9826ad: nil-check en `Store.Apply` con `TestApplySinArtefactoNoPanica`; prueba de cadena `services/go-run-controller/cmd/go-run-controller/e2e_artifact_test.go` (payload real de `NewRunConfirmed` vía fixture generado, `ParseRunConfirmed`, `buildConfig`, `POST /deploys` con kind/ref); el orden de despliegue está en `services/go-run-controller/README.md`. El límite entre módulos impide importar ui-api desde el controlador, por eso el fixture (indicado en la bitácora).
F-04: Resuelto por el orquestador (hash 3ba38acc…); CA-6 da 0 con el hash nuevo.
F-05: Corregido en e9826ad: el tipo `Artifact` sale de entre el comentario de `Run` en `runctl/ports.go`.
Candidatas del revisor (no implementadas, fuera de alcance): registradas en la bitácora, sección Ronda 2.
