# Ronda 1 — U2-T01

VEREDICTO: VERDE

Corrí yo mismo CA-1..CA-6 sobre el worktree (sha f9a1c45, base 2c30f64 = origin/main). Todos pasan. No encontré ningún ROJO ni NARANJA, solo AMARILLOS. El worktree quedó limpio después de la revisión (`git status --short` vacío, también tras `go mod tidy`, que no cambió nada en ninguno de los tres módulos).

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Pruebas de los 3 módulos | `go test -count=1 ./...` en cada módulo | pasa: `plan` y `stubs` ok en go-run-controller; go-warm-manager ok; go-reset ok |
| 2 | ajv real: válidos/inválidos | los dos bucles de CA-2 con ajv-cli 5.0.0 + ajv-formats 3.0.1 | pasa: 8 `valid rc=0`, 18 `invalid rejected=0` (mínimos 8/12) |
| 3 | Stubs fallan cerrado y validan | `go test -race -count=1 -run 'Stub\|Fake' -v ./stubs/` | pasa: 3×`DefaultFails`, `TestStubsReturnValidPlans`, `ProgrammedError`, `Concurrent`, todo PASS con `-race` |
| 4 | Esquemas cerrados, 2020-12 | los `jq` de CA-4 | pasa: 4×`true`, `1`, `ready,dirty,cuarentena,idle-escalado` |
| 5 | Higiene y alcance | bloque CA-5 | pasa: 3×`ok`, `sin go.work`, `0`, `0`, `0` |
| 6 | Árbol limpio, sin desborde | bloque CA-6 | pasa: `0` y `0` |

Comprobaciones adicionales:
- `git diff --name-status` contra el merge-base solo muestra archivos `A` (añadidos). Ningún archivo existente de `contracts/` ni de ningún otro sitio fue modificado ni borrado.
- Los 18 inválidos fallan por la razón que anuncia su nombre. Lo comprobé leyendo el `keyword` de ajv en cada uno: `additionalProperties`, `required`, `enum`, `minItems`, `pattern`, `maximum`, `minLength`. Ninguno falla por una causa accidental.
- No hay `Dockerfile` ni `main.go` en los tres módulos. `list-services.sh` no los lista.
- No hay `k8s.io` ni `client-go` en los deps. No hay `replace` ni `go.work`.
- La prueba negativa (valid con `warm_id` vacío) está registrada en la bitácora con salida literal (`minLength: got 0, want 1`, FAIL), y no se commiteó, como pedía el plan.
- Rojo inicial presente en la bitácora (`ls: cannot access 'services/go-run-controller'`).
- Los esquemas siguen el estilo de `contracts/events`: `$schema` 2020-12, `$id` bajo `https://agentic-qa-swarm.example/contracts/plans/v1/...`, `additionalProperties:false`, `required`, enums como los de `warm.ready`.
- Los campos coinciden con la tarea. `component-methods.md` solo nombra estos tipos y no define sus campos, así que la fuente de campos es la propia tarea.
- Los stubs fallan cerrado: usan `slot[T]` con `sync.Mutex` y devuelven `ErrNotProgrammed` hasta que se programan.

## Hallazgos
### F-01 · AMARILLO · contracts/plans/*.schema.json · Restricciones añadidas sin reportar en la bitácora
La tarea no listaba estas restricciones y la bitácora no las menciona:
- `minLength:1` en `run_id`, `workflow`, `flow_id`, `name`, `invariant`, `warm_id`, `baseline_version`.
- `steps.minItems:1`.
- `pattern ^(s3|https)://` en `uris`.
- `pattern ^https?://` en `base_url`, junto con `format:uri`.

Ninguna contradice la tarea: son coherentes con "no inventes campos", y `warm_id` vacío lo exige el plan de pruebas. Pero U3 las heredará como contrato y la nota de la tarea pide reportar lo no listado. Conviene que el orquestador las registre, y que decida si `endpoints` (sin `minItems`, por tanto vacío es válido) es intencional.

### F-02 · AMARILLO · services/go-warm-manager/doc_test.go, services/go-reset/doc_test.go · Prueba de relleno para sostener go.sum
La tarea dice "solo `doc.go`", pero se añadió un `doc_test.go` con `rapid`. Es la única forma de que `go.sum` no esté vacío (exigido por `test -s go.sum` en CA-5), y sigue el patrón U1-T01, así que está justificado.
- La prueba es vacua: `IntRange(0,5)` nunca es `> 5`.
- El comentario lo admite ("fija rapid como dependencia").
- Los módulos pasan `go mod tidy` limpio.
- Aceptable ahora. T03 y T06 deberían reemplazarla por pruebas reales.

### F-03 · AMARILLO · services/go-run-controller/stubs/stubs.go · Alcance de "programar" y entrada vacía
- `Program()` no recibe valor. `slot[T]` guarda un `val` que siempre es el cero y nunca se lee, así que es un genérico con campo muerto.
- `Flows("")`, `Surface("")` y `Evidence("")` devuelven objetos que no validan contra el esquema (`run_id` con `minLength:1`) y que el stub no rechaza.
- No hay prueba de que `ProgramError` tras `Program` (o al revés) sobrescriba el estado.
- No bloquea nada. Es una mejora para T02, T04 y T05.

### F-04 · AMARILLO · services/go-run-controller/plan/plan_test.go:57,76 · Comentarios de documentación desalineados
- `// examplesDir permite...` precede a `checkExamples`.
- `// Validate valida v contra el esquema <name>...` precede a `TestExamplesDecodeIntoTypes`, y esa prueba no valida nada contra esquema.
- Son comentarios residuales que describen otra cosa.

### F-05 · AMARILLO · services/go-run-controller/*/..._test.go · Compilación de esquema duplicada
`compile` en `plan_test.go` y `validate` en `stubs_test.go` repiten el mismo código de compilación del esquema. La tarea acepta explícitamente la duplicación entre servicios, y entre paquetes del mismo módulo es menor.

## Tareas candidatas (defectos reales fuera de alcance)
- Incluir `contracts/plans/` en `contracts/validate.sh` (la tarea ya lo declara candidata).
- Prueba de contrato que decodifique cada ejemplo válido en los tipos Go de `plan` para los 4 objetos (hoy solo se decodifica `flow-plan.tres-flujos.json`), lo que detectaría deriva entre las etiquetas JSON y los esquemas.

## Rutas de transcripciones largas
- Ninguna. Las salidas cabían en línea.

VEREDICTO: VERDE
AMARILLO|contracts/plans/*.schema.json|Restricciones añadidas (minLength, steps minItems, patterns) sin reportar en la bitácora
AMARILLO|services/go-warm-manager|go-reset/doc_test.go|Prueba de relleno con rapid para sostener go.sum
AMARILLO|services/go-run-controller/stubs/stubs.go|Program() sin valor, val muerto, run_id vacío no validado
AMARILLO|services/go-run-controller/plan/plan_test.go:57,76|Comentarios de doc desalineados
AMARILLO|services/go-run-controller/*_test.go|Compilación de esquema duplicada
INFORME: revisiones/U2-T01/ronda-1.md
