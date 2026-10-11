# Ronda 1 — U8-T01

VEREDICTO: NO-VERDE

Hay dos hallazgos NARANJA (F-01 y F-02) y ningún ROJO. Los cinco criterios cumplen con el comando literal, salvo CA-3, que cumple con la variante del codificador. Tengo solo lectura, así que no escribí el archivo del informe. El orquestador debe guardar este texto en `revisiones/U8-T01/ronda-1.md`.

Worktree `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U8-T01`, rama sha e2f1c6d. Quedó limpio (`git status --short` da 0 líneas). No hay conflictos de fusión con el origin/main actual: `git merge-tree` da 0 marcas de conflicto, y `deploy.sh` y `control-plane.yaml` no chocan.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Políticas en verde, sin tocar `policy/` | `bash scripts/ci/policies.sh \| grep -c '^FALLA'`; `git diff --name-only <base> -- policy \| wc -l` | pasa: rc=0, `0` y `0` (36 líneas, todas OK, incluidos kubeconform y conftest en dev, prod y kind) |
| 2 | NATS con imagen fijada en los tres overlays | el `for` de la tarea | pasa: `1`, `1`, `1` |
| 3 | Desplegado y stream leído de vuelta | `deploy.sh \| grep nats`, `kubectl run --rm -i … nats-probe` literal, y la variante | `deploy.sh` pasa. El literal **falla**. La variante pasa y la acepto (ver abajo) |
| 4 | Aislamiento por etiqueta | `bash scripts/kind/nats-netpol-check.sh; echo rc=$?` | pasa: `OK nats-cliente-permitido`, `OK nats-otro-pod-bloqueado`, `rc=0`; sin pods residuales |
| 5 | Alcance | los dos comandos de CA-5 | pasa: `0` y `0` |

Detalle de CA-3, todo dentro de un único bloque `flock` con kind-up, build-images, load-images y deploy, y kind-down al final:
- `deploy.sh` imprimió `OK aqs-system/statefulset.apps/nats` y `OK aqs-system/job/nats-init`, sin `FALLA`. Lo repetí con el grep de la tarea y dio lo mismo.
- El comando literal falla en mi ejecución con `nats: error: setup failed: dial tcp 10.96.12.16:4222: i/o timeout`, así que `jq` falla. La causa es que el pod no lleva la etiqueta de cliente y la NetworkPolicy lo bloquea. Es el caso que la tarea prevé: documentar el pod etiquetado y usarlo.
- Variante con pod etiquetado, espera a `Succeeded` y `kubectl logs | jq`: dio `aqs.events.>`, `120000000000`, `604800000000000` (7 días), `limits` y `file`. Los valores salen del servidor real, no del manifiesto.
- Arbitraje: acepto la variante para CA-3. Es lo que la tarea pide si la política bloquea al pod de prueba, y lee el estado de vuelta con la herramienta real.

Comprobaciones extra mías:
- **Persistencia:** borré `nats-0`. El PVC `data-nats-0` (2Gi) sigue enlazado y `stream info AQS_EVENTS` responde después del reinicio.
- **Idempotencia:** borré el Job `nats-init` y lo reaplicé con `apply -k`. Completó y registró `stream AQS_EVENTS ya existe`.
- **Aislamiento:** con el Endpoints de `nats` solo en el 4222 y el 8222 fuera del Service.

## Arbitraje de los puntos del codificador
- **`allow-same-namespace` (U4-T05): es más estricta, no más débil.** Solo añade un `NotIn [nats]` al `podSelector`, que es el pod destino. El tráfico entre el resto de pods de `aqs-system` no cambia. Sin ese cambio las NetworkPolicy se suman y CA-4 no puede pasar. `allow-metrics-from-observability` solo abre el 8080, y `allow-public-http` solo abarca go-intake y ui-api, así que ninguna abre el 4222 ni el 8222 de NATS. No toca `policy/*.rego` y conftest sigue en verde. Es un cambio justificado y dentro de `deploy/flux/base`. Su costo es que `docs/seguridad/aislamiento.md` queda desactualizado (ver tareas candidatas).
- **`nats-init` con la etiqueta:** necesario, porque el Job habla con NATS. Es una desviación mínima respecto de «solo tres servicios», y está documentada en el README y en el comentario de `networkpolicy.yaml`. Aceptable.
- **Chequeo de reinicios en `deploy.sh`:** correcto. El regex `^<nombre>-[0-9]+$` solo se aplica a StatefulSet y evita contar el pod del Job `nats-init-…`. En mi ejecución `nats-init` marcó 3 reinicios mientras NATS arrancaba, y con el prefijo antiguo eso daba una `FALLA` falsa. También corrige el mismo caso de `minio-init` contra `minio-`. Los Deployments no cambian.
- **Etiqueta en `control-plane.yaml`:** son tres líneas en `labels` del template, sin tocar selectors. Hoy no hay conflicto con origin/main. Riesgo bajo para U8-T02.

## Hallazgos

