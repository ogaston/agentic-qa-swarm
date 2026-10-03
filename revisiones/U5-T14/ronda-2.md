# Ronda 2 — U5-T14

VEREDICTO: NO-VERDE

Hash de la tarea: b945bc39... (coincide). HEAD del worktree: 16939f2. Al terminar, el worktree sigue limpio (`git status --short` da 0 líneas). Borré mis `mktemp`. Queda `/tmp/tmp.26pOAch9Nu`, que ya existía en la ronda 1 y no es mío.

Estado de los hallazgos de la ronda 1:
- F-02 y F-03 están cerrados.
- F-04 queda como AMARILLO.
- F-01 no está cerrado. La guardia ya no falla abierta por el prefiltro `grep`, pero el análisis estructural nuevo tiene evasiones propias (ver F-05 y F-06).

## 1. Historial completo
`git log -p fd60529..tarea/U5-T14` (5 commits, 986 líneas) no contiene `AGE-SECRET-KEY-`, `PRIVATE KEY` ni `age1...` largas. Tampoco contiene `ENC[AES256_GCM,data:` con payload, ni blobs en base64 o hex largos, ni ningún `*.sops.yaml`, `.key` o `.pem` añadido. El único `hunter2` es el literal de prueba de las pruebas negativas. `git ls-files deploy | grep -c '\.sops\.yaml$'` da 0. **Limpio.**

## 2. Evasiones del parseo de `check-secrets.sh`
Corrí cada caso contra el árbol copiado. Lo esperado era rc=1, salvo donde se indica.

| Caso | Devolvió |
|---|---|
| Secret en claro (línea base) | rc=1 |
| Valor con tabulación | rc=1 |
| Clave `"foo ENC[AES256_GCM,data:x"` con valor en claro | rc=1 |
| Valor `hunter2 ENC[AES256_GCM,data:x` | rc=1 |
| Documentos vacíos (`---`) antes y después del Secret | rc=1 |
| Archivo con solo documentos vacíos (esperado rc=0) | rc=0 |
| BOM | rc=1 |
| CRLF | rc=1 |
| `data: {}` con `stringData` en claro | rc=1 |
| `data` en claro con `sops.mac` y sin `ENC` | rc=1 |
| `binaryData` en claro | rc=1 |
| Nombre de archivo con espacios | rc=1 |
| Nombre de archivo con `\|` (`x\|y.yaml`, `a\|Secret\|b.yaml`) | rc=1 |
| Array JSON de Secrets | rc=1 (yq falla: "cannot index array with 'kind'", falla cerrado) |
| Array YAML de Secrets | rc=1 (falla cerrado) |
| `metadata.name` numérico o nulo, o sin `metadata` | rc=1 |
| Valor tipo lista o tipo mapa | rc=1 |
| Anclas y merge key | rc=1 |
| `sops.mac: ""` | rc=1 |
| `name: "x\|1\|0"` (pipes sin salto de línea) | rc=1 |
| Claves duplicadas con el claro **primero** | rc=0, pero kustomize rechaza ese archivo ("mapping key already defined"), así que no es evasión real |
| **`name: "x\|1\|0\n"`** (pipes más salto final) | **rc=0, evadido** (F-05) |
| Lo mismo con `name: \|` en bloque y un 2º Secret trampa en multi-doc | **rc=0, evadido** (F-05) |
| **`data` en claro, y `stringData` con la misma clave en `ENC[AES256_GCM,data:x`** | **rc=0, evadido** (F-05) |
| **Valor `"ENC[AES256_GCM,data:x\nhunter2"`** | **rc=0, evadido** (F-06) |
| Valor `ENC[AES256_GCM,data:hunter2` con `mac: x` | rc=0 (F-06) |
| **Archivos `a.YAML`, `a.txt`, `secret`, `a.yaml.tpl`, `a.jsonc`** | **rc=0, evadidos** (F-07) |
| `List` anidada, `SecretList` | rc=0, sin tratar (AMARILLO, F-08) |

## 3. F-02 (permisos): cerrado
- Usé `TMPDIR` propio (700) y `AQS_STAT_DEBUG=1`. La salida muestra que el directorio de trabajo, `plain/` y `enc/` están en 700, y que los 13 archivos de `plain/` están en 600. No hay ningún `chmod 755`.
- Un sondeo cada 20 ms de `find -perm /077` durante toda la ejecución dio 0 aciertos.
- Para probar la guarda de permisos, forcé en una copia `kms` a 644, `minio-tls.yaml` a 640, `enc/` a 755 y `tls.key` a 604. En los 4 casos `generate.sh` abortó con rc=1 y "permisos demasiado abiertos". El árbol quedó intacto y no quedó ningún directorio temporal.

