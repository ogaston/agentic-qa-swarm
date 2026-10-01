# Ronda 1 — U5-T03

VEREDICTO: NO-VERDE

Worktree /home/omarjayg/Javeriana/topicos-especiales/wt-U5-T03, HEAD b2f7795. El hash de la tarea coincide (79025a9…). Dejé el worktree limpio: `git status --short` da 0 líneas antes y después de mis pruebas.

## Criterios de aceptación, verificados por mí (ejecución fresca, comandos literales de la tarea)
| # | Criterio | Resultado |
|---|---|---|
| 1 | workflow y guard existen | `OK` |
| 2 | actionlint 1.7.7 | solo `rc=0` |
| 3 | seis jobs | `6` |
| 4 | SHA de 40 caracteres y permisos | `0`, `1`, `1` |
| 5 | publish solo en tags y sin latest | `1`, `0`, `N/A` (la enmienda se cumple, no se usa metadata-action) |
| 6 | guard | `rc=0` (deploy/flux/prod no existe), `rc=1` nombrando a.yaml (`:latest`), `rc=1` nombrando a.yaml (sin tag), `rc=0` (tag más digest) |
| 7 | list-services | `[]` con `rc=0`; con fixtures `["services/go-a","agents/planner"]` |
| 8 | árbol limpio y sin `.gitkeep` | `0` y `0` |
| Plan | shellcheck v0.10.0 sobre `scripts/ci/*.sh` | sin salida, `rc=0` |

Todos pasan literalmente. El rojo inicial de CA-1 (`FALTA`) está en la bitácora, y la salida de la bitácora coincide con la mía.

Alcance "Fuera": `git diff --name-status 93f0546..tarea/U5-T03` muestra solo D `.github/workflows/.gitkeep` (exigido por CA-8), A `ci.yml`, A `scripts/ci/check-no-latest.sh`, A `scripts/ci/list-services.sh` y A `bitacoras/U5-T03.md`. No hay `contracts/`, `deploy/`, servicios, Dockerfiles, cosign ni README. Sin desborde. `list-services.sh` no figura en el alcance, pero CA-7 lo exige.

## Los cuatro puntos que pediste juzgar
1. **Fail-open del guard con directorio inexistente.** Es razonable hoy y CA-6 lo exige (`rc=0` sobre `deploy/flux/prod`, que aún no existe). Queda como riesgo AMARILLO, ver F-04: cuando U5-T04 cree el árbol, el job `no-latest` seguirá en verde aunque alguien renombre o mueva `deploy/flux/prod`. Un directorio que existe pero no tiene `*.yaml` también pasa en silencio. Mi veredicto: aceptable en esta tarea, pero hay que cerrarlo después (tarea candidata).
2. **SHA de las acciones.** Verifiqué las siete con `git ls-remote` contra el tag declarado en el comentario. Seis coinciden con un commit (tags ligeros). **Una no es un commit**, ver F-01.
3. **publish.** Cumple. `if: startsWith(github.ref, 'refs/tags/v') && services != '[]'`. Etiquetas `:${github.ref_name}` y `:${github.sha}`, sin `latest`. `permissions: contents: read, packages: write` solo a nivel del job, y a nivel de workflow solo `contents: read`. Depende de build, test, vuln y sbom. Tiene un matiz menor, ver F-05.
4. **Verde y rojo en falso.**
   - Lista vacía: `discover` produce `[]` y los cuatro jobs de matriz se saltan por `if`. `publish` también se salta, y `no-latest` corre aparte. No hay rojo. Si `list-services.sh` fallara, `services=` quedaría vacío y `fromJSON('')` pondría el job en rojo, no en verde.
   - No hay `continue-on-error` ni `|| true` en `ci.yml`. El único `|| true` está en el guard, sobre el `grep` sin coincidencias bajo `pipefail`, y es legítimo.
   - Sí hay un verde en falso con servicios reales, ver F-02.

