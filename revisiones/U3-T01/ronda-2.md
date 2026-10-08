# Ronda 2 — U3-T01

VEREDICTO: VERDE

Los seis criterios de aceptación pasan con mis propias ejecuciones frescas (rama `tarea/U3-T01`, sha `9b89dca4d1be6d8469d3c838a3402ed11bac75f4`). Los cinco hallazgos de la ronda 1 están corregidos y los verifiqué uno por uno. No queda ROJO ni NARANJA. Las mutaciones las apliqué en copias en el scratchpad; el worktree sigue limpio (`git status --short | wc -l` da 0). El diff contra la ronda 1 solo toca `agents/` y los archivos de bitácora y revisiones (`bitacoras/U3-T01.md` y `revisiones/U3-T01/`).

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Suites offline en entorno limpio | CA-1 literal (`rm -rf .venv`, venv nuevo, `pip install -r requirements-dev.txt -e .`, `pytest -q \| tail -n 2`) | pasa: `17 passed` en planner y `17 passed` en reporter |
| 2 | FakeLLM determinista y falla cerrado | CA-2 literal | pasa: 7 `PASSED` por servicio y `jq` imprime `7` (ahora hay 7 vectores) |
| 3 | Cero red y sin SDK | CA-3 literal | pasa: 6 `PASSED` por servicio, 0 `SKIPPED`, grep de SDK da `0`, dependencias exactas `hypothesis==6.122.3 jsonschema==4.23.0 pytest==8.3.4 rfc3339-validator==0.1.4` |
| 4 | Dataset y esquemas con ajv real | CA-4 literal con `ajv-cli@5.0.0` | pasa: `bug-sembrado=4 golden=3 no-arranca=2 trampa-esquema=3`, `12`, contador `0`, `1 passed`; los 2 `no-arranca` no tienen `evidence-uris.json` |
| 5 | Nada desplegable, marcadores | CA-5 literal | pasa: `0`; dos líneas `python`; `pyproject.toml:4` en ambos; ambos `compila` |
| 6 | Árbol limpio, sin desborde | CA-6 literal | pasa: `0`, `0`, `0` |

## Verificación de los hallazgos de la ronda 1

**F-01 (NARANJA): cerrado.**
- `bug-fintech-02/planner.response.json` ahora planifica `POST /transfers/{id}/reverse` con la invariante "revertir una transferencia es idempotente".
- `bug-logis-01` también estaba mal, y el codificador lo corrigió por la nueva comprobación. Su planner planificaba `POST /shipments` y no el endpoint de la causa raíz. Ahora planifica `PATCH /shipments/{id}/status`.
- `check_cross` añade al `test_dataset_consistent` varias comprobaciones sobre los 12 artefactos:
  - `meta.id` coincide con el directorio, y `meta.kind` con el manifest.
  - El veredicto esperado es coherente con el tipo.
  - `root_cause` solo existe en `bug-sembrado`.
  - El resultado esperado del planner es coherente con el tipo.
  - Cada `must_cover_invariants` está cubierto por las invariantes del planner.
  - `root_cause` coincide con `reporter.response`.
  - `root_cause` pertenece a `surface.json`.
  - El planner planifica el endpoint de la causa raíz.
- Mutaciones sobre el dataset con la prueba endurecida, todas con `1 failed`:
  - cambiar `meta.kind`;
  - cambiar `meta.id`;
  - dejar `root_cause` en null;
  - cambiar el path del `root_cause` en `reporter.response`;
  - mover el `root_cause` fuera de `surface`;
  - quitar del planner los pasos del endpoint de la causa raíz;
  - invariantes del planner distintas de las esperadas;
  - `outcome` incoherente en un golden;
  - veredicto incoherente en un `trampa`;
  - restaurar los dos `planner.response` viejos de la ronda 1: falla.
