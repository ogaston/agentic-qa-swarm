# Ronda 1 — U5-T06

VEREDICTO: VERDE

Hash de la tarea verificado: 41cd3e53e0e3dcac03d5657741a5cfed8c61ee03 (coincide). HEAD del worktree: d555102. Corrí todo con los alias literales K, Y y C, sin tocar el clúster. Los fixtures negativos los construí en el scratchpad de la sesión. Worktree limpio antes y después (`git status --short` sin salida).

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | NetworkPolicies de aqs-test en el build | comando literal de CA-1 | pasa: `allow-dns`, `allow-from-control-plane`, `allow-same-namespace`, `default-deny` |
| 2 | kubeconform estricto, dev y prod | comando literal de CA-2 | pasa: 2 líneas `37 resources ... Valid: 37, Invalid: 0, Errors: 0, Skipped: 0` |
| 3 | pruebas Rego | `$C verify --no-color --policy policy` | pasa: `20 tests, 20 passed, ... 0 skipped`, rc=0. Tardó 0,2 s: es normal, las pruebas son unitarias con `with input` y no dependen de estado externo (la regla 7 no aplica) |
| 4 | build cumple políticas | comando literal de CA-4 | pasa: `259 tests, 259 passed, 0 failures` y rc=0, en dev y prod |
| 4b | regla de conjunto con `--combine` (adicional) | `$K build deploy/flux/$e \| $C test ... --combine -` | pasa: `7 tests, 7 passed`, rc=0, en dev y prod |
| 5 | negativas CA-5 | comando literal de CA-5 | pasa: `crb rc=1` e `ipblock rc=1`. Comprobé además que el fallo es por la regla correcta (mensaje `ClusterRoleBinding/x: ... prohibidos`) |
| 6 | SA asignados | comando literal de CA-6 | pasa: las 5 líneas esperadas, idénticas |
| 7 | solo líneas permitidas | comando literal de CA-7 (`origin/main` = eebfdfc = base) | pasa: `1 0 deploy/flux/base/kustomization.yaml`, `0`, `security`. Leí también el diff: control-plane.yaml tiene 3 líneas `+serviceAccountName` y warm.yaml tiene 2, sin nada más |
| 8 | árbol limpio | `git status --short \| wc -l` | pasa: `0` |

## Puntos clave del orquestador
1. **Reglas Rego no triviales.** Las 6 reglas del listado más la de conjunto tienen pruebas que pasan y pruebas que fallan. Probé fixtures propios concatenados al build real de prod. Todos fallan con el mensaje esperado y con `1 failure`:
   - RoleBinding a `aqs-runner` en aqs-test, solo o mezclado con `aqs-reset`.
   - ClusterRole suelto.
   - CronJob y Job sin SA en aqs-test.
   - Deployment `go-reset` sin SA.
   - NetworkPolicy con `ipBlock` en ingress (`from`) y con `ipBlock: {}`.
   - `aqs-runner` sin `automountServiceAccountToken: false`, borrando esa línea del build real.
2. **default-deny (CA-4b).** Prueba negativa real y bien documentada, con 4 variantes sobre el build real, todas con rc=1 y "falta NetworkPolicy default-deny":
   - Quitar la política.
   - Ponerla en otro namespace.
   - Dejar un `podSelector` no vacío.
   - Dejar solo `Egress`.

   La documentación está en la cabecera de `policy/default_deny.rego` y en la bitácora, "Decisiones", con el comando exacto. Lo permite la tarea ("vale implementarla como test separado si se documenta"). Hay 3 pruebas unitarias con input de arreglo (`test_missing_default_deny_denied`, `test_default_deny_only_egress_denied`, `test_default_deny_present_allowed`).
3. **NetworkPolicies.**
   - `allow-dns`: solo `namespaceSelector kubernetes.io/metadata.name: kube-system`, puertos 53 UDP y TCP.
   - `allow-same-namespace`: `podSelector: {}` en ingress y egress, sin `namespaceSelector`, así que solo abre el propio namespace.
   - `allow-from-control-plane`: ingress solo desde `aqs-system`.
   - No hay `ipBlock` ni egress fuera del clúster.
4. **RBAC.** El Role `aqs-test-operator` está en aqs-test. No hay wildcards, secrets, `pods/exec` ni `pods/create`. Los RoleBindings cubren los 3 SA del control plane y `aqs-reset`. `aqs-runner` no tiene binding y tiene `automountServiceAccountToken: false`. No hay ClusterRole ni ClusterRoleBinding en el build (CA-4 pasa con esa regla activa).
5. **Tabla `can-i` de la bitácora.** Es coherente con el RBAC entregado. Las 8 filas se derivan correctamente del Role y de los bindings, incluida la negación para aqs-system y default. No ejecuté nada contra clúster.

**Bloqueo reportado por el codificador (API server).** El diagnóstico es correcto. `default-deny` con Egress más `allow-dns`, `allow-same-namespace` y sin `ipBlock` deja a los pods de `aqs-test` sin ruta al API server (está fuera del namespace). Los CronJobs `housekeeping` y `rebuild` en `warm.yaml:193-260` corren `go-reset housekeeping` y `go-reset rebuild` con SA `aqs-reset`, que tiene Role precisamente para llamar a la API. En un clúster real fallarán. El codificador hizo lo correcto: no abrió nada, lo reportó y no es un defecto suyo. La decisión es del humano. Opciones: una candidata con egress acotado al API server para `aqs-reset` (por etiqueta de pod, que el diseño tendría que aprobar), o mover esa lógica a `go-reset` en aqs-system, que ya tiene binding.

## Hallazgos
No hay ROJO ni NARANJA.

### F-01 · AMARILLO · policy/security.rego:46-57 · `serviceAccountName: ""` pasa la regla
Un Job sin SA se deniega. Con `serviceAccountName: ""` (cadena vacía, equivale al SA `default`), el fixture `job-emptysa` pasa con 266/266. Es un borde secundario que no rompe ningún criterio. Convendría `pod_sa(o) != ""`.

### F-02 · AMARILLO · policy/default_deny.rego · La regla de conjunto no la ejerce el CA-4 literal
Es una consecuencia permitida por la tarea y está documentada. Solo corre si alguien ejecuta CA-4b. No hay CI en el repo que la invoque (`grep` de conftest en workflows: nada).

## Tareas candidatas (fuera de alcance)
- Egress al API server para los CronJobs `aqs-reset` en aqs-test, o reubicar housekeeping y rebuild en el control plane. Es el bloqueo reportado. Requiere decisión del humano.
- Integrar `conftest test --combine` (CA-4b) en el pipeline de CI cuando exista.
- Regla Rego contra un egress "abierto sin ipBlock" en aqs-test. Con `egress: [{}]` o `to: [{namespaceSelector: {}}]` en aqs-test el build pasa 266/266 y abre egress a todo. Con la política actual el único guardia contra egress abierto es `ipBlock`. `egress: [{}]` no está en la lista de CA-4 de esta tarea.
- Un Job puede fijar `automountServiceAccountToken: true` a nivel de pod y sobrescribir el SA de `aqs-runner`. Es una regla para la capa de políticas de U4.

## Rutas de transcripciones largas
- Ninguna. Salidas citadas arriba.

VEREDICTO: VERDE
INFORME: revisiones/U5-T06/ronda-1.md
