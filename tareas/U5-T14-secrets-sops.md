# U5-T14 — Secrets de la plataforma con SOPS + age (candidata C-08)

**Unidad:** U5 — Plataforma & GitOps (endurecimiento previo al despliegue)
**Historias que implementa:** US-M10 (NF-SEG: ningún secret en claro en el repo)
**Depende de:** U5-T13 (`deploy/flux/clusters/<env>/aqs.yaml`) y U5-T15 (`scripts/ci/policies.sh`), las dos fusionadas en main antes de despachar esta tarea.
**Origen:** C-08 y C-18. Decisión del humano: **opción A, SOPS + age**. El repo es **público**.

---

## Alcance

**Dentro** (una línea, concreta):

> **1. Descifrado en Flux.** Añadir a `deploy/flux/clusters/{dev,prod}/aqs.yaml` el bloque `decryption: {provider: sops, secretRef: {name: sops-age}}`. El Secret `sops-age` (en `flux-system`, con la clave privada) lo crea **a mano** un humano y **nunca** entra al repo.
>
> **2. Estructura cifrada.** Crear `deploy/flux/{dev,prod}/secrets/kustomization.yaml` con `resources: []` e incluir `secrets` en los `resources` de cada overlay. El script de generación es quien rellena esa lista.
>
> **3. Generación.** Crear `scripts/secrets/generate.sh <env> <age-recipient>`, que ejecuta un **humano** (no el agente, salvo en pruebas con una clave desechable). El script:
> - genera los valores de los Secrets que referencian los manifiestos: contraseñas aleatorias; una CA y un certificado de MinIO con SAN `minio.aqs-system.svc` y `minio.aqs-system.svc.cluster.local`; y `MINIO_KMS_SECRET_KEY` con el formato `aqs-kms:<base64 de 32 bytes>`;
> - lee los valores de `backup-target` de variables de entorno (`BACKUP_ENDPOINT`, `BACKUP_BUCKET`, `BACKUP_ACCESS_KEY_ID`, `BACKUP_SECRET_ACCESS_KEY`) y falla si falta alguna;
> - cifra cada Secret con `sops --age <recipient> --encrypted-regex '^(data|stringData)$'` en `deploy/flux/<env>/secrets/<nombre>.sops.yaml`;
> - actualiza el `kustomization.yaml` de `secrets/`;
> - no deja ningún archivo en claro: los temporales van en `mktemp -d` y se borran con `trap`.
>
> **4. Guardia.** Crear `scripts/ci/check-secrets.sh`, que falla si bajo `deploy/` hay algún objeto `kind: Secret` cuyos `data`/`stringData` no estén cifrados por SOPS (cada valor como `ENC[...]` y bloque `sops:` presente). Añadir su llamada a `scripts/ci/policies.sh`, con una línea de salida `OK|FALLA check-secrets`.
>
> **5. Documentación.** Escribir `docs/operaciones/secrets.md`: generar la clave age, crear `sops-age`, ejecutar el script, rotar, y qué hacer si se filtra la clave. Enlazarlo desde `docs/operaciones/README.md` y desde `bootstrap-flux.md`.

El **contrato** de Secrets que debe cubrir el script se obtuvo del build de prod en main:

