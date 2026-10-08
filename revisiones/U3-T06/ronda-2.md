# Ronda 2 — U3-T06

VEREDICTO: NO-VERDE

Worktree `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U3-T06`, sha 240ba21 (código en 03fbc51). Corrí con `PYTHONDONTWRITEBYTECODE=1` y `__pycache__` limpio. Las mutaciones las apliqué en copias del scratchpad, con `PYTHONPATH` apuntando a la copia.

Una nota de mi lado: una carrera entre dos mutantes paralelos creó un symlink `contracts/contracts` sin versionar dentro del worktree. Lo borré (`git status --short` da 0 al terminar). No afecta a los resultados.

## Criterios de aceptación, verificados por mí
| # | Comando | Resultado |
|---|---|---|
| CA-1 | `-k pbt -m 'not pbt_demo' -v` | `agent-planner passed=17 seeds=1 failed=0`; `agent-reporter passed=6 seeds=1 failed=0`. Pasa. |
| CA-2 | demo planner | `Falsifying example … flow_id='00000'`; las dos últimas líneas son idénticas. Pasa. |
| CA-3 | `pytest` normal | `168 passed, 7 deselected` / 0 y `144 passed, 2 deselected` / 0 (24-35 s). Pasa. |
| CA-4 | `-k pbt_generator_coverage` | PASSED en ambos. Pasa. |
| CA-5 | mutaciones a-f, detalle abajo | Las 6 mueren. |
| CA-6 | PBT.md | `1` y `1`; 39 y 40 líneas; 3 y 2 coincidencias. Pasa. |
| CA-7 | secretos, compileall, alcance | Secretos `0`; los dos `compileall` ok; `git status` 0; fuera del filtro solo `redact.py` y `reporter.py`. Pasa. |

## Puntos pedidos
**(3) F-01, barrido de seeds.**
- reporter: `PBT_SEED=301..311` más tres tramos de 11 seeds (44 seeds), 44 de 44 con `6 passed`.
- planner: 32 seeds (401..432), 32 de 32 con `17 passed`.
- Resuelto.

**(2) Mutaciones contra el redact.py final.**
- a-f mueren, y cada una con la propiedad esperada:
  - (a) `validator_accept_implies_invariants` y `validator_each_class[unobserved/wrong_method]`.
  - (b) `k6_render_extract_roundtrip` (solo por `isascii`).
  - (c) y (d) `redact_removes_seeded_secrets…`.
  - (e) `report_validator_accept_implies_invariants`.
  - (f) `evidence_truncation…`.
- Mutaciones propias que mueren, con la propiedad indicada:
  - planner: sin `max_flows`, `flow_id` duplicado, `<` sin escapar, `>=` en el tope, `sort_keys=False`, `match` por `fullmatch`, sin tope de pasos.
  - reporter: URIs fuera de la lista blanca, `sin-hallazgos` con flujos no pasados, sin private_key / url / github / authorization, tope total ignorado, sin cabeza, `truncated=False`, `_AWS` demasiado amplio, sin retirar el centinela, sin el pre-paso `_ESC_END`, sin normalizar a UTF-8.
- Mutación `run_id` (F-06): muere con los seeds 1 a 5, por `test_pbt_validator_each_class[wrong_run]`.
- Superviviente: revertir el valor crudo de `_ASSIGN` (F-02) sobrevive a las propiedades con los seeds 1 a 7 y también con el 21. Solo la mata la regresión fija (ver F-04).

**(1) Rediseño del centinela.**
- Centinela: 30 000 cadenas aleatorias por la API de texto, con escapes, ANSI y U+2400..2402. En ninguna aparece en la salida. Un texto legítimo que ya contiene U+2400/U+2401 queda intacto.
- Idempotencia: 20 000 cadenas con secretos separados, 0 casos no idempotentes (main igual). Con secretos pegados sin separador, new y main empatan en 1723 de 20 000, es decir, es preexistente.
- Casos de la ronda 1:
  - Bearer, JWT y URL tras `\x1b`, `é` escapado (`\u00e9`), `日`, `\x00`, `\x7f`, `😀`, ANSI crudo y escapado, todos redactados.
  - `password=…\n\npassword:…` queda bien, con `\n` real o escapado.
  - Quedan los fallos de F-01 y F-02.
