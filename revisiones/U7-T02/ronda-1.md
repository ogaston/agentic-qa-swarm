# Ronda 1 — U7-T02

VEREDICTO: VERDE

Revisé el sha f92298e (base 6dd7f2a) y corrí yo mismo los 6 CA contra un clúster `aqs` creado por mí. CA-4 lo evalué con el comando de la errata (PR #82, blob 80cf5d0).

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Scripts válidos | `bash -n scripts/kind/*.sh` y shellcheck v0.10.0 con `wc -l` | `0`, pasa |
| 2 | Construcción de las 9 | Borré antes las imágenes `ghcr.io/ogaston/...` de podman (había 0). Luego `build-images.sh` y `podman images \| grep -c` | `rc=0` y `9`, pasa. Tardó 22 s y la salida tiene 9 «construyendo». |
| 3 | Carga leída en el nodo | Antes de cargar, `crictl images \| grep -c` dio `0`. Luego `load-images.sh` y `crictl images \| grep -c` | `rc=0` y `9`, pasa |
| 4 | Referencias de `image:` vs nodo (errata) | `comm -23 <(grep ... image: ...) <(crictl images -o json ...) \| wc -l` | `0`, pasa |
| 5 | Subconjunto y guarda | `AQS_IMAGES=go-identity build-images.sh \| grep -c`, y `load-images.sh` con el contexto `aqs-guard-test` | `0` y `rc=3`, pasa. El mensaje fue «contexto aqs-guard-test no es kind-aqs». |
| 6 | Alcance | `git status --short \| wc -l` y `git diff --name-only $b \| grep -v ... \| wc -l` | `0` y `0`, pasa |

Sobre CA-4: no es una comparación vacía. Los `image:` propios de los manifiestos son 7 (go-governance, go-identity, go-intake, go-reset, go-run-controller, go-warm-manager, ui-api). Las otras `image:` del mismo grep son de terceros: nginx, postgres y redis.

Pruebas extra que hice:
- `load-images.sh` con una imagen ausente en podman (`go-reset`, tras `podman rmi`) sale con `rc=1` y el mensaje «falta ... ejecuta build-images.sh».
- `AQS_IMAGES=nope` sale con `rc=1` y el nombre rechazado.
- No quedaron `tmp.*` en `/tmp` tras `load-images.sh`.

Estado al terminar:
- Destruí `aqs` con `bash scripts/kind/kind-down.sh`.
- Restauré el contexto con `kubectl config use-context kind-ckad`; el contexto actual es `kind-ckad`.
- Quedan los clústeres `aqs-poc` y `ckad`, intactos.
- El worktree está limpio (`git status --short` da 0 líneas).
- Esta revisión no tocó `aqs-poc` ni `ckad`.

## Hallazgos
### F-01 · AMARILLO · `scripts/kind/lib.sh` (`platform_images`, bucle final) · la validación de nombres de `AQS_IMAGES` acepta subcadenas
El bucle final usa `case "$out" in *"$n "*)` y busca el nombre como subcadena de todo `$out`.
- `AQS_IMAGES="go-identity identity"` pasa sin error, con `rc=0`. «identity» está contenido en «go-identity ».
- Debería rechazar «identity» como nombre desconocido, igual que rechaza `nope`.
- Es un caso de borde secundario y solo afecta al filtro opcional. No rompe ningún CA. Se puede arreglar comparando el nombre completo, por ejemplo `*$'\n'"$n "*` sobre `$'\n'$out`.

### F-02 · AMARILLO · `scripts/kind/lib.sh` y `build-images.sh` · el literal 9 está en el código
`platform_images` compara con un 9 fijo. La tarea lo pide expresamente («si la lista no da 9, el script falla»), así que no lo subo a NARANJA. Si la lista de la plataforma cambia, hay que editar el script.

### Bitácora (nota, no bloquea)
- La bitácora del codificador declara CA-4 en rojo con el comando original. La errata se decidió después, y la bitácora anota el bloqueo y lo escala. Con la errata aplicada, CA-4 pasa.
- El rojo de `load-images.sh` quedó omitido y declarado en la bitácora. El CA-5 cubre la guarda con salida real, que yo reproduje.

## Tareas candidatas (fuera de alcance)
- C-100 (ya decidida): imágenes de ejecución `target-app`, `rehearsal` y `runner` (`control-plane.yaml:184-190`) sin Dockerfile.
- `ci.yml` publica con el nombre `${SERVICE//\//-}` (p. ej. `services-ui-api`), mientras los manifiestos y U7-T02 usan el basename (`ui-api`). Hay que decidir la referencia canónica. Lo anotó el codificador.
- `kind-down.sh` deja `current-context` vacío al destruir `aqs` estando en `kind-aqs`. Ya estaba registrado en U7-T01. Yo restauré `kind-ckad` a mano.

## Rutas de transcripciones largas
- Salida completa de mi build y de mi carga, en el scratchpad de la sesión:
  - `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/0ecca73d-838d-45a3-8b36-868c95cd55c0/scratchpad/b.log`
  - `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/0ecca73d-838d-45a3-8b36-868c95cd55c0/scratchpad/l.log`

VEREDICTO: VERDE
AMARILLO|scripts/kind/lib.sh platform_images|La validación de nombres de AQS_IMAGES acepta subcadenas (p. ej. "identity" pasa junto a "go-identity")
INFORME: revisiones/U7-T02/ronda-1.md
