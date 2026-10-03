# Ronda 3 — U5-T14

VEREDICTO: VERDE

Hash de la tarea: b945bc39... (coincide). HEAD del worktree: d0eae29 (rama `tarea/U5-T14`). Al terminar, `git status --short -uall` da 0 líneas. Borré mis `mktemp` y mis copias de trabajo. Queda `/tmp/tmp.26pOAch9Nu`, que ya existía en las rondas anteriores y no es mío.

Nota de alcance: el diff de esta ronda (`16939f2..d0eae29`) toca solo `scripts/ci/check-secrets.sh`, `scripts/test/check-secrets-test.sh` y la bitácora. `policies.sh`, `generate.sh`, `deploy/` y `docs/` no cambiaron.

## 1. Historial completo de la rama
- Corrí `git log -p fd60529..tarea/U5-T14` (1329 líneas) y también `origin/main..tarea/U5-T14` (7 commits).
- No aparece `AGE-SECRET-KEY-`, `PRIVATE KEY` ni `age1...` largas.
- No hay blobs base64 o hex de 60 o más caracteres.
- Las tres apariciones de `ENC[AES256_GCM,data:` llevan `YWJj`, `bWFj` y `hunter2`, es decir, literales de prueba y no cifrado real.
- `hunter2` solo aparece en las pruebas negativas.
- Ningún `*.sops.yaml`, `.key` ni `.pem` fue añadido en ningún commit. `git ls-files deploy | grep -c '\.sops\.yaml$'` da 0.
- **Limpio.**

## 2. Evasiones contra la guardia nueva
Todos los casos que esperaba rc=1 dan rc=1. Corrí unos 100 casos (scripts en `.../scratchpad/c1.sh`, `c2.sh`, `c3.sh`, `c4.sh`, `c5.sh`, `n1.sh`, `n2.sh`). En todos leí el mensaje de salida, no solo el rc.

Casos que no dan rc=1 pero son correctos:
- `doc vacio solo`, `cifrado válido` y `stringData: null` dan rc=0, como se esperaba.
- `dup kind` (rc=0) es inerte, porque kustomize rechaza claves duplicadas.
- `kind: [Secret]` y `kind: secret` en minúsculas no son un `kind: Secret` aplicable.

| Grupo | Resultado |
|---|---|
| Rondas 1 y 2 completas: pipes con salto final, `data` tapado por `stringData`, valor multilínea, `binaryData`, mac mal formado, `.txt`, `.YAML`, sin extensión, `.tpl`, `.jsonc`, BOM, CRLF, nombre numérico o nulo, arrays | rc=1. Cada mensaje corresponde al campo atacado (`sops.mac`, `data`, `stringData` o `binaryData`). |
| Secret solo en `clusters/`, referenciado o no | rc=1 (el build de `clusters/dev` y el barrido de archivos lo ven). |
| Directorio fuera de `deploy/` referenciado (`../../../scripts/zz`) | rc=1. kustomize lo construye y el build lo detecta. |
| Archivo suelto fuera de `deploy/` | rc=1. kustomize lo rechaza ("not in or below") y la guardia falla cerrada. |
| Remoto inalcanzable (`https://invalid.invalid/...`) | rc=1 (falla cerrada). |
| Remoto github alcanzable | rc=0. La guardia construyó el remoto y solo contenía Secrets cifrados. |
| Overlay roto o YAML inválido | rc=1, "kustomize build fallo". Falla cerrada. |
| Valor ENC con espacio final, espacio inicial, `\n` final o bloque `\|`; mac con espacio final | rc=1 (el regex está anclado entero). |
| `type:STR` en mayúsculas | rc=1. |
| `type:weird` | rc=0 (ver F-11). |
| Alias a mapa, merge key, alias a escalar, `!!binary`, int, null, vacío, lista, escalar en `stringData` | rc=1. |
| Escapes unicode en `kind` o en las claves, clave compleja `? kind`, tag `!!str` | rc=1. |
| UTF-16 con BOM, referenciado o no | rc=1. |
| UTF-16LE sin BOM no referenciado | rc=0, inerte (ver sección 5). Si kustomize lo parseara, el build lo vería. |
| Binario con NUL y `kind: Secret` | rc=1 ("no analizable y menciona Secret"). |
| Binario aleatorio | rc=0. No es un objeto Secret. |
| Archivo oculto `.leak` | rc=1. |
| `List` anidada y `SecretList` referenciadas | rc=1 (kustomize las expande y el build las ve). |
| Espacios finales en `kind: Secret   ` | rc=1. |

