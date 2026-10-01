# Ronda 2 — U5-T03 (la bitácora la llama "Ronda 3")

VEREDICTO: VERDE

Worktree /home/omarjayg/Javeriana/topicos-especiales/wt-U5-T03, HEAD d31010e. El hash de la tarea coincide (79025a9…). Dejé el worktree limpio: `git status --short` da 0 líneas antes y después de mis pruebas. Ya no queda ningún ROJO ni NARANJA.

## Criterios de aceptación, verificados por mí (ejecución fresca, comandos literales de la tarea)
| # | Criterio | Resultado |
|---|---|---|
| 1 | workflow y guard existen | `OK` |
| 2 | actionlint 1.7.7 (con `--security-opt label=disable`) | solo `rc=0` |
| 3 | seis jobs | `6` |
| 4 | SHA de 40 caracteres y permisos | `0`, `1`, `1` |
| 5 | publish solo en tags y sin latest | `1`, `0`, `N/A` |
| 6 | guard | `rc=0` sobre `deploy/flux/prod`; `rc=1` nombrando a.yaml (`:latest`); `rc=1` nombrando a.yaml (sin tag); `rc=0` (tag más digest) |
| 7 | list-services | `[]` con `rc=0`; con fixtures `["services/go-a","agents/planner"]` |
| 8 | árbol limpio y sin `.gitkeep` | `0` y `0` |
| Plan | shellcheck v0.10.0 sobre `scripts/ci/*.sh` | sin salida, `rc=0` |

`deploy/flux/prod` existe en el árbol: `git ls-tree` muestra `deploy/flux/{base,dev,prod}/.gitkeep` tanto en 93f0546 como en la rama, es decir, vienen de U5-T01. Por eso el caso 1 de CA-6 ahora corre sobre un directorio existente sin `*.yaml` y da `rc=0` sin aviso, como dice el codificador. El diff de la tarea no toca `deploy/`.

## Verificación por hallazgo de la ronda 1
- **F-01 (NARANJA): corregido.** Verifiqué las 8 acciones con `git ls-remote` contra `refs/tags/<tag>` y `refs/tags/<tag>^{}`.
  - `anchore/sbom-action` ahora fija `3ad7283483fc7af8ff2b4ea19663c2d5ca935e26`, que es la línea `^{}` (el commit) de v0.24.2. El `006b7ce…` anterior era el objeto del tag anotado.
  - `actions/setup-node` v4.4.0 fija `49933ea5288caeca8642d1e84afbd3f7d6820020`, que coincide con el tag ligero (sin línea `^{}`, así que es un commit).
  - Las otras 6 (checkout, setup-go, setup-python, upload-artifact, login-action, build-push-action) coinciden con tags ligeros y son commits.
- **F-02 (NARANJA): corregido.** Nuevo `scripts/ci/detect-lang.sh`, cableado en `test` y `vuln` con `id: lang`.
  - Mis fixtures dan: `go.mod` imprime `go`; `pyproject.toml` imprime `python`; `requirements.txt` imprime `python`; `package.json` imprime `node`; `go.mod` más `package.json` imprime `go` y `node`. Todos con `rc=0`.
  - Un directorio vacío da `::error::… no tiene marcador reconocido` y `rc=1`.
  - Simulé el paso con `bash -e -c 'langs=$(bash scripts/ci/detect-lang.sh "${SERVICE}")…'`: con directorio vacío el paso sale en `rc=1` (la asignación propaga el fallo). Con `go.mod` y `package.json` escribe `go=true` y `node=true` en `$GITHUB_OUTPUT`.
  - Cobertura: el nuevo paso de setup-node más `npm ci`, `npm test --if-present` y `npm audit --audit-level=high` cubre Node. `requirements.txt` ya no se omite. Los pasos Go, Python y Node de `test` y `vuln` quedan condicionados a los outputs `lang`.
  - Un servicio Go bajo workspace sin `go.mod` propio ahora cae en rojo, no en verde en falso. Es una decisión documentada y conservadora, aceptable. Ver tarea candidata.