- Con la copia del reporter, cambiar `meta.kind` también falla (`1 failed`).
- Una mutación que en realidad era un no-op (`bug-ecom-01` con veredicto `bug`, que ya lo era) la descarto; el caso de veredicto está cubierto por la mutación en `trampa-ecom-01`.
- Barrido de la misma clase sobre el dataset real, con mi propio script de cruce:
  - `run_id` de la evidencia igual al de `surface`.
  - Flujos de la evidencia iguales a los flujos del planner.
  - Pasos dentro de `surface`.
  - Campo `violated` igual a la invariante de la causa raíz.
  - Path de la causa raíz presente en los logs.
  - `findings` de golden vacío, sin fallos en la evidencia golden.
  - Los `trampa` fallan por formato, sin `violated`.
  - Respuestas de los `no-arranca` en error.
  - Los 12 artefactos salen `ok`.

**F-02 (guarda sin red): cerrado.** El `conftest.py` ahora parchea también `connect_ex`, `sendto`, `sendmsg`, `gethostbyname` y `gethostbyname_ex`, con 3 pruebas nuevas (`connect_ex`, UDP y `gethostbyname`). Quité cada parche en una copia:
- sin `sendto`, sin `sendmsg`, sin `gethostbyname` o sin `gethostbyname_ex`, falla la prueba correspondiente (`DID NOT RAISE`).
- sin `connect_ex`, la prueba se cuelga hasta mi timeout en vez de fallar con una aserción (ver Y-01).
- Probé `sendto` con tres argumentos, `connect` UDP e IPv6: los tres quedan bloqueados.
- Las dos mutaciones de ronda 1 que devolvían siempre loopback o nunca loopback siguen detectadas.

**F-03 (`\r` suelto): cerrado.** Hay un vector nuevo `"a\rb"` en `fake-llm-vectors.json`. El codificador reporta que quitar el `replace("\r", ...)` rompe la prueba; los 7 pasan con el vector actual.

**F-04 (`-vv`): cerrado.** El comentario explicativo está en el `pyproject.toml` de ambos servicios (visto en el diff).

**F-05 (bitácora): cerrado.** La salida literal de la negativa quedó pegada: `2 passed`, con la nueva `test_dataset_negative_kind_mismatch_and_uncovered_invariant`.

## Hallazgos nuevos (AMARILLO, no bloquean)
- **Y-01 · `tests/test_no_network.py` · detección por cuelgue.** Si se quita el parche de `connect_ex`, el test del socket sin `settimeout` queda colgado. Con red real devolvería `EINPROGRESS` y tampoco levantaría una aserción. Poner `s.settimeout(0.3)` en esa prueba.
- **Y-02 · `conftest.py` · `getaddrinfo` redundante.** Quitar el parche de `getaddrinfo` deja las 6 pruebas pasando, porque `create_connection` resuelve el DNS real y después `connect` queda bloqueado. Falta una prueba directa de que `getaddrinfo("example.com")` lanza `NetworkBlocked` (la resolución DNS real se escaparía sin que ninguna prueba falle). `gethostbyaddr` tampoco está parcheado (la prueba se colgó en mi sondeo).
- **Y-03 · `test_dataset.py` · cobertura residual de la coherencia cruzada.**
  - `reporter.response.verdict` no se compara con `expected.reporter.verdict`.
  - Cambiar a `failed` un `result.json` de un golden tampoco se detecta.
  - Los datos actuales son coherentes (mi barrido lo confirma), así que es endurecimiento, no un defecto.

## Tareas candidatas (fuera de alcance)
- Endurecer más la guarda (`gethostbyaddr`) y mover la prueba de `getaddrinfo` al adaptador HTTP de U3-T07.
- Corregir en los criterios de T02–T07 la expresión `-q -v` (se cancelan) o fijar `-vv` en la plantilla; y compartir una sola copia del `FakeLLM` o probar que las dos son idénticas salvo el nombre del paquete.

VEREDICTO: VERDE
AMARILLO|agents/agent-*/tests/test_no_network.py|connect_ex sin settimeout se detecta por cuelgue, y getaddrinfo/gethostbyaddr sin prueba directa
AMARILLO|agents/agent-*/tests/test_dataset.py|reporter.response.verdict y evidencia vs tipo sin cruzar
