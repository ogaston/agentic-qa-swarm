# U5-T12 — La regla de `ipBlock` no se desactiva con `null` (candidata C-34)

**Unidad:** U5 — Plataforma & GitOps
**Historias que implementa:** US-M10 (refuerza US-M8.2)
**Depende de:** U5-T06 y U5-T10 (fusionadas). Ola 4, en paralelo con U5-T08 y U5-T11.
**Origen:** hallazgo M-01 de `revisiones/U5-T09/ronda-3.md`.

---

## Alcance

**Dentro** (una línea, concreta):

> En `policy/security.rego`, cambiar **solo** la regla de NetworkPolicy que prohíbe `ipBlock` en `aqs-test` (hoy líneas 30-36). Hay dos defectos que corregir:
> - `spec.egress`, `spec.ingress`, `rule.to` y `rule.from` con valor `null`, o que no sean un arreglo, deben tratarse como `[]`. Hoy `object.get` devuelve `null`, `array.concat` queda indefinido y la regla no se evalúa.
> - Cualquier peer que tenga la **clave** `ipBlock`, con el valor que sea, debe denegarse.
>
> Añadir al **final** de `policy/security_test.rego` las pruebas nuevas, sin modificar ni borrar las existentes.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Cualquier otra regla de `security.rego`, y los archivos `default_deny.rego`, `egress*.rego`, `isolation*.rego` y `namespace*.rego`. U5-T09 está en el PR #9 y U5-T11 en retrabajo.
- Modificar o borrar líneas existentes de `security_test.rego`. U5-T09 cambia su línea 28; si esta tarea solo añade al final, las dos ramas se fusionan sin conflicto.
- Cualquier manifiesto bajo `deploy/`.
- Cualquier comando contra un clúster.

---

## Archivos de contexto

- `revisiones/U5-T09/ronda-3.md` (sección M-01: diagnóstico y fixtures)
- `tareas/candidatas.md` (C-34)
- `policy/security.rego` y `policy/security_test.rego`
- `policy/egress.rego` (cubre el egress; esta tarea cierra el `ipBlock` en el ingress)

---

## Criterios de aceptación

Desde la raíz del worktree. Se usan estos alias:

```bash
K='docker run --rm --security-opt label=disable -v '"$PWD"':/w -w /w registry.k8s.io/kustomize/kustomize:v5.4.3'
C='docker run --rm -i --security-opt label=disable -v '"$PWD"':/project -w /project openpolicyagent/conftest:v0.56.0'
```

- [ ] **CA-1** — El bypass queda cerrado: `ipBlock` en el ingress con `egress: null`, concatenado al build real.
  ```bash
  t=$(mktemp -d); $K build deploy/flux/prod > "$t/b.yaml"
  { cat "$t/b.yaml"; printf -- '---\n%s\n' '{apiVersion: networking.k8s.io/v1, kind: NetworkPolicy, metadata: {name: a, namespace: aqs-test}, spec: {podSelector: {}, policyTypes: [Ingress], ingress: [{from: [{ipBlock: {cidr: 0.0.0.0/0}}]}], egress: null}}'; } | $C test --no-color --policy policy --all-namespaces - >/dev/null; echo "rc=$?"; rm -rf "$t"
  ```
  Esperado: `rc=1`. Antes de la tarea: `rc=0` (rojo inicial; el orquestador lo reprodujo sobre main).

