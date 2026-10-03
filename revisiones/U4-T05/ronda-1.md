# Ronda 1 — U4-T05

VEREDICTO: VERDE

Corrí CA-1 a CA-11 desde la raíz del worktree (HEAD 917ae15, base d1fbe47 = origin/main). Todos pasan y no queda ningún ROJO ni NARANJA. Cinco hallazgos AMARILLOS que no bloquean. Worktree limpio tras la revisión, incluso después de la mutación de CA-8.

## Criterios de aceptación, verificados por el revisor
| # | Criterio | Resultado |
|---|---|---|
| 1 | Cuatro SA nuevos con `automount=false`; los tres con API sin definir | pasa (CA-1 con yq corregido, ver nota 1) |
| 2 | Cada Deployment usa su SA; los cuatro nuevos sin token a nivel de pod | pasa: siete líneas exactas y 0 líneas fuera de lo permitido en control-plane.yaml |
| 3 | Cuatro NetworkPolicy de `aqs-system`, ninguna de egress | pasa |
| 4 | `kubeconform` estricto dev y prod | pasa: `Valid: 59, Invalid: 0, Errors: 0, Skipped: 0` ×2 (con `--network host`, nota 2) |
| 5 | Pruebas Rego, ≥ 14 nuevas | pasa: 103 tests, 103 passed; nuevas=37 (base 66) |
| 6 | Build real cumple políticas, con `--combine` | pasa: cuatro `rc=0` |
| 7 | Diez fixtures negativos y regla de conjunto | pasa: los diez `rc=1` con el motivo correcto, y `sin-default-deny rc=1` |
| 8 | Matriz RBAC y mutación | pasa: CSV 228 líneas; `OK rbac-matrix dev/prod`; mutado `rc=1` (`difiere go-run-controller,aqs-test,deletecollection,jobs esperado=no efectivo=yes`); restaurado. Cinco mutaciones extra (ClusterRole, Role con `*`, RoleBinding de aqs-runner, RoleBinding de ui-api a aqs-test-operator, verbo de menos) fallan `rc=1` |
| 9 | Integración en `policies.sh` | pasa con el shim (nota 2): once `OK`, `rc=0`; 0 líneas eliminadas |
| 10 | Documentación | pasa: 48 líneas; ingress-controller=2; can-i=11; C-51=1; egress=2 |
| 11 | Higiene y alcance | pasa: 0, 0, 0 |

## Notas de ejecución
1. La expresión yq propuesta en el arbitraje (`if has(...) then ... end | tostring`, sintaxis de jq) no parsea en mikefarah/yq 4.44.3. Forma que funciona y distingue `false` de ausente: `(select(has("automountServiceAccountToken")) | .automountServiceAccountToken | tostring) // "sin-definir"`.
2. Imágenes locales: kustomize `registry.k8s.io/kustomize/kustomize:v5.4.3` es en realidad `line/kubectl-kustomize` (kustomize v5.8.1); kubeconform v0.6.7 empaquetado localmente. Sustitución legítima: los overlays solo usan `resources` y parches JSON6902, sin transformadores sensibles a la versión. `policies.sh` literal falla en kubeconform por un proxy 127.0.0.1 inalcanzable desde el contenedor (fallo de entorno); con un shim de docker `--network host` (fuera del repo) las once comprobaciones dan OK.
3. Las tres pruebas antiguas de `security_test.rego` cambiadas son legítimas (afirmaban lo contrario de lo que exige la tarea; solo cambiaron entradas, 0 pruebas borradas; cobertura equivalente en `test_cp_sa_*`, `test_ingress_ipblock_*`, `test_combine_*`).
4. La decisión del codificador de denegar una regla de ingress con `ports` y sin `from` es correcta (en Kubernetes abre el puerto a cualquier origen) y está probada.
5. Alcance respetado: rbac.yaml, egress.rego, default_deny.rego y security.rego no se tocan; policies.sh +3 líneas, 0 eliminadas.

## Hallazgos
### F-01 · AMARILLO · bitácora, «Entorno» · Imágenes re-etiquetadas sin digest registrado
Registrar digests (`sha256:1503c366...` kustomize, `sha256:fb12e541...` kubeconform). Limitación del entorno; no afecta la validez del diff.

### F-02 · AMARILLO · `policy/ingress.rego` · `namespaceSelector: {}` no se deniega
`from: [{namespaceSelector: {}}]` abre el ingreso a todos los namespaces y pasa las políticas. Fuera de lo pedido: tarea candidata.

### F-03 · AMARILLO · `policy/workloads.rego:9` y `policy/security.rego:7` · Lista de Deployments duplicada
`todos_cp` (siete) convive con `cp_deployments` (tres). La tarea prohíbe tocar `security.rego` salvo adiciones.

### F-04 · AMARILLO · Plan de pruebas · Las mutaciones de `rbac-matrix.sh` no están versionadas como prueba
Verificadas por el revisor con `--manifest` y registradas en la bitácora, pero no hay script de prueba en el repo.

### F-05 · AMARILLO · `scripts/ci/rbac-matrix.sh` · Dependencia de `jq` en el host
Declarada en la cabecera; los runners ubuntu la traen.

## Tareas candidatas (defectos reales fuera de alcance)
- Rechazar `namespaceSelector: {}` en el ingress de `aqs-system` (F-02).
- Unificar `cp_deployments` y `todos_cp` (F-03).
- `rbac-matrix_test.sh` que automatice las cinco mutaciones (F-04).
- Corregir en las tareas la sintaxis yq de CA-1/CA-2.

VEREDICTO: VERDE
AMARILLO|bitacoras/U4-T05.md (Entorno)|Imagenes re-etiquetadas sin digest registrado (F-01)
AMARILLO|policy/ingress.rego|namespaceSelector: {} no se deniega en el ingress de aqs-system (F-02)
AMARILLO|policy/workloads.rego:9, policy/security.rego:7|Lista de Deployments del control plane duplicada (F-03)
AMARILLO|plan de pruebas|Mutaciones de rbac-matrix.sh no versionadas como prueba (F-04)
AMARILLO|scripts/ci/rbac-matrix.sh|Dependencia de jq en el host (F-05)
INFORME: revisiones/U4-T05/ronda-1.md