| Namespace | Secret | Claves |
|---|---|---|
| `aqs-system` | `minio-root` | `root-user`, `root-password` |
| `aqs-system` | `minio-kms` | `kms-secret-key` |
| `aqs-system` | `minio-tls` | `tls.crt`, `tls.key`, `ca.crt` |
| `aqs-system` | `backup-target` | `endpoint`, `bucket`, `access-key-id`, `secret-access-key` |
| `aqs-test` | `warm-db-credentials` | `password` |
| `aqs-observability` | `grafana-admin` | `admin-user`, `admin-password` |

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- **Commitear cualquier Secret cifrado con una clave real o con valores reales.** El repo es público. En esta tarea solo se commitean el mecanismo y `secrets/kustomization.yaml` con `resources: []`. Las pruebas usan una clave age **desechable**, en `mktemp -d`, que se borra al terminar. Ninguna clave privada, de ningún tipo, puede entrar al repo.
- Un `.sops.yaml` con un destinatario inventado. El destinatario se pasa como argumento; si se crea un `.sops.yaml`, va sin `age:` y se documenta.
- Cambiar los nombres o las claves de los Secrets que referencian los manifiestos, o cualquier archivo de `deploy/flux/base/`.
- Cambiar `gotk-*.yaml`, `policy/` o las comprobaciones existentes de `policies.sh`. En ese archivo solo se **añade** `check-secrets`, con **una única excepción** aprobada por el humano tras la ronda 1 (C-A): la invocación de kubeconform puede añadir `-skip Secret`. Los Secrets cifrados por SOPS llevan una clave `sops:` de nivel superior que el esquema estricto rechaza, y su estructura la valida `check-secrets.sh`. Ningún otro kind se puede omitir.
- Ejecutar `kubectl`, `flux` o cualquier comando contra un clúster.

---

## Archivos de contexto

- `tareas/candidatas.md` (C-08, C-18)
- `deploy/flux/clusters/` y `docs/operaciones/bootstrap-flux.md` (U5-T13)
- `scripts/ci/policies.sh` y `.github/workflows/policies.yml` (U5-T15)
- `deploy/flux/base/minio/`, `backup/`, `observability/helmreleases.yaml` y `warm.yaml` (dónde y cómo se consumen los Secrets)
- `bitacoras/U5-T05.md` y `U5-T08.md` (claves y formatos decididos)

---

## Criterios de aceptación

Desde la raíz del worktree. Se usan estos alias:

```bash
K='docker run --rm --security-opt label=disable -v '"$PWD"':/w -w /w registry.k8s.io/kustomize/kustomize:v5.4.3'
Y='docker run --rm -i --security-opt label=disable mikefarah/yq:4.44.3 -N'
```

- [ ] **CA-1** — Flux descifra con SOPS en ambos clústeres.
  ```bash
  for e in dev prod; do $K build deploy/flux/clusters/$e | $Y 'select(.kind == "Kustomization" and .metadata.name == "aqs-'$e'") | .spec.decryption.provider + "/" + .spec.decryption.secretRef.name'; done
  ```
  Esperado: `sops/sops-age` dos veces. Antes de la tarea: dos líneas `/` (rojo inicial).

- [ ] **CA-2** — De extremo a extremo con una clave desechable, sobre una copia: el script genera los 6 Secrets, cifrados, y cubren exactamente el contrato.
  ```bash
  t=$(mktemp -d); git archive HEAD | tar -x -C "$t"; chmod -R a+rwX "$t"
  docker run --rm --security-opt label=disable -v "$t":/k alpine:3.20 sh -c 'apk add -q age >/dev/null 2>&1 && age-keygen -o /k/clave.txt 2>/dev/null && chmod 644 /k/clave.txt'
  R=$(grep -o 'age1[0-9a-z]*' "$t/clave.txt")
  (cd "$t" && BACKUP_ENDPOINT=https://s3.example.invalid BACKUP_BUCKET=b BACKUP_ACCESS_KEY_ID=a BACKUP_SECRET_ACCESS_KEY=s bash scripts/secrets/generate.sh prod "$R"); echo "gen rc=$?"
  ls "$t"/deploy/flux/prod/secrets/*.sops.yaml | wc -l
  docker run --rm --security-opt label=disable -v "$t":/w -w /w -e SOPS_AGE_KEY_FILE=/w/clave.txt ghcr.io/getsops/sops:v3.9.1-alpine sh -c 'for f in deploy/flux/prod/secrets/*.sops.yaml; do sops decrypt "$f"; echo ---; done' | $Y 'select(.kind == "Secret") | .metadata.namespace + "/" + .metadata.name + ":" + (((.data // {}) + (.stringData // {})) | keys | sort | join(","))' | sort
  rm -rf "$t"
  ```
  Esperado: `gen rc=0`, `6`, y exactamente:
  ```
  aqs-observability/grafana-admin:admin-password,admin-user
  aqs-system/backup-target:access-key-id,bucket,endpoint,secret-access-key
  aqs-system/minio-kms:kms-secret-key
  aqs-system/minio-root:root-password,root-user
  aqs-system/minio-tls:ca.crt,tls.crt,tls.key
  aqs-test/warm-db-credentials:password
  ```

