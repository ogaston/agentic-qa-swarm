# Ronda 1 — U2-T03b

VEREDICTO: VERDE

El worktree quedó limpio (`git status --short | wc -l` = 0, sha 1991cd9). Las mutaciones las apliqué sobre una copia en el scratchpad, nunca sobre el worktree.

## Criterios de aceptación, verificados por mí (ejecución fresca)
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Pruebas en verde con -race | `go test -race -count=1 ./...` y `-v \| grep -c '--- PASS'` | pasa: 0 FAIL y 168 PASS (≥100). El único SKIP es `TestEventsDump`, ya omitido en main (necesita una variable de entorno). |
| 2 | 3 intentos exactos y nunca un Job | `-run 'Deploy(Retry\|Exhausted\|NeverCreatesJobs\|RolloutTimeout)'` | pasa: 7 pruebas PASS en dos paquetes. Entre ellas está `TestDeployRolloutTimeoutNeverSucceeds` (+ `OnFakeClientset`). `TestDeployExhaustedFailClosedOnFakeClientset` lee los 3 parches del clientset falso. |
| 3 | Otras dos pruebas y valla de build-from-repo | `-run 'SurfaceRequiresFinishedDeploy\|StartDeployIdempotent\|BuildFromRepoRejected'` | pasa: 3 PASS más `TestAPIBuildFromRepoRejectedIs422AndTouchesNothing`. |
| 4 | El camino de Jobs desapareció | `grep -rn -E 'batchv1\|render-deploy-job\|WARM_DEPLOYER_IMAGE\|ResolveOrphans' ... \| wc -l` | pasa: `0`. Un grep de `job` sin distinguir mayúsculas solo encuentra la frase «NO crea Jobs» del README. |
| 5 | Arranque y API fail-closed | `-run 'API\|Config\|Fence'` | pasa: 36 PASS (≥20). |
| 6 | Higiene, imagen y alcance | `go vet` (con y sin `-tags minio`), `gofmt -l`, `podman build`, `git status`, diff fuera de alcance | pasa: `ok`, `65532:65532`, `0`, `0`. |
| — | MinIO real (adaptado, no ejecutado por el codificador) | `go test -tags minio -race -count=1 -run ObjectStoreS3 -v ./adapters/objstore` | pasa: `--- PASS: TestObjectStoreS3 (0.76s)`. Levantó el contenedor de verdad. |

## Mutaciones propias (copia en el scratchpad)
Maté 35 de 41 mutantes válidos. Los 6 que sobreviven están en F-01..F-04. Cuento aparte los inválidos (no compilaban o eran equivalentes) y los que se colgaron, que cuento como muertos.

| Grupo | Mutaciones | Resultado |
|---|---|---|
| Deploy (service.go) | M1 timeout como éxito; M2 sin CAS ready→dirty antes de parchear; M3 `MaxRetries=3`; M4 `MaxRetries=1`; M5b ignorar `ok` del rollout; M6 ignorar el error de `SetImage`; M7 sin handoff; M28 parche solo en el primer intento; M29 contador de intentos desplazado | todas muertas, por varias pruebas cada una |
| Éxito con generación stale / sin imagen en plantilla / sin imagen en pods | M10, M8, M9 | muertas por `TestRolloutCompleteCriteria` |
| `RolloutComplete`, otros criterios | M12 sin Ready, M13 sin Running, M14 sin DeletionTimestamp, M15 sin `availableReplicas`, M16 sin `updatedReplicas` | muertas por `TestRolloutCompleteCriteria` |
| Parche y adaptador | M11 sin verificar el contenedor; M36 nombre de contenedor erróneo; M37 imagen alterada en el parche | muertas |
| Errores y lecturas | M27b error de lectura tratado como éxito; M24 y M25 tope de 5 lecturas fallidas | muertas |
| Eventos | M30b y M31b sin publicar `deploy.failed` / `deploy.done` | muertas |
| Idempotencia y superficie | M20b `InferSurface` sin exigir deploy `done`; M21 sin exigir warm `dirty` | muertas |
| Fail-closed del warm | M18 y M19 sin `reset_verified` | muertas |
| build-from-repo | M22 y M23 sin la valla de 422 | muertas |
| Configuración | M43 y M44 cotas de `WARM_DEPLOY_TIMEOUT` | muertas |
| API | M45 el 422 pasa a 400 | muerta |
| Reloj | M34b sin comprobar el deadline | colgó la prueba, cuenta como muerto |
| Inválidos | M5, M20, M30, M31, M34 no compilaban; M27 y M46 no tuvieron efecto | descartados |

