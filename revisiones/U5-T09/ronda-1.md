# Ronda 1 — U5-T09

VEREDICTO: NO-VERDE

Los siete criterios pasan con mi propia ejecución. Queda en pie un hallazgo NARANJA: la regla `isolation.rego` se evade de tres formas con sujetos que Kubernetes sí acepta. La tarea declara la regla como mecanismo para que «ningún pod del namespace de prueba» tenga permisos sobre la API.

## Criterios de aceptación, verificados por mí
Usé los alias `K`, `Y` y `C` literales, desde el worktree `/home/omarjayg/Javeriana/topicos-especiales/wt-U5-T09` (HEAD ae2215c). El hash de la tarea coincide: 5939ffb8. `origin/main` está en cefd4d1 y el merge-base es 14e015f.

| # | Criterio | Resultado |
|---|---|---|
| 1 | CronJobs en `aqs-system` con `go-reset` | pasa: `aqs-system/housekeeping=go-reset` y `aqs-system/rebuild=go-reset` |
| 2 | kubeconform estricto en dev y prod | pasa: dos líneas `Valid: 40, Invalid: 0, Errors: 0, Skipped: 0` |
| 3 | Sin SA con permisos en `aqs-test` | pasa: `0`, `0`, `aqs-system` |
| 4 | Políticas | pasa: `verify` da 22 de 22 con rc=0 (main tenía 20, así que hay 2 más). Dev y prod dan 320 de 320, con rc=0 y combine rc=0 |
| 5 | Prueba negativa y positiva sobre el build real | pasa: `sa-local rc=1` (el mensaje es `RoleBinding/x: el ServiceAccount cualquiera de aqs-test no puede tener permisos sobre la API`) y `sa-control-plane rc=0` |
| 6 | Solo archivos y líneas permitidos | pasa: `0`, `0`, `0`. El diff de `security_test.rego` es exactamente una línea `-` y una `+`, el sujeto de `test_rolebinding_reset_allowed` |
| 7 | Árbol limpio | pasa: `git status --short \| wc -l` da `0`, antes y después de mis pruebas |

La evidencia de la bitácora (ronda 2) coincide con la mía. El rojo inicial de CA-1 está registrado en la bitácora, línea 13.

## Punto 1: completitud del movimiento
- **Referencias a `aqs-reset`:** no queda ninguna en manifiestos ni en RBAC. El SA y su sujeto en el RoleBinding están eliminados. El grep fuera de bitácoras, revisiones y tareas solo encuentra `policy/security_test.rego:63` (ver F-02).
- **Tabla `can-i`:** `aqs-reset` aparece como «eliminado» y `go-reset` puede `create jobs -n aqs-test` y no `-n default`.
- **Permisos de `go-reset` en `aqs-test`:** el Role `aqs-test-operator` le da lo necesario para `housekeeping` y `rebuild` como están definidos. Puede borrar Pods, crear y parchear Deployments y StatefulSets (el restart es un patch) y crear Jobs.
- **Red:** `allow-from-control-plane` permite el ingress desde `aqs-system`. `aqs-system` no tiene NetworkPolicy, así que el egress está abierto. El CronJob puede llegar al API server, que era el bloqueo original de U5-T06.
- **Huecos del Role:** son candidatas, no bloqueo (ver abajo).

## Hallazgos

### F-01 · NARANJA · `policy/isolation.rego:8-12` y `policy/isolation_test.rego` · La regla se evade con sujetos que Kubernetes acepta
La regla exige `s.kind == "ServiceAccount"` y `s.namespace == "aqs-test"`. Probé variantes sobre el build real de prod con el comando de CA-5. Con `roleRef` a `edit` y un sujeto concreto, el resultado es:

| Fixture (RoleBinding en `aqs-test`) | rc | Lectura |
|---|---|---|
| SA `x` sin `namespace` | 0 | **evade** |
| `kind: User`, `system:serviceaccount:aqs-test:aqs-runner` | 0 | **evade** |
| `kind: Group`, `system:serviceaccounts:aqs-test` | 0 | **evade** |
| `kind: Group`, `system:serviceaccounts` | 0 | **evade** |
| SA local `x` con `roleRef` a ClusterRole | 1 | denegado, correcto |
| SA `go-reset` de `aqs-system` con ClusterRole | 0 | permitido, correcto |
| mezcla de `go-reset` y SA local | 1 | denegado, correcto |

