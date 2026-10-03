# Ronda 1 — U5-T13

VEREDICTO: VERDE

Worktree `/home/omarjayg/Javeriana/topicos-especiales/wt-U5-T13`, HEAD f07dcef, hash de la tarea 8effeaa8… coincide. Corrí todo con mis propias ejecuciones frescas, con los alias literales y `pwd` correcto.

## Criterios de aceptación, verificados por mí
| # | Criterio | Resultado |
|---|---|---|
| 1 | Entrada por clúster | `OK` |
| 2 | GitRepository y 2 Kustomization por clúster | las 6 líneas idénticas a lo esperado |
| 3 | v2.4.0, 4 controladores, igual al export | helm, kustomize, notification y source-controller; solo `version: v2.4.0`; `IGUAL dev` e `IGUAL prod`. Hice el `diff -q` contra el export oficial de flux-cli v2.4.0 con **cada** archivo, no solo con el de prod. |
| 4 | Overlays sin objetos de Flux | `0`, `0`, `2` |
| 5 | kubeconform estricto | clústeres: `Valid 24, Invalid 0, Errors 0, Skipped 10`; overlays: `Valid 51, Invalid 0, Errors 0, Skipped 0` (dev y prod) |
| 6 | `flux build` sobre copia | `51`, `dev rc=0`; `51`, `prod rc=0` |
| 7 | conftest sobre overlays | `dev rc=0`, `prod rc=0` |
| 8 | Documento | `5`, `4`, `1` |
| 9 | Árbol limpio | `0` (HEAD f07dcef). El worktree quedó limpio tras mi revisión. |

## Puntos que me pediste juzgar
1. **`gotk-components` sin editar a mano:** los de dev y de prod son byte a byte iguales al export oficial (ver CA-3).
2. **Coherencia del arranque:**
   - El orden `apply gotk-components`, luego `apply gotk-sync` es el correcto. La `Kustomization` `flux-system` apunta a `clusters/<env>`, que lista `gotk-components`, `gotk-sync` y `aqs.yaml`. Es el mismo patrón que genera `flux bootstrap`.
   - No hay doble gestión. Comparé el inventario `Kind/ns/name` de `clusters/<env>` con el del overlay `<env>`, y la intersección es vacía en dev y en prod. `aqs-<env>` solo está en `clusters/`. La base no referencia `flux-system`. Los Namespaces de la app son `aqs-system`, `aqs-observability` y `aqs-test`, y `flux-system` no está entre ellos.
   - `prune: true` en `flux-system` solo puede podar lo que está en `clusters/<env>`: componentes, sync y `aqs-<env>`. Es el comportamiento estándar de Flux. El riesgo es el habitual: quitar un archivo de `clusters/` quita Flux o `aqs-<env>`. Es aceptable y no hace falta documentarlo aquí.
   - El documento dice que `conftest` no se aplica a `clusters/` y da el motivo (ClusterRole y ClusterRoleBinding). También dice que los Secrets son U5-T14 y que sin ellos MinIO, el backup y Grafana no arrancan.
3. **Alcance:** `git diff 7741740..HEAD --name-only`, filtrado, deja solo `deploy/flux/{dev,prod}/kustomization.yaml`.
   - En cada overlay, el único cambio es quitar la línea `- flux-kustomization.yaml` y borrar ese archivo (26 líneas borradas en total).
   - `deploy/flux/base/`, `policy/`, `.github/` y `decryption` no se tocan.
   - `aqs.yaml` es un renombre 100 % idéntico del archivo original.
4. **Decisiones no fijadas:**
   - `GitRepository` a 1m: sin riesgo (repo público, polling ligero).
   - `prune: true`: ya estaba así en `aqs-<env>` y es coherente.
   - `kubectl apply` en lugar de `flux bootstrap`: razonable, porque no hay deploy key y el repo es público. Está documentado.

## Hallazgos
### F-01 · AMARILLO · docs/operaciones/bootstrap-flux.md, sección Arranque · Carrera de CRDs entre los dos `apply`
El segundo `kubectl apply` (`gotk-sync.yaml`) puede fallar con "no matches for kind GitRepository" si los CRDs aún no están establecidos. `flux install` y `flux bootstrap` esperan por esto, y el `apply` manual no. Es reintentable e idempotente, y no bloquea. Convendría añadir `kubectl -n flux-system wait --for=condition=available deploy --all --timeout=3m` entre los dos pasos.

### F-02 · AMARILLO · docs/operaciones/bootstrap-flux.md · `aqs-prod` sigue `main` sin compuerta
Con el `GitRepository` en `ref.branch: main`, todo merge a `main` se reconcilia solo en prod, igual que en dev. Esto ya era así antes de la tarea (el `aqs-prod` original tenía el mismo `sourceRef`), y la tarea fija `branch: main`, así que no es un defecto de este diff. Vale la pena documentarlo como riesgo conocido.

## Tareas candidatas (fuera de alcance)
- Dar a prod una compuerta de promoción: un tag o rama de release en lugar de `main`, o `suspend` manual. Esto toca el PRD ("confirm humano") y debe decidirlo el orquestador.
- Añadir `dependsOn` o `wait` a `aqs-<env>`, para que la salud del `Kustomization` refleje que los pods arrancan una vez resueltos los Secrets (U5-T14).

## Notas
- La bitácora tiene la salida literal de CA-1 en rojo, las pruebas negativas de CA-3 y CA-4, y las decisiones no fijadas. No ejecuté las pruebas negativas por mi cuenta. Las dos son comportamientos triviales de `diff` y de conteo, y los CA-3 y CA-4 positivos pasan.
- En `docs/operaciones/README.md` la entrada nueva dice "clúster" con tilde, y el resto del archivo escribe sin tildes (cosmético).

VEREDICTO: VERDE
AMARILLO|docs/operaciones/bootstrap-flux.md (Arranque)|Carrera de CRDs entre los dos kubectl apply, falta wait
AMARILLO|docs/operaciones/bootstrap-flux.md|aqs-prod sigue main sin compuerta (riesgo conocido, preexistente)
INFORME: revisiones/U5-T13/ronda-1.md