## Hallazgos
### F-01 · NARANJA · ci.yml (paso `anchore/sbom-action`) · SHA fijado es el del objeto tag anotado, no el del commit
`git ls-remote` devuelve `006b7ce… refs/tags/v0.24.2` y `3ad7283… refs/tags/v0.24.2^{}`. El workflow fija `006b7ce8314066bdf1765b4500370d40fa6917a3`, que es el objeto de la etiqueta anotada. Evidencia:
- `gh api repos/anchore/sbom-action/git/commits/006b7ce…` devuelve 404, porque no es un commit.
- `gh api repos/anchore/sbom-action/git/tags/006b7ce…` devuelve `object.type=commit` con `object.sha=3ad7283483fc7af8ff2b4ea19663c2d5ca935e26`.
- El endpoint tarball sí resuelve ambos SHA, así que el runner probablemente funcione. Pero no es un SHA de commit.

Las herramientas de pinning y la política "exigir SHA completo de commit" lo rechazan o lo marcan, y el codificador afirmó que los SHA salen de los tags sin distinguir el caso anotado. CA-4 no lo detecta porque solo comprueba 40 hex. Arreglo: usar `3ad7283483fc7af8ff2b4ea19663c2d5ca935e26 # v0.24.2`. Las otras seis son tags ligeros (sin línea `^{}`) y son commits válidos.

### F-02 · NARANJA · ci.yml jobs `test` y `vuln` · verde en falso para servicios sin `go.mod` ni `pyproject.toml`
Los pasos de prueba y de escaneo solo corren si `hashFiles('<svc>/go.mod')` o `hashFiles('<svc>/pyproject.toml')` no están vacíos. `unit-of-work.md:49-50` y `:61` define los agentes con `requirements.txt|package.json` (Python o TS) y los servicios Go con "`go.mod` propio o workspace".
- Un agente con `requirements.txt` o `package.json`, o un servicio Go bajo workspace sin `go.mod` propio, entra en la matriz, pasa `build` y deja `test` y `vuln` en verde sin ejecutar nada ni avisar.
- La nota de la tarea pide "el escáner equivalente del lenguaje de cada agente", y TS no está cubierto.
- Es un verde en falso, precisamente lo que el punto 4 pide descartar.

Mínimo exigible: un paso final que falle (o al menos avise con `::warning::`) cuando el servicio no tiene ningún marcador de lenguaje reconocido. Recomendable: cubrir `requirements.txt` y `package.json`.

### F-03 · AMARILLO · ci.yml `test`/`vuln` · el flujo Python asume `pyproject.toml` con extra `[test]`
`pip install -e '.[test]'` falla si el extra no existe. Con F-02 resuelto conviene fijar la convención del proyecto en las notas.

### F-04 · AMARILLO · check-no-latest.sh:6-9 · fail-open silencioso
El aviso va a stderr sin anotación de Actions, y no hay forma de volver el guard estricto cuando U5-T04 aterrice. Tarea candidata abajo.

### F-05 · AMARILLO · ci.yml (paso `Calcular imagen`) · colisión de nombre de imagen
La imagen sale de `basename "$SERVICE"`, así que `services/planner` y `agents/planner` publicarían en la misma imagen `ghcr.io/<owner>/planner`.

### F-06 · AMARILLO · bitácora · falta CA-8 y la etiqueta de los casos de CA-6
La bitácora no pega la salida de CA-8. En CA-6 no etiqueta el caso con digest, que sale como un `rc=0` suelto. La ronda 1 incluye una prueba extra (puerto sin tag) que no consta en la ronda 2. La tarea pide el comando literal y su salida por criterio.

## Tareas candidatas (fuera de alcance)
- Cuando U5-T04 deje `deploy/flux/prod` en main, endurecer `no-latest`: que falle si el directorio no existe o no tiene ningún `*.yaml` (por ejemplo con un flag `--strict` en el workflow).
- El guard no cubre `kustomization.yaml` con `images: - newTag: latest`, que es el formato típico de Kustomize. Valorar si U5-T04 lo usa.
- Firmado con cosign (ya propuesto por la tarea).

## Rutas de transcripciones largas
Ninguna. Todo lo corrí en la sesión.

VEREDICTO: NO-VERDE
NARANJA|.github/workflows/ci.yml (anchore/sbom-action)|SHA fijado es el del objeto tag anotado, no del commit (correcto: 3ad7283483fc7af8ff2b4ea19663c2d5ca935e26)
NARANJA|.github/workflows/ci.yml (test, vuln)|Verde en falso: servicios sin go.mod ni pyproject.toml (requirements.txt, package.json, workspace Go) omiten pruebas y escaneo sin avisar
INFORME: revisiones/U5-T03/ronda-1.md