- **SA sin `namespace`:** el RBAC authorizer de Kubernetes toma por defecto el namespace del RoleBinding. Ese binding concede permisos a los SA de `aqs-test` igual que uno con namespace explícito.
- **Afirmación falsa en la bitácora:** la sección «Decisiones no fijadas» dice «el esquema de RoleBinding lo exige para SA». No es cierto: la validación solo lo exige para sujetos de ClusterRoleBinding. Es una afirmación sin verificar y es la causa de un hueco.
- **El `rc=1` de `aqs-runner` sin namespace** sale de la regla vieja de `security.rego`, por el nombre. Con otro nombre pasa.
- **Pruebas:** `isolation_test.rego` solo cubre los dos casos felices. Ningún caso de evasión.
- **Lo que sí está bien:** la regla no es trivial. Mutantes en copias temporales (en el scratchpad, no en el worktree): sin `isolation.rego` falla `test_isolation_local_sa_denied`; quitando la condición de namespace, o las dos condiciones, fallan `test_isolation_control_plane_sa_allowed` y `test_rolebinding_reset_allowed`. No deniega los bindings legítimos del control plane.
- **Qué falta:** denegar en `aqs-test` los sujetos `ServiceAccount` con `namespace` ausente o vacío, los `User` que empiecen por `system:serviceaccount:aqs-test:`, y los `Group` `system:serviceaccounts`, `system:serviceaccounts:aqs-test`, `system:authenticated` y `system:unauthenticated`. Añadir un caso negativo por cada uno en `isolation_test.rego`.
- **Mismo hueco en otros namespaces:** la regla solo mira RoleBindings en `aqs-test`. Un RoleBinding en `aqs-system` o `default` con un SA de `aqs-test` como sujeto da a esos pods permisos sobre la API y no se deniega (rc=0). La tarea fija el alcance a bindings en `aqs-test`, así que lo dejo anotado como parte de este hallazgo para que el orquestador decida si se amplía la regla o va a candidata.

### F-02 · AMARILLO · `policy/security_test.rego:63` · Resto de `aqs-reset` en un caso de prueba
`test_cronjob_with_sa_allowed` usa `serviceAccountName: aqs-reset`. Es un dato de prueba y no rompe nada. La enmienda prohíbe tocar esa línea. Se limpia en una tarea futura.

## Tareas candidatas (defectos reales fuera de alcance)
- **Role `aqs-test-operator` sin subrecursos `deployments/scale` ni `statefulsets/scale`:** C5 describe scale-down en idle y pausa de runners. Si `go-reset` usa el subrecurso `scale` en lugar de parchear `spec.replicas`, recibirá 403. La tarea prohíbe tocar el Role y la imagen es un placeholder 0.0.0, así que no es bloqueo.
- **Role sin `persistentvolumeclaims` ni `events`:** C5 habla de «teardown total + reprovisionamiento». Solo aplica si el diseño borra PVC. Confirmarlo al implementar `go-reset rebuild`.
- **Regla de aislamiento en todo el repo:** una regla que prohíba un SA de `aqs-test` como sujeto en cualquier RoleBinding de cualquier namespace (ver F-01, última viñeta).

## Rutas de transcripciones largas
Ninguna. Mis fixtures están en `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/4ed52fd5-a142-46f0-ac9b-995972cfe97d/scratchpad/` y no tocaron el worktree. Creé un `mktemp -d` por error en el directorio padre del worktree y ya lo borré.

```
VEREDICTO: NO-VERDE
NARANJA|policy/isolation.rego:8-12|La regla se evade con SA sin namespace, sujetos User y Group de ServiceAccounts; faltan pruebas negativas y la bitácora afirma sin verificar que el esquema exige namespace
INFORME: revisiones/U5-T09/ronda-1.md
```
