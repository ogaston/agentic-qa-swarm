# Ronda 1 — U5-T14

VEREDICTO: NO-VERDE

Hash de la tarea: bd712b16854c1969503166028c9388076ac4fad6 (coincide). HEAD del worktree: 00624d9. El worktree quedó limpio (`git status --short` da 0 líneas) y no quedó ningún `mktemp` mío (el `/tmp/tmp.26pOAch9Nu` es anterior a mi sesión, del 10:04, y no contiene claves).

## Prioridad 1: historial de la rama
`git log -p 47c3c26..tarea/U5-T14` (2 commits) no contiene `AGE-SECRET-KEY-`, `PRIVATE KEY`, `hunter2` ni claves `age1...` largas, ni archivos `*.sops.yaml`. Las coincidencias con `.sops.yaml` son solo texto de scripts y docs. `git ls-files` no lista ningún `.sops.yaml`, `.key` ni `.pem`. **Limpio.**

## Prioridad 2: temporales y limpieza
- Corrí `generate.sh` con un shim de `docker` que se detiene en la llamada a sops. Le envié INT, TERM y HUP al grupo de procesos, como haría un Ctrl-C. En los tres casos murió y el `mktemp -d` se borró (el `trap EXIT` funciona). Un fallo a mitad de camino (`set -e`) también limpia. Con SIGKILL queda el directorio, lo cual es inevitable.
- `secrets-e2e.sh` limpia con trap (el segundo trap incluye `$t` y `$t2`) y no dejó residuos tras la corrida completa.
- No hay `chown` en los contenedores, así que no quedan archivos imborrables. Con Docker real como root también se podrían borrar, porque `plain/` es del usuario.
- `generate.sh` solo toca el árbol al final (`rm` más `cp`), después de cifrar los 6. El fallo sin `BACKUP_*` deja solo `kustomization.yaml`.
- Los valores no pasan por argv ni por variables de entorno de contenedor. Los valores `BACKUP_*` con caracteres raros (`'`, `: #`, `$(id)`, `"`, `\`) hacen viaje de ida y vuelta intactos.
- **Defecto: ver F-02.**

## Prioridad 3: validez de los valores generados
Los verifiqué descifrando con una clave desechable:
- `tls.crt`: SAN `minio.aqs-system.svc` y `minio.aqs-system.svc.cluster.local`, RSA 2048, SHA-256.
- `openssl verify -CAfile ca.crt tls.crt` da OK. La clave pública del certificado y `tls.key` coinciden. La CA tiene `CA:TRUE`.
- `kms-secret-key` es `aqs-kms:` más 44 caracteres base64, que decodifican a 32 bytes.
- Las contraseñas son hex aleatorio de 48 caracteres. Los usuarios `aqs-minio` y `admin` son fijos.
- Cada Secret cifrado trae `ENC[` y bloque `sops:` con `mac`. Todo **correcto**.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Descifrado en dev y prod | CA-1 literal | pasa: `sops/sops-age` x2 |
| 2 | E2E: 6 Secrets cifrados que cubren el contrato | `bash scripts/test/secrets-e2e.sh` (16 s) | pasa: gen rc=0, 6, lista exacta |
| 3 | Sin valores en claro, overlay construye, formatos | mismo script | pasa: 0, 0, 6, 1, 1 |
| 4 | La guardia detecta el Secret en claro | CA-4 literal | pasa: `arbol rc=0`, `claro rc=1`; sobre los 6 cifrados, `rc=0` |
| 5 | `policies.sh` verde e incluye la guardia | CA-5 literal | pasa: todo OK, `rc=0` |
| 6 | Sin claves ni valores, alcance limpio, shellcheck | CA-6 literal | pasa: `0`, `0`, `0`, `shellcheck rc=0`, `0` |

Fidelidad de `secrets-e2e.sh` frente a CA-2 y CA-3: es fiel. Las comprobaciones y las salidas esperadas son las mismas. `--entrypoint sh` es el único cambio y está justificado. No se salta ni debilita nada.

Negativas, todas con el resultado esperado:
- Sin `BACKUP_*`: rc=2.
- Sin destinatario: rc=2.
- Secret con `sops:` y un valor sin `ENC[`: rc=1.

Los dos `decryption` están en `aqs-dev` y `aqs-prod`. Los overlays solo ganaron `- secrets`. El diff de `policies.sh` es solo la adición de `check-secrets`, y no se tocó `base/`, `gotk-*` ni `policy/`. Todo está dentro del alcance.

## Prioridad 4: intentos de evadir `check-secrets.sh`
Los cuatro casos que pediste fallan cerrado (rc=1):
- Secret con `data:` en base64 sin cifrar.
- Secret en un `.yml`.
- Secret con `sops:` y un valor sin `ENC[`.
- Secret dentro de un archivo con varios documentos (en primera y en segunda posición).

También fallan cerrado: un YAML inválido con `kind: Secret`, un valor numérico, `stringData` escalar y los alias YAML. Hay otras evasiones reales, en F-01.

## Hallazgos

### F-01 · NARANJA · `scripts/ci/check-secrets.sh:23` · la guardia falla abierta por el prefiltro `grep` (clase: descubrimiento de archivos)
La guardia solo analiza los archivos que cumplan `grep -rlE '^kind:[[:space:]]*Secret[[:space:]]*$' --include='*.yaml' --include='*.yml'`. Todo Secret que no cumpla esa regex se ignora **sin avisar**. Lo comprobé en un árbol copiado, con `stringData: {password: hunter2}` y sin `sops:`. Todas estas variantes dieron **rc=0, o sea pasan**:
- `kind: Secret # comentario`. Es YAML muy común, y es la evasión más grave por lo trivial.
- `kind: "Secret"` y `kind: 'Secret'`.
- Estilo flow en una línea: `{apiVersion: v1, kind: Secret, ...}`.
- Archivo `.json` (kustomize lo admite).
- Un Secret como item de un `kind: List`.

Barrido de clase: el defecto es único (el prefiltro de texto sustituye a un análisis estructural). Cualquier variante sintáctica lo esquiva. El análisis con yq (que sí es estructural) ya existe, pero solo se ejecuta sobre los archivos que pasan el grep.
Arreglo esperado: analizar con yq todos los `*.y*ml` y `*.json` bajo `deploy/`, filtrando por `.kind == "Secret"` y recorriendo `.items[]`. Añadir pruebas negativas para cada variante.
La bitácora solo declara como límites JSON y `secretGenerator`. Las otras cuatro variantes no están declaradas.

### F-02 · NARANJA · `scripts/secrets/generate.sh:41` · los secretos en claro quedan legibles por otros usuarios durante la ejecución
`chmod 755 "$work"` abre el `mktemp -d`, que nace con 700. Medido con un shim que hace `ls` y `stat` justo antes de cifrar:
- `/tmp/tmp.XXXX` queda en 755 y `plain/` en 755.
- `minio-root-password`, `warm-db-password`, `grafana-admin-password` y `kms` quedan en **644**.
- Los 6 YAML en claro (`backup-target.yaml`, `minio-tls.yaml` con la `tls.key` embebida, etc.) quedan en **644**. Solo `tls.key` suelto tiene 600.

Cualquier usuario local puede leerlos mientras corre el script. En un repo público con valores reales, eso es un fallo de higiene.
El `chmod 755` es innecesario: con Podman rootless la raíz del contenedor es el usuario, y con Docker la raíz atraviesa cualquier directorio. Quitarlo, poner `umask 077` al inicio y usar `chmod 600` en `plain/*`.
AMARILLO relacionado (la clave es desechable): `secrets-e2e.sh` hace `chmod -R a+rwX "$t"`, que deja el propio `$t` en 777 y `clave.txt` en 644. La tarea lo pide así en CA-2, pero se puede limitar a lo necesario.

### F-03 · AMARILLO · `scripts/test/secrets-e2e.sh` · el script imprime pero no afirma
Termina siempre con rc=0 (último comando: `echo`), aunque una salida difiera de la esperada. La tarea pedía solo que imprima las salidas. Aun así, un script de pruebas que no puede fallar es débil. Sugerencia: comparar contra los valores esperados y salir distinto de cero.

### F-04 · AMARILLO · `check-secrets.sh:21` · «`ENC[`» como prefijo y `sops:` solo no nulo
`stringData: {password: "ENC[hunter2"}` junto con `sops: {mac: x}`, o con `sops: {}`, pasa (rc=0). Es lo que la tarea literalmente pide («`ENC[...]` y bloque `sops:` presente»). Un patrón más estricto, como `^ENC\[AES256_GCM,data:` y exigir `.sops.mac` y `.sops.age` o `.sops.lastmodified`, cerraría el hueco barato.

## Tareas candidatas (defectos reales fuera de alcance)
- **C-A (alta prioridad): romper el CI al primer uso.** Lo comprobé generando dev y prod en una copia con una clave desechable y corriendo `policies.sh` completo sobre ese árbol: `FALLA kubeconform dev` y `FALLA kubeconform prod`, rc=1. Los 6 Secrets cifrados tienen una clave `sops` de nivel superior y kubeconform en modo estricto la rechaza (`additionalProperties 'sops' not allowed`).
  - El flujo que el propio diseño prescribe (generar, commitear los `*.sops.yaml`, PR) deja `policies.yml` en rojo. Quien lo use lo descubrirá en su primer PR real.
  - Arreglarlo exige tocar la comprobación existente de kubeconform en `policies.sh` (por ejemplo `-skip Secret`, o validar sin `secrets/`), y la tarea lo prohíbe expresamente. Por eso no lo cuento como defecto del codificador. El orquestador debe decidir si lo resuelve ampliando el alcance o con una tarea nueva.
  - `secrets-e2e.sh` no corre `policies.sh` sobre el árbol generado, y por eso no lo detectó.
- **C-B:** la guardia no cubre `secretGenerator` con `literals:`/`files:` ni `HelmRelease.values` con credenciales. La bitácora admite el primero.
- **C-C:** `generate.sh` no es atómico si `cp` falla a mitad (después del `rm -f`). Es un riesgo menor.

## Rutas de transcripciones largas
No pegué ninguna. Mi salida de `secrets-e2e.sh` coincide línea por línea con la de `/home/omarjayg/Javeriana/topicos-especiales/wt-U5-T14/bitacoras/U5-T14.md`.

VEREDICTO: NO-VERDE
NARANJA|scripts/ci/check-secrets.sh:23|La guardia falla abierta (prefiltro grep): `kind: Secret # c`, `kind: "Secret"`, flow, JSON y `List` pasan con rc=0
NARANJA|scripts/secrets/generate.sh:41|`chmod 755` en el directorio de trabajo deja los secretos en claro con modo 644 legibles por otros usuarios durante la ejecución
INFORME: revisiones/U5-T14/ronda-1.md