## 4. F-03 (`secrets-e2e.sh`): cerrado
Con un `generate.sh` que genera solo 5 Secrets, el script imprime `[DIFIERE] 6 archivos: obtenido='5'`, `[DIFIERE] contrato de Secrets` y `[DIFIERE] Secrets en build` (obtenido 5, esperado 6). Termina con `E2E FALLA`, **rc=1**. Sobre el árbol real, `E2E OK` con rc=0 en 47 s.

## 5. CA-7 y C-A
- Contra la base de la tarea, el diff de `policies.sh` son dos cambios y nada más: `-skip Secret` en la línea de kubeconform, y el bloque nuevo de `check-secrets`.
- `grep -c -- '-skip Secret'` da 1, y ese es el único `-skip` del script.
- `policies-generado rc=0` es real: `policies.sh` completo sobre dev y prod generados dio 8 líneas `OK`, y `kubeconform-valid-generado 51`.
- La guarda de cero trabajo sigue activa. Con una entrada solo de Secret, kubeconform da rc=0 pero `Valid: 0`, y el `n>0` de `policies.sh` lo convierte en fallo.
- La prueba negativa a través de `policies.sh` da `claro via policies rc=1` y `FALLA check-secrets`.

## Criterios de aceptación, verificados por mí
| # | Comando que corrí | Resultado |
|---|---|---|
| CA-1 | literal | pasa: `sops/sops-age` x2 |
| CA-2 | literal. Nota: con el alias tal cual, el `sh -c` de la imagen sops falla porque su entrypoint es `sops`. El codificador usa `--entrypoint sh` en `secrets-e2e.sh`; el defecto está en la redacción de la tarea. | pasa: `gen rc=0`, 6, contrato exacto (vía `secrets-e2e.sh`) |
| CA-3 | literal y vía el script | pasa: 0, 0, 6, 1, 1 |
| CA-4 | literal, más `secrets-e2e.sh` | pasa: `arbol rc=0`, `claro rc=1`, cifrados `rc=0` |
| CA-5 | literal | pasa: 8 líneas OK, incluida `OK check-secrets`; rc=0 |
| CA-6 | literal | pasa: 0, 0, 0, `shellcheck rc=0`, 0 |
| CA-7 | literal | pasa: `policies-generado rc=0`, `kubeconform-valid-generado 51`, 1, `-skip Secret` |
| Negativas | `check-secrets-test.sh`, `generate.sh` sin `BACKUP_*` y sin destinatario | pasan: 11/11, rc=2 y rc=2 |

## Hallazgos

### F-05 · NARANJA · `scripts/ci/check-secrets.sh:15-18` y `:34` · la guardia falla abierta por el parseo línea a línea (regresión de F-01)
La expresión emite `archivo|kind|nombre|mac|nbad` y bash lo trocea con `IFS='|' read`, filtrando `kind == Secret`. Un campo controlado por el atacante con `|` y un salto de línea final puede partir la línea.
- **Variante 1.** Con `metadata.name: "x|1|0\n"`, yq emite `|Secret|x|1|0` y luego `|0|1`. La primera línea parece un Secret con `mac=1` y `nbad=0`, y la segunda tiene `kind=0`, así que se ignora. Un Secret con `stringData: {password: hunter2}` sin `sops` da **rc=0 y `OK check-secrets`**. También pasa dentro de `kind: List` o como segundo documento de un multi-doc.
- **Variante 2.** El `+` de `(.data // {}) + (.stringData // {})` hace que `stringData` pise a `data` para la misma clave. Con `data: {password: aHVudGVyMg==}` y `stringData: {password: "ENC[AES256_GCM,data:x"}` más `sops: {mac: x}`, el valor en claro de `data` queda en el repo y la guardia da rc=0. kustomize construye ese Secret tal cual.

Ambas llevan el plano al repo público sin que la guardia lo vea. Arreglo esperado:
- Que yq emita JSON, una línea por objeto con `-o=json -I=0`, y se consuma con `jq` o `yq`, o que no se haga ningún parseo con separador.
- Evaluar `data` y `stringData` por separado.
- Añadir las dos variantes a `check-secrets-test.sh`.

