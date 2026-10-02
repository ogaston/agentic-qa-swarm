# Ronda 2 — U5-T09

VEREDICTO: NO-VERDE

CA-1 a CA-7 pasan con mi propia ejecución. F-01 queda a medias: todas mis evasiones de la ronda 1 dan `rc=1`, pero `namespace: null` sigue evadiendo la regla. Es el mismo hueco de «SA sin namespace» con otra forma.

Worktree `/home/omarjayg/Javeriana/topicos-especiales/wt-U5-T09`, HEAD f9312b6. El hash de la tarea coincide (5939ffb8). El merge-base es 14e015f.

## Criterios de aceptación, verificados por mí
Usé los alias `K`, `Y` y `C` literales.

| # | Criterio | Resultado |
|---|---|---|
| 1 | CronJobs en `aqs-system` con `go-reset` | pasa: `aqs-system/housekeeping=go-reset` y `aqs-system/rebuild=go-reset` |
| 2 | kubeconform estricto en dev y prod | pasa: dos líneas `Valid: 40, Invalid: 0, Errors: 0, Skipped: 0` |
| 3 | Sin SA con permisos en `aqs-test` | pasa: `0`, `0`, `aqs-system` |
| 4 | Políticas | pasa: `verify` da 30 de 30 con rc=0. Dev y prod dan 320 de 320, con rc=0 y combine rc=0 |
| 5 | Regla sobre el build real | pasa: `sa-local rc=1` y `sa-control-plane rc=0` |
| 6 | Solo archivos y líneas permitidos | pasa: `0`, `0`, `0` |
| 7 | Árbol limpio | pasa: `0`, antes y después de mis pruebas |

- **Tests de `verify`:** main tenía 20, ahora hay 30. Son 10 nuevos (2 de la ronda 1 y 8 de esta), con 7 negativas nuevas.
- **Diff de la ronda:** `git diff --stat ae2215c..tarea/U5-T09` toca solo `policy/isolation.rego`, `policy/isolation_test.rego` y `bitacoras/U5-T09.md`.
- **Afirmación falsa:** la línea de «Decisiones no fijadas» quedó marcada `[CORREGIDO en ronda 3]`. La sección de la ronda 3 la explica con una prueba de kubeconform estricto con SA sin namespace (Valid: 1). Repetí esa prueba con `namespace: null` y también da Valid: 1.

## Verificación de F-01 (fixtures sobre el build real de prod, comando de CA-5, `roleRef` a ClusterRole `edit`)

Los fixtures de la ronda 1 dan todos `rc=1`:

| Fixture | rc |
|---|---|
| SA sin `namespace` | 1 |
| `User system:serviceaccount:aqs-test:aqs-runner` | 1 |
| `User system:serviceaccount:aqs-test:zzz` | 1 |
| `Group system:serviceaccounts:aqs-test` | 1 |
| `Group system:serviceaccounts` | 1 |
| `Group system:authenticated` | 1 |
| `Group system:unauthenticated` | 1 |

Evasiones nuevas:

| Fixture | rc | Juicio |
|---|---|---|
| `Group` `system:serviceaccounts:aqs-test`, `apiGroup: ""` | 1 | denegado |
| mismo `Group`, `apiGroup: rbac.authorization.k8s.io` | 1 | denegado |
| SA con `namespace: ""` | 1 | denegado |
| SA con `apiGroup: ""` | 1 | denegado |
| lista mixta (`go-reset` más SA sin namespace) | 1 | denegado |
| `User` con prefijo en mayúsculas | 0 | benigno, ver abajo |
| `User` con espacio al inicio del nombre | 0 | benigno |
| `kind: USER` | 0 | benigno |
| `kind: serviceaccount` (minúsculas) | 0 | benigno: el API server la rechaza |
| `Group` con espacio al final del nombre | 0 | benigno |
| `Group system:serviceaccounts:AQS-TEST` | 0 | benigno |
| SA con `namespace: null` o `~` | **0** | **evade**, ver F-03 |

Por qué los `rc=0` marcados como benignos no se evaden:
- **`kind`:** la validación de sujetos de RBAC en Kubernetes acepta solo `ServiceAccount`, `User` y `Group` con esa capitalización exacta. `serviceaccount` y `USER` los rechaza el API server (NotSupported). Esta parte la afirmo por conocimiento de Kubernetes, no la corrí: no hay clúster.
- **Nombres:** el authorizer compara nombres de usuario y de grupo de forma sensible a mayúsculas y exacta. `SYSTEM:SERVICEACCOUNT:...` o un nombre con espacios no coincide con la identidad de ningún pod.

Bindings legítimos:
- **Fixtures:** SA `go-reset` de `aqs-system`, `User` de `aqs-system` y `Group system:serviceaccounts:aqs-system` dan `rc=0`.
- **Build real:** CA-3 y CA-4 en dev y prod confirman que `go-reset`, `go-run-controller` y `go-warm-manager` no se deniegan (320 de 320, `combine` rc=0).
- **Prueba positiva:** `test_isolation_user_other_allowed` y `test_isolation_control_plane_sa_allowed` existen y pasan.

## Hallazgos

### F-03 · NARANJA · `policy/isolation.rego:21-24` · `namespace: null` evade la regla (resto de F-01)
La condición es `object.get(s, "namespace", "") in {"", "aqs-test"}`. Con `namespace: null` explícito, `object.get` devuelve `null`, que no está en el conjunto, y la regla no deniega.

```
t sa-ns-null aqs-test '[{kind: ServiceAccount, name: x, namespace: null}]'  -> rc=0 (esperado 1)
t sa-ns-tilde ... namespace: ~                                               -> rc=0 (esperado 1)
kubeconform -strict con namespace: null                                      -> Valid: 1
```

- **Equivalencia en Kubernetes:** el API server decodifica `null` en un campo string como cadena vacía, igual que si faltara. El authorizer usa entonces el namespace del RoleBinding, de modo que el binding concede permisos a los SA de `aqs-test`.
- **Alcance:** es el mismo caso que F-01 («SA sin namespace») y está dentro del alcance. La ronda 3 lo cerró para ausente y vacío, pero no para `null`.
- **Arreglo esperado:** tratar `null` como ausente, por ejemplo `object.get(s, "namespace", "") in {"", "aqs-test", null}`. Añadir un caso negativo en `isolation_test.rego`.
- **Barrido de clase:** revisé `kind` (obligatorio y con valores exactos) y `apiGroup`. Ninguno depende de `null` para evadir. Solo `namespace` tiene el hueco.

## Tareas candidatas
- C-28 (bindings en otros namespaces) y C-30 (`aqs-reset` en `security_test.rego:63`) siguen como las arbitró el orquestador. No se tocaron y no los cuento como hallazgos.

## Notas
- No encontré desborde de alcance, pruebas omitidas ni literales nuevos visibles al usuario en puntos de decisión. Las listas de grupos y el prefijo de la regla son del propio mecanismo de seguridad.
- El worktree quedó limpio. Mis fixtures están en el scratchpad de la sesión.

```
VEREDICTO: NO-VERDE
NARANJA|policy/isolation.rego:21-24|namespace: null en un sujeto ServiceAccount evade la regla (Kubernetes lo trata como ausente); falta caso negativo
INFORME: revisiones/U5-T09/ronda-2.md
```
