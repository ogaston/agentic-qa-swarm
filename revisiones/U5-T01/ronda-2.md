# Ronda 2 — U5-T01

VEREDICTO: VERDE

F-01 y F-02 están cerrados. Corrí CA-1..CA-4 y el plan de pruebas yo mismo, y todo pasa. Quedan dos amarillos que no bloquean.

Contexto verificado:
- La rama es `tarea/U5-T01` y HEAD es `fc94e2c6afc50cdff609ba5e5324718bb5775ca6`. `208b4cf` es ancestro de HEAD, así que el rebase es real.
- El hash de la tarea en el worktree es `fec7040b16fb9715fdeb249f149829c09296b4dd` y coincide con el esperado. `git diff 208b4cf..tarea/U5-T01 -- tareas` da 0 líneas.
- El diff rebasado tiene 11 archivos, todos nuevos. Son 8 `.gitkeep`, `README.md`, `bitacoras/U5-T01.md` y `revisiones/U5-T01/ronda-2-respuesta.md`.

## Criterios de aceptación, verificados por mí
Los corrí en `/home/omarjayg/Javeriana/topicos-especiales/wt-U5-T01`.

| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Existe el árbol | el bucle `for d in services agents contracts/openapi ...` literal del archivo de la tarea | pasa: `ARBOL OK` |
| 2 (enmendado) | `.gitkeep` trackeados en las rutas del monorepo | `git ls-files -- services agents contracts deploy .github \| grep -c -E '(^\|/)\.gitkeep$'` | pasa: `8` |
| 3 | README nombra cada directorio | `grep -E 'services/\|agents/\|contracts/\|deploy/flux/\|\.github/workflows/' README.md \| wc -l` | pasa: `8` (≥5) |
| 4 | Árbol limpio tras el commit | `git status --short` | pasa: 0 líneas |

Plan de pruebas:
- **Rojo en la base.** Extraje `208b4cf` con `git archive` al scratchpad y corrí el CA-1 literal. Da `FALTA services` con rc=1.
- **Ignorados.** `git check-ignore services/.gitkeep .github/workflows/.gitkeep` no imprime nada y da rc=1.
- **Trackeados.** `git ls-files --error-unmatch services/.gitkeep` imprime `services/.gitkeep`.
- **Comprobación negativa.** Sobre copias de HEAD hechas con `git archive`, sin tocar el worktree, borré uno por uno los 8 directorios. CA-1 falla con rc=1 en los 8 casos y dice `FALTA <dir>` (`services`, `agents`, `contracts/openapi`, `contracts/events`, `deploy/flux/base`, `deploy/flux/dev`, `deploy/flux/prod`, `.github/workflows`). Esto amplía los 4 casos de la ronda 1.

El worktree quedó limpio después de mi revisión (`git status --short` da 0 líneas).

## Estado de los hallazgos de la ronda 1
- **F-01 (ROJO, CA-2 daba 10 y no 8): cerrado.** El humano enmendó CA-2 acotándolo a las rutas del monorepo. El comando enmendado da `8` sobre este diff sin ningún cambio de código, tal como predije. El codificador no tocó `bitacoras/.gitkeep` ni `revisiones/.gitkeep`.
- **F-02 (AMARILLO, evidencia en la bitácora): cerrado en lo sustancial.** La sección "Ronda 2" de la bitácora ahora trae el rojo literal sobre un `git archive` de la base, con rc=1. Trae también la comprobación negativa con el CA-1 completo sobre copias temporales, no moviendo directorios del worktree real. Reproduje cada una de esas comprobaciones con el mismo resultado. Ver F-03 por lo que queda.

## Hallazgos
### F-03 · AMARILLO · `bitacoras/U5-T01.md` (sección "Ronda 2") · Quedan abreviaturas en la bitácora
La sección "Ronda 2" todavía escribe `$ CA-1` y `$ grep -E ... README.md | wc -l` en vez del comando completo. No hay CA-2 ni CA-4 literales en esa sección. La bitácora dice que CA-4 se reporta en el mensaje de vuelta, lo cual es razonable porque no puede registrar el estado posterior a su propio commit. Yo reproduje todo con los comandos literales y pasa, así que no bloquea. Para próximas tareas conviene pegar el comando entero.

### F-04 · AMARILLO · `revisiones/U5-T01/ronda-2-respuesta.md` · Clasificación del archivo bajo `revisiones/`
- **Es aceptable y no es desborde.** `.claude/agents/codificador.md`, líneas 50 y 56, obliga al codificador a responder cada hallazgo en `ronda-N-respuesta.md`.
- **No toca el alcance "Fuera".** No hay contratos, workflows, manifiestos, servicios, licencias ni linters.
- **No hay conflicto de nombres con el informe del revisor.** En main `revisiones/U5-T01/` solo tiene `ronda-1.md`. El archivo del codificador es `ronda-2-respuesta.md` y mi informe de esta ronda será `ronda-2.md`.
- **Qué debe vigilar el orquestador.** Es un archivo de andamiaje del loop dentro del diff de la tarea. Al fusionar, el orquestador decide si persiste `ronda-2.md` junto a él, y debe cuidar que el informe del revisor y la respuesta del codificador no colisionen.

## Alcance "Fuera", sobre el diff rebasado
- **Contratos.** No hay contenido en `contracts/`. Solo hay dos `.gitkeep`.
- **Workflows.** `.github/workflows/` solo contiene `.gitkeep`.
- **Manifiestos Flux.** `deploy/flux/{base,dev,prod}` solo contienen `.gitkeep`.
- **Servicios y agentes.** `services/` y `agents/` solo contienen `.gitkeep`.
- **Licencias y linters.** No hay licencias, `.gitignore`, linters ni toolchain.
- **Clúster.** No hay comandos contra clúster.
- **Layout.** Coincide con el canónico. No hay directorios inventados ni renombrados.
- **`.gitkeep`.** Cada uno está solo en un directorio que de otro modo quedaría vacío.
- **README.** `README.md` es nuevo, único y sin variantes con sufijo. Nombra los 8 directorios.
- **Literales incrustados.** No hay valores nuevos visibles para el usuario en puntos de decisión.
- **Suite.** La suite no depende de estado externo ni se salta ninguna prueba.

## Tareas candidatas (defectos reales fuera de alcance)
- Sigue en pie la de la ronda 1. Los criterios futuros que cuenten archivos deben acotarse siempre por rutas, para que el andamiaje del loop (`bitacoras/`, `revisiones/`) no los contamine.

## Rutas de transcripciones largas
- No hay transcripciones largas. Las copias temporales están en `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/4ed52fd5-a142-46f0-ac9b-995972cfe97d/scratchpad/` (directorios `b2/` y `n/`).
- No escribí el informe en disco porque mis herramientas son de solo lectura. El orquestador debe guardarlo en `revisiones/U5-T01/ronda-2.md`.

```
VEREDICTO: VERDE
AMARILLO|bitacoras/U5-T01.md (Ronda 2)|Quedan abreviaturas ($ CA-1, grep -E ...) en la bitácora; no bloquea
AMARILLO|revisiones/U5-T01/ronda-2-respuesta.md|Archivo de andamiaje del loop en el diff; exigido por codificador.md, no es desborde
INFORME: revisiones/U5-T01/ronda-2.md
```
