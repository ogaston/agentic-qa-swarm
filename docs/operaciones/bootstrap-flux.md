# Bootstrap de Flux por clúster

Procedimiento único de arranque, ejecutado por un humano. Se asume **un clúster por entorno** (dev y prod). Si se quisieran ambos entornos en un mismo clúster, la estructura de `deploy/flux/clusters/` cambia.

## Requisitos

- Un clúster Kubernetes vacío por entorno y `kubectl` apuntando a él.
- `flux` CLI v2.4.0.
- Acceso de lectura al repo `https://github.com/ogaston/agentic-qa-swarm` (es público: el `GitRepository` no necesita secret).
- Los Secrets de la app (C-08) se resuelven en U5-T14. Sin ellos, MinIO, el backup y Grafana no arrancan.

## Estructura

Cada clúster tiene su entrada en `deploy/flux/clusters/<env>/`:

- `flux-system/gotk-components.yaml`: generado con `flux install --export --components=source-controller,kustomize-controller,helm-controller,notification-controller` (v2.4.0). No se edita a mano.
- `flux-system/gotk-sync.yaml`: `GitRepository` `agentic-qa-swarm` y `Kustomization` `flux-system`.
- `aqs.yaml`: `Kustomization` `aqs-<env>`, que aplica `deploy/flux/<env>`.

Los overlays `deploy/flux/<env>` no contienen objetos de Flux, para que ninguno se aplique a sí mismo.

## Arranque

Una sola vez por clúster, con el contexto de `kubectl` del entorno correcto:

```bash
# dev
kubectl apply -f deploy/flux/clusters/dev/flux-system/gotk-components.yaml
kubectl apply -f deploy/flux/clusters/dev/flux-system/gotk-sync.yaml
# prod: lo mismo con deploy/flux/clusters/prod/...
```

A partir de ahí, Flux se reconcilia solo: `flux-system` aplica `deploy/flux/clusters/<env>`, que incluye `aqs-<env>`.

## Verificación

```bash
flux get sources git -n flux-system
flux get kustomizations -n flux-system
```

Esperado: `flux-system` y `aqs-<env>` con `READY=True`.

## Políticas

`conftest` se aplica a los overlays (`deploy/flux/<env>`) y no a `deploy/flux/clusters/`: los componentes de Flux traen `ClusterRole` y `ClusterRoleBinding` legítimos, y la política de la app los prohíbe a propósito.

## Secrets

Pendiente de U5-T14 (C-08): descifrado de los Secrets de la app. Hasta entonces `aqs-<env>` queda reconciliado pero MinIO, el backup y Grafana no arrancan.
