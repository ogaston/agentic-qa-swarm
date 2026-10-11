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

## Imágenes (U7-T02)

Las imágenes de la plataforma (hoy 10: 8 de `services/*`, incluida `aqs-runner` desde U8-T03, y 2 de
`agents/agent-*`) se construyen con podman y se cargan en el clúster `aqs` sin registro. La cuenta no
es fija: sale de los Dockerfile que hay en el repo.

```bash
bash scripts/kind/build-images.sh                       # construye todas con podman
bash scripts/kind/load-images.sh                        # las carga en kind-aqs (exige el contexto kind-aqs)
AQS_IMAGES="ui-api go-identity" bash scripts/kind/build-images.sh   # subconjunto
```

- La lista sale de `scripts/ci/list-services.sh` (directorios con `Dockerfile`). `scripts/kind/lib.sh` la
  cruza con un conteo independiente de `services/*/Dockerfile` y `agents/*/Dockerfile`; si no coinciden
  o la lista está vacía, el script falla. Ya no hay un número fijo que mantener a mano.
- Contexto de build = directorio del servicio (igual que `ci.yml`). Tag `ghcr.io/ogaston/agentic-qa-swarm/<nombre>:0.0.0`; sin `latest`.
- Una imagen que no construye corta el script con su nombre.
- `load-images.sh` hace `podman save --format docker-archive` a un directorio temporal (borrado al salir) y `kind load image-archive --name aqs`.
- No se publica en ningún registro ni se firma; eso lo hace la CI.

## Despliegue de la plataforma (U7-T03)

```bash
bash scripts/kind/secrets.sh   # crea en kind-aqs los Secrets que referencia deploy/flux/kind (idempotente)
bash scripts/kind/deploy.sh    # secrets.sh → kubectl apply -k deploy/flux/kind → rollout de cada Deployment/StatefulSet → Job minio-init
```

- `deploy/flux/kind/` es el overlay de la plataforma sin observabilidad ni backups (kind no trae los CRDs
  de Flux ni de prometheus-operator, y el destino de backup es S3 externo). Usa `../base` completo y
  borra con parches `$patch: delete` los objetos de `observability` y `backup`; la desviación respecto a
  «piezas sueltas de base» y su motivo están en el comentario de `deploy/flux/kind/kustomization.yaml`.
- Los Secrets los genera `secrets.sh` con valores aleatorios y nunca van al repo. Las contraseñas de los
  usuarios demo `demo` (rol `user`) y `admin` (rol `admin`), y el `mfa_secret` del admin, quedan en
  `${XDG_RUNTIME_DIR:-/tmp}/aqs-kind/` con permisos `600`.
- **Prerrequisitos del despliegue:** `podman` (la imagen `go-identity:0.0.0` ya construida con `build-images.sh`:
  `secrets.sh` genera los hashes de `go-identity` con `podman run … hash-password`; sin podman no hay hash),
  `kubectl`, `openssl`, `jq` y `base32`. No se usa el Go del host.
- `secrets.sh` crea también el ConfigMap `go-reset-baseline` de **relleno** (`aqs.io/kind-stub=true`) si no existe:
  `clean` sale `0`, `verify` imprime `0`, `version` imprime `kind-stub`. No verifica nada: el reset verificado
  en kind queda `PENDIENTE` (U7-T04). Si ya existe (p. ej. un baseline real aplicado a mano), no se toca.
- `secrets.sh` descubre los Secrets desde `kubectl kustomize deploy/flux/kind`; si uno no está en su lista,
  falla con su nombre antes de crear nada.
- `deploy.sh` imprime `OK|FALLA <objeto>` por cada comprobación y sale `0` solo si todas pasan. Exige el
  contexto `kind-aqs` (sale `3` en otro contexto). Un objeto con reinicios de pod cuenta como `FALLA`
  aunque en un instante tenga una réplica `Ready`.
- `scripts/ci/policies.sh` valida también el overlay `kind` (kubeconform estricto, conftest, default-deny)
  sin relajar ninguna política.

## Advertencia

El clúster `aqs` es efímero y local. Sus Secrets y datos no son de ningún entorno compartido.
Cualquier cambio de contexto hacia dev o prod está fuera de este flujo.
