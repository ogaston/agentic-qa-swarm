# Respuesta a la ronda 1 — U11-T03 (codificador)

F-01: Corregido en 9e6f608: `imagePrefixRe` acepta repositorios de 3 o más segmentos (`registro/owner/repo[/sub…]`) en ambos módulos (services/go-warm-manager/types.go y services/go-run-controller/adapters/imagen.go). `ghcr.io` y `ghcr.io/owner` siguen rechazados, y también los segmentos `.` y `..`, los comodines, las mayúsculas y los elementos vacíos. Rojo previo: `TestManifiestoPrefijosImagenWarmSeCargan` y `TestManifiestoPrefijosImagenControladorSeCargan` (leen el valor literal de deploy/flux/base/control-plane.yaml y lo pasan por `ParseImagePrefixes` y por `loadConfig`) fallaron con `prefijo "ghcr.io/ogaston/agentic-qa-swarm/target-app" mal formado`. Verde tras el arreglo: ambas pasan, y `RUN_ARTIFACT_REF` del mismo manifiesto queda aceptado mientras `…/target-app-evil:1` queda rechazado.

F-02: Corregido en 9e6f608: una sola fuente de casos, `testdata/imagen_casos.json` (40 casos), copiada byte a byte en `services/go-warm-manager/testdata/` y en `services/go-run-controller/adapters/testdata/`. Cada módulo verifica su copia contra el SHA-256 fijado en código (`d71a94d52ed8206cbd7959c152258ff3a537d681ae42fbe4aae2c6050f768888`). Se añadieron los casos pedidos: registro en mayúsculas, `ghcr.io:443`, `ghcr.io.`, comilla, LF, CRLF, barra invertida, tag y digest a la vez, digest de 63/65 hex y en mayúsculas, tag de 128/129, ref vacía, kind distinto de published-image. Además, casos con el registro o el carácter malo también en el prefijo, porque sin ellos el prefijo exacto enmascara M3, M5 y M6. Mutaciones (cada una restaurada con `cmp` idéntico):
- M3 (controlador deja de rechazar comilla, `\n`, `\`): `TestParidadImagenCasosIdenticos` FAIL en 3 casos (`prefijo con comilla`, `prefijo con salto de linea`, `prefijo con barra invertida`).
- M5 (warm compara el registro con EqualFold): FAIL, `registro en mayusculas aunque el prefijo coincida … desplegable=true, esperado false`.
- M6 (controlador sin validar el registro): FAIL, `registro docker.io aunque el prefijo coincida … desplegable=true, esperado false`.
- M7 (solo cambia la copia del controlador): FAIL, `testdata/imagen_casos.json cambió (sha256 bba36f…)`.

F-03: Fuera de alcance: tarea candidata propuesta, gramática estricta de la ref completa en `ValidateArtifact` (`…:1:2`, `@x@sha256:…`, `@sha512:` como tag). Preexistente; registrada en la bitácora, sección «Candidatas».

F-04: Corregido en 9e6f608, con pruebas:
- (a) Elemento vacío (`a,,b`, coma final) es error de configuración; ParseImagePrefixes ya no lo omite. Al arrancar hay un INFO con el conteo (`logImagePrefixes`) y un WARN si la lista está vacía, sin imprimir valores. Pruebas: `TestLogImagePrefixesVacioWarnYConteo` (warm y controlador) y `TestParseImagePrefixesVacioEsFailClosed`.
- (b) `fallbackSeen` pasa de `sync.Map` a `seenBounded` con tope `maxFallbackSeen = 10000` (FIFO). Prueba: `TestFallbackSeenAcotado` (25 inserciones con tope 10: el conjunto no supera 10 y la más antigua puede volver a avisarse).
- (c) `TestDeployImageAllowedLlegaAlWarm` renombrado a `TestArtifactDeployImageAllowedLlegaAlWarm`; entra en el `-run 'Artifact|Fallback|ImageNotAllowed'` de CA-2 (verificado en la salida de CA-2 de esta ronda).
- (d) Bitácora corregida por adición: la sección «Ronda 2» contiene la salida real de CA-4 y la nota de que `origin/tarea/U11-T01` sí existe (07c8bd8). La ronda 1 se deja intacta, porque la bitácora solo se añade y no se reescribe.

Candidatas del revisor: gramática estricta (F-03, anotada). Revisión de `scripts/test/u8-target-app-kind.sh` para que su registro y repositorio entren en la regla: candidata para U11-T02. Documentar `WARM_ALLOWED_IMAGE_PREFIXES` en los README: queda como candidata, porque la documentación es de U11-T02.
