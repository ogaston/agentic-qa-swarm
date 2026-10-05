# U1-T05 — PBT parcial: round-trip de payloads/eventos, generadores de dominio y seed (`rapid`)

**Unidad:** U1 — Ingesta & Inbox
**Historias que implementa:** US-M1 (calidad de la ingesta; extensión Property-Based Testing parcial: PBT-02, PBT-07, PBT-08, PBT-09)
**Depende de:** U1-T02, U1-T03 y U1-T04 (hay código real que probar: parseo de GitHub, `notify.created`, almacenes JSONL de ambos servicios).

---

## Alcance

**Dentro** (una línea, concreta):

> Añadir pruebas basadas en propiedades con `pgregory.net/rapid` a `services/go-intake` y `services/ui-api`: generadores de dominio reutilizables (PBT-07), propiedades de **round-trip** de parseo y serialización (PBT-02), registro del seed en cada ejecución y reproducción por seed (PBT-08), y un `PBT.md` por servicio que documenta el framework, cómo reproducir y cómo ejecutar (PBT-09).

Propiedades mínimas (nombres que empiezan por `TestPBT`, para que `-run PBT` las seleccione):

1. **Payloads de GitHub (go-intake).** Para todo payload generado de `push`, `pull_request`, `release`: `Parse(Marshal(Parse(payload)))` es igual a `Parse(payload)` (round-trip del modelo normalizado, PBT-02).
2. **`notify.created` (go-intake y ui-api).** Para toda notificación generada: `UnmarshalEvent(MarshalEvent(e)) == e`, y el JSON producido valida siempre contra `contracts/events/notify.created.schema.json`.
3. **Almacenes JSONL.** Para toda secuencia generada de notificaciones/recibos: escribir, cerrar, reabrir y leer devuelve la misma secuencia en el mismo orden (go-intake: notificaciones y de-duplicación; ui-api: confirmaciones).
4. **Firma.** Para todo `(secret, body)` generado: `Verify(secret, body, Sign(secret, body))` es `nil`, y alterar un byte del cuerpo o del secreto lo hace fallar.
5. **Artefacto (go-intake).** Para todo evento válido generado, `Resolve` produce un `ref` que cumple el patrón de U1-T03 y nunca termina en `:latest`.

Generadores de dominio (PBT-07), en un paquete interno `internal/gen` por servicio (no compartido entre módulos): `Owner`, `RepoName`, `Sha40`, `Tag` (válido y con variantes inválidas controladas), `GitHubPushBranch`, `GitHubPushTag`, `GitHubPullRequest` (con y sin fork), `GitHubRelease`, `Notification`, `ConfirmationReceipt`. Los generadores de cadenas arbitrarias incluyen Unicode, cadenas vacías y de longitud límite. Una prueba `TestPBT_GeneratorCoverage` demuestra que cada clase de evento y cada estado aparece al menos una vez en 500 sorteos.

**Seed (PBT-08).** El seed se registra siempre: un `TestMain` o helper imprime `rapid seed: <n>` al iniciar los `TestPBT*` (rapid solo lo imprime al fallar). Con `-rapid.seed=<n>` la ejecución se repite idéntica. El **shrinking** se deja activo (valor por defecto de `rapid`; prohibido desactivarlo).

**Framework (PBT-09).** `rapid` fijado en `go.mod`/`go.sum` en ambos módulos. `services/go-intake/PBT.md` y `services/ui-api/PBT.md` explican en menos de 40 líneas: por qué `rapid` (Go, shrinking, sin dependencias), cómo correr (`go test -run PBT ./...`), cómo reproducir un fallo (`-rapid.seed`), cuántas comprobaciones (`-rapid.checks`) y dónde están los generadores.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- PBT-03 (invariantes de gates) y cualquier otra regla PBT: en este plan solo aplican PBT-02/07/08/09 a U1. Los invariantes de gates son de U4-T06.
- Cambiar código de producción para que las propiedades pasen sin que un humano lo vea: **si una propiedad encuentra un defecto real, se registra en la bitácora con el ejemplo reducido, se arregla en la ronda y el arreglo lleva su prueba de ejemplo fija** (regresión).
- Fuzzing nativo de Go (`go test -fuzz`), `testing/quick` y cualquier otra librería de propiedades.
- Pruebas de integración HTTP, de rendimiento o de carga.
- Modificar `ci.yml` (las pruebas PBT corren dentro de `go test ./...`, que el CI ya ejecuta).
- Generadores compartidos entre `go-intake` y `ui-api` vía `replace`/`go.work` (prohibidos): se duplican con el nombre del paquete y una nota.