- [ ] **CA-3** — Sobre la misma copia, tras generar: los archivos no contienen ningún valor en claro, el overlay construye con los Secrets cifrados, y los valores tienen el formato correcto.
  ```bash
  # (mismo prólogo que CA-2, con el script ya ejecutado sobre "$t")
  grep -L 'ENC\[' "$t"/deploy/flux/prod/secrets/*.sops.yaml | wc -l
  grep -l -E '^\s+(root-password|password|admin-password|kms-secret-key|secret-access-key|tls.key):\s+[^E]' "$t"/deploy/flux/prod/secrets/*.sops.yaml | wc -l
  (cd "$t" && docker run --rm --security-opt label=disable -v "$t":/w -w /w registry.k8s.io/kustomize/kustomize:v5.4.3 build deploy/flux/prod | grep -c '^kind: Secret')
  docker run --rm --security-opt label=disable -v "$t":/w -w /w -e SOPS_AGE_KEY_FILE=/w/clave.txt ghcr.io/getsops/sops:v3.9.1-alpine decrypt deploy/flux/prod/secrets/minio-kms.sops.yaml | grep -c -E 'aqs-kms:[A-Za-z0-9+/]{43}='
  docker run --rm --security-opt label=disable -v "$t":/w -w /w -e SOPS_AGE_KEY_FILE=/w/clave.txt ghcr.io/getsops/sops:v3.9.1-alpine decrypt --extract '["stringData"]["tls.crt"]' deploy/flux/prod/secrets/minio-tls.sops.yaml | docker run --rm -i alpine:3.20 sh -c 'apk add -q openssl >/dev/null 2>&1; openssl x509 -noout -ext subjectAltName' | grep -c 'minio.aqs-system.svc'
  ```
  Esperado: `0`, `0`, `6`, `1` y `1`. El codificador debe convertir CA-2 y CA-3 en un único script reproducible, `scripts/test/secrets-e2e.sh`, que imprima esas salidas, y pegar su salida literal. Si el script usa `data` en lugar de `stringData`, el `--extract` y los formatos se adaptan, y se documenta.

- [ ] **CA-4** — La guardia detecta un Secret en claro y deja pasar el árbol actual y uno cifrado.
  ```bash
  bash scripts/ci/check-secrets.sh; echo "arbol rc=$?"
  t=$(mktemp -d); git archive HEAD | tar -x -C "$t"; printf -- '---\napiVersion: v1\nkind: Secret\nmetadata: {name: fuga, namespace: aqs-system}\nstringData: {password: hunter2}\n' >> "$t/deploy/flux/base/minio/service.yaml"; (cd "$t" && bash scripts/ci/check-secrets.sh >/dev/null 2>&1); echo "claro rc=$?"; rm -rf "$t"
  ```
  Esperado: `arbol rc=0` y `claro rc=1`. Además, sobre la copia de CA-2, con los 6 Secrets cifrados: `rc=0`. Pega esa salida.

- [ ] **CA-5** — `policies.sh` incluye la guardia y todo sigue en verde.
  ```bash
  bash scripts/ci/policies.sh 2>&1 | grep -E '^(OK|FALLA) ' | sort | uniq -c | awk '{print $2, $3, $4}'
  bash scripts/ci/policies.sh >/dev/null 2>&1; echo "rc=$?"
  ```
  Esperado: todas las líneas `OK`, incluida `OK check-secrets`, y `rc=0`.

