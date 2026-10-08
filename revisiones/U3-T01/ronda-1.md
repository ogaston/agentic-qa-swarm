# Ronda 1 — U3-T01

VEREDICTO: NO-VERDE

Los seis criterios de aceptación pasan con mis propias ejecuciones frescas. Queda un hallazgo NARANJA: el dataset es incoherente consigo mismo en `bug-fintech-02`, y la prueba `test_dataset_consistent` no lo detecta.

Comprobé el sha de la rama (`04872ca62ade74fade87ba091e0917b7f71fd44f`) y el hash de la tarea (`3d5aed10662a6434c6e46957d59408db29d30e2e`, coincide con el de la bitácora). Las mutaciones las apliqué en una copia en el scratchpad, nunca en el worktree. Al terminar el worktree está limpio (`git status --short | wc -l` da 0).

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Suites offline en entorno limpio | Comando CA-1 literal (`rm -rf .venv`, venv nuevo, `pip install -r requirements-dev.txt -e .`, `pytest -q \| tail -n 2`) | pasa: `13 passed` en planner y `13 passed` en reporter |
| 2 | FakeLLM determinista y falla cerrado | Comando CA-2 literal | pasa: 7 `PASSED` por servicio (mismo resultado, sin registrar, `\r\n`, conteo, error programado, latencia, vectores) y `jq` imprime `6` |
| 3 | Cero red y sin SDK | Comando CA-3 literal | pasa: 3 `PASSED` por servicio, 0 `SKIPPED`, grep de SDK da `0`, dependencias exactas `hypothesis==6.122.3 jsonschema==4.23.0 pytest==8.3.4 rfc3339-validator==0.1.4` |
| 4 | Dataset y esquemas con herramienta real | Comando CA-4 literal con `ajv-cli@5.0.0` | pasa: `bug-sembrado=4 golden=3 no-arranca=2 trampa-esquema=3`, `12`, 22 validaciones ajv (12 surface y 10 evidence) con 0 líneas distintas de `rc=0`, `1 passed`; los 2 `no-arranca` no tienen `evidence-uris.json` |
| 5 | Nada desplegable, marcadores de lenguaje | Comando CA-5 literal | pasa: `0`; dos líneas `python`; `pyproject.toml:4` en ambos; ambos `compila` |
| 6 | Árbol limpio y sin desborde | Comando CA-6 literal | pasa: `0`, `0`, `0` |

## Hallazgos

### F-01 · NARANJA · `agents/dataset/artifacts/bug-fintech-02` y `agents/agent-*/tests/test_dataset.py` · La respuesta canned del planner no cubre la invariante que el propio `expected.json` exige, y la prueba de consistencia no lo detecta
- `expected.json` de `bug-fintech-02` dice `planner.must_cover_invariants = ["revertir una transferencia es idempotente"]`.
- `planner.response.json` de ese artefacto planifica solo `POST /accounts/{id}/transfers` y `GET /accounts/{id}/balance`, ambos con la invariante "una transferencia conserva la suma total de saldos".
- Nunca planifica `POST /transfers/{id}/reverse`, que es el endpoint de la causa raíz y está en `surface.json`.
- Lo comprobé con un script que cruza `expected` y `planner.response` en los 12 artefactos. Es el único que incumple, pero es el insumo que T02 y T05 usan como verdad de referencia para el caso del planner.
- Al ejecutarse T02 o T05, una respuesta canned correcta por construcción saldría como "no cubre la invariante" y mancharía la métrica.
- Es el mismo fallo que un bloque `test_dataset_consistent` más estricto habría atrapado. Cruzar los archivos entre sí es justo lo que la prueba promete ("composición, cada surface/evidence válido, cada uri con su archivo"), pero hoy no lo hace.
- Mutaciones sobre el dataset (scratchpad):
  - Archivo de evidencia borrado: la prueba falla (bien).
  - URI fuera de la disposición `runs/<run_id>/`: falla (bien).
  - `surface.source` inválido: falla (bien).
  - Secreto sembrado: lo atrapa `test_dataset_no_secret_shaped_text` (bien).
  - Cambiar `meta.json:kind` de golden a bug-sembrado: la prueba **sigue en `1 passed`**. La composición se cuenta solo desde `manifest.json` y nunca se compara con `meta.kind`, ni con el veredicto esperado según el tipo.
- Pedir al codificador, como una clase única:
  1. Que `planner.response.json` de `bug-fintech-02` cubra la invariante esperada y planifique el endpoint de la causa raíz.
  2. Que `test_dataset_consistent` (o una prueba hermana) verifique, para los 12 artefactos:
     - `meta.id` y `meta.kind` coinciden con el manifest;
     - cada `must_cover_invariants` aparece en alguna invariante de `planner.response.json`;
     - el `root_cause` esperado coincide con `reporter.response.json`;
     - `root_cause.method` y `root_cause.path` pertenecen a `surface.json`;
     - el veredicto esperado es coherente con el tipo (golden = `sin-hallazgos`, bug = `bug` con causa raíz, trampa = `inconcluso`, no-arranca = `error`).
  3. Una mutación registrada en la bitácora que demuestre que la nueva comprobación falla.

### F-02 · AMARILLO · `agents/agent-*/tests/conftest.py` · La guarda sin red tiene huecos más allá de lo literal de la tarea
- Probé en el scratchpad, con la guarda activa:
  - `connect_ex(("93.184.216.34", 80))` devuelve `11`, es decir, intentó la conexión real.
  - `sendto` por UDP a una IP pública devuelve `1` (envió un byte).
  - `socket.gethostbyname("example.com")` resolvió `104.20.23.154` (DNS real).
  - `getaddrinfo("localhost.evil.com")` sí queda bloqueado.
  - `getaddrinfo(None, ...)` devuelve loopback, correcto.
