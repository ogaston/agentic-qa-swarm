# Ronda 1 — U4-T08

VEREDICTO: NO-VERDE

Los diez criterios pasan al correrlos el revisor, pero hay dos hallazgos NARANJA que bloquean: uno en la documentación y otro en la política. Worktree limpio; todo lo del revisor en el scratchpad.

## Criterios de aceptación, verificados por el revisor
| # | Resultado |
|---|---|
| 1 | `Recreate 65532`, siete variables literales en orden, `go-governance-service-token/token valor=null` (tercer comando con `(.value // "null")`, defecto de especificación arbitrado) |
| 2 | `IDENTITY_TRUST_PROXY=false`, `IDENTITY_USERS_FILE=...`, `TZ=UTC`; `go-identity-users users.json users.json 288`; `/etc/aqs/identity true`; `65532 RollingUpdate` |
| 3 | `dev: dev`, `prod: prod`, `mismas variables` |
| 4 | `0`, `0`, `0` y `OK check-secrets` |
| 5 | build contra build: los cinco Deployments `igual` en dev y prod; Services y PVC iguales; el diff son 35 líneas añadidas, todas en `go-governance` y `go-identity` |
| 6 | `62 Valid, Invalid: 0, Errors: 0, Skipped: 0` en dev y prod (con el shim `--network host`, `HTTPS_PROXY` y CA del proxy) |
| 7 | `verify` 117/117; build real 1426/1426 en dev y prod; los seis negativos se deniegan; 14 pruebas nuevas |
| 8 | `policies.sh` con el shim: 11 `OK`, ninguna `FALLA` (sin el shim fallan los dos `kubeconform` por el proxy del entorno) |
| 9 | contadores `6/6/1/4/1/2/2`; 2 líneas eliminadas |
| 10 | `0`, `0`, `37` líneas cambiadas en `control-plane.yaml`; sin tocar `scripts/`, `services/` ni `contracts/` |

CI del PR #28 sobre `6c26d0c` (HEAD): `ci`, `contracts` y `policies` en success.

## Hallazgos
### F-01 · NARANJA · `docs/operaciones/secrets.md`, secciones 6.1 a 6.3 · El procedimiento de cifrado no funciona al seguirlo al pie de la letra
Seguidos los bloques bash en una copia con un `docker` falso que simula el entrypoint de SOPS:
1. **`sops sops --encrypt`.** El comando es `docker run ... ghcr.io/getsops/sops:v3.9.1-alpine sops --encrypt ...`, pero el entrypoint de esa imagen ya es `sops`. `scripts/secrets/generate.sh:108` pasa `encrypt ...` sin repetir `sops`, y `secrets-e2e.sh:34` usa `--entrypoint sh` para ejecutar `sh`. Habría que quitar `sops` o usar `--entrypoint sops` (la imagen real no pudo ejecutarse: está bloqueada).
2. **Dos `$d` distintos.** 6.1 y 6.2 hacen cada uno `d=$(mktemp -d)` con su `trap ... EXIT`; 6.3 monta solo el directorio de 6.2 (`-v "$d":/in:ro`), así que el YAML de `go-governance-service-token` no se monta (`cifrado fallido: go-governance-service-token`). Con una shell por sección, `$d` ya no existe en 6.3.
3. **Fuga del token en claro.** El segundo `trap` reemplaza al primero: el directorio de 6.1 sigue en `/tmp` con el token en claro (`token`, 65 bytes, y el YAML del Secret).
Funciona: `printf '%s' "<pass>" | go-identity hash-password` produce un hash `argon2id`; `head -c 20 /dev/urandom | base32` da 32 caracteres y go-identity lo acepta para un admin (con 16 bytes lo rechaza: `se exigen al menos 160`); un admin sin `mfa_secret` se rechaza; un `user` con `mfa_secret` se acepta; el `users.json` con el formato del documento levanta go-identity con `/readyz` = 200.

### F-02 · NARANJA · `policy/secretrefs.rego:21` · La política ignora `initContainers`
La regla solo recorre `spec.template.spec.containers`. Un `initContainer` con `{name: A_TOKEN, value: abc}` pasa `conftest --all-namespaces` sobre todo el build; igual dentro de un `CronJob`. La tarea dice «variable de entorno de un contenedor de Deployment/StatefulSet/Job/CronJob» y un initContainer es un contenedor del pod. Hay que recorrer `initContainers` (y `ephemeralContainers`) y añadir prueba.

