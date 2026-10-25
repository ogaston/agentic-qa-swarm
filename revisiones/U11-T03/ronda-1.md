# Ronda 1 — U11-T03 (informe del revisor, persistido por el orquestador; resumen fiel del hand-back)

VEREDICTO: NO-VERDE

Trabajó en una copia del worktree (scratchpad), GOTOOLCHAIN=auto; worktree original limpio. `origin/tarea/U11-T01` existe (07c8bd8), contra lo que dice la bitácora.
CA-1, CA-2, CA-3 pasan; CA-4 pasa en sus comandos (policies rc=0 en docker) pero el valor cableado no arranca (F-01); CA-5 pasa (el `tail -1` de go-warm-manager imprime
`[no tests to run]` por un paquete sin tests: defecto de la tarea, no del codificador).

## Hallazgos
### F-01 · ROJO · deploy/flux/base/control-plane.yaml:240 y :412
`imagePrefixRe` (services/go-warm-manager/types.go y services/go-run-controller/adapters/imagen.go) exige exactamente 3 segmentos (`registro/owner/repo`). El respaldo que el manifiesto
manda incluir, `ghcr.io/ogaston/agentic-qa-swarm/target-app`, tiene 4. Con el valor literal del manifiesto: go-warm-manager devuelve
`WARM_ALLOWED_IMAGE_PREFIXES: prefijo "ghcr.io/ogaston/agentic-qa-swarm/target-app" mal formado`, y `loadConfig` del controlador el mismo error: ambos pods en CrashLoop.
El caso «prefijo con barras» de la tabla de paridad pasa porque inyecta la lista directamente y se salta ParseImagePrefixes; los tests de loadConfig usan `ghcr.io/x/app`.
Arreglo esperado: aceptar repositorios de 3 o más segmentos, rechazando siempre `ghcr.io` y `ghcr.io/owner`; prueba que lea el valor de control-plane.yaml, lo pase por
ParseImagePrefixes en ambos módulos y compruebe que `RUN_ARTIFACT_REF` del mismo manifiesto queda aceptado.

### F-02 · NARANJA · paridad_imagen_test.go (ambos módulos)
Mutaciones detectadas: M1 (warm `==`→HasPrefix), M2 (controlador EqualFold en el prefijo), M4 (controlador acepta latest).
No detectadas: M3 (controlador deja de rechazar comilla, `\n`, `\`), M5 (warm compara el registro con EqualFold), M6 (controlador no valida el registro).
Faltan casos: registro en mayúsculas o con puerto (`ghcr.io:443/…`, `ghcr.io./…`), comilla/salto de línea/barra invertida en la ref, tag y digest a la vez, digest de 63/65 hex o en mayúsculas,
tag de 129 caracteres, ref vacía, `ValidateImage` con kind distinto de published-image. La paridad se mantiene a mano con dos copias de la tabla.

### F-03 · AMARILLO (preexistente)
Diferencial de 3 millones de refs entre warm y controlador sin divergencias; ninguna evade la lista de repositorios. Pero solo se valida lo que va tras el último `:` y se aceptan refs
que no cumplen la gramática de imagen bajo un repositorio permitido (`…:1:2`, `…@x@sha256:<64hex>`, `…@sha512:<hex>` como tag). No hay inyección (json.Marshal). Candidata: gramática estricta de la ref completa.

### F-04 · AMARILLO
`a,,b` omite el vacío en silencio y una variable ausente/vacía arranca rechazando todo sin log al iniciar; `fallbackSeen` (sync.Map) crece sin límite; `TestDeployImageAllowedLlegaAlWarm` no entra en el `-run`
de CA-2; la bitácora dice que `origin/tarea/U11-T01` no existe y su CA-4 dice «ver salida abajo» sin pegarla.

## Lo que resistió
42 refs hostiles (tag+digest, mayúsculas, puerto, punto final, zero-width/cirílico, %2e%2e, doble barra, espacios/CR/LF/tab/NUL, comillas, barra invertida, `-evil`, digest 63/65/mayúsculas, tag 129,
vacía, latest): ambas reglas decidieron igual y las 38 hostiles se rechazaron. `SetImage` solo se llama desde `Service.Deploy`; `Deploy` y `StartDeploy` validan; en el controlador `Warm.Deploy` solo se llama
desde `RealPhases.Launch(deploy)`, validado antes. Arranque fail-closed (ghcr.io, ghcr.io/ogaston, `…/a:1`, bueno+`,ghcr.io` impiden arrancar; lista vacía arranca rechazando todo).
`aqs_run_artifact_fallback_total` está en el registro que expone /metrics; WARN una vez por corrida.

## Candidatas
- Gramática estricta de la ref completa en ValidateArtifact (F-03).
- scripts/test/u8-target-app-kind.sh debe revisarse en U11-T02 para que su registro/repositorio entren en la regla.

VEREDICTO: NO-VERDE