- La tarea pide literalmente sustituir `connect` y `getaddrinfo`, y eso está implementado, así que no es un rojo. Pero "cero llamadas reales" no se garantiza para el cliente que escriba T07.
- Mutaciones de la guarda (copia en el scratchpad):
  - Quitar `autouse`: `external_connect_blocked` falla (bien).
  - Hacer que `_is_loopback` devuelva siempre `False`: falla `bind` a loopback (bien).
  - `connect` que no bloquea: la prueba de IP externa se cuelga en vez de fallar con aserción. Se detecta por timeout y solo es limpio porque `getaddrinfo` sigue bloqueando `example.com`.
  - `_is_loopback` que devuelve siempre `True`: también se cuelga sin asertar. Con red real, `create_connection` conectaría y fallaría con "DID NOT RAISE", pero hoy la detección es débil.
- Mejora: parchear también `connect_ex`, `sendto`/`sendmsg` y `gethostbyname*`, y añadir pruebas para eso.

### F-03 · AMARILLO · `fake_llm.py` · La normalización de `\r` suelto no está fijada por ninguna prueba
- La mutación que quita `.replace("\r", "\n")` pasa las 7 pruebas en ambas copias. Los vectores solo cubren `\r\n`.
- La tarea dice "saltos de línea normalizados a `\n`" y solo exige un vector con `\r\n`. Una copia podría divergir en `\r` solo.
- Mejora: añadir un vector con `\r` suelto.

### F-04 · AMARILLO · Desviación `-vv` en `addopts` · Aceptable
- Confirmado: sin `-vv`, `pytest -q -v` se cancela a verbosidad 0 y no lista nombres de prueba. Con `-o addopts` sin `-vv`, la salida son solo cabeceras y nada que `grep PASSED` pueda capturar.
- Por eso CA-2 y CA-3 (y los criterios análogos de T02–T07, que usan `-q -k ... -v | grep PASSED | sed`) solo se satisfacen con `-vv`. Con `-vv`, `-q -v` suma 2 y lista nombres. El `pytest -q` simple queda en verbosidad 1: más ruidoso, pero `tail -n 1` y `tail -n 2` siguen mostrando `N passed`.
- Veredicto sobre la nota del codificador: **desviación aceptable**, y T02–T06 no pueden editar `pyproject.toml`, así que esta es la única forma de que sus criterios funcionen.
- Solo pido que quede documentada con un comentario en `pyproject.toml` o en la bitácora. Hoy solo hay una línea escueta en la bitácora.

### F-05 · AMARILLO · Bitácora
- La prueba negativa del `surface.json` (base_url sin esquema o campo extra) quedó commiteada como `test_dataset_negative_bad_base_url_and_extra_field`, aunque la tarea decía "se ejecuta y se registra, no se commitea". No sobra ni desborda el alcance, pero la salida literal de esa negativa no está pegada en la bitácora; solo está descrita.

## Revisiones sin hallazgo
- **Alcance:** el diff son 137 archivos, todos bajo `agents/agent-planner`, `agents/agent-reporter`, `agents/dataset` y `bitacoras/U3-T01.md`. No hay `Dockerfile` ni servidor. No toca `contracts/`, `deploy/`, `policy/`, workflows ni el `.gitignore` raíz. No hay `.venv` ni `__pycache__` versionados.
- **Sin red:** `grep` de `environ|getenv|http` en `src` da 0. No hay SDK de LLM ni cliente HTTP. Cada proyecto usa solo `jsonschema`, `rfc3339-validator`, `pytest` y `hypothesis`. Las copias de `FakeLLM` y de las pruebas difieren solo en el nombre del paquete (`diff` entre servicios).
- **Mutaciones del FakeLLM**, aplicadas en ambos servicios y todas detectadas salvo F-03:
  - Responder con texto vacío en vez de `UnscriptedPrompt`.
  - No normalizar `\r\n`.
  - No contar llamadas.
  - Ignorar el error programado.
  - Codificar en latin-1.
  - Usar sha1 en lugar de sha256.
  - Ignorar el timeout de latencia.
  - Intercambiar `input_tokens` y `output_tokens`.
  - Quitar la espera simulada.
- **Vectores:** cambiar un byte de `fake-llm-vectors.json` rompe ambas copias (`1 failed` en cada una).
- **Inyección y secretos:** hay texto con forma de instrucción dirigida al modelo en `bug-ecom-01/evidence/flow-2/logs.txt`. Ningún archivo del dataset tiene un secreto o algo con forma de uno (barrido por la prueba, y el secreto sembrado se detecta).

## Tareas candidatas (defectos reales fuera de alcance)
- Endurecer la guarda sin red (`connect_ex`, `sendto`/`sendmsg`, `gethostbyname*`), preferiblemente en la tarea que introduzca el adaptador HTTP (T07).
- Corregir en las tareas T02–T07 y en la plantilla el idioma `-q -v` (se cancelan) y fijar el patrón de salida que dependen de `-vv`.
- Compartir una sola copia del `FakeLLM` o añadir una prueba que verifique que ambas son idénticas salvo el nombre del paquete.

VEREDICTO: NO-VERDE
NARANJA|agents/dataset/artifacts/bug-fintech-02 y tests/test_dataset.py|Respuesta canned del planner no cubre la invariante esperada y test_dataset_consistent no cruza expected con las respuestas ni meta.kind con el manifest
