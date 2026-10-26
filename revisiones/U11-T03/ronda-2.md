# Ronda 2 — U11-T03 (informe del revisor, persistido por el orquestador; resumen fiel del hand-back)

VEREDICTO: NO-VERDE

HEAD cfa4a0d, hash de la tarea e63a2c15…, árbol limpio. GOTOOLCHAIN=auto. Pruebas y mutaciones en una copia del scratchpad (un primer directorio `wt` fue pisado por otro proceso que comparte el scratchpad; se repitió en una copia nueva).
CA-1..CA-5 pasan con ejecución propia (policies rc=0; 0 FAIL en ambos módulos; alcance 0).

## Hallazgos de la ronda 1, por identificador
- F-01 cerrado: con el valor literal del manifiesto el binario real de go-warm-manager arranca (`prefijos de imagen permitidos cargados count=2`) y el controlador (`loadConfig`/`run()`) también.
  POST /deploys: `…/agentic-qa-swarm/target-app:0.0.0` y `…/demo-target-app-qa-swarm:abc123` pasan la validación; `target-app-evil:1`, `demo-target-app-qa-swarm-evil:1`, `…/x:1`, `target-app/extra:1`,
  `ghcr.io/evil/x:1`, `ghcr.io/ogaston/agentic-qa-swarm:1` → 422 image_not_allowed. Impiden arrancar: `ghcr.io`, `ghcr.io/ogaston`, `.`, `..`, `…/../x`, comodines, mayúsculas, barra final, `x:1`, coma final, `,a`, `a,,b`.
  kind, dev y prod rinden el mismo valor y el RUN_ARTIFACT_REF intacto; ningún overlay pisa la variable.
- F-02 cerrado con una laguna (F-05): copias idénticas, SHA-256 fijado coincide, 40 casos con todos los pedidos; mutaciones M3, M5, M6, M7 y extras fallan por la prueba de paridad.
- F-04 parcial: logs INFO/WARN presentes en ambos; `fallbackSeen` acotado (quitar la cota o el delete hace fallar la prueba); renombre dentro del -run de CA-2; faltan las pruebas del controlador para el vacío (F-05); efecto de descarte sin declarar (F-06).

## Hallazgos nuevos
### F-05 · NARANJA · services/go-run-controller/adapters/imagen_test.go y cmd/go-run-controller
El parser del controlador no tiene prueba de elemento vacío ni de segmentos `.`/`..`. Mutaciones con la suite completa en verde: sustituir el error de «elemento vacío» por `continue` en
`ParseImagePrefixes` del controlador; quitar `|| hasDotSegment(p)`. El warm, con las mismas mutaciones, falla. La tabla de paridad inyecta los prefijos y se salta `ParseImagePrefixes`.
Efecto posible: el controlador omitiría una coma final en silencio mientras el warm se niega a arrancar (la divergencia que F-02 quería cerrar).
Arreglo: replicar en `adapters/imagen_test.go` los casos `a,,b`, coma final, `,a`, `a, ,b`, `ghcr.io/o/../x`, `…/a/..`, `…/a/./b`; mejor, una tabla de paridad del parser (valor crudo → error o lista) en el mismo JSON.

### F-06 · AMARILLO · fallback_seen.go
Tras 10000 corridas de respaldo más recientes (o tras reiniciar el controlador), una corrida activa podría reemitir el WARN y subir el contador si su fase deploy se reintenta. Aceptable, pero sin declarar: una línea en el comentario de `maxFallbackSeen`.

### F-07 · AMARILLO · adapters/imagen.go:15
`warmAllowedRegistry = "ghcr.io"` replica `DefaultAllowedRegistries`; el warm lo lee de `WARM_ALLOWED_REGISTRIES`. Si se amplía, el controlador rechaza en silencio (falla cerrado). Sin aviso al operador.

### F-08 · AMARILLO · go-warm-manager/cmd/go-warm-manager/main.go
`registries()` sigue omitiendo vacíos y `ParseImagePrefixes` ya no. Coherente con fail-closed; no hay ningún valor con coma final en el repo. Documentar al escribir los README.

## Regresiones buscadas
Diferencial de 96257 refs hostiles (semilla 20261025): cero divergencias; la comparación es por igualdad exacta del repositorio, un prefijo más largo no admite nada que antes no. Ningún test que construya wm.Config se rompe.

## Candidatas
- Gramática estricta de la ref completa (F-03 de la ronda 1).
- Documentar WARM_ALLOWED_IMAGE_PREFIXES en los README y en `docs/operaciones/app-objetivo.md` §4 (hoy enseña a cambiar RUN_ARTIFACT_REF sin decir que hay que añadir el repositorio al prefijo) — U11-T02.
- Revisar `scripts/test/u8-target-app-kind.sh` para que su registro/repositorio entren en la regla (U11-T02).

VEREDICTO: NO-VERDE
