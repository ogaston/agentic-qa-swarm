# Ronda 1 — U7-T01

VEREDICTO: VERDE

Revisé HEAD `c392847` con mi propia ejecución de los seis criterios. Hash de la tarea en el worktree: `e57d8f6aa12e5511af9a32eaa5286c7c81d4fa2d`, el despachado. Precondición: `fs.inotify.max_user_instances = 512`. Al empezar, el contexto era `kind-ckad` y existían los clústeres ajenos `aqs-poc` y `ckad`. Copié `~/.kube/config` al scratchpad para comparar al final.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Imagen fijada y lint | `grep -c -E 'image: kindest/node:v[0-9.]+@sha256:[0-9a-f]{64}' deploy/kind/cluster.yaml`; `bash -n scripts/kind/*.sh`; shellcheck v0.10.0 por podman (el binario local no está instalado) con `wc -l` | `1`, bash -n rc=0, `0` líneas. El contenedor sí corrió (`--version` imprime 0.10.0, rc=0). |
| 2 | Creación y lectura de vuelta | `bash scripts/kind/kind-up.sh`, luego `kubectl config current-context`, `kubectl get nodes`, `kubectl get storageclass` | rc=0 en 30 s. Salida: `kind-aqs`, `Ready`, `1`. |
| 3 | Idempotencia | segundo `kind-up.sh`; `kind get clusters \| grep -c '^aqs$'` | rc=0 y `1`. Mensaje «ya existe; no se recrea». La edad del nodo pasó de 24 s a 25 s y `podman inspect` da la misma fecha de creación (`18:08:10.984`) antes y después. |
| 4 | La guarda rechaza otro contexto | los comandos del criterio | `contexto aqs-guard-test no es kind-aqs` y `rc=3`. Restauración a `kind-aqs` y borrado del contexto de prueba correctos. |
| 5 | Destrucción | `kind-down.sh` dos veces, `kind get clusters`, `podman ps -a` | rc=0, `0`, `0` y segundo rc=0 («no existe; nada que hacer»). |
| 6 | Alcance | los dos comandos del criterio | `0` y `0`. El diff neto toca solo `deploy/kind/` (2 archivos), `scripts/kind/` (3 archivos) y `bitacoras/U7-T01.md`. `git diff $b -- aidlc-docs` está vacío. |

Verificaciones adicionales:
- **Digest de la imagen:** `podman inspect aqs-control-plane` muestra `docker.io/kindest/node@sha256:a1ed56cf…`, el mismo de `cluster.yaml`. Ese digest es también el que `strings $(which kind)` extrae del binario kind 0.33.0, que es lo que la tarea pide.
- **Guarda con contexto ajeno:** con `kind-ckad` activo, `kind-up.sh` sobre un clúster existente exporta el kubeconfig y deja `kind-aqs`. La tarea lo permite para ese script.
- **Requisitos:** con un PATH sin `kind`, `kind-up.sh` sale rc=1 con «falta 'kind' en el PATH…» y no crea nada.
- **Limpieza:** `kind-down.sh` no toca `aqs-poc`, `ckad` ni `hermes-1a36faa8` (los contenedores de podman siguen ahí). El kubeconfig final es byte a byte idéntico al inicial.

## Decisión sobre el punto abierto: el contexto tras `kind delete cluster`
No es defecto dentro del alcance. Va como nota de documentación y tarea candidata.
- Lo comprobé: creé `aqs` y puse `kind-ckad` como contexto actual. `kind-down.sh` terminó con rc=0 y `kubectl config current-context` siguió dando `kind-ckad`.
- Cuando se corre `kind-down.sh` estando en `kind-aqs`, kind borra el contexto activo y `current-context` queda vacío. Eso lo hace `kind delete cluster`, no un `use-context` del script. El efecto aparece solo si el usuario venía de `kind-up.sh`, que la tarea autoriza a cambiar el contexto.
- La tarea no pide restaurar el contexto previo ni da dónde guardarlo. Hacerlo exigiría un mecanismo de estado que el alcance no declara, y eso sí sería desborde.
- Los criterios no lo miden: CA-5 comprueba rc, `kind get clusters` y `podman ps`. CA-4 deja `kind-aqs` y no vuelve a probarse después de destruir.

## Hallazgos
### F-01 · AMARILLO · deploy/kind/README.md · No documenta que `kind-down.sh` deja `current-context` vacío
Si el contexto activo es `kind-aqs`, tras destruir el clúster `kubectl` queda sin contexto actual. Salida: `error: current-context is not set`. Basta una línea en el README (hacer `kubectl config use-context <el-que-tenías>` después). No bloquea.

### F-02 · AMARILLO · bitácora ronda 1 · Guarda sin prueba roja previa
El propio codificador declara que no capturó el rojo de `require_kind_context` antes de escribirla. Es honesto y la guarda queda probada con salida real (CA-4), pero es una desviación del procedimiento «rojo primero». No bloquea.

### F-03 · AMARILLO · bitácora ronda 1 · Hash de tarea desactualizado en el acuse
El acuse de la ronda 1 cita `ba7ae5b…`, de una versión anterior de la tarea. La ronda 2 lo corrige a `e57d8f6…`, que coincide con lo que verifiqué. Sin consecuencia.

No hay ROJO ni NARANJA.
- **Literales:** el digest de la imagen y el timeout de 180 s son literales que la tarea exige. `KIND_CLUSTER_NAME` y `KIND_CONTEXT` se definen una sola vez en `lib.sh`.
- **Lectura de vuelta:** los criterios leen el estado con `kubectl`, `kind` y `podman`, no solo con códigos de salida.
- **Tiempos:** la creación tardó 30 s, así que no hay señal de pruebas saltadas.
- **Alcance:** el commit de auditoría `ddbfe41` y su reversión `c392847` se anulan en el diff neto, y CA-6 da `0`.

## Tareas candidatas (defectos reales fuera de alcance)
- **Restaurar el contexto previo en `kind-down.sh`:** guardar el contexto anterior a `kind-up.sh`, por ejemplo en un archivo bajo `$XDG_STATE_HOME`, y restaurarlo si el activo era `kind-aqs`. Requiere ampliar la regla de «scripts no cambian el contexto actual». Prioridad baja.
- **Hacer más robusto el CA-4 de la tarea:** su restauración final usa `use-context kind-aqs`. Si el clúster ya no existe, ese paso falla, como le pasó al codificador en la ronda 1. Se arregla recordando el contexto original y restaurándolo.

## Rutas de transcripciones largas
- Bitácora del codificador: `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U7-T01/bitacoras/U7-T01.md`
- Respaldo del kubeconfig inicial, que usé para la comparación: `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/72f12caf-6243-4c0e-8d38-3c9cbf7ef62e/scratchpad/kubeconfig.bak`

VEREDICTO: VERDE
AMARILLO|deploy/kind/README.md|No documenta que kind-down deja current-context vacío si el activo era kind-aqs
AMARILLO|bitácora ronda 1|Guarda escrita sin prueba roja previa (declarado)
AMARILLO|bitácora ronda 1|Hash de tarea desactualizado en el acuse (corregido en ronda 2)
INFORME: revisiones/U7-T01/ronda-1.md