- **F-03 (AMARILLO): resuelto.** Python ya no asume el extra `[test]`: instala `requirements.txt`, `requirements-dev.txt` y `pip install -e .` si existen, más `pytest`. Los `[ -f x ] && pip install …` fallan correctamente si el `pip install` falla (es el último comando de la lista `&&`).
- **F-04: aceptado como tarea candidata, según tu arbitraje.** El único cambio permitido está hecho: el aviso del guard ahora es `::warning:: <dir> no existe…` (probado con `/nonexistent`: `rc=0` más el aviso). No hay otro cambio de comportamiento en `check-no-latest.sh`: el diff son 1 línea.
- **F-05 (AMARILLO): corregido.** La imagen sale de `${SERVICE//\//-}`, así que `services-planner` y `agents-planner` ya no colisionan. actionlint y shellcheck lo aceptan.
- **F-06 (AMARILLO): corregido en parte.**
  - Los casos de CA-6 de la bitácora ya están etiquetados (caso 1 a 4 más el extra de puerto sin tag), y se pegan CA-1 a CA-7, shellcheck y los fixtures de `detect-lang`.
  - Sigue faltando la salida de CA-8 en la bitácora: dice que "va en el informe de vuelta". Pegarla en el mismo commit que la altera es circular, así que no lo bloqueo. Mi ejecución de CA-8 da `0` y `0`.

## Verdes en falso: juicio
- Lista vacía: `discover` da `[]` y `build`, `test`, `vuln`, `sbom` y `publish` se saltan por `if`. `no-latest` corre aparte. No hay rojo.
- Servicio sin marcador de lenguaje: ahora falla en `test` y en `vuln`.
- Servicio con marcador: Go sin archivos `_test.go` pasa con "no test files", comportamiento estándar. Python sin pruebas falla (pytest devuelve 5). Node con `npm test --if-present` y sin script `test` pasa en verde sin ejecutar nada. Ver F-07.
- No hay `continue-on-error` ni `|| true` en `ci.yml`. El único `|| true` está en el guard, sobre el `grep` sin coincidencias, y es legítimo.

## Hallazgos nuevos o en pie
### F-07 · AMARILLO · ci.yml job `test`, paso "Pruebas Node" · `npm test --if-present` es un verde en falso residual
Un `package.json` sin script `test` pasa sin ejecutar nada. Mi juicio es que es aceptable y no bloquea:
- Es explícito y está documentado en la bitácora.
- No existe ningún agente Node todavía.
- El hueco es mucho más estrecho que el de F-02: hay marcador reconocido y es una decisión de nivel de servicio.
- Python y `npm ci` ya fallan cuando falta algo esencial.

Recomendación: usar `npm test` a secas, que falla con "missing script" y es simétrico con pytest, o pedir la convención en U-posterior.

### F-06r · AMARILLO · bitácora · falta la salida de CA-8
Ver arriba. No bloquea.

## Desborde de alcance
No hay desborde nuevo. `git diff b2f7795..tarea/U5-T03 --stat` muestra solo `ci.yml`, `check-no-latest.sh` (1 línea), `detect-lang.sh` (nuevo, que resuelve F-02 por arbitraje) y la bitácora. No hay `contracts/`, `deploy/`, servicios, Dockerfiles, cosign ni README. La corrida real en GitHub queda para el humano, como pide la tarea.

## Tareas candidatas (defectos reales fuera de alcance)
- F-04: cuando U5-T04 deje `deploy/flux/prod` con manifiestos, endurecer `no-latest` (`--strict`: falla si el directorio no existe o no tiene `*.yaml`).
- El guard no cubre `kustomization.yaml` con `images: - newTag: latest`. Valorar si U5-T04 usa Kustomize.
- Servicios Go "bajo workspace sin `go.mod` propio" (`unit-of-work.md`): hoy `detect-lang.sh` los marca en rojo. Si U5 adopta `go.work`, ampliar el marcador (por ejemplo `go.work` en el directorio padre).
- Firmado con cosign (ya propuesto por la tarea).

## Rutas de transcripciones largas
Ninguna. Todo lo corrí en la sesión.

VEREDICTO: VERDE
AMARILLO|.github/workflows/ci.yml (Pruebas Node)|`npm test --if-present` pasa en verde sin script de prueba; mejor `npm test` (no bloquea)
INFORME: revisiones/U5-T03/ronda-2.md