- [ ] **CA-6** — Ninguna clave privada ni ningún valor en claro en el repo, y alcance limpio.
  ```bash
  git grep -n -E 'AGE-SECRET-KEY-|BEGIN (RSA |EC )?PRIVATE KEY' -- . ':!tareas/*' ':!revisiones/*' | wc -l
  git ls-files deploy | grep -c '\.sops\.yaml$'
  git diff --name-only $(git merge-base HEAD origin/main) | grep -v -E '^(deploy/flux/clusters/(dev|prod)/aqs\.yaml|deploy/flux/(dev|prod)/kustomization\.yaml|deploy/flux/(dev|prod)/secrets/kustomization\.yaml|scripts/secrets/generate\.sh|scripts/ci/check-secrets\.sh|scripts/ci/policies\.sh|scripts/test/secrets-e2e\.sh|scripts/test/check-secrets-test\.sh|docs/operaciones/(secrets|bootstrap-flux|README)\.md|bitacoras/U5-T14\.md|\.sops\.yaml)$' | wc -l
  docker run --rm --security-opt label=disable -v "$PWD":/mnt koalaman/shellcheck:v0.10.0 scripts/secrets/generate.sh scripts/ci/check-secrets.sh scripts/test/secrets-e2e.sh; echo "shellcheck rc=$?"
  git status --short | wc -l
  ```
  Esperado: `0`, `0`, `0`, `shellcheck rc=0` y `0`.

- [ ] **CA-7** — *(Enmienda C-A, aprobada por el humano tras la ronda 1.)* La CI completa pasa sobre un árbol con los Secrets cifrados generados, y kubeconform sigue validando todo lo demás.
  ```bash
  bash scripts/test/secrets-e2e.sh 2>&1 | grep -E '^(policies-generado rc=|kubeconform-valid-generado )'
  grep -c -- '-skip Secret' scripts/ci/policies.sh
  grep -o -E -- '-skip [A-Za-z,]+' scripts/ci/policies.sh | sort -u
  ```
  Esperado:
  - `secrets-e2e.sh` corre `scripts/ci/policies.sh` completo sobre la copia con los Secrets cifrados de dev **y** prod, e imprime `policies-generado rc=0`.
  - Imprime también `kubeconform-valid-generado <N>`, con el número de recursos válidos de prod, que debe ser > 0.
  - La guarda de "cero trabajo" de U5-T15 sigue activa.
  - `1`, y `-skip Secret` como único `-skip` del script.
  - La prueba negativa de CA-4 (un Secret en claro) sigue dando `rc=1` también a través de `policies.sh`.

---

## Plan de pruebas

- Rojo inicial: la salida literal de CA-1 sobre la base.
- CA-2 y CA-3 son la prueba de extremo a extremo con la herramienta real (sops y age), automatizada en `scripts/test/secrets-e2e.sh`.
- Pruebas negativas:
  - `generate.sh` sin `BACKUP_*` falla y no deja archivos;
  - `generate.sh` sin destinatario falla;
  - `check-secrets.sh` con un Secret que tiene `sops:` pero un valor sin `ENC[` falla.

**Rojo primero:** el codificador registra en su bitácora la salida literal de CA-1 antes de cambiar nada.

---

## Notas

- `sops` corre con `ghcr.io/getsops/sops:v3.9.1-alpine`. `age-keygen` y `openssl` corren con `alpine:3.20` más `apk add`, porque la imagen de sops no los trae. El orquestador ya probó el ciclo cifrar/descifrar con una clave desechable.
- Una clave age es una línea `AGE-SECRET-KEY-…`: la guardia de CA-6 la detecta. Nunca se escribe una clave fuera de un `mktemp -d`.
- Si el contrato de la tabla no coincide con lo que ves en el build de main, repórtalo como bloqueo; no cambies los manifiestos.
- Archivos temporales: siempre en tu propio `mktemp -d`. Cada salida pegada en la bitácora empieza con `pwd`.
- La bitácora pega el **comando literal** de cada criterio y su salida. El último `git status` va en el informe de vuelta, con una nota en la bitácora que lo diga.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