### F-06 · NARANJA · `scripts/ci/check-secrets.sh:15-18` · «cifrado» se decide por prefijo y presencia de `mac`
Esto agrava el AMARILLO F-04 de la ronda 1. La guardia solo exige el prefijo `^ENC\[AES256_GCM,data:` y que `sops.mac` no esté vacío, con lo que sigue siendo un control de formato y no de integridad.
- `stringData: {password: "ENC[AES256_GCM,data:x\nhunter2"}` con `sops: {mac: x}` da rc=0. El valor multilínea lleva el claro en la línea 2, y kustomize lo construye así: `password: |- ENC[AES256_GCM,data:x / hunter2`.
- `"ENC[AES256_GCM,data:hunter2"` con `mac: x` también da rc=0.

Arreglo esperado: exigir el patrón completo, `^ENC\[AES256_GCM,data:[A-Za-z0-9+/=]+,iv:[^,]+,tag:[^,]+,type:[a-z]+\]$` (una sola línea, sin saltos), y que `sops.mac` también cumpla el formato `ENC[...]`. Si el codificador prefiere mantenerlo como mejora, debe declararlo en la bitácora. Lo marco NARANJA porque el salto de línea hace que la guardia quede anulada en un caso que ya estaba en tu lista de pruebas del encargo.

### F-07 · NARANJA · `scripts/ci/check-secrets.sh:21` · descubrimiento por extensión, `*.yaml`, `*.yml` y `*.json` solamente
kustomize acepta como recurso cualquier ruta. Con `deploy/flux/base/zz/leak.txt` referenciado en `resources: [leak.txt]`, `kustomize build` emite el Secret con `stringData.password: hunter2`, y `check-secrets.sh` da **rc=0**. Lo mismo con `a.YAML`, un archivo `secret` sin extensión, `a.yaml.tpl` y `a.jsonc`.

Con `-skip Secret` en kubeconform ya no hay ninguna otra comprobación que lo atrape. Arreglo esperado, cualquiera de los dos:
- Analizar todos los archivos bajo `deploy/`, no solo los de extensión yaml, yml o json.
- Complementar el análisis con la salida real de `kustomize build` de dev y prod, que además cubriría los Secrets que llegan por `secretGenerator` (candidata C-40).

Esto es el mismo defecto de clase que F-01 de la ronda 1 (la guardia solo mira lo que cree que es un manifiesto), y por eso va como NARANJA.

### F-08 · AMARILLO · `check-secrets.sh` · mensajes de error sin nombre de archivo
`filename` sale vacío en la expresión de yq 4.44, así que los errores dicen `: f: falta sops.mac` en vez de `deploy/...: f: ...`. Dificulta el diagnóstico en CI. Tampoco se cubren `kind: List` anidados ni `SecretList` (kustomize no los expande, así que el riesgo es bajo).

### F-04 (ronda 1) · AMARILLO, sin cambios
Las observaciones sobre `sops:` y `mac` que quedan están absorbidas por F-06.

## Tareas candidatas (defectos reales fuera de alcance)
- C-40 y C-41 ya están registradas (cobertura de `secretGenerator` y `HelmRelease.values`, y atomicidad de `generate.sh`). No son defectos de esta ronda.
- El CA-2 literal de la tarea no funciona tal cual: la imagen `ghcr.io/getsops/sops` tiene `sops` como entrypoint, así que el `sh -c` falla. Corregir la redacción en una futura enmienda (el codificador ya usa `--entrypoint sh`).

## Rutas de transcripciones largas
- Salida de `secrets-e2e.sh` sobre el árbol real: `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/4ed52fd5-a142-46f0-ac9b-995972cfe97d/scratchpad/e2e-real.out`
- Salida de `secrets-e2e.sh` con 5 Secrets: `.../scratchpad/e2e-5.out`
- Mis casos de evasión: `.../scratchpad/c1.sh`, `c2.sh` y `c3.sh`.

VEREDICTO: NO-VERDE
NARANJA|scripts/ci/check-secrets.sh:15-34|F-05 evasión del parseo línea a línea (nombre con `|1|0` y salto final; `data` en claro tapado por `stringData`)
NARANJA|scripts/ci/check-secrets.sh:15-18|F-06 «cifrado» solo por prefijo `ENC[AES256_GCM,data:` y mac no vacío (valor multilínea con el claro en la línea 2)
NARANJA|scripts/ci/check-secrets.sh:21|F-07 descubrimiento por extensión (`.txt`, `.YAML`, sin extensión) que kustomize sí construye
INFORME: revisiones/U5-T14/ronda-2.md