## 3. Suite de 28 casos: es real
- Con `check-secrets-test.sh` corrí la suite: 28 `ok` y rc=0 (`.../scratchpad/suite3.out`).
- Corrí otra versión que imprime la razón de cada caso (`suite-verbose.out`). Cada caso dispara la detección que dice, con el campo correcto.
- Sabotaje 1: sustituí la guardia por `exit 0` en una copia. La suite da 25 `INESPERADO` y rc=1.
- Sabotaje 2: sustituí la guardia por `exit 1`. La suite da 3 `INESPERADO` y rc=1.
- 25 + 3 = 28, así que no hay casos saltados ni silenciosos.
- Los 3 casos positivos protegen contra una guardia que falle siempre.

## Criterios de aceptación, verificados por mí
| # | Comando | Resultado |
|---|---|---|
| CA-1 | literal | pasa: `sops/sops-age` x2 |
| CA-2 | vía `secrets-e2e.sh` (82 s) | pasa: `gen rc=0`, 6 archivos, contrato exacto de 6 Secrets |
| CA-3 | vía `secrets-e2e.sh` | pasa: 0, 0, 6, 1, 1 |
| CA-4 | literal | pasa: `arbol rc=0`, `claro rc=1`; sobre los 6 cifrados, `cifrados rc=0` |
| CA-5 | literal | pasa: 9 líneas `OK` (incluida `OK check-secrets`, tras la sustitución `awk`) y `rc=0` |
| CA-6 | bloque COMPLETO, tras `d0eae29` | pasa: `0`, `0`, `0`, `shellcheck rc=0`, `0` |
| CA-7 | literal | pasa: `policies-generado rc=0`, `kubeconform-valid-generado 51`, `1`, `-skip Secret` único |
| Extra | shellcheck de `check-secrets-test.sh` y `policies.sh` | `rc=0` |
| Extra | negativas de `generate.sh` (sin `BACKUP_*`, sin destinatario) | pasan, rc=2 y rc=2; el árbol queda intacto |

Observaciones:
- La guardia sobre el árbol real tarda 5,4 s. La CA-6 pegada por el codificador mostraba `shellcheck rc=1`, antes de añadir las directivas SC2016. Mi corrida es posterior y da 0.
- El CA-2 literal de la tarea sigue sin funcionar tal cual, porque el entrypoint de la imagen sops es `sops` (ya anotado en la ronda 2). `secrets-e2e.sh` usa `--entrypoint sh`.
- La salida real de sops/age pasa el regex estricto: `policies-generado rc=0` con dev y prod generados.

## 4. Dictamen sobre la acotación de "menciona Secret"
**No abre una evasión que Flux pueda aplicar.**
- Todo lo que Flux aplica pasa por kustomize. Los archivos referenciados salen en el build (a), que se evalúa con la misma expresión de yq sobre el YAML resultante. Si kustomize no puede parsearlos, el build falla y la guardia falla cerrada.
- Los archivos parseables pasan por la expresión estructural (b) y no dependen del patrón `MENCION`.
- Solo quedan fuera archivos NO referenciados, no parseables y que no casan `kind[^[:alnum:]]*secret([^[:alnum:]]|$)`. Esos solo quedarían publicados.
  - Ejemplo 1: UTF-16LE sin BOM.
  - Ejemplo 2: un `.sh` con `kubectl create secret generic x --from-literal=password=hunter2` (rc=0).
  - Ejemplo 3: un `kind:` y su `Secret` en líneas distintas dentro de un archivo roto (rc=0).
- Eso es otra clase, la de credenciales sueltas en scripts, que no son un objeto `kind: Secret`. Va como tarea candidata, no como defecto de esta tarea.
- Sí se detectan: `kind: Secret` en heredocs, en `.md`, `Kind: SECRET` en mayúsculas y los nombres `AWS_SECRET_ACCESS_KEY` no disparan falsos positivos (caso de la suite).

## Hallazgos
No hay ROJO ni NARANJA en pie. Todos son AMARILLOS y no bloquean.