### F-03 · AMARILLO · `docs/operaciones/secrets.md`, sección 6 y 6.4 · el mecanismo de la advertencia de `/readyz` es impreciso
`go-governance` real con las variables del manifiesto: con token de 64 caracteres, `/healthz` 200 y `/readyz` 200 (`audit_chain`, `data_dir`, `identity`, `policies` en `ok`); sin token o con token corto el proceso termina con `GOVERNANCE_SERVICE_TOKEN debe tener al menos 32 caracteres`; con go-identity caído, `/readyz` 503 (`identity: fail`). En un clúster, sin los Secrets el proceso ni llega a `/readyz`: un `secretKeyRef` ausente deja `go-governance` en `CreateContainerConfigError` y un volumen `secret` ausente deja `go-identity` en `ContainerCreating` con `FailedMount` (razonado, no ejecutado). 6.4 debería indicar `kubectl describe pod` y esos estados.

### F-04 · AMARILLO · `docs/operaciones/secrets.md`, sección 6.2 · no se indica cómo obtener el binario `go-identity`
El comando se escribe como `go-identity hash-password` a secas (el README de go-identity también).

## Barrido de la política (mutantes del revisor, de a uno, sobre el build de prod)
Se deniegan: sufijos `TOKEN`, `SECRET`, `PASSWORD`, `PASSWD`, `SECRET_KEY` en mayúsculas; `value: ""`; `value` junto a `valueFrom`; nombre exacto `TOKEN` y `APITOKEN`; contenedor con varias variables; `StatefulSet`, `Job`, `CronJob`.
Pasan (solo F-02 es hallazgo de la tarea): `initContainers` en Deployment y CronJob; nombres en minúsculas o mixtos (`api_token`, `Foo_Token`); `Pod`, `DaemonSet`, `ReplicaSet`; `args`, `command` y `envFrom` con `configMapRef`; `valueFrom` con `configMapKeyRef`/`fieldRef` sobre un nombre sensible; `API_KEY`, `PRIVATE_KEY`, `PASSPHRASE`, `DB_CREDENTIALS`, `TOKEN_FILE`.
Alcance: workloads en `aqs-test`, `aqs-observability`, `default` o sin namespace no se evalúan. Sobre todo el build de dev y prod, ninguna variable cuyo nombre contenga token, secret, passw o key lleva `value` (las diez variables sensibles usan `valueFrom`: go-governance, MinIO, warm-db, backup, mc); sin falsos positivos.
Fuerza de las pruebas: 8 mutantes de `secretrefs.rego` (quitar `SECRET_KEY`, `PASSWD`, `CronJob`, `StatefulSet`, el filtro de namespace, aceptar solo `value: ""`, `endswith` por `contains`, anular la condición de `value`): cada uno falla al menos una prueba.

## Puntos pedidos
- (c) `Recreate` solo en `go-governance`; `go-identity` en `RollingUpdate`; `fsGroup` 65532 en ambos; los dos Dockerfile con `USER 65532:65532`. Razonado con el modelo de Kubernetes (no ejecutado): con `fsGroup`, los archivos del volumen secret quedan `root:65532` y `0440` da lectura al grupo; montaje `readOnly: true`; sin otros volúmenes en go-identity.
- (d) el Service `go-identity` existe en `aqs-system` con puerto `http` = 8080 y `IDENTITY_URL` coincide.
- (e) el parche de dev es estratégico por nombre: moviendo `GOVERNANCE_ENV` al primer lugar en una copia, dev da `dev`, prod `prod`, sin duplicados ni pérdidas.
- (g) `go-governance` arrancó con el entorno del manifiesto sin variables obligatorias faltantes; `GOVERNANCE_ENV` solo lo lee el código con `GOVERNANCE_AUTH=fake` y con `identity` solo lo registra (no es hallazgo: la tarea lo pide); `IDENTITY_TRUST_PROXY` e `IDENTITY_USERS_FILE` los lee go-identity.
- gitleaks: HEAD y `origin/main` dan los mismos 5 aciertos preexistentes (`obs_test.go`, tareas U4-T02/T07); el diff no añade ninguno.

## Tareas candidatas (fuera de alcance)
- Extender la política con nombres en minúsculas o mixtos, `Pod`, `DaemonSet`, `ReplicaSet`, y `args`, `command` y `envFrom` sensibles.
- `generate.sh` con `go-governance-service-token` y `go-identity-users` (ya prevista).

VEREDICTO: NO-VERDE
NARANJA|docs/operaciones/secrets.md secciones 6.1-6.3|El procedimiento de cifrado no funciona tal cual: `sops --encrypt` repetido tras el entrypoint de la imagen, `$d` pisado entre 6.1 y 6.2 (el YAML del token no se monta) y fuga del token en claro en /tmp por el `trap` reemplazado
NARANJA|policy/secretrefs.rego:21|La política ignora `initContainers`: un `A_TOKEN` con `value` literal en un initContainer pasa
INFORME: revisiones/U4-T08/ronda-1.md