## Puntos de las instrucciones
1. **Deploy en proceso.** Leí `Deploy`, `takeWarm` y `waitRollout`. El CAS ready→dirty va antes de `SetImage`. Hay 3 intentos exactos y solo un rollout completo cuenta como éxito. Las mutaciones M1..M10 lo confirman.
2. **Decisiones del codificador.**
   - Un parche rechazado que consume intento y se reintenta es aceptable: está acotado a 3 intentos y un parche rechazado no deja efecto.
   - 5 lecturas fallidas seguidas cuentan como intento fallido: es fail-closed. M24 y M25 están cubiertas por prueba.
   - `RolloutComplete` más estricto que el de go-reset (exige imagen en plantilla y en pods) es aceptable. Cada criterio tiene una prueba que lo mata.
   - `SetImage` que lee antes y exige el contenedor `warm-app` es aceptable: evita que un parche por nombre inexistente agregue un contenedor.
   - `WARM_JOB_TIMEOUT` ignorado en silencio: **no es fail-open**. Si el operador la conserva, rige el default de 10 min, que solo acorta la espera (más fallos, nunca un éxito falso). Queda en F-04.
   - `go.mod` con `sigs.k8s.io/yaml` indirecto es correcto: ya no se importa.
3. **Reinicio con un deploy en vuelo.** `TestDeployRestartLeavesWarmDirtyAndForgetsDeploys` lee el ConfigMap con un `StateStore` nuevo y encuentra `dirty` con `reset_verified=false`. Una instancia nueva responde `ErrNotReady` y los parches siguen en 1. `TestDeployStateIsMemoryOnly` y `TestAPIDeployStateIsMemoryOnlyAfterRestart` cubren lo mismo. El README documenta que el deploy en vuelo queda sin cierre.
4. **Camino de Jobs y vallas de U2-T03.** CA-4 da 0 y no hay código muerto nuevo; `var _ = errors.Is` ya estaba en main. Los 36 tests `API|Config|Fence` siguen verdes.
5. **build-from-repo.** Da 422 sin tocar el warm ni el Deployment (M22, M23 y M45 muertas). Los registros no permitidos y `latest` siguen rechazados por `ValidateArtifact`, intacto.
6. **Barrido de la tabla camino→prueba.** Hice más de 4 mutaciones por camino y todas las críticas murieron. Solo sobreviven guardas redundantes o de bajo impacto (F-01..F-04).

## Hallazgos (ninguno bloquea)
### F-01 · AMARILLO · service.go StartDeploy (segunda comprobación de `s.deploys` bajo cerrojo) · rama de carrera sin prueba
La mutación M32 (quitar esa segunda comprobación) sobrevive. La prueba de idempotencia es secuencial. Sin la guarda, dos `StartDeploy` concurrentes del mismo `run_id` lanzarían dos goroutines. La segunda fallaría en `takeWarm` y emitiría un `deploy.failed` espurio mientras el primero sigue. El código es correcto y está protegido; falta la prueba. Es el candidato nº 1 si hay ronda 2 por otra causa.

### F-02 · AMARILLO · adapters/kube/kube.go `want == 0` · guarda redundante en la prueba
La mutación M17 sobrevive. El caso «replicas 0» de `TestRolloutCompleteCriteria` deja un pod en la lista, así que lo detiene el conteo de pods y no la guarda. Un caso con 0 replicas y 0 pods probaría la guarda. Con el warm en `ready` no es alcanzable en la práctica.

### F-03 · AMARILLO · service.go waitRollout · detalles sin prueba
- M26: `readFails` no se reinicia en el caso `default`. Sobrevive; la prueba no distingue «seguidas» de «acumuladas».
- M33: el default de `DeployTimeout` pasa a 0 y sobrevive. Falla cerrado, pero no está probado.
- M35: ignorar el error de `Sleep`. Sobrevive; solo importa con ctx cancelado y hoy se usa `context.Background()`.
- M39 y M40 (error al listar pods, `!=` frente a `<`) son equivalentes en la práctica.

### F-04 · AMARILLO · README / cmd/go-warm-manager/main.go · rename silencioso de variable
Una `WARM_JOB_TIMEOUT` que siga definida se ignora sin aviso. No es fail-open, pero conviene una línea de nota en el README o un aviso en el log al arrancar.

## Tareas candidatas (fuera de alcance, ya declaradas por la tarea)
- Soporte de `build-from-repo`.
- Estado del deploy persistido y cierre de deploys en vuelo tras un reinicio.
- Titular del warm en `WarmState`.
- Validación OCI estricta.
- Presupuesto de reintentos persistido.
- RBAC de `patch` sobre `deployments` (U2-T07).

VEREDICTO: VERDE
AMARILLO|service.go StartDeploy (rama de carrera)|F-01 guarda concurrente sin prueba (M32 sobrevive)
AMARILLO|adapters/kube/kube.go want==0|F-02 caso replicas 0 sin pods no probado (M17 sobrevive)
AMARILLO|service.go waitRollout|F-03 readFails/default timeout/Sleep sin prueba (M26, M33, M35 sobreviven)
AMARILLO|README/main.go|F-04 WARM_JOB_TIMEOUT ignorada en silencio
INFORME: revisiones/U2-T03b/ronda-1.md
