# Ronda 1 — U5-T01

VEREDICTO: NO-VERDE

El código hace lo que la tarea pide. El veredicto es NO-VERDE por un solo motivo: CA-2, tal como está escrito, da `10` y no el `8` esperado. La causa está en la especificación, no en el diff. La corrección le corresponde al orquestador, no al codificador (ver F-01).

Contexto verificado: el hash de la tarea en el worktree y en main es `7112d0c3db25fbb46ed1975f2a3933ca54cc1464` (coincide). La rama es `tarea/U5-T01` y HEAD es `2ed2e63`. El diff `087cf7e..tarea/U5-T01` tiene 10 archivos: 8 `.gitkeep`, `README.md` (nuevo, no existía en la base) y `bitacoras/U5-T01.md`.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Existe el árbol | el bucle `for d in services agents ... .github/workflows; do test -d ...` del archivo de la tarea, en `/home/omarjayg/Javeriana/topicos-especiales/wt-U5-T01` | pasa: `ARBOL OK` |
| 2 | `.gitkeep` trackeados, esperado `8` | `git ls-files \| grep -c -E '(^\|/)\.gitkeep$'` | **falla: `10`** (los 8 del árbol, más `bitacoras/.gitkeep` y `revisiones/.gitkeep`) |
| 3 | README nombra cada directorio, ≥5 líneas | `grep -E 'services/\|agents/\|contracts/\|deploy/flux/\|\.github/workflows/' README.md \| wc -l` | pasa: `8` |
| 4 | Árbol limpio tras el commit | `git status --short` | pasa: 0 líneas |

Plan de pruebas, también corrido por mí:
- **Rojo en la base.** Extraje `087cf7e` con `git archive` a un directorio temporal y corrí CA-1 allí: `FALTA services`, rc=1. El par falla-pasa queda confirmado.
- **`git check-ignore services/.gitkeep .github/workflows/.gitkeep`:** sin salida, rc=1. Además corrí `git check-ignore --no-index -v` sobre los 8 `.gitkeep`: ninguno está ignorado. No hay ningún `.gitignore` en el repo.
- **`git ls-files --error-unmatch services/.gitkeep`:** imprime la ruta, rc=0.
- **Comprobación negativa**, sobre copias de `tarea/U5-T01` hechas con `git archive` en el scratchpad, sin tocar el worktree. Borré el directorio y su `.gitkeep` y corrí el CA-1 literal. En los cuatro casos CA-1 falla con rc=1:
  - `services` da `FALTA services`
  - `contracts/events` da `FALTA contracts/events`
  - `deploy/flux/prod` da `FALTA deploy/flux/prod`
  - `.github/workflows` da `FALTA .github/workflows`

  Sobre la copia intacta da `ARBOL OK`.
- **CA-2 acotado al árbol de la tarea:** `git ls-files -- services agents contracts deploy .github | grep -c -E '(^|/)\.gitkeep$'` da `8`.

Alcance:
- **Cumple el alcance "Fuera".** Los únicos archivos bajo `services/`, `agents/`, `contracts/`, `deploy/` y `.github/` son `.gitkeep` vacíos. No hay contratos, workflows, manifiestos, licencias ni linters.
- **Respeta el layout canónico** (`unit-of-work.md`, líneas 43-58): no hay directorios inventados ni renombrados.
- **Cada `.gitkeep` está solo** en un directorio que, sin él, quedaría vacío.

El worktree quedó limpio después de mi revisión (`git status --short` da 0 líneas).

## Hallazgos

### F-01 · ROJO · CA-2 (`tareas/U5-T01-esqueleto-monorepo.md`, CA-2) · El criterio da 10 y no 8, por un defecto de la especificación
- **Qué pasa:** el comando de CA-2 cuenta todos los `.gitkeep` del repo. La base `087cf7e`, que es el mismo commit que creó la tarea, ya trackeaba `bitacoras/.gitkeep` y `revisiones/.gitkeep`. Lo comprobé con `git ls-tree -r --name-only 087cf7e | grep gitkeep`.
- **Por qué es ROJO:** el criterio, tal como está escrito, no se cumple. Eso hace el NO-VERDE obligatorio y no lo suavizo.
- **No se puede cumplir sin desbordar:** el codificador solo podría llegar a `8` borrando andamiaje del loop, y eso sería un desborde de alcance, también ROJO.
- **No se le pide ningún cambio de código al codificador.** El codificador lo detectó y lo dejó anotado en la bitácora ("Nota CA-2"), que es lo correcto.
- **Remedio, para el orquestador:** enmendar CA-2 para que cuente solo las rutas del monorepo. Por ejemplo: `git ls-files -- services agents contracts deploy .github | grep -c -E '(^|/)\.gitkeep$'` con esperado `8`. Lo corrí sobre el HEAD actual y devuelve `8`. Con esa enmienda, este diff pasaría sin cambios.

### F-02 · AMARILLO · `bitacoras/U5-T01.md` · Evidencia incompleta o sustituida en la bitácora (barrido de clase, tres casos)
1. **"Rojo primero"** registra `$ CA-1` como pseudo-comando, sin el comando literal que se corrió.
2. **Comprobación negativa:** el codificador corrió `bash -c "test -d services || echo FALTA services"`, que es un único `test` y no el CA-1 que el plan prometía. Además lo hizo moviendo el directorio dentro del worktree real.
3. **CA-4:** no hay ningún registro de `git status --short` limpio. La única salida de `git status` que aparece en la bitácora muestra `M bitacoras/U5-T01.md`.

No bloquea: rehíce las tres comprobaciones yo mismo con los comandos literales y pasan (ver arriba). Para las próximas tareas conviene que la bitácora pegue el comando literal del criterio.

## Tareas candidatas (defectos reales fuera de alcance)
- Revisar los criterios de las próximas tareas que cuenten archivos a nivel de todo el repo (`git ls-files | grep -c ...`). El andamiaje del loop (`bitacoras/`, `revisiones/`) los contamina. Conviene acotarlos siempre por rutas.

## Rutas de transcripciones largas
- No hay transcripciones largas. Las copias temporales de la base y de la rama están en `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/4ed52fd5-a142-46f0-ac9b-995972cfe97d/scratchpad/` (directorios `base/`, `neg/` y `neg2/`).
- No escribí el informe en disco porque mis herramientas son de solo lectura. El orquestador debe guardarlo en `revisiones/U5-T01/ronda-1.md`.

```
VEREDICTO: NO-VERDE
ROJO|CA-2 (tareas/U5-T01-esqueleto-monorepo.md)|CA-2 da 10 y no 8: la base ya trackea bitacoras/.gitkeep y revisiones/.gitkeep; el orquestador debe acotar el comando a las rutas del monorepo
INFORME: revisiones/U5-T01/ronda-1.md
```
