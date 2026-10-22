# Ronda 1 — U9-T05

VEREDICTO: NO-VERDE

Worktree /home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U9-T05, sha af398dd, hash de la tarea c04872c… (coincide). El worktree quedó sin cambios míos: `git status` solo muestra `?? revisiones/U9-T05/` (la evidencia CA-3 del orquestador). No corrí `--rotate-demo` ni toqué Secrets o ficheros de estado. Solo corrí el smoke bajo flock (lectura).

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1a | Sin rutas tmpfs | `grep -rn 'XDG_RUNTIME_DIR:-/tmp}/aqs-kind\b' scripts/kind/ \| grep -v -E 'aqs-kind\.lock\|lib\.sh' \| wc -l` | `0` pasa |
| 1b | `shellcheck scripts/kind/*.sh` rc=0 | No hay shellcheck en el host. Lo corrí con `podman run -i docker.io/koalaman/shellcheck:stable -s bash -f gcc -` pasando cada fichero por stdin, en HEAD y en origin/main | **No verificable como rc=0** (ver F-02). La base ya da rc=1 con este shellcheck; el diff añade 3 warnings nuevos |
| 2 | `u9-kind-state.sh` | `bash scripts/test/u9-kind-state.sh \| grep -E '^(OK\|FALLA) '` | rc=0, los 6 `OK` exigidos. Tarda 0,22 s, y es normal: kubectl y podman son falsos y no hay clúster |
| 3 | CA-3 en kind | No volví a rotar. Con `flock ... kind-smoke.sh` corrí el smoke fresco; leí modos y el Secret | smoke rc=0, `FALLA`=0, 25 `OK`; dir 700 y `demo.txt`/`admin.txt`/`admin-mfa-secret.txt` 600; Secret `go-identity-users` y `deploy/go-identity` 1/1. La rotación en sí consta solo en `revisiones/U9-T05/ca3-evidencia.md` (salida pegada por el humano: rc=0, migración avisada, rollout OK) |
| 4 | Alcance | `git diff --name-only $b \| grep -v <lista> \| wc -l` | `0` pasa. `git status --short \| wc -l` da 1, solo por `revisiones/U9-T05/` sin commitear (orquestador, no un defecto del codificador) |

## Hallazgos
### F-01 · NARANJA · docs/operaciones/kind-local.md:28, 148, 149 · La documentación sigue apuntando a la ruta tmpfs vieja
La tarea exige actualizar la documentación, y el diff solo añadió secciones al final. Quedan tres referencias a la ruta vieja, que contradicen la sección nueva «Insumos y credenciales persistentes»:
- línea 28: `${XDG_RUNTIME_DIR:-/tmp}/aqs-kind/demo.txt`
- líneas 148-149: `${XDG_RUNTIME_DIR}/aqs-kind/deepseek.key` y `.../litellm.env`, descritos como «insumos del humano».

Un humano que siga la sección U8 pondrá `litellm.env` y `deepseek.key` donde `secrets.sh` ya no los busca (la migración solo mueve lo que falta en el nuevo). Es una clase: `grep -n 'XDG_RUNTIME_DIR.*aqs-kind' docs/ scripts/` debe quedar solo con locks y la sección de migración.

### F-02 · NARANJA · CA-1 (shellcheck) · El diff añade 3 variables muertas y el codificador no pudo correr shellcheck
Con shellcheck vía stdin, el diff introduce tres `SC2034: STATE_DIR appears unused`, que la base no tiene:
- `scripts/kind/kind-smoke.sh:22`
- `scripts/kind/dashboard-smoke.sh:18`
- `scripts/kind/github-webhook-check.sh:26`

Se reasignó `STATE_DIR="$AQS_KIND_STATE_DIR"` pero el resto del código ahora usa `$AQS_KIND_STATE_DIR` directamente. Hay que eliminar la asignación o usar `$STATE_DIR`.

La bitácora dice «NO VERIFICADO» para shellcheck. El rc=0 de CA-1 tampoco se cumple en la base con esta versión de shellcheck (SC2329, SC1010, etc. ya existen), así que el orquestador debe decidir cómo se mide CA-1. Mínimo exigible: sin warnings nuevos respecto de origin/main.

Nota: `kind-e2e.sh` asigna `STATE_DIR` dos veces (líneas 87 y 125). Ya estaba así y es inofensivo.

### F-03 · NARANJA · scripts/test/u9-kind-state.sh · Falta cobertura del camino de la imagen go-identity
La tarea exige que, si la imagen no está en podman, `--rotate-demo` la exporte del nodo kind o falle con un mensaje accionable. El fake de podman ya soporta `FAKE_IMAGE_MISSING`, pero ningún caso lo usa (`grep FAKE_IMAGE_MISSING` solo da la definición y el export). Así que `ensure_go_identity_image` (exportar con `ctr`, `podman cp`, `podman load`, el mensaje de fallo) no se ejercita ni en la prueba ni en CA-3. En la corrida real la imagen estaba en podman local: la salida pegada no tiene «exportada del nodo kind».

Faltan dos casos con podman falso:
- imagen ausente y export fallido: rc≠0, mensaje con `build-images.sh`/`load-images.sh`, cero mutaciones de Secret.
- imagen ausente y export OK: sigue adelante.

Comprobé en solo lectura que el nodo tiene `ghcr.io/ogaston/agentic-qa-swarm/go-identity:0.0.0`, así que el nombre coincide con el que exporta el script. Que `podman load` del tar de `ctr` etiquete igual no está demostrado.

## Amarillos (no bloquean)
- `kind_state_check` solo se llama desde `secrets.sh`, y ahí viene después de `kind_state_init`, que hace `chmod 700` y corrige el directorio en silencio. La rama «directorio ≠ 700 falla» no se alcanza desde `secrets.sh`, y la prueba solo cubre el fichero 644. kind-smoke, kind-e2e y los demás leen `demo.txt` sin comprobar permisos.
- Las pruebas no verifican el orden «apply del Secret antes de publicar los ficheros», ni que un `apply` fallido no deje `demo.txt` nuevo. Tampoco verifican `rollout restart/status`.
- `publish_go_identity_aux` hace `install` tras el `apply`. Si el `install` fallara, el Secret quedaría rotado sin contraseña guardada (poco probable).

## Tareas candidatas (fuera de alcance)
- `ctr images export` deja `/tmp/go-identity.tar` dentro del nodo kind; limpiarlo.
- `kind-e2e.sh` tiene `STATE_DIR` duplicado y 8 warnings SC1010 en la base.
- Los warnings de shellcheck de la base impiden usar `rc=0` como criterio: normalizar con un `.shellcheckrc` o un umbral de severidad.

## Rutas de transcripciones largas
- Ninguna. La evidencia de CA-3 está en /home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U9-T05/revisiones/U9-T05/ca3-evidencia.md

VEREDICTO: NO-VERDE
NARANJA|docs/operaciones/kind-local.md:28,148,149|La documentación sigue citando la ruta tmpfs vieja para litellm.env, deepseek.key y demo.txt
NARANJA|CA-1 shellcheck (kind-smoke.sh:22, dashboard-smoke.sh:18, github-webhook-check.sh:26)|SC2034 nuevo: STATE_DIR muerto; shellcheck no verificado por el codificador
NARANJA|scripts/test/u9-kind-state.sh|Sin prueba de ensure_go_identity_image (export desde nodo kind / fallo accionable); FAKE_IMAGE_MISSING sin usar
INFORME: revisiones/U9-T05/ronda-1.md
