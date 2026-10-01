# Ronda 1 — U5-T04

VEREDICTO: NO-VERDE

Los 8 criterios pasan en mi ejecución fresca y no hay desborde de alcance. Lo que bloquea es la bitácora (evidencia no literal, ya señalada en U5-T01) y la falta de registro de las decisiones que la tarea no fija.

Worktree `/home/omarjayg/Javeriana/topicos-especiales/wt-U5-T04`, HEAD `fa18a7d940fe3f687a0b5a18c94ced92162ca1ae`. El hash de la tarea coincide (9d9207a...). `git status --short` quedó vacío antes y después de mis pruebas, porque las negativas las hice sobre copias temporales de `git archive` en el scratchpad.

## Criterios de aceptación, verificados por mí
Comandos literales de la tarea, con `K` como en la tarea.

| # | Criterio | Resultado |
|---|---|---|
| 1 | Los dos overlays construyen | `BUILD OK` |
| 2 | kubeconform estricto con CRDs de Flux | Dos líneas `26 resources ... Valid: 26, Invalid: 0, Errors: 0, Skipped: 0`. El objeto Flux Kustomization quedó validado, no omitido. |
| 3 | Warm completo en `aqs-test` | `6` y `6` |
| 4 | 7 servicios en `aqs-system` | `14` |
| 5 | Imágenes de prod sin `latest` ni sin tag | `0` y `0` |
| 6 | Kustomization de Flux con path propio y prune | `2` y `2` |
| 7 | Contenedores, limits y probes | `12`, `12`, `6` |
| 8 | Árbol limpio y sin `.gitkeep` | `0` y `0` |

**Pruebas negativas, corridas por mí sobre copias temporales:**
- **CA-5, `go-intake:latest`:** el primer conteo dio `0` y el segundo `1`, así que detecta.
- **CA-5, `image: redis` sin tag:** el primer conteo dio `1`, así que detecta.
- **CA-2, `replicaz`:** salió `additionalProperties 'replicaz' not allowed` y `Valid: 25, Invalid: 1`.

**Evidencia vs mi ejecución:** las cifras de la bitácora coinciden con las mías, así que sí pudieron salir del diff actual. El rojo inicial de CA-1 (`FALLA dev`) está registrado.

**Alcance "Fuera":** el diff solo toca `deploy/flux/**` y `bitacoras/U5-T04.md`. No hay cambios en `contracts/`, `.github/`, `scripts/`, `README.md` ni `aidlc-docs`. Busqué `Role`, `ServiceAccount`, `NetworkPolicy`, `Secret`, `ServiceMonitor` y `minio` en el build de prod y no aparece nada. El único Secret es la referencia `secretKeyRef` a `warm-db-credentials`, sin crearlo, como pide la tarea. No hay comandos contra clúster.

## Los cuatro puntos que declaró el codificador

**1. `env: TZ=UTC` para el grep de CA-7: legítimo, y CA-7 está mal especificado (F-03).** Reproduje el problema. En el commit `fc01e68`, sin el env, el build de prod da 3 líneas `^\s+image:` contra 9 `- image:`, mientras que `limits:` da 12. Kustomize ordena las claves y emite `- image:` cuando `image` es la primera. El codificador no forzó el criterio: dejó el criterio a la altura de lo que mide, que es 12 contenedores con 12 limits. Es inocuo, porque TZ=UTC no cambia el comportamiento. Aun así es una variable muerta que existe solo para el grep, y está sin comentario en el manifiesto.

**2. Namespace `flux-system` y `sourceRef` GitRepository/agentic-qa-swarm:** son valores razonables, la convención de Flux. Pero nada en el repo define ni crea ese GitRepository, y la bitácora no lo registra (F-02).

**3. Placeholders `nginx:1.27.2` y `go-reset:0.0.0`:** son aceptables y la nota de la tarea los prevé. Quedan sin registro en la bitácora (F-02).

**4. Historial de 4 commits:** `fbe9222` solo borra los tres `.gitkeep`, pero su mensaje dice "manifiestos Flux base, dev y prod", así que el mensaje miente (AMARILLO, F-05).

## Hallazgos

