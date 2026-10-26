# Ronda 3 — U11-T03 (informe del revisor, persistido por el orquestador; resumen fiel del hand-back)

VEREDICTO: VERDE

HEAD b0c518a, hash de la tarea e63a2c15… sin cambios. Copia `git archive HEAD` en un scratchpad único. GOTOOLCHAIN=auto. CA-1..CA-5 pasan con ejecución propia (policies rc=0; 0 FAIL en ambos módulos; alcance 0).

## Hallazgos de la ronda 2, reproducidos
- F-05 cerrado: las dos mutaciones en el parser del controlador (error de elemento vacío → `continue`; quitar `|| hasDotSegment(p)`) hacen FALLAR ahora la suite (3 líneas FAIL cada una). `testdata/imagen_parse_casos.json` (20 casos) idéntico byte a byte en ambos módulos, SHA-256 fijado coincide; ambos parsers deciden igual por caso; M7 (cambiar una sola copia) falla.
- F-06 cerrado (comentario de `maxFallbackSeen`). F-07 cerrado en lo funcional: `WarmAllowedRegistry` es `const`, solo lo usan imagen.go y main.go:239; el INFO de arranque muestra `registry=ghcr.io`. F-08 sin código, anotado.
- Regresiones: pruebas de rondas 1-2 en verde; diferencial de 49 refs hostiles entre warm y controlador, cero divergencias; las únicas aceptadas llevan exactamente uno de los dos repos permitidos (las 4 de control + 4 de gramática laxa ya anotada como candidata).

## Hallazgos nuevos (no bloquean)
- F-09 · AMARILLO · cmd/go-run-controller/main.go:239: el punto de llamada del INFO de arranque `registry=` no está cubierto (cambiar el argumento por `""` deja la suite en verde); es solo observabilidad.
- F-10 · AMARILLO · testdata/imagen_parse_casos.json: falta el caso `x:1` suelto (cubierto de forma equivalente).

## Candidatas
- Gramática estricta de la ref completa (`:1:2`, `@x@sha256:`, `@sha512:`, `:tag@sha256:`).
- Documentar WARM_ALLOWED_IMAGE_PREFIXES y el fail-closed en los README y en app-objetivo.md §4 (U11-T02).
- Revisar scripts/test/u8-target-app-kind.sh (U11-T02).

VEREDICTO: VERDE
