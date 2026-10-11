# Ronda 2 — U8-T01

VEREDICTO: NO-VERDE

Solo queda un hallazgo, NARANJA (F-05). Los cinco criterios pasan y F-01 a F-04 de la ronda 1 están corregidos. El hallazgo nuevo es un conflicto de fusión con origin/main, que introdujo el commit de esta ronda.

Tengo solo lectura, así que no escribí el archivo del informe. El orquestador debe guardar este texto en `revisiones/U8-T01/ronda-2.md`.

Worktree `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U8-T01`, HEAD 53f752f. Quedó limpio (`git status --short | wc -l` da 0).

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Políticas en verde, sin tocar `policy/` | `bash scripts/ci/policies.sh` (rc=0), `grep -c '^FALLA'`, `git diff --name-only <base> -- policy \| wc -l` | pasa: `0` y `0` (16 líneas OK) |
| 2 | NATS con imagen fijada en dev, prod y kind | el `for` de la tarea | pasa: `1`, `1`, `1` |
| 3 | Desplegado y stream leído de vuelta | `deploy.sh \| grep nats`; el literal de la tarea; el comando literal del README | `deploy.sh` pasa: `OK .../statefulset.apps/nats` y `OK .../job/nats-init`, sin `FALLA`. El literal falla (ver abajo) y acepto la variante, ya arbitrada. El README, copiado tal cual, da `aqs.events.>`, `120000000000`, `604800000000000`, `limits`, con `jq` rc=0 |
| 4 | Aislamiento | `bash scripts/kind/nats-netpol-check.sh; echo rc=$?` | pasa: `OK nats-cliente-permitido`, `OK nats-otro-pod-bloqueado`, `rc=0`. No quedaron pods residuales |
| 5 | Alcance | los dos comandos de CA-5 | pasa: `0` y `0` |

- **Literal de CA-3:** falla con `i/o timeout` porque el pod no lleva la etiqueta de cliente. Es el caso que la tarea prevé.
- **Clúster:** todo corrió en un único bloque `flock` (kind-up, build-images, load-images, deploy, pruebas, kind-down). El contexto estaba vacío al empezar y quedó vacío al terminar. `kind get clusters` ya no lista `aqs`; los clústeres `aqs-poc` y `ckad` no son míos y no los toqué.
- **Persistencia:** borré `nats-0`. El StatefulSet volvió a estar listo y `data-nats-0` sigue `Bound` (2Gi).
- **Idempotencia:** borré el Job `nats-init` y reapliqué con `apply -k`. Completó y registró `stream AQS_EVENTS ya existe`.
- **Service:** los Endpoints de `nats` solo exponen el 4222.

## Respuestas de la ronda 1, verificadas por identificador
- **F-01 (guarda antes del trap): corregido.** `require_kind_context` va ahora en la línea 25, antes de `cleanup` y `trap`. Lo probé con un `kubectl` falso con contexto `other-cluster`: salió con rc=3 y la única llamada registrada fue `kubectl config current-context`. En la ronda 1 se llamaba además a `kubectl delete pod …`. La bitácora tiene rojo y verde de la misma prueba.
- **F-02 (README): corregido.** Documenta pod etiquetado, `wait` a `Succeeded`, `kubectl logs | jq` y borrado del pod. Copié el bloque literal del README y funciona, sin errores de `jq`.
- **F-03 (evidencia de la bitácora): corregido.**
  - Está el rojo de `stream info` antes del despliegue. Es un `NotFound` del namespace, aceptable porque aún no existe nada.
  - Está la salida de CA-5, que verifiqué yo mismo (0 y 0).
  - Está la nota de los 3 reinicios de `nats-init`, que es benigna.
- **F-04 (documentación de kind): corregido.** `docs/operaciones/kind-local.md` tiene una sección «NATS JetStream (U8-T01)». Es correcta, y el cambio es la causa de F-05.

## Hallazgos

### F-05 · NARANJA · `docs/operaciones/kind-local.md` (final del archivo) · conflicto de fusión con origin/main
Revisé esto aparte, como pedías. El commit a115f1e añade la sección NATS al final de `docs/operaciones/kind-local.md`. Después de la ronda 1, origin/main incorporó el commit 89599e4 (U7-T05), que añade al mismo final del archivo la sección «Dashboard (U7-T05)». Resultado:
```
git merge-tree --write-tree HEAD origin/main
CONFLICT (content): Merge conflict in docs/operaciones/kind-local.md   (rc=1)
```
- **Sin conflicto en la ronda 1:** `merge-tree` de `e2f1c6d` contra origin/main y de `86f78d2` contra origin/main dan rc=0.
- **Conflicto en esta ronda:** solo aparece con la edición de F-04 de esta ronda.
- **`control-plane.yaml`:** `deploy/flux/base/control-plane.yaml` se fusiona automáticamente (`Auto-merging`, sin conflicto). U8-T06 no choca.
- **Efecto:** el humano no puede fusionar la rama tal cual.
- **Arreglo esperado:** integrar origin/main en la rama, o rebasar, conservando ambas secciones (Dashboard y NATS), y volver a comprobar `git merge-tree` con rc=0. No hace falta cambiar el contenido de la sección NATS. Tras integrar, CA-5 sigue igual, porque ese archivo está en la lista permitida.

## Tareas candidatas (defectos reales fuera de alcance, repetidas de la ronda 1)
- Actualizar `docs/seguridad/aislamiento.md`: `allow-same-namespace` ahora excluye NATS y falta `nats-clients-only`.
- Corregir el CA-3 de la tarea: el pod de prueba necesita `--labels=aqs.io/nats-client=true`, y `kubectl run --rm -i` pierde la salida de forma intermitente.
- NKeys y TLS en NATS. La etiqueta de cliente es autoservicio.
- Revisar en U8-T02 el reinicio de `nats-init` (3 reinicios) mientras NATS arranca.

## Rutas de transcripciones largas
- `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/1c7b66eb-a3e9-4dad-81e8-f56a81f889ed/scratchpad/r2u8t01-run.out` (bloque completo de kind: CA-3, CA-4, persistencia, idempotencia)
- `/tmp/claude-1000/pol2.txt` (policies.sh) y `/tmp/claude-1000/mt.txt` (merge-tree con el conflicto)

VEREDICTO: NO-VERDE
NARANJA|docs/operaciones/kind-local.md (final del archivo)|conflicto de fusión con origin/main (la sección NATS choca con la sección Dashboard de U7-T05); integrar origin/main conservando ambas secciones
INFORME: revisiones/U8-T01/ronda-2.md
