# Clúster kind local `aqs` (U7-T01)

Clúster **efímero y local** para ejercitar los manifiestos de la plataforma con un clúster real.
**Nunca se apunta a dev ni a prod**: toda operación de `scripts/kind/` exige el contexto `kind-aqs`
(la guarda `require_kind_context` sale con código `3` en cualquier otro contexto).

## Requisitos (Fedora)

- `kind` 0.33.0, `kubectl` y `podman` instalados.
- cgroup v2 (`stat -fc %T /sys/fs/cgroup` debe devolver `cgroup2fs`).
- Podman **sin root** (recomendado): el cgroup del usuario debe delegar `cpu`, `memory` y `pids`.
  En Fedora suele bastar con `systemctl --user` activo y el servicio de podman del usuario.
- Podman **con root**: no requiere delegación; ejecutar los scripts como root en ese caso.

`kind-up.sh` comprueba estos requisitos y, si algo falta, sale con un mensaje sin crear nada.

## Uso

```bash
bash scripts/kind/kind-up.sh     # crea (o reutiliza) el clúster aqs; deja kubectl en kind-aqs
bash scripts/kind/kind-down.sh   # destruye el clúster aqs; idempotente
```

- `kind-up.sh` es idempotente: si `aqs` ya existe no lo recrea y sale `0`.
- `kind-down.sh` solo borra `aqs`; no toca otros clústeres ni contenedores de podman.
- El nodo se espera con `kubectl wait --for=condition=Ready node --all --timeout=180s`.

## Configuración

- `cluster.yaml`: un nodo `control-plane`, kindnet como CNI (aplica NetworkPolicy), sin
  `extraPortMappings`. La imagen de nodo es `kindest/node:v1.37.0` fijada por **digest**
  (la que kind 0.33.0 declara por defecto); prohibido un tag sin digest.
- Acceso a la plataforma solo por `kubectl port-forward` en loopback (sin ingress).

## Advertencia

El clúster `aqs` es efímero y local. Sus Secrets y datos no son de ningún entorno compartido.
Cualquier cambio de contexto hacia dev o prod está fuera de este flujo.
