# Ronda 1 — U11-T01 (informe del revisor, persistido tal cual por el orquestador)

VEREDICTO: NO-VERDE

CA-1..CA-6 pasan con ejecución propia del revisor (CA-6 parcial: conteo de alcance = 1 por `unidades-y-tareas.md`, ver F-04).
Worktree limpio antes y después. `GOTOOLCHAIN=auto` (el go del host es 1.26.9). CA-5 corrió `scripts/ci/policies.sh` en docker:
kubeconform 87/87, conftest 2001/2001 y 23/23, RC=0.

## Hallazgos

### F-01 · ROJO · services/ui-api/inbox/published.go:21-27,54-75 + store.go:61-98
Tras reiniciar ui-api, `run.confirmed` se republica SIN artefacto y se marca publicado. `artifactOf(id)` lee `s.items`, que solo se
reconstruye reproduciendo `notify.created` en una goroutine aparte (cmd/ui-api/main.go:149-153). El registro de confirmaciones
(`record`, `ConfirmedFile`) persiste recibo, flows y trace, pero no el artefacto. `pub.Run` ejecuta `RepublishPending` al arrancar;
con un recibo sin publicar e `items` vacío, `artifactOf` devuelve nil, se publica sin `artifact` y `MarkPublished` lo deja definitivo.
El controlador cae a `RUN_ARTIFACT_REF` y despliega la imagen estática en silencio.
Reproducción (copia de ui-api en el scratchpad): `Apply + ConfirmTraced; OpenStore(mismo dir); RepublishPending` →
`republicados=1 artifact=<nil> published=true` → FAIL.
CA-4(d) solo prueba la persistencia del lado del controlador. Salidas: (a) persistir `Artifact` en el registro de confirmación;
(b) que `Publish` no publique si el artefacto no está disponible (error y reintento). Añadir prueba de reinicio.

### F-02 · NARANJA · go-run-controller/adapters/nats_test.go:324-347 y adapters/testdata/rapid/.../...-20261025165026-395550.fail
PBT-02 sin semilla registrada (el plan de pruebas lo pide) ni variante `Fixed_` como `TestPBT_Fixed_ArtifactRef` de go-intake.
El commit incluye un `.fail` de rapid que es la huella de la mutación del par falla-pasa, no un caso de regresión legítimo.

### F-03 · AMARILLO · cadena de punta a punta solo probada por tramos
Cada tramo está probado y revisado, pero no hay prueba que una el payload real de `NewRunConfirmed` con el parser del controlador y
`RealPhases`. `store.Apply` de ui-api desreferencia `ev.Data.Artifact` sin nil-check (store.go:121); hoy inalcanzable
(`ParseNotifyCreated` rechaza nil, event.go:167). El parser de run.confirmed es estricto (`DisallowUnknownFields`): un controlador
viejo rechaza eventos con `artifact`; desplegar controlador antes que ui-api o ambos a la vez.

### F-04 · AMARILLO · CA-6 de la tarea es inalcanzable tal como está escrito (defecto de la especificación)
`unidades-y-tareas.md` queda fuera de las excepciones del `grep -v`, pero el commit docs a55d74b que define la tarea lo modifica.
El codificador no lo tocó.

### F-05 · AMARILLO · go-run-controller/runctl/ports.go:54-60
El tipo `Artifact` quedó entre el comentario «Run es la corrida persistida.» y `type Run`.

## Puntos pedidos, verificados
- Allowlist fail-closed y previa a resolver/persistir: sí (paso 5 de handler.go, antes de GetByDelivery/Resolve/Put). Vacía, nil y {} → 422.
- `ping` no salta la firma: se procesa tras la verificación HMAC; sin firma o firma inválida 401; no persiste ni publica; omite la
  allowlist a propósito. Nota U11-T02: el `ping` exige `Content-Type: application/json` o devuelve 415.
- `artifact` malicioso en run.confirmed: ParseRunConfirmed rechaza kind desconocido, ref vacío, extras y null; go-warm-manager
  revalida (`docker.io/evil/x:1` rechazado sin parchear). La validación solo comprueba el registro (`ghcr.io`), no el repositorio.
- Alcance: todo dentro de lo permitido salvo F-04.

## Tareas candidatas (del revisor)
- Restringir en go-warm-manager el prefijo del repositorio de imagen (p. ej. `WARM_ALLOWED_IMAGE_PREFIXES=ghcr.io/ogaston/`).
- Alarma/métrica cuando el controlador usa el respaldo `RUN_ARTIFACT_REF` donde go-intake siempre pone artefacto.
- Los PR no tienen imagen publicada (la CI del objetivo solo publica en push a `main`): el deploy esperaría los 10 min de
  `WARM_DEPLOY_TIMEOUT`. Documentarlo en U11-T02.

VEREDICTO: NO-VERDE
ROJO|services/ui-api/inbox/published.go:21-27 (artifactOf) + store.go|run.confirmed republicado tras reinicio de ui-api sale sin artifact y queda marcado publicado (se despliega la imagen estática)
NARANJA|services/go-run-controller/adapters/nats_test.go:324 + testdata/rapid/*.fail|PBT-02 sin semilla registrada y .fail de mutación commiteado
