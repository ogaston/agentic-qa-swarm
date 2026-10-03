# U5-T13 — Bootstrap de Flux por clúster (candidata C-07)

**Unidad:** U5 — Plataforma & GitOps (endurecimiento previo al despliegue)
**Historias que implementa:** US-M10 (M10: "Control plane + entorno warm desplegados y reconciliados por Flux")
**Depende de:** U5 completa. Ola 5, en paralelo con U5-T15. U5-T14 (Secrets) va después, porque toca los mismos `Kustomization` de Flux.
**Origen:** C-07. Hoy los `Kustomization` `aqs-dev` y `aqs-prod` apuntan a un `GitRepository` `agentic-qa-swarm` en `flux-system` que no existe en el repo, y además viven dentro del mismo overlay que aplican.

---

## Alcance

**Dentro** (una línea, concreta):

> Crear `deploy/flux/clusters/{dev,prod}/` con:
> - `flux-system/gotk-components.yaml`, generado con `flux install --export` de **flux-cli v2.4.0** con los componentes `source-controller`, `kustomize-controller`, `helm-controller` y `notification-controller`, sin editarlo a mano;
> - `flux-system/gotk-sync.yaml`, con:
>   - el `GitRepository` `agentic-qa-swarm` (url `https://github.com/ogaston/agentic-qa-swarm`, `ref.branch: main`), porque el repo es público y no necesita secret;
>   - el `Kustomization` `flux-system`, con `path: ./deploy/flux/clusters/<env>`;
> - `aqs.yaml`, con el `Kustomization` `aqs-<env>` que **se mueve** desde `deploy/flux/<env>/flux-kustomization.yaml`.
>
> Se borra ese archivo de los overlays y se quita de sus `resources`, para que ningún overlay se aplique a sí mismo.
>
> Escribir también `docs/operaciones/bootstrap-flux.md`, con el procedimiento único de arranque que ejecuta un humano y la verificación con `flux get kustomizations`.

Supuesto de diseño: **un clúster por entorno** (dev y prod). Si se quisieran los dos entornos en un mismo clúster, la estructura cambia; se registra en la bitácora.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Secrets y descifrado (SOPS u otros): es **U5-T14**. No añadas `decryption` a ningún `Kustomization`.
- Cualquier cambio en `deploy/flux/base/**`. En los overlays, solo se borra `flux-kustomization.yaml` y su entrada en `resources`.
- Workflows de CI (**U5-T15**, en paralelo) y `policy/`.
- Ejecutar `flux bootstrap`, `kubectl apply` o cualquier comando contra un clúster. El arranque solo se **documenta**.

---

## Archivos de contexto

- `tareas/candidatas.md` (C-07; C-08 y C-23 como contexto)
- `deploy/flux/dev/` y `deploy/flux/prod/` (kustomizations y `flux-kustomization.yaml` actuales)
- `bitacoras/U5-T04.md` (sección "Decisiones no fijadas": de dónde salen `flux-system` y `agentic-qa-swarm`)
- `aidlc-docs/inception/requirements/requirements.md` (M10, AR9 Flux)

---

## Criterios de aceptación

Desde la raíz del worktree. Se usan estos alias:

```bash
K='docker run --rm --security-opt label=disable -v '"$PWD"':/w -w /w registry.k8s.io/kustomize/kustomize:v5.4.3'
Y='docker run --rm -i --security-opt label=disable mikefarah/yq:4.44.3 -N'
KC='docker run --rm -i ghcr.io/yannh/kubeconform:v0.6.7 -strict -summary -schema-location default -schema-location https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json'
```

- [ ] **CA-1** — Existe la entrada de cada clúster.
  ```bash
  for e in dev prod; do test -f deploy/flux/clusters/$e/kustomization.yaml || { echo "FALTA $e"; exit 1; }; done; echo OK
  ```
  Esperado: `OK`. Antes de la tarea: `FALTA dev` (rojo inicial).

- [ ] **CA-2** — La fuente y las dos `Kustomization` de Flux de cada clúster son las correctas.
  ```bash
  for e in dev prod; do $K build deploy/flux/clusters/$e | $Y 'select(.kind == "GitRepository" or .apiVersion == "kustomize.toolkit.fluxcd.io/v1") | .kind + "/" + .metadata.namespace + "/" + .metadata.name + " " + (.spec.url // .spec.path) + " " + (.spec.ref.branch // .spec.sourceRef.name)' | sort; done
  ```
  Esperado:
  ```
  GitRepository/flux-system/agentic-qa-swarm https://github.com/ogaston/agentic-qa-swarm main
  Kustomization/flux-system/aqs-dev ./deploy/flux/dev agentic-qa-swarm
  Kustomization/flux-system/flux-system ./deploy/flux/clusters/dev agentic-qa-swarm
  GitRepository/flux-system/agentic-qa-swarm https://github.com/ogaston/agentic-qa-swarm main
  Kustomization/flux-system/aqs-prod ./deploy/flux/prod agentic-qa-swarm
  Kustomization/flux-system/flux-system ./deploy/flux/clusters/prod agentic-qa-swarm
  ```