### F-01 · NARANJA · bitacoras/U5-T04.md · Evidencia abreviada y CA-8 final ausente
La nota de la tarea exige "el comando literal de cada criterio y su salida, no abreviaturas (hallazgos F-02 y F-03 de U5-T01)". Es la misma clase de defecto que en U5-T01. Barrido de todas las apariciones:
- **Criterios:** los CA-1..CA-8 aparecen como `$ CA-1`, `$ CA-2`, etc., con la salida de un `/tmp/ca.sh` que no está en el repo. No hay ningún comando literal.
- **CA-8:** el valor de la bitácora es `2` y `0`, y la propia nota lo declara previo al commit. Dice "ver CA-8 final al pie", pero no existe ese pie. No hay evidencia registrada de `0` y `0`.
- **Pruebas negativas de CA-5 y CA-2:** solo hay prosa con el resultado. No aparece ni el comando ni el `sed` o edición que se aplicó.
- **Rojo inicial:** el comando registrado usa `break` en lugar del `exit 1` de CA-1, así que no es el literal.

### F-02 · NARANJA · bitacoras/U5-T04.md y deploy/flux/{dev,prod}/flux-kustomization.yaml · Decisiones no fijadas por la tarea, sin registro
La tarea no fija estos valores y la bitácora no los menciona (solo estaban en el mensaje de encargo):
- **`flux-system` y `sourceRef` GitRepository/`agentic-qa-swarm`:** el repo no define ni crea ese GitRepository. Si no existe al hacer bootstrap, el Kustomization queda en not-ready. Debe constar como supuesto o contrato para el humano que haga el bootstrap.
- **Placeholders de imagen:** `nginx:1.27.2` y `go-reset:0.0.0`.
- **Valores inventados:** `idleScaleDownAfter: 30m`, `minReplicasIdle: "0"` y los schedules de los CronJobs (`0 * * * *` y `0 3 * * 0`). Ninguno aparece en `requirements.md`, que solo dice "réplicas mínimas".
- **Secret referenciado y no creado:** `warm-db-credentials`, a cargo de quien lo provea (no está indicado en ningún lado).

La corrección es documentar cada decisión en la bitácora. No pido cambiar los valores.

### F-03 · AMARILLO (hallazgo de especificación) · tarea CA-7 y CA-5 · Los greps asumen `image:` en columna de clave
`^\s+image:` no cuenta `- image:`. Con ese grep, CA-7 solo funciona si cada contenedor tiene una clave alfabéticamente anterior a `image`, por ejemplo `env`. La primera línea de CA-5 tiene el mismo punto ciego: una imagen sin tag en un contenedor sin env pasaría inadvertida. Hoy CA-5 detecta porque el hack cubre los 12 contenedores. Sugiero al orquestador corregir la especificación a `^\s*(-\s+)?image:` en ambos criterios. No exijo nada al codificador. Pido que `TZ=UTC` lleve un comentario o una nota en la bitácora que explique su razón de ser.

### F-04 · AMARILLO · deploy/flux/dev/kustomization.yaml · Los patches de dev son no-ops
`dev` pone replicas 1 y memoria 128Mi, que ya son los valores de base. `diff` entre `kustomize build base` y `dev` solo muestra el objeto Flux Kustomization añadido. La diferencia dev/prod es real solo en prod (réplicas 2, memoria 256Mi), y el warm no se parchea. Cumple la letra de "patches", pero dev no se diferencia de base.

### F-05 · AMARILLO · historial · Commit `fbe9222` con mensaje engañoso
Solo borra los `.gitkeep`, pero el mensaje dice "manifiestos Flux base, dev y prod".

### F-06 · AMARILLO · deploy/flux/base/warm.yaml (warm-db) · Montar `data` directamente en `/var/lib/postgresql/data`
Con un PVC, el directorio `lost+found` suele hacer fallar el `initdb` salvo que se fije `PGDATA` a un subdirectorio. No bloquea ningún criterio y es un defecto de runtime que no se puede verificar sin clúster.

## Tareas candidatas
- Corregir CA-7 y CA-5 en la plantilla de criterios para que el grep de `image:` tolere `- image:` (F-03).
- U5-T01 o U5-T03: definir y documentar el GitRepository/`flux-system` de bootstrap que consumen estos Kustomizations.
- U5-T05..T08: quien cree `warm-db-credentials` (el Secret referenciado) y aplique `PGDATA`/securityContext al warm.

## Rutas de transcripciones largas
Ninguna, todo cabe en este informe. Las copias temporales de las pruebas negativas están en el scratchpad de la sesión.

VEREDICTO: NO-VERDE
NARANJA|bitacoras/U5-T04.md|Evidencia abreviada ($ CA-n), CA-8 final ausente, pruebas negativas sin comando, rojo inicial no literal
NARANJA|bitacoras/U5-T04.md y flux-kustomization.yaml|Decisiones no fijadas (flux-system, GitRepository agentic-qa-swarm, placeholders, 30m/schedules, Secret warm-db-credentials) sin registro
INFORME: revisiones/U5-T04/ronda-1.md
