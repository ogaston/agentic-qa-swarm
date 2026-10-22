# Ronda 2 — U9-T05

VEREDICTO: VERDE

Worktree /home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U9-T05, sha 1b21564. No edité nada. `git status --short` da 0 líneas al terminar. No corrí `--rotate-demo`, no escribí Secrets y no toqué `~/.local/state/aqs-kind` ni `${XDG_RUNTIME_DIR}/aqs-kind`. Mis copias de trabajo están en el scratchpad (`.../scratchpad/rev2/`).

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1a | Sin rutas tmpfs | `grep -rn 'XDG_RUNTIME_DIR:-/tmp}/aqs-kind\b' scripts/kind/ \| grep -v -E 'aqs-kind\.lock\|lib\.sh' \| wc -l` | `0`, pasa |
| 1b | shellcheck, medido según el arbitraje: cero warnings nuevos respecto de origin/main | `podman run --rm -i docker.io/koalaman/shellcheck:stable -s bash -f gcc - < fichero` sobre cada `scripts/kind/*.sh`, en HEAD y con `git show origin/main:<fichero>`. Normalicé quitando fichero y línea:columna y comparé con `comm -13` por fichero | Hay 96 warnings en HEAD y 96 en la base. `comm -13` no da ninguna línea nueva en ningún fichero. Los 3 SC2034 de la ronda 1 desaparecieron. Pasa con el criterio arbitrado; rc≠0 viene de la base |
| 2 | `u9-kind-state.sh` | `bash scripts/test/u9-kind-state.sh 2>&1 \| grep -E '^(OK\|FALLA) '; echo rc=${PIPESTATUS[0]}` | rc=0 y 9 `OK`. Están los 6 exigidos más `imagen-ausente-export-falla`, `imagen-ausente-export-ok` y `directorio-abierto-falla`. Tarda 0,35 s, y es normal: kubectl y podman son falsos y no hay clúster |
| 3 | CA-3 en kind | Compruebo con `git diff af398dd HEAD --quiet -- scripts/kind/secrets.sh scripts/kind/lib.sh` que ambos son idénticos a la ronda 1. Corrí `flock ${XDG_RUNTIME_DIR:-/tmp}/aqs-kind.lock bash scripts/kind/kind-smoke.sh` (solo lectura) y leí modos y el deployment | smoke rc=0, `FALLA`=0, 25 `OK`. Directorio 700; `demo.txt`, `admin.txt` y `admin-mfa-secret.txt` 600; `deploy/go-identity` 1/1. La rotación en sí consta solo en `revisiones/U9-T05/ca3-evidencia.md`, que es la evidencia vigente aceptada |
| 4 | Alcance | `git status --short \| wc -l` y `git diff --name-only $(git merge-base HEAD origin/main) \| grep -v <lista> \| wc -l` | `0` y `0`, pasa |

## Respuestas a los hallazgos de la ronda 1
- **F-01 (docs con la ruta tmpfs vieja): resuelto.** `docs/operaciones/kind-local.md` líneas 28, 148 y 149 ahora apuntan a `AQS_KIND_STATE_DIR` y dan `~/.local/state/aqs-kind` como valor por defecto. `grep -rn 'XDG_RUNTIME_DIR.*aqs-kind' docs/ scripts/` solo devuelve los locks, la migración en `lib.sh:222`, la sección de migración de la doc (línea 263) y la prueba de migración. Un nivel más abajo, el fichero `webhook-github-cloudflared.md` no tiene referencias.
- **F-02 (SC2034 nuevo, shellcheck no verificado): resuelto.** El diff `af398dd..HEAD` elimina `STATE_DIR` en `kind-smoke.sh`, `dashboard-smoke.sh` y `github-webhook-check.sh`. Mi propio shellcheck confirma cero warnings nuevos. El codificador ya corrió shellcheck con el mismo método y obtuvo 96/96.
- **F-03 (sin prueba del camino de la imagen): resuelto.** El fake de podman tenía un `exit 0` global que anulaba `FAKE_IMAGE_MISSING`; ahora cada rama sale con su código y `cp`, `exec` y `load` están modelados. Hice mutaciones sobre una copia en el scratchpad:
  - Si `ensure_go_identity_image` deja de fallar (`return 0` en lugar de `fail`), `imagen-ausente-export-falla` pasa a `FALLA` (rc=0, 2 mutaciones de Secret).
  - Si se quita `podman load` de la cadena de export, `imagen-ausente-export-ok` pasa a `FALLA` (rc=1).
  Las dos pruebas nuevas detectan regresiones reales, no son decorativas.

## Hallazgos
Ningún ROJO ni NARANJA en pie.

### AMARILLOS (no bloquean, heredados de la ronda 1 y no atendidos)
- `kind_state_check` solo se llama desde `secrets.sh`, después de `kind_state_init`, que corrige el directorio a 700 en silencio. Por eso el caso `directorio-abierto-falla` ejercita `lib.sh` directamente y no el camino de `secrets.sh`. kind-smoke, dashboard-smoke y github-webhook-check leen `demo.txt` sin comprobar permisos.
- Las pruebas no verifican el orden «apply antes de publicar los ficheros», ni que un `apply` fallido no deje un `demo.txt` nuevo, ni `rollout restart/status`.
- `publish_go_identity_aux` hace `install` después del `apply`. Si el `install` falla, el Secret queda rotado sin contraseña guardada (poco probable).

## Tareas candidatas (defectos fuera de alcance)
- `ctr images export` deja `/tmp/go-identity.tar` dentro del nodo kind; limpiarlo.
- `kind-e2e.sh` asigna `STATE_DIR` dos veces (líneas 87 y 125) y tiene 7 SC1010 en la base.
- Normalizar shellcheck con un `.shellcheckrc` o un umbral de severidad, para poder usar `rc=0` como criterio.

## Rutas de transcripciones largas
- Warnings de shellcheck por fichero: `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/26ce36b2-c0da-4f2c-9423-bab8b003fb48/scratchpad/rev2/{base,head}/*.w`
- Salida del smoke: `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/26ce36b2-c0da-4f2c-9423-bab8b003fb48/scratchpad/rev2/smoke.txt`

VEREDICTO: VERDE
INFORME: revisiones/U9-T05/ronda-2.md
