# Ronda 3 — U5-T09

VEREDICTO: VERDE

Worktree `/home/omarjayg/Javeriana/topicos-especiales/wt-U5-T09`, HEAD da4360f. El hash de la tarea coincide (5939ffb8). El merge-base es 14e015f. F-03 queda cerrado. Corrí CA-1..CA-7 yo mismo, con los alias `K`, `Y` y `C` literales. Los fixtures están en el scratchpad de la sesión.

## Criterios de aceptación, verificados por mí
| # | Criterio | Resultado |
|---|---|---|
| 1 | CronJobs en `aqs-system` con `go-reset` | pasa: `aqs-system/housekeeping=go-reset` y `aqs-system/rebuild=go-reset` |
| 2 | kubeconform estricto en dev y prod | pasa: dos líneas `Valid: 40, Invalid: 0, Errors: 0, Skipped: 0` |
| 3 | Sin SA con permisos en `aqs-test` | pasa: `0`, `0` y `aqs-system` |
| 4 | Políticas | pasa: `verify` da 31 de 31 con rc=0 (main tenía 20). Dev y prod dan 320 de 320, con rc=0 y combine rc=0 |
| 5 | Regla sobre el build real | pasa: `sa-local rc=1` y `sa-control-plane rc=0` |
| 6 | Solo archivos y líneas permitidos | pasa: `0`, `0`, `0` |
| 7 | Árbol limpio | pasa: `0`, antes y después de mis pruebas |

- **Diff de la ronda:** `git diff --name-only f9312b6..tarea/U5-T09` toca solo `policy/isolation.rego`, `policy/isolation_test.rego` y `bitacoras/U5-T09.md`. No hay desborde de alcance.
- **Prueba nueva:** `test_isolation_sa_null_namespace_denied` existe y se ejecuta. No hay pruebas omitidas.
- **Bitácora:** la sección "Ronda 4" pega el rojo (`namespace: null rc=0`) y el verde (`rc=1`) sobre el build real, además de los CA literales.

## 1. Verificación de F-03 y repetición de fixtures (build real de prod, comando de CA-5, `roleRef` a ClusterRole `edit`)

**Evasiones, todas `rc=1`:**
- SA sin namespace.
- `User system:serviceaccount:aqs-test:aqs-runner` y `...:zzz`.
- `Group` con `system:serviceaccounts:aqs-test`, `system:serviceaccounts`, `system:authenticated` y `system:unauthenticated`.
- `Group` `system:serviceaccounts:aqs-test` con `apiGroup: ""` y con `apiGroup: rbac.authorization.k8s.io`.
- SA con `namespace: ""`.
- SA con `apiGroup: ""`.
- Lista mixta: `go-reset` en `aqs-system` más un SA sin namespace.
- Lista mixta: `go-reset` en `aqs-system` más un SA con `namespace: null`.
- SA con `namespace: null`.
- SA con `namespace: ~`.
- SA con `namespace: aqs-test`.

**Formas no-string, todas `rc=1`:** `namespace: 123`, `namespace: false`, `namespace: [aqs-test]` y `namespace: {a: b}`.
- La regla trata cualquier valor no-string como ausente. Es conservadora.
- Kubernetes solo aceptaría `null` en un campo string, y lo decodifica como cadena vacía. Un número, una lista o un mapa fallan la decodificación del API server.
- Es decir, el único caso realista de evasión (`null`) queda cerrado. Las demás formas quedan denegadas por exceso de cautela, sin falsos negativos.
- Que `null` llegue a un RoleBinding es consistente con la ronda 2: para un RoleBinding, el API server no exige namespace en el sujeto SA. Esto lo afirmo por conocimiento de Kubernetes, no lo corrí, porque no hay clúster.

**Legítimos, todos `rc=0`:**
- SA `go-reset` en `aqs-system`.
- SA `go-intake` en `aqs-system`.
- `User system:serviceaccount:aqs-system:go-reset`.
- `Group system:serviceaccounts:aqs-system`.
- Lista solo con `go-reset`.
- `subjects: []`.