---

## Archivos de contexto

- `unidades-y-tareas.md` (sección U1)
- `aidlc-docs/inception/application-design/unit-task-plans/U1.md`
- `aidlc-docs/inception/requirements/requirements.md` (NF-PBT-01 a NF-PBT-05, extensión PBT parcial)
- `services/go-intake/`, `services/ui-api/` (código de U1-T01…T04)
- `contracts/events/notify.created.schema.json`

---

## Criterios de aceptación

Desde la raíz del worktree.

- [ ] **CA-1** — Las propiedades corren y pasan en ambos servicios, con shrinking habilitado y seed visible.
  ```bash
  for m in go-intake ui-api; do (cd services/$m && l=$(mktemp) && go test -run PBT -v ./... > "$l" 2>&1; grep -c -E '^\s*--- PASS: TestPBT' "$l"; grep -c -E 'rapid seed: [0-9-]+' "$l"; grep -c FAIL "$l"); done
  ```
  Esperado, para cada servicio: un número ≥ `3` en `go-intake` (≥ `3` en `ui-api`), un número ≥ `1` y `0`. Antes de la tarea: `0 0 0` y `no tests to run` (rojo inicial).

- [ ] **CA-2** — Hay al menos 5 propiedades en `go-intake` y 2 en `ui-api`, y cada una de las cinco de la lista existe.
  ```bash
  grep -r -h -o -E '^func TestPBT[A-Za-z0-9_]*' services/go-intake --include='*_test.go' | sort -u | wc -l; grep -r -h -o -E '^func TestPBT[A-Za-z0-9_]*' services/ui-api --include='*_test.go' | sort -u | wc -l
  grep -r -l -E 'rapid\.Check|rapid\.MakeCheck' services/go-intake services/ui-api --include='*_test.go' | wc -l
  ```
  Esperado: ≥ `6` (cinco propiedades más `GeneratorCoverage`), ≥ `3` y ≥ `4` archivos de prueba que usan `rapid`.

- [ ] **CA-3** — Un fallo muestra el seed y un contraejemplo reducido, y se reproduce con ese seed.
  ```bash
  cd services/go-intake && l=$(mktemp) && go test -tags pbt_demo -run 'PBT_Demo' -v ./... > "$l" 2>&1; grep -E 'rapid\.seed=|Falsifying|failed after|FAIL' "$l" | head -n 5
  s=$(grep -o -E 'rapid\.seed=[0-9-]+' "$l" | head -n1); go test -tags pbt_demo -run 'PBT_Demo' ./... "-$s" 2>&1 | grep -c -E 'Falsifying|failed after|FAIL'
  ```
  Esperado: líneas con `rapid.seed=`, `failed after` (o `Falsifying example`) y `FAIL`; y el segundo comando imprime un número ≥ `1` (el mismo fallo con el mismo seed). `TestPBT_Demo*` existe **solo** detrás de la etiqueta de compilación `pbt_demo` (una propiedad deliberadamente falsa, p. ej. «todo `Sha40` generado empieza por `a`»), de modo que `go test ./...` normal **no** la ejecuta.

- [ ] **CA-4** — El `go test ./...` normal no incluye el demo y sigue en verde (lo que corre en CI).
  ```bash
  for m in go-intake ui-api; do (cd services/$m && go test ./... 2>&1 | grep -c -E 'FAIL|Demo'); done
  ```
  Esperado: `0` y `0`.

- [ ] **CA-5** — Si una propiedad encuentra un defecto de producción, existe su regresión fija. (Si no se encontró ninguno, la bitácora lo dice y este criterio se cumple vacío.)
  ```bash
  grep -n -i -E 'defecto|hallazgo|contraejemplo' bitacoras/U1-T05.md | head -n 5; grep -r -c -E 'Regression|regresion|regresión' services/go-intake services/ui-api --include='*_test.go' | awk -F: '{s+=$2} END {print s}'
  ```
  Esperado: si la bitácora enumera un defecto, el segundo número es ≥ `1`; si la bitácora declara «sin defectos hallados», no hay requisito.