- [ ] **CA-3** — Los componentes de Flux están fijados a v2.4.0 con los 4 controladores, y coinciden con el export oficial.
  ```bash
  $K build deploy/flux/clusters/prod | $Y 'select(.kind == "Deployment" and .metadata.namespace == "flux-system") | .metadata.name' | sort
  grep -h 'app.kubernetes.io/version:' deploy/flux/clusters/*/flux-system/gotk-components.yaml | sort -u
  docker run --rm ghcr.io/fluxcd/flux-cli:v2.4.0 install --export --components=source-controller,kustomize-controller,helm-controller,notification-controller | diff -q - deploy/flux/clusters/prod/flux-system/gotk-components.yaml && echo IGUAL
  ```
  Esperado: `helm-controller`, `kustomize-controller`, `notification-controller`, `source-controller`; luego solo `app.kubernetes.io/version: v2.4.0`; luego `IGUAL`. El archivo de dev también debe ser igual al export.

- [ ] **CA-4** — Ningún overlay contiene ya un objeto de Flux, para que ninguno se aplique a sí mismo.
  ```bash
  for e in dev prod; do $K build deploy/flux/$e | $Y 'select(.apiVersion == "kustomize.toolkit.fluxcd.io/v1") | .metadata.name' | wc -l; done; ls deploy/flux/dev/flux-kustomization.yaml deploy/flux/prod/flux-kustomization.yaml 2>&1 | grep -c 'No such file'
  ```
  Esperado: `0`, `0` y `2`.

- [ ] **CA-5** — `kubeconform` estricto sobre los clústeres y los overlays. En los clústeres solo se omiten los 10 CRDs de Flux, que kubeconform no puede validar.
  ```bash
  for e in dev prod; do $K build deploy/flux/clusters/$e | $KC -skip CustomResourceDefinition -; $K build deploy/flux/$e | $KC -; done
  ```
  Esperado: en los clústeres, `Invalid: 0, Errors: 0, Skipped: 10`; en los overlays, `Invalid: 0, Errors: 0, Skipped: 0`.

- [ ] **CA-6** — `flux build` (sin clúster) reconstruye cada `aqs-<env>` desde su `Kustomization`, sobre una copia del repo.
  ```bash
  for e in dev prod; do t=$(mktemp -d); git archive HEAD | tar -x -C "$t"; chmod -R a+rwX "$t"; docker run --rm --security-opt label=disable -v "$t":/w -w /w ghcr.io/fluxcd/flux-cli:v2.4.0 build kustomization aqs-$e --path ./deploy/flux/$e --kustomization-file ./deploy/flux/clusters/$e/aqs.yaml --dry-run | grep -c '^kind:'; echo "$e rc=${PIPESTATUS[0]}"; rm -rf "$t"; done
  ```
  Esperado: en cada entorno, un número ≥ 40 y `rc=0`.

- [ ] **CA-7** — Las políticas siguen pasando sobre los overlays.
  ```bash
  C='docker run --rm -i --security-opt label=disable -v '"$PWD"':/project -w /project openpolicyagent/conftest:v0.56.0'
  for e in dev prod; do $K build deploy/flux/$e | $C test --no-color --policy policy --all-namespaces - >/dev/null; echo "$e rc=$?"; done
  ```
  Esperado: `dev rc=0` y `prod rc=0`.

- [ ] **CA-8** — Existe el procedimiento de arranque y cubre lo necesario.
  ```bash
  grep -c -i -E '^#+ .*(requisitos|arranque|bootstrap|verificaci|secrets)' docs/operaciones/bootstrap-flux.md
  grep -c -E 'flux get kustomizations|deploy/flux/clusters/(dev|prod)' docs/operaciones/bootstrap-flux.md
  grep -c 'bootstrap-flux.md' docs/operaciones/README.md
  ```
  Esperado: un número ≥ 4, otro ≥ 3, y `1`.

- [ ] **CA-9** — Árbol limpio tras el commit.
  ```bash
  git status --short | wc -l
  ```
  Esperado: `0`.

---

## Plan de pruebas

- Rojo inicial: la salida literal de CA-1 sobre la base.
- Prueba negativa de CA-3, sobre una copia: alterar una línea de `gotk-components.yaml` hace que el `diff` deje de decir `IGUAL`.
- Prueba negativa de CA-4, sobre una copia: devolver `flux-kustomization.yaml` a un overlay hace que su conteo deje de ser `0`.

**Rojo primero:** el codificador registra en su bitácora la salida literal de CA-1 antes de crear nada.

---

## Notas

- `conftest` se aplica a los overlays y no a `clusters/`. Los componentes de Flux traen `ClusterRole` y `ClusterRoleBinding` legítimos, y la política de la app los prohíbe a propósito. Hay que documentarlo en `bootstrap-flux.md`.
- El procedimiento debe decir que los Secrets de la app (C-08) se resuelven en U5-T14, y que sin ellos MinIO, el backup y Grafana no arrancan.
- Archivos temporales: siempre en tu propio `mktemp -d`. Cada salida pegada en la bitácora empieza con `pwd`.
- La bitácora pega el **comando literal** de cada criterio y su salida. El CA-9 posterior al último commit va en el informe de vuelta, con una nota en la bitácora que lo diga.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