- Costo, 51 formas a 64 y 128 KiB: el peor es `token` con 103 ms a 128 KiB (main 67 ms). El escalado de 32K a 512K es ×~2 por duplicación. `secret=` y `password=a\` muestran un salto ×4 aislado de 64K a 128K y vuelven a ×2; no es cuadrático. Sin regresión de orden.

**(4) F-04.** La normalización a UTF-8 antes de recortar mantiene el tope y las marcas: la propiedad 9 con bytes arbitrarios pasa en 44 seeds y la mutación sin normalizar la mata (`rt`).

**(5) F-07.** La demo por seed funciona: seeds 1, 1, 2, 3 dan (1524,1524,5399), (1524,1524,5399), (5323,6945,9076) y (769,769,769). Mismo seed igual, otro seed distinto.

**(6) Higiene.**
- Secretos con forma real en `agents/` y `contracts/`: 0.
- Fusión de prueba (rama T06 + `origin/main`, con T05) en un clon del scratchpad: fusiona limpia.
- Suites en ese clon: planner 168 passed, reporter 144 passed (incluye `test_repo_hygiene`, que recorre `agents/eval`), eval 102 passed.

**(7) T04 y T05.**
- La suite de T04 y la de eval pasan con el `redact.py` nuevo (`agents/eval` usa `redact_text_counted` como detector).
- No corrí la suite completa de eval fuera de ese clon.

## Hallazgos

### F-01 · NARANJA · `redact.py:_BEARER` · `\b` reintroduce una fuga en Bearer tras caracteres no ASCII
- El rediseño cambió `(?<![A-Za-z0-9_])` por `\b`, que en `str` es Unicode.
- El redactor trabaja sobre bytes decodificados en latin-1, y cualquier carácter cuyo último byte UTF-8 sea ª ² ³ µ ¹ º ¼ ½ ¾ cuenta como letra o dígito.
- Medido: de 12 000 caracteres (U+0080..U+2FFF) pegados delante de `Bearer 00000000`, **1710 filtran el secreto** (≈14 %), por ejemplo `ªBearer 00000000`.
- JWT y URL, con lookbehind ASCII, no fugan. En r1 también eran 0.
- Con el `redact.py` de main eran 1710, así que es la vuelta al comportamiento de main.
- El generador no lo ve: `GLUE` es una tupla fija (`é`, `日`, emoji, U+2028, ANSI), no un carácter Unicode arbitrario.
- Se pide: lookbehind ASCII en bearer y `GLUE` con `st.characters()`, más una regresión.

### F-02 · NARANJA · `redact.py:_ASSIGN` · El arreglo de F-02 fuga la cola de contraseñas con barra invertida
- `(?:\\(?![nrtbf])|[^\s"',;&\\])+` va bajo `(?i)`, así que `\B`, `\N`, `\R`, `\T`, `\F` cortan el valor igual que los escapes JSON en minúscula. Un `\\` seguido de `b/n/…` y un literal `\n` también.
- Medido con `password=<v>\n`:
  - `AAAA\BBBB1111` fuga `BBBB1111`.
  - `C:\Users\Tom\pw1111` fuga la parte tras `\T`.
  - `AAAA\NBBBB1111` y `AAAA\\BBBB1111` también fugan.
  - Con el `redact.py` de main y el de ddf91c7 se redactaba entero (`ok` en los 6 casos). Es una regresión.
- Se pide: `(?-i:[nrtbf])` y una regresión; decidir si `\n` literal dentro de un valor es un corte aceptado.

### F-03 · NARANJA · `redact.py:_redact_str` · Centinela: `StopIteration` y centinela numérico
- Un texto con los 256 caracteres U+2400..U+24FF hace `next(...)` lanzar `StopIteration`.
- Fuera de la API de bytes (donde nunca ocurre) es alcanzable por el modelo: `correlate_postmortem` pasa el resumen del LLM por `redact_text_counted`. Con un resumen que los contenga todos, la llamada de extremo a extremo revienta con `StopIteration`, no con `ReportRejected`.
- Si faltan U+2400..U+245F, el centinela sería U+2460 (círculo numérico, que `\w` toma como palabra). Entonces `\nBearer ZZZZ9999` no se redacta (fuga confirmada).
- Es un caso fabricado, pero está en el modelo de amenaza (inyección en la salida del modelo).
- Se pide: elegir el centinela entre caracteres no `\w` y con fallback (por ejemplo un plano privado), y que nunca lance.

### F-04 · AMARILLO · propiedad 7 vs F-02
La mutación que revierte el valor crudo sobrevive a las propiedades con los seeds 1 a 7 y 21. La fijó el seed 21 de la ronda 1, y tras cambiar `GLUE` la propiedad ya no la reproduce. Solo la regresión fija la protege, que cumple lo que pide la tarea.

### F-05 · AMARILLO · `Secret`/`secret_text`
Los generadores nunca colocan dos secretos pegados, lo que oculta la no idempotencia preexistente (1723 de 20 000 en main también). Es preexistente, no es de T06.

## Tareas candidatas (fuera de alcance)
- Escáner de higiene que excluya `.hypothesis/` y trate `agents/eval/` de forma explícita.
- La no idempotencia con secretos pegados sin separador es preexistente en `redact_secrets`.

VEREDICTO: NO-VERDE
NARANJA|redact.py:_BEARER (+ tests/gen.py:GLUE)|`\b` Unicode: Bearer tras 1710 caracteres no ASCII (ª, µ, ² …) se filtra; GLUE fijo no lo ve
NARANJA|redact.py:_ASSIGN (arreglo F-02)|Regresión: valores con barra invertida (`\B`, `\T`, `\\`, `C:\Users\Tom\pw`) fugan la cola; `[nrtbf]` bajo `(?i)`
NARANJA|redact.py:_redact_str (centinela)|`StopIteration` con los 256 U+24xx (extremo a extremo vía resumen del modelo) y centinela U+2460 `\w` que fuga Bearer tras `\n`
