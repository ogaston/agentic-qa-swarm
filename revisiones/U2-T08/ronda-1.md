# Revisión U2-T08 — ronda 1

**Veredicto:** VERDE

La revisión independiente contra `tareas/U2-T08-integracion-u3-dev.md` no
encontró hallazgos materiales.

## Evidencia

- CA-1: las diez pruebas `FlowSourceU3` pasan con `-race`; cubren plan válido,
  inválido, vacío, paso fuera de `/`, timeout, URL no HTTP(S), respuesta no 200,
  redirecciones y circuito.
- CA-2: las tres pruebas `Contract(U3)` pasan sin `FAIL` ni `SKIP` contra un
  stub `httptest` de loopback: plan válido, respuesta inválida y servicio
  detenido. El planner real solo se usa si se define `U3_URL`; CI no lo define.
- CA-3: `scripts/test/u2-demo-local.sh` emite las cinco comprobaciones `OK`
  requeridas y ninguna `FALLA`.
- CA-4: ocho marcadores de aprobación humana, cero órdenes de clúster sin
  marcador, cero credenciales detectadas y todos los temas requeridos en el
  guion.
- CA-5: las acciones están fijadas por SHA, coinciden con `ci.yml`, no se usan
  secretos y el workflow invoca las pruebas de contrato. La revisión no ejecutó
  `actionlint` para evitar arrancar un contenedor.
- CA-6: el grep devuelve un único resultado preexistente en
  `runner/runner_test.go`, una prueba negativa que nombra `OPENAI_API_KEY`.
  `origin/main` produce el mismo resultado y no hay otra coincidencia; no es
  una credencial ni un cambio atribuible a esta tarea. Las políticas no emiten
  `FALLA`.
- CA-7: `go vet`, `go vet -tags contract` y `gofmt` están limpios; el árbol de
  trabajo está limpio y no hay cambios fuera del alcance permitido.

## Comprobación de diseño

El adaptador valida planes antes de caché/uso, rechaza URL no seguras y
redirecciones, limita y decodifica estrictamente la respuesta, y devuelve los
errores sin rutas de éxito alternativas. La configuración impide `u3` fuera de
fases reales y conserva las protecciones de `fake`.

## Pendiente humano

CA-8 sigue pendiente. Ningún comando contra clúster o nube se ejecutó durante
el loop; el humano debe ejecutar `docs/demo-dev-u2.md` en dev y registrar los
resultados reales en `bitacoras/U2-T08.md`.