- [ ] **CA-2** — Matriz de casos sobre el build real.
  ```bash
  t=$(mktemp -d); $K build deploy/flux/prod > "$t/b.yaml"
  f() { { cat "$t/b.yaml"; printf -- '---\n%s\n' "$1"; } | $C test --no-color --policy policy --all-namespaces - >/dev/null; echo "$2 rc=$?"; }
  f '{apiVersion: networking.k8s.io/v1, kind: NetworkPolicy, metadata: {name: b, namespace: aqs-test}, spec: {podSelector: {}, policyTypes: [Ingress], ingress: [{from: [{ipBlock: {cidr: 10.0.0.0/8}}]}], egress: "x"}}' egress-no-lista
  f '{apiVersion: networking.k8s.io/v1, kind: NetworkPolicy, metadata: {name: c, namespace: aqs-test}, spec: {podSelector: {}, policyTypes: [Ingress], ingress: [{from: [{ipBlock: {}}]}]}}' ipblock-vacio
  f '{apiVersion: networking.k8s.io/v1, kind: NetworkPolicy, metadata: {name: d, namespace: aqs-test}, spec: {podSelector: {}, policyTypes: [Ingress], ingress: [{from: [{ipBlock: null}]}]}}' ipblock-null
  f '{apiVersion: networking.k8s.io/v1, kind: NetworkPolicy, metadata: {name: e, namespace: aqs-test}, spec: {podSelector: {}, policyTypes: [Ingress], ingress: [{from: null}, {from: [{ipBlock: {cidr: 10.0.0.0/8}}]}], egress: null}}' from-null-mas-ipblock
  f '{apiVersion: networking.k8s.io/v1, kind: NetworkPolicy, metadata: {name: g, namespace: aqs-test}, spec: {podSelector: {}, policyTypes: [Ingress], ingress: [{from: [{podSelector: {}}]}], egress: null}}' ok-ingress-intra-egress-null
  f '{apiVersion: networking.k8s.io/v1, kind: NetworkPolicy, metadata: {name: h, namespace: aqs-system}, spec: {podSelector: {}, policyTypes: [Ingress], ingress: [{from: [{ipBlock: {cidr: 10.0.0.0/8}}]}], egress: null}}' ok-otro-namespace
  rm -rf "$t"
  ```
  Esperado: `rc=1` en los 4 primeros, `rc=0` en `ok-ingress-intra-egress-null` y `ok-otro-namespace` (la regla es solo para `aqs-test`).

- [ ] **CA-3** — Todas las políticas pasan sus pruebas y el build real.
  ```bash
  $C verify --no-color --policy policy; echo "verify rc=$?"
  for e in dev prod; do $K build deploy/flux/$e | $C test --no-color --policy policy --all-namespaces - >/dev/null; echo "$e rc=$?"; done
  ```
  Esperado: `verify rc=0`, con al menos 5 pruebas más que en main; `dev rc=0`, `prod rc=0`.

- [ ] **CA-4** — Solo se tocaron los archivos permitidos, y en `security_test.rego` solo se añadieron líneas.
  ```bash
  b=$(git merge-base HEAD origin/main); git diff --name-only $b | sort
  git diff --numstat $b -- policy/security_test.rego | cut -f2
  ```
  Esperado: `bitacoras/U5-T12.md`, `policy/security.rego` y `policy/security_test.rego`; y luego `0` (ninguna línea borrada en las pruebas).

- [ ] **CA-5** — Árbol limpio tras el commit.
  ```bash
  git status --short | wc -l
  ```
  Esperado: `0`.

---

## Plan de pruebas

- Rojo inicial: la salida literal de CA-1 sobre la base (`rc=0`).
- En `security_test.rego` añadir, como mínimo, una prueba negativa por cada caso con `rc=1` de CA-1 y CA-2, y una positiva con `egress: null` y un ingress intra-namespace.
- Mutación: en una copia en tu `mktemp -d`, vuelve a poner el `object.get` sin normalizar y comprueba que las pruebas nuevas fallan.

**Rojo primero:** el codificador registra en su bitácora la salida del comando literal de CA-1 antes de cambiar nada.

---

## Notas

- `import rego.v1`. Se sugiere una función auxiliar (por ejemplo `lista(x) := x if is_array(x)` y `lista(x) := [] if not is_array(x)`) que se aplique a los cuatro campos.
- Archivos temporales: siempre en tu propio `mktemp -d`. Cada salida pegada en la bitácora empieza con `pwd`.
- La bitácora pega el **comando literal** de cada criterio y su salida. El CA-5 posterior al último commit va en el informe de vuelta, con una nota en la bitácora que lo diga.
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