### F-01 · NARANJA · `scripts/kind/nats-netpol-check.sh:25-27` · el `trap` de limpieza se arma antes de la guarda de contexto
`trap cleanup EXIT` va en la línea 25 y `require_kind_context` en la 27. Si el contexto no es `kind-aqs`, la guarda sale con 3, pero el `trap` ya está armado y ejecuta `kubectl delete pod nats-netpol-ok nats-netpol-ko -n aqs-system …` contra el clúster equivocado. Lo demostré con un `kubectl` falso en el PATH que devuelve el contexto `other-cluster`:
```
contexto other-cluster no es kind-aqs
rc=3
kubectl config current-context
kubectl delete pod nats-netpol-ok nats-netpol-ko -n aqs-system --ignore-not-found --wait=false
```
Esto viola C-96 (solo el contexto kind-aqs) y el propio comentario del script («No toca otros pods ni otros namespaces»). `kind-smoke.sh:16` pone la guarda antes de crear nada y `load-images.sh` la pone antes del `trap`. El impacto es acotado (nombres fijos, `--ignore-not-found`), pero un script de `scripts/kind/` no debe lanzar `kubectl` contra otro contexto.
Arreglo esperado: llamar a `require_kind_context` antes de `trap cleanup EXIT`. Añadir una prueba, aunque sea con un `kubectl` falso, que muestre que con contexto ajeno solo se ejecuta `config current-context`.

### F-02 · NARANJA · `deploy/flux/base/nats/README.md` (sección «Comprobación manual») · comando documentado que el propio codificador declara roto, sin verificarlo
El README ofrece `kubectl run --rm -i --restart=Never --labels=aqs.io/nats-client=true … nats … stream info AQS_EVENTS --json` como comprobación. La bitácora dice que `kubectl run --rm -i` pierde la salida. La bitácora no muestra que ese comando documentado se haya corrido. Lo corrí 3 veces, con la etiqueta y filtrando con `jq`:
```
run1: jq: parse error: Invalid numeric literal at line 42, column 4 / aqs.events.> / 120000000000 / rc=0 5
run2: jq: parse error: Invalid numeric literal at line 1, column 4  (sin salida)   / rc=0 5
run3: jq: parse error: Invalid numeric literal at line 42, column 4 / aqs.events.> / 120000000000 / rc=0 5
```
Sale un `jq: parse error` en las tres corridas y en una se pierde la salida entera. La lectura de vuelta que el README promete no es fiable.
Arreglo esperado: documentar la variante verificada (pod etiquetado, esperar `Succeeded`, `kubectl logs`, borrar el pod). Debe ser la misma que se usó para CA-3.

### F-03 · AMARILLO · `bitacoras/U8-T01.md` · evidencia incompleta
- El «rojo primero» no incluye la salida de `nats stream info` fallando, que el plan de pruebas pedía. Solo muestra `NotFound` del Service.
- La línea de CA-5 `git status --short | wc -l` quedó como «(tras commit, ver abajo)» y no tiene salida pegada. La verifiqué yo (0), pero la bitácora no lo prueba.
- La salida de CA-3 pegada es la de la variante. La tarea pedía documentar la desviación, y está hecha en la sección de desviaciones.
- La bitácora califica `nats-init` de verde sin mencionar que el pod del Job reinicia 3 veces mientras NATS arranca. Es benigno (`restartPolicy: OnFailure`, `backoffLimit: 6`), pero conviene anotarlo.

### F-04 · AMARILLO · `docs/operaciones/kind-local.md`, `deploy/kind/README.md`
La tarea las admite «si aplica». Ninguna menciona NATS ni `nats-netpol-check.sh`. No bloquea.

## Tareas candidatas (defectos reales fuera de alcance)
- Actualizar `docs/seguridad/aislamiento.md`: la fila de `allow-same-namespace` ya no describe el comportamiento (excluye NATS) y falta `nats-clients-only`. Está fuera de la cerca de CA-5 de esta tarea.
- Arreglar el CA-3 de la tarea: `kubectl run --rm -i` pierde la salida de forma intermitente, y el pod de prueba necesita `--labels=aqs.io/nats-client=true`.
- NKeys y TLS en NATS. La etiqueta de cliente es autoservicio: cualquier pod que pueda crear pods en `aqs-system` puede etiquetarse y alcanzar NATS.
- Los servicios no declaran dependencia de arranque con NATS (el Job reinicia 3 veces mientras NATS arranca). Revisar en U8-T02 cuando los consumidores se conecten.

## Rutas de transcripciones largas
- `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/1c7b66eb-a3e9-4dad-81e8-f56a81f889ed/scratchpad/pol.txt` (policies.sh)
- `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/1c7b66eb-a3e9-4dad-81e8-f56a81f889ed/scratchpad/rev.out` (corrida completa de CA-3 y CA-4 en kind)
- `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/1c7b66eb-a3e9-4dad-81e8-f56a81f889ed/scratchpad/rev2.out` (README x3, persistencia, idempotencia)

Nota de entorno: en mi primer intento falló `build-images` (`image not known`, almacenamiento de podman reiniciado, probablemente por otra tarea en paralelo). Repetí todo desde cero dentro del `flock`, y el clúster `aqs` quedó eliminado (el `trap` llama a `kind-down.sh` y restaura el contexto previo).

VEREDICTO: NO-VERDE
NARANJA|scripts/kind/nats-netpol-check.sh:25-27|trap de limpieza antes de la guarda de contexto: con contexto ajeno ejecuta kubectl delete contra otro clúster (C-96)
NARANJA|deploy/flux/base/nats/README.md (Comprobación manual)|comando documentado con kubectl run --rm -i que pierde la salida (3 corridas: 3 errores de jq, una sin salida); documentar la variante verificada
INFORME: revisiones/U8-T01/ronda-1.md