- [ ] **CA-6** — PBT-09: framework fijado y documentado.
  ```bash
  grep -c 'pgregory.net/rapid' services/go-intake/go.mod services/ui-api/go.mod; wc -l < services/go-intake/PBT.md; wc -l < services/ui-api/PBT.md
  grep -c -E 'rapid\.seed|go test -run PBT' services/go-intake/PBT.md services/ui-api/PBT.md
  ```
  Esperado: `…go.mod:1` dos veces; dos números entre `10` y `40`; y al menos `1` coincidencia en cada `PBT.md`.

- [ ] **CA-7** — Los generadores cubren las clases de dominio (el cubrimiento no es de oídas).
  ```bash
  cd services/go-intake && go test -run 'PBT_GeneratorCoverage' -v ./... | grep -E '^(--- |ok|FAIL)'; cd ../ui-api && go test -run 'PBT_GeneratorCoverage' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS: TestPBT_GeneratorCoverage` y `ok` en ambos.

- [ ] **CA-8** — Higiene y alcance.
  ```bash
  for m in go-intake ui-api; do (cd services/$m && go vet ./... && test -z "$(gofmt -l .)" && echo "$m ok"); done; git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/(go-intake|ui-api)/|bitacoras/U1-T05.md)' | wc -l
  ```
  Esperado: `go-intake ok`, `ui-api ok`, `0` y `0`.

---

## Plan de pruebas

- Las propiedades **son** las pruebas. Además, una prueba de ejemplo fija por propiedad (un caso canónico) para que un fallo de generadores no deje la propiedad sin cobertura.
- Mutación manual (se ejecuta y se registra, no se commitea): romper `Sign` (devolver la firma sin prefijo `sha256=`) y comprobar que la propiedad 4 falla con seed y contraejemplo reducido.
- Se documenta en la bitácora el número de comprobaciones por defecto de `rapid` (100) y el tiempo total de `go test -run PBT`; no debe pasar de 30 s por servicio.

**Rojo primero:** el codificador registra en su bitácora que `go test -run PBT ./...` no encuentra pruebas (`testing: warning: no tests to run`) antes de escribir la primera propiedad.

---

## Notas

- **Go (aprendido en U4).** El módulo va en `go 1.26.8` (no `go 1.24`: con 1.24 el job `vuln` de la CI de GitHub falla por avisos de la biblioteca estándar), con las dependencias más recientes compatibles con esa versión. Patrón de referencia: `services/go-identity` y `services/go-governance`. `govulncheck` no corre en el entorno del loop (`vuln.go.dev` da 403): la confirmación es el job `vuln` de `ci` en el PR de GitHub (el orquestador lo abre como borrador para que corra).
- **PBT (aprendido en U4-T06).** Mismo patrón y dependencia que `services/go-governance` y `services/go-identity` (`pgregory.net/rapid` v1.3.0, `internal/gen`, `PBT.md`, demo `pbt_demo`, mutantes `.patch` generados con `git diff` y comprobados con `git apply --check`). rapid imprime `failed after`, no `Falsifying`. Comprueba que las invariantes son verdaderas sobre el código real; si una propiedad halla un defecto de producción, se registra con el contraejemplo reducido y se fija con una prueba `Limit`, sin arreglarlo si el alcance lo prohíbe.
- **Informes del loop.** El diff de `revisiones/<tarea>/` (informes del revisor) no cuenta como desborde en los criterios de alcance.
- Archivos que se **modifican en su sitio**: `go.mod`/`go.sum` de ambos servicios (nueva dependencia) y, si hace falta, constructores exportados internos para poder generar el modelo. Nada de duplicados con sufijo.
- Las propiedades no deben depender de red, reloj real ni disco fuera de `t.TempDir()`.
- El seed se imprime con `t.Logf`; el CI lo guarda en el log de la ejecución. No se persiste en artefactos.
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
