# Ronda 2 — U4-T08

VEREDICTO: VERDE

El revisor corrió los diez criterios y los cuatro hallazgos de la ronda 1 están resueltos. Sin ROJO ni NARANJA. Worktree limpio antes y después. Entre `6c26d0c` y HEAD no cambió nada en `deploy/`, `scripts/`, `services/` ni `contracts/`; aun así se repitió CA-1 a CA-10.

## Criterios de aceptación, verificados por el revisor
| # | Resultado |
|---|---|
| 1 | `Recreate 65532`; siete variables literales en orden; `go-governance-service-token/token valor=null` (tercer comando con `(.value // "null")`, arbitrado) |
| 2 | `IDENTITY_TRUST_PROXY=false`, `IDENTITY_USERS_FILE=/etc/aqs/identity/users.json`, `TZ=UTC`; `go-identity-users users.json users.json 288`; `/etc/aqs/identity true`; `65532 RollingUpdate` |
| 3 | `dev: dev`, `prod: prod`, `mismas variables` |
| 4 | `0`, `0`, `0`, `OK check-secrets` |
| 5 | build contra build de `origin/main` en base, dev y prod: cinco Deployments `igual`; Services y PVC iguales |
| 6 | `62 Valid, Invalid: 0, Errors: 0, Skipped: 0` en dev y prod (con el shim de `--network host`, `HTTPS_PROXY` y CA) |
| 7 | `verify` 124/124; build real 1426/1426 en dev y prod; seis negativos `rc=1`; 21 pruebas nuevas |
| 8 | `policies.sh` con el shim: 11 `OK`, ninguna `FALLA` (incluye `rbac-matrix`, `check-secrets` y `kubeconform`) |
| 9 | contadores 10/10/2/6/2/2/2; 2 líneas eliminadas |
| 10 | `0`, `0`, `37` líneas en `control-plane.yaml` |

CI del PR #28 sobre `145c06b` (eventos `pull_request` y `push`): `ci`, `contracts` y `policies` en success.

## Verificación de los hallazgos de la ronda 1
- **F-01 (procedimiento SOPS): resuelto.** La sección 6.3 es un solo bloque con un `d`, un `trap` y `umask 077`. Con un `docker` falso que falla si recibe `sops` tras la imagen y comprueba el montaje en `/in`: `rc` 0, `OK: 2 Secrets cifrados`, ambos `*.sops.yaml` escritos; ambos `docker run` montan el mismo directorio y pasan `encrypt ...`. Sin residuos en `/tmp` ni con éxito ni con fallo inyectado en el primer o el segundo cifrado, y sin escribir ningún `*.sops.yaml` en esos fallos. Marcadores sin sustituir: aborta (`quedan marcadores <...> en users.json`, 0 archivos, 0 llamadas a `docker`); `AGE_RECIPIENT` literal con marcador: error de sintaxis y no se ejecuta nada; `ENV_NAME=../x`: aborta. Sin fuga del token, los hashes ni el `mfa_secret` a stdout, stderr o al log de argumentos. Flags de `sops` idénticos a `generate.sh:108` (`encrypt --age <dest> --encrypted-regex '^(data|stringData)$' /in/<n>.yaml`, `grep -q 'ENC\['`); diferencias sin consecuencia: plano y cifrado en el mismo directorio, sin la guarda `find -perm /077`, sin `rm -f` ni reescritura de `kustomization.yaml`. `bash -n` pasa, ejecuta con `set -u` y `pipefail`, `shellcheck -s bash` 0 avisos. `go run ./cmd/go-identity hash-password` produce `$argon2id$v=19$m=19456,t=2,p=1...`; con ese hash y un `mfa_secret` de `head -c 20 /dev/urandom | base32` el `users.json` levanta go-identity con `users:2` y `/readyz` = 200. Limitación: el SOPS real no se ejecutó (imagen bloqueada); ensayo con entrypoint simulado, como acordó el arbitraje.
- **F-02 (política e `initContainers`): resuelto.** 60 mutantes (`Deployment`, `StatefulSet`, `Job`, `CronJob` por `containers`, `initContainers` y `ephemeralContainers` con los cinco sufijos): todos `rc=1`. Pasan con `rc=0`: initContainer con `valueFrom.secretKeyRef`, nombres parecidos sin sufijo (`A_TOKENS`, `TOKEN_FILE`, `A_SECRETS`), initContainer sin `env`, el mismo caso en `aqs-test`. `value: ""` en un initContainer: `rc=1`; un contenedor bueno con un initContainer malo: `rc=1`. Sin falsos positivos sobre el build real (1426/1426). Quitar `initContainers` o `ephemeralContainers` hace fallar las pruebas correspondientes.
- **F-03 y F-04: resueltos y veraces.** La sección 6 y 6.4 documentan `CreateContainerConfigError` (go-governance) y `ContainerCreating` con `FailedMount` (go-identity) y `kubectl describe pod`; el comando de obtención del binario funciona.
- **gitleaks:** HEAD y `origin/main` dan los mismos 5 aciertos preexistentes; el diff no añade ninguno.

## Hallazgos (AMARILLO, no bloquean)
### F-05 · `docs/operaciones/secrets.md`, 6.3 · el código de salida del bloque no refleja el fallo
El `unset ENV_NAME AGE_RECIPIENT` final corre después del subshell y deja `$?` en 0. El mensaje de error se imprime y no se escribe nada, pero quien encadene el bloque en un script no verá el fallo.

### F-06 · `policy/secretrefs_test.rego` · cobertura parcial por kind
Las pruebas de `initContainers` cubren solo Deployment y CronJob, y `ephemeralContainers` solo un kind. Los mutantes del revisor sí cubren `StatefulSet` y `Job` y pasan, porque el código es genérico.

## Tareas candidatas (fuera de alcance)
- Extender la política a nombres en minúsculas o mixtos, `Pod`, `DaemonSet`, `ReplicaSet`, y `args`, `command` y `envFrom` sensibles.
- Extender `generate.sh` con `go-governance-service-token` y `go-identity-users`.

VEREDICTO: VERDE
AMARILLO|docs/operaciones/secrets.md 6.3|El código de salida del bloque no refleja el fallo (`unset` final enmascara el `rc`); no bloquea
AMARILLO|policy/secretrefs_test.rego|Pruebas de initContainers solo en Deployment y CronJob; no bloquea
INFORME: revisiones/U4-T08/ronda-2.md