### F-09 · AMARILLO · `check-secrets.sh:3-5,49` · los 4 destinos del build están fijos, y el comentario dice "todo lo que Flux aplicaría"
Probé un overlay `deploy/flux/other/kustomization.yaml` con `secretGenerator` y `literals: [password=hunter2]`. Lo referencié con un `Kustomization` de Flux en `clusters/dev/other.yaml` (`path: ./deploy/flux/other`). El resultado es rc=0, y Flux lo aplicaría. Un Secret en YAML plano en ese directorio sí se detectaría por el barrido de archivos; solo falla el generado. Entra en C-40 (secretGenerator), que ya está registrada y fuera de alcance. Sugerencia para esa candidata: construir cada directorio con `kustomization.yaml` bajo `deploy/`, o leer los `spec.path` de los Kustomization de Flux. Mientras tanto, el comentario sobre-afirma.

### F-10 · AMARILLO · `check-secrets.sh:25` · el barrido de archivos solo expande `kind: List`
Un `SecretList` o una `List` anidada **no referenciados** dan rc=0 en el barrido (b). Si están referenciados, el build los detecta (probado: kustomize los expande). Es solo "publicado, no aplicado". Es el F-08 de la ronda 2, ahora acotado a ese caso.

### F-11 · AMARILLO · `check-secrets.sh:19`, `docs/operaciones/secrets.md:37` · control de formato, no de integridad
- Un valor `ENC[AES256_GCM,data:aHVudGVyMg==,iv:AAAA,tag:AAAA,type:str]` con el claro en base64 dentro de `data:` pasa (rc=0).
- Un `sops.mac` válido copiado de otro archivo también pasa, y `type:weird` también.
- Eso no importa para fuga accidental. Si se copia un valor válido de otro archivo, Flux falla al descifrar por MAC distinto: es un fallo de despliegue, no una fuga. Fabricar el sobre falso exige intención.
- Sin la clave privada no se puede hacer más; descifrar en CI no es viable en un repo público.
- La cabecera del script es honesta ("coincide ENTERO con el formato SOPS"), pero `secrets.md` dice "falla si algún Secret no está cifrado". Convendría matizarlo.

### F-12 · AMARILLO · `check-secrets-test.sh` · los casos negativos solo comparan el rc
Un caso podría dar rc=1 por la razón equivocada, por ejemplo un fallo de build. Lo verifiqué yo con la versión verbosa: los 25 negativos fallan por la razón que declaran. Los 3 positivos descartan una guardia que falle siempre. Si el codificador quiere blindarlo, puede asertar un fragmento de la salida (`campo":"stringData`).

## Tareas candidatas (defectos reales fuera de alcance)
- Ampliar C-40: la guardia debería construir todos los destinos de Flux (F-09) y cubrir `HelmRelease.values`, que sigue fuera de alcance.
- Escáner genérico de credenciales (por ejemplo gitleaks) en CI: no detecta nada si la credencial no está en un `kind: Secret` (`--from-literal=password=...` en un `.sh`, ConfigMaps con contraseñas).
- Corregir el CA-2 de la tarea: el `sh -c` no funciona con el entrypoint de la imagen sops (hay que usar `--entrypoint sh`).

## Rutas de transcripciones largas
- `.../scratchpad/suite3.out` (suite normal, 28 `ok`).
- `.../scratchpad/suite-verbose.out` (razón de cada caso).
- `.../scratchpad/e2e3.out` (`secrets-e2e.sh` completo, E2E OK).
- `.../scratchpad/r3-old.txt`, `n1.out`, `n2.out`, `c4.out` (evasiones).
- `.../scratchpad/hist3.txt` (historial analizado).
- Todas bajo `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/4ed52fd5-a142-46f0-ac9b-995972cfe97d/scratchpad/`.

VEREDICTO: VERDE
AMARILLO|scripts/ci/check-secrets.sh:3-5,49|F-09 destinos de build fijos: un secretGenerator en un overlay no construido (referenciado por un Kustomization de Flux) pasa; entra en C-40
AMARILLO|scripts/ci/check-secrets.sh:25|F-10 el barrido de archivos solo expande kind: List (SecretList y List anidada no referenciadas pasan)
AMARILLO|scripts/ci/check-secrets.sh:19|F-11 control de formato y no de integridad (sobre ENC falso con claro en base64, type:weird); matizar secrets.md:37
AMARILLO|scripts/test/check-secrets-test.sh|F-12 los casos negativos solo comparan el rc (verificado que fallan por la razón correcta)
INFORME: revisiones/U5-T14/ronda-3.md
