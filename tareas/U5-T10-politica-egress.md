# U5-T10 — Política contra egress abierto en `aqs-test` (candidata C-19)

**Unidad:** U5 — Plataforma & GitOps
**Historias que implementa:** US-M10 (refuerza US-M8.2: runners sin egress ni acceso a LLM)
**Depende de:** U5-T06 (fusionada). Ola 4, en paralelo con U5-T08 y U5-T09.
**Origen:** hallazgo del revisor de U5-T06, ronda 1. Hoy `egress: [{}]` o `to: [{namespaceSelector: {}}]` en `aqs-test` pasan las políticas.

---

## Alcance

**Dentro** (una línea, concreta):

> Añadir `policy/egress.rego` y `policy/egress_test.rego`, con una regla que, para toda NetworkPolicy de `aqs-test` con reglas de `egress`, solo acepte dos tipos de regla:
> - **(a) intra-namespace:** cada peer de `to` es solo un `podSelector`, sin `namespaceSelector` ni `ipBlock`.
> - **(b) DNS:** cada peer de `to` es un `namespaceSelector` con `matchLabels` `kubernetes.io/metadata.name: kube-system` (con o sin `podSelector`), y los `ports` son únicamente el 53 en UDP y/o TCP.
>
> Cualquier otra regla se deniega:
> - `to` vacío o ausente (abierto a todo destino);
> - `namespaceSelector: {}` (todos los namespaces);
> - cualquier otro namespace;
> - `matchExpressions`;
> - `ipBlock`;
> - DNS con un puerto distinto de 53, o sin `ports`.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Modificar `policy/security.rego`, `policy/security_test.rego`, `policy/default_deny.rego` o los archivos de U5-T09 (`policy/isolation*.rego`). La regla nueva va en archivos nuevos.
- Modificar cualquier NetworkPolicy o manifiesto. Las actuales ya cumplen la regla; si alguna no cumple, se reporta como bloqueo y no se cambia.
- Reglas de ingress o de otros namespaces (candidata C-12).
- Integrar conftest en CI (candidata C-20).
- Cualquier comando contra un clúster.

---

## Archivos de contexto

- `tareas/candidatas.md` (C-19)
- `revisiones/U5-T06/ronda-1.md` (sección de candidatas: el hueco de `egress: [{}]`)
- `deploy/flux/base/security/networkpolicies.yaml` (las 4 políticas actuales, que deben seguir pasando)
- `policy/` (estilo y convenciones de las reglas existentes)
- `aidlc-docs/inception/requirements/requirements.md` (M8: NetworkPolicy namespace-only que bloquea el acceso a LLM)

---

## Criterios de aceptación

Desde la raíz del worktree. Se usan estos alias:

```bash
K='docker run --rm --security-opt label=disable -v '"$PWD"':/w -w /w registry.k8s.io/kustomize/kustomize:v5.4.3'
C='docker run --rm -i --security-opt label=disable -v '"$PWD"':/project -w /project openpolicyagent/conftest:v0.56.0'
```

- [ ] **CA-1** — El hueco existe antes y queda cerrado después: `egress: [{}]` en `aqs-test`, concatenado al build real.
  ```bash
  { $K build deploy/flux/prod; printf -- '---\napiVersion: networking.k8s.io/v1\nkind: NetworkPolicy\nmetadata: {name: abierta, namespace: aqs-test}\nspec: {podSelector: {}, policyTypes: [Egress], egress: [{}]}\n'; } | $C test --no-color --policy policy --all-namespaces - >/dev/null; echo "rc=$?"
  ```
  Esperado: `rc=1`. Antes de la tarea: `rc=0` (rojo inicial: el hueco existe).

- [ ] **CA-2** — Las pruebas unitarias de todas las políticas pasan.
  ```bash
  $C verify --no-color --policy policy; echo "rc=$?"
  ```
  Esperado: `rc=0`, con al menos 9 pruebas nuevas en `egress_test.rego`. Al menos una prueba que pasa para (a) y para (b), y al menos una que falla para cada caso denegado del alcance.

- [ ] **CA-3** — El build real sigue pasando: las 4 NetworkPolicies actuales cumplen la regla.
  ```bash
  for e in dev prod; do $K build deploy/flux/$e | $C test --no-color --policy policy --all-namespaces -; echo "rc=$?"; done
  ```
  Esperado: `rc=0` dos veces, con `0 failures`.

- [ ] **CA-4** — Cada caso denegado falla sobre el build real, y los dos permitidos no.
  ```bash
  t=$(mktemp -d); $K build deploy/flux/prod > "$t/b.yaml"
  np() { printf -- '---\napiVersion: networking.k8s.io/v1\nkind: NetworkPolicy\nmetadata: {name: f, namespace: aqs-test}\nspec: {podSelector: {}, policyTypes: [Egress], egress: [%s]}\n' "$1"; }
  for c in \
    'abierta|{}' \
    'solo-puertos|{ports: [{port: 443}]}' \
    'todos-ns|{to: [{namespaceSelector: {}}]}' \
    'otro-ns|{to: [{namespaceSelector: {matchLabels: {kubernetes.io/metadata.name: aqs-system}}}]}' \
    'expr|{to: [{namespaceSelector: {matchExpressions: [{key: x, operator: Exists}]}}]}' \
    'ipblock|{to: [{ipBlock: {cidr: 10.0.0.0/8}}]}' \
    'dns-443|{to: [{namespaceSelector: {matchLabels: {kubernetes.io/metadata.name: kube-system}}}], ports: [{port: 443, protocol: TCP}]}' \
    'dns-sin-puertos|{to: [{namespaceSelector: {matchLabels: {kubernetes.io/metadata.name: kube-system}}}]}' \
    'ok-intra|{to: [{podSelector: {}}]}' \
    'ok-dns|{to: [{namespaceSelector: {matchLabels: {kubernetes.io/metadata.name: kube-system}}}], ports: [{port: 53, protocol: UDP}, {port: 53, protocol: TCP}]}'; do
    n=${c%%|*}; { cat "$t/b.yaml"; np "${c#*|}"; } | $C test --no-color --policy policy --all-namespaces - >/dev/null; echo "$n rc=$?"; done
  rm -rf "$t"
  ```
  Esperado: `rc=1` en los 8 primeros y `rc=0` en `ok-intra` y `ok-dns`.

- [ ] **CA-5** — Solo se añadieron los dos archivos nuevos y la bitácora.
  ```bash
  git diff --name-only $(git merge-base HEAD origin/main) | sort
  ```
  Esperado, exactamente: `bitacoras/U5-T10.md`, `policy/egress.rego` y `policy/egress_test.rego`.

- [ ] **CA-6** — Árbol limpio tras el commit.
  ```bash
  git status --short | wc -l
  ```
  Esperado: `0`.

---

## Plan de pruebas

- Rojo inicial: la salida literal de CA-1 sobre la base (`rc=0`, el hueco existe).
- CA-2 son las pruebas unitarias; CA-4 es la matriz de casos sobre el build real.

**Rojo primero:** el codificador registra en su bitácora la salida del comando literal de CA-1 antes de crear nada.

---

## Notas

- La política usa `import rego.v1`. Se recomienda un mensaje de denegación que nombre la NetworkPolicy y el índice de la regla de egress.
- La bitácora pega el **comando literal** de cada criterio y su salida. El CA-6 posterior al último commit va en el informe de vuelta, con una nota en la bitácora que lo diga.
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