**`rc=0` benignos:**
- `User` con prefijo en mayúsculas.
- `Group` con espacio al final.
- `kind: serviceaccount`.
- `kind: null`.
- `User` con `name: null`.
- `Group` con `name: 5`.
- El API server los rechaza (kind o name inválidos), o no coinciden con ninguna identidad real. Esto también es conocimiento de Kubernetes.

No encontré otra forma que evada la regla dentro del alcance acordado (RoleBindings en `aqs-test`).

## Hallazgos de U5-T09
Ninguno ROJO ni NARANJA. Sin hallazgos AMARILLOS que bloqueen.

## 3. Hallazgo colateral sobre main (NO es defecto de U5-T09)

### M-01 · NARANJA (sobre main) · `policy/security.rego:33-34` · `egress: null` o `ingress: null` desactiva la regla de `ipBlock`
El hallazgo del codificador es correcto y tiene el efecto que preguntaste. Lo comprobé extrayendo `main` (commit 4c751e9, que incluye `policy/egress.rego` de U5-T10) con `git archive` a un directorio del scratchpad. Concatené su build de prod con una NetworkPolicy en `aqs-test`, usando el comando de CA-5:

| Fixture (NetworkPolicy en `aqs-test`) | Resultado con las políticas de main |
|---|---|
| A: `ingress.from[].ipBlock 0.0.0.0/0` con `egress: null` | **`rc=0`, 400 de 400 pasan** |
| D: igual que A, con `policyTypes: [Ingress]` | **`rc=0`, 400 de 400 pasan** |
| B (control): `ingress` con `ipBlock`, sin la clave `egress` | `rc=1`: `NetworkPolicy/evil: ipBlock prohibido en aqs-test` |
| E (control): `ingress` con `ipBlock` y `egress: []` | `rc=1`: `ipBlock prohibido en aqs-test` |
| C: `egress` con `ipBlock` e `ingress: null` | `rc=1`: lo detiene `egress.rego` con `egress[0] no es intra-namespace ni DNS a kube-system:53` |

- **Por qué pasa A:**
  - `object.get(input.spec, "egress", [])` devuelve `null` si la clave existe con valor `null`.
  - `array.concat(null, ...)` queda indefinido y la regla no dispara.
  - `egress.rego` itera solo `egress`. Con `egress: null` no hay reglas que evaluar y no mira `ingress`.
- **Aceptación por kubeconform:** el fixture A pasa `kubeconform -strict` (`Valid: 50, Invalid: 0`), y Kubernetes acepta `null` en una lista.
- **Resultado:** una NetworkPolicy con `ipBlock` en `ingress` y `egress: null` pasa TODAS las políticas de main, incluida `egress.rego`. Es el caso que preguntaste.
- **Asimetría:** `egress: null` con `ipBlock` en `egress` sí lo detiene `egress.rego`. La brecha afecta sobre todo al `ipBlock` en `ingress`, que ninguna otra regla cubre.
- **Severidad:** NARANJA y no ROJO.
  - Es un bypass real de una invariante (`ipBlock` prohibido en `aqs-test`).
  - Hay que escribir deliberadamente el `null` en un manifiesto que pasa por PR en GitOps.
  - Egress, el vector de exfiltración, sigue cubierto.
- **Arreglo sugerido, tarea nueva:** tratar `null` como `[]`, por ejemplo con una función auxiliar `lista(x)` que devuelva `[]` si `not is_array(x)`. Aplicarla también a `to` y `from` de cada regla. Añadir pruebas negativas con `egress: null` e `ingress: null`.

## Tareas candidatas
- **M-01:** la de arriba (NARANJA sobre main, `policy/security.rego:33-34`). Conviene barrer con ella la misma clase de `null` en `to` y `from`, y revisar si `egress.rego` necesita cubrir `ingress`.
- **C-28:** bindings en otros namespaces. Arbitrada, sin cambios.
- **C-30:** `aqs-reset` en `security_test.rego:63`. Arbitrada, sin cambios.

## Notas
- El worktree `wt-U5-T09` quedó limpio.
- `git status` del repo principal muestra `aidlc-docs/audit.md` modificado y `.serena/` sin seguimiento. Ya estaban así antes de mi revisión y no son del bucle de U5-T09.

```
VEREDICTO: VERDE
INFORME: revisiones/U5-T09/ronda-3.md
```
