# Ronda 3 — U8-T01

VEREDICTO: VERDE

F-05 está resuelto y no queda ningún ROJO ni NARANJA. Los manifiestos y scripts de NATS no cambiaron desde la ronda 2, así que no repetí CA-3 y CA-4 con clúster, como pediste. Tengo solo lectura, así que no escribí el archivo del informe. El orquestador debe guardar este texto en `revisiones/U8-T01/ronda-3.md`.

Worktree `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U8-T01`, HEAD 3e401dd, limpio (`git status --short | wc -l` da 0). Hice `git fetch` antes de comprobar; origin/main está en 78a691b y es también la base del merge.

## Comprobaciones pedidas
| # | Qué | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Sin conflictos con origin/main | `git merge-tree --write-tree HEAD origin/main` | pasa: rc=0 |
| 2 | `kind-local.md` conserva las dos secciones | `grep -n '^## ' docs/operaciones/kind-local.md`; `grep -c '<<<<<<<\|>>>>>>>'` | pasa: están «NATS JetStream (U8-T01)» (línea 63) y «Dashboard (U7-T05)» (línea 72), y hay 0 marcas de conflicto. `git diff origin/main -- docs/operaciones/kind-local.md` solo añade la sección NATS, sin tocar la del dashboard |
| 3 | El merge no trae cambios propios fuera de lo revisado | ver abajo | pasa |
| 4 | CA-1, CA-2 y CA-5 contra el estado final | ver abajo | pasan |

**Detalle de (3)**
- El merge tiene un solo padre 3be2cac (la rama del codificador) más main. La diferencia `53f752f..HEAD` en `deploy/`, `scripts/` y `policy/` es lo que entró desde main (U7-T05, U8-T06).
- Comparé cada archivo de código y manifiesto de la tarea contra su versión revisada en 53f752f. Son idénticos todos los de NATS: `nats/*` (README, job-init, kustomization, nats-init.sh, networkpolicy, service, statefulset), `base/kustomization.yaml`, `security/networkpolicies-system.yaml`, `scripts/kind/deploy.sh` y `scripts/kind/nats-netpol-check.sh`. No hay cambios en `policy/`, `deploy/flux/{dev,prod,kind}` ni en ningún script propio.
- La única excepción es `deploy/flux/base/control-plane.yaml`, que difiere de 53f752f solo porque main trajo las imágenes `rehearsal` y `runner` de U8-T06. Su diferencia contra origin/main son exactamente las 3 líneas `aqs.io/nats-client: "true"` ya revisadas. La combinación es correcta y los selectors no cambian.

**Detalle de (4)**
- **CA-1:** `bash scripts/ci/policies.sh` da rc=0 con `0` líneas `FALLA` y 16 `OK`; `git diff --name-only <base> -- policy | wc -l` da `0`. Son los mismos 16 OK de la ronda 2, ahora con los manifiestos de U8-T06 de main incluidos.
- **CA-2:** el `for` de la tarea da `1`, `1`, `1` en dev, prod y kind.
- **CA-5:** `git status --short | wc -l` da `0`. La lista de archivos fuera de la cerca da `0`. El diff contra la base son 17 archivos, todos dentro de la lista permitida.
- **CA-3 y CA-4:** siguen como los verifiqué en la ronda 2. Los archivos de NATS no cambiaron, así que mi evidencia sigue valiendo. El sondeo literal de CA-3 sigue sin cumplirse por la política, y la variante etiquetada pasa; es el arbitraje ya cerrado.

## Hallazgos
Ninguno en pie. F-01 a F-04 se corrigieron en la ronda 2 y F-05 en esta.

## Tareas candidatas (defectos reales fuera de alcance, repetidas)
- Actualizar `docs/seguridad/aislamiento.md`: `allow-same-namespace` ahora excluye NATS y falta `nats-clients-only`.
- Corregir el CA-3 de la tarea: el pod de prueba necesita `--labels=aqs.io/nats-client=true`, y `kubectl run --rm -i` pierde la salida de forma intermitente.
- NKeys y TLS en NATS. La etiqueta de cliente es autoservicio.
- Revisar en U8-T02 el reinicio de `nats-init` (3 reinicios) mientras NATS arranca.

## Rutas de transcripciones largas
- `/tmp/claude-1000/pol3.txt` (policies.sh sobre el estado final)
- `/tmp/claude-1000/mt3.txt` (merge-tree sin conflictos)
- `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/1c7b66eb-a3e9-4dad-81e8-f56a81f889ed/scratchpad/r2u8t01-run.out` (corrida de clúster de la ronda 2, vigente)

VEREDICTO: VERDE
INFORME: revisiones/U8-T01/ronda-3.md
