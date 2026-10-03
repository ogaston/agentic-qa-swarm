# Aislamiento de la plataforma (U4-T05)

Todo son manifiestos revisables bajo `deploy/flux/base/security/`. Nada de esto se aplica
a mano: lo reconcilia Flux y lo verifican `scripts/ci/policies.sh` y `scripts/ci/rbac-matrix.sh`.

## Que aisla cada capa

| Capa | Que hace | Donde |
|---|---|---|
| ServiceAccount propio | `ui-api`, `go-intake`, `go-governance` y `go-identity` no usan `default`, sin permisos y sin token montado (`automountServiceAccountToken: false` en SA y pod) | `serviceaccounts.yaml`, `control-plane.yaml` |
| RBAC minimo | Un solo Role (`aqs-test-operator`) en `aqs-test`; sin ClusterRole, sin wildcards, sin `secrets`, `pods/exec` ni escalada | `rbac.yaml` + `policy/rbac.rego` |
| NetworkPolicy `aqs-test` | default-deny de ingress y egress, sin salida al LLM desde los runners | `networkpolicies.yaml` |
| NetworkPolicy `aqs-system` | default-deny de **ingress** y permisos explicitos | `networkpolicies-system.yaml` |
| Rego | Deniega los huecos anteriores y `hostNetwork`/`hostPID`/`hostIPC` en `aqs-test` | `policy/*.rego` |
| Matriz RBAC | Permisos efectivos del build contra `rbac-matriz.csv`, sin cluster | `scripts/ci/rbac-matrix.sh` |

## Ingress de aqs-system

- `allow-same-namespace`: trafico entre pods de `aqs-system`.
- `allow-metrics-from-observability`: desde `aqs-observability`, puerto 8080/TCP.
- `allow-public-http`: solo `go-intake` y `ui-api`, puerto 8080/TCP, desde namespaces con la
  etiqueta `aqs.io/ingress-controller: "true"`.

El operador debe poner esa etiqueta al namespace de su ingress controller:
`kubectl label namespace <ns-del-controlador> aqs.io/ingress-controller=true`.
Sin la etiqueta el trafico externo queda **bloqueado** (falla cerrado).

## Matriz kubectl auth can-i esperada (solo en dev, la ejecuta el humano)

```bash
kubectl auth can-i create jobs -n aqs-test --as=system:serviceaccount:aqs-system:go-run-controller    # yes
kubectl auth can-i create jobs -n default --as=system:serviceaccount:aqs-system:go-run-controller     # no
kubectl auth can-i create jobs -n aqs-system --as=system:serviceaccount:aqs-system:go-reset           # no
kubectl auth can-i get secrets -n aqs-test --as=system:serviceaccount:aqs-system:go-warm-manager      # no
kubectl auth can-i create pods/exec -n aqs-test --as=system:serviceaccount:aqs-system:go-reset        # no
kubectl auth can-i get pods -n aqs-test --as=system:serviceaccount:aqs-test:aqs-runner                # no
kubectl auth can-i get pods -n aqs-system --as=system:serviceaccount:aqs-system:ui-api                # no
kubectl auth can-i get pods -n aqs-system --as=system:serviceaccount:aqs-system:go-intake             # no
kubectl auth can-i get pods -n aqs-system --as=system:serviceaccount:aqs-system:go-governance         # no
kubectl auth can-i get pods -n aqs-system --as=system:serviceaccount:aqs-system:go-identity           # no
```

## Lo que queda abierto

- **C-51:** `aqs-system` no tiene reglas de **egress**: el control plane necesita la API de
  Kubernetes, MinIO, el LLM y GitHub, y esas direcciones dependen del cluster.
- `aqs-observability` no tiene NetworkPolicy (resto de C-12).
- El Role `aqs-test-operator` no se toco aqui (`deployments/scale`, PVC: C-29, depende de U2).
