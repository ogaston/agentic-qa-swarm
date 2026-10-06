# Ronda 3 — U1-T05

VEREDICTO: NO-VERDE (un único NARANJA, F-01, que el orquestador puede arbitrar; ver el final)

CA-1 a CA-8 pasan, producción no cambió y la CI está en verde. Las justificaciones del codificador se sostienen salvo dos (J7 e I05). Sobreviven mutantes nuevos de barrido de caracteres en las clases de `Resolve`, que son el mismo patrón de F-01 de la ronda 2.

## CA (comandos literales, `GOTOOLCHAIN=go1.26.8`, `-count=1`, HEAD ff48726)
| CA | Resultado |
|---|---|
| 1 | Pasa. go-intake `24 4 0`, ui-api `19 2 0`. |
| 2 | Pasa. `25` y `20` propiedades, `9` archivos con rapid. |
| 3 | Pasa. El segundo comando con `./internal/gen` da `5` en go-intake y `5` en ui-api. Con `./...` da `6` en go-intake, porque `contract` no enlaza rapid (la bitácora y `PBT.md` lo documentan). El contraejemplo es `Sha40` = ceros, `failed after 0 tests`. |
| 4 | Pasa. `0` y `0`. |
| 5 | Pasa. La bitácora enumera H-1 y hay `2` coincidencias de regresión. |
| 6 | Pasa. `go.mod:1` y `:1`, `PBT.md` de 24 y 24 líneas, `4` y `4` coincidencias. |
| 7 | Pasa. `--- PASS: TestPBT_GeneratorCoverage` y `ok` en ambos. |
| 8 | Pasa. vet y gofmt ok en ambos, `git status` = 0. El diff fuera de alcance son solo los 4 informes de `revisiones/U1-T05/`, excluidos por la nota de la tarea. |

## Otras verificaciones
- **Producción:** el diff contra el merge-base `dfff676` solo añade tests, `internal/gen`, `PBT.md`, los `.patch` de mutantes, `go.mod`/`go.sum` y la bitácora. No hay `.go` de producción modificado.
- **fixed.go:** `go list -deps ./cmd/...` no incluye rapid ni `internal/gen` en ninguno de los dos servicios, así que no llega a los binarios `cmd/*`.
- **Skip, Sleep y red:** no hay `t.Skip`, `Sleep` ni red. Las menciones a `Skipped` son un contador del almacén. El único `time.Now` está en `gen.Main`, para el seed.
- **Parches:** los 7 `.patch` pasan `git apply --check`.
- **`PBT.md` y bitácora:** son veraces con las dos salvedades de los hallazgos F-02 y F-03. Los `TestPBT_Fixed_*` de ui-api tienen otros nombres, pero existen. La frase "0 supervivientes no equivalentes" de la clase firma, resolvedor, `Classify` y parser sigue siendo falsa; ver F-01.
- **Tiempo y estabilidad:** go-intake tarda ~1,6 s y ui-api ~2,4 s con `-run PBT`, en 6 corridas cada uno sin fallos. 150 corridas del binario PBT de `intake` dieron 0 fallos. Un 1/10 aislado en un mutante equivalente (A33) coincidió con el disco lleno; con 60 corridas dio 0 fallos.
- **CI del PR #36:** el borrador sigue abierto y su head es ff48726. Las 22 check-runs de ff48726 terminaron en `success` (`test`, `vuln`, `build` y `sbom` de go-intake, ui-api, go-identity y go-governance, más `discover`, `no-latest`, `validate` x2 y `policies` x2). `publish` quedó `skipped`, lo normal en un PR. No hay legacy statuses.

## Batería de ronda 2
Corrí 242 mutantes únicos.
- **Muertos del todo:** 212 murieron en todas las corridas, igual que el codificador. Los de ronda 2 los corrí a 20 o a 10 corridas según el tiempo; ningún mutante de la clase murió parcialmente.
- **G5:** el mutante de mi lista mutsG era un no-op mío, porque sustituía la palabra "States" en un comentario. Con la versión corregida muere 20/20 en `TestPBT_GeneratorCoverage`.
- **Los otros 29:** son los que el codificador lista (más G5, que no cuenta).

### Validación de las 29 justificaciones
| Mutantes | Mi veredicto |
|---|---|
| S19, R32 | Equivalentes. `hmac.Equal` ya compara longitudes y `""` nunca es `EqualFold` de un repo válido. |
| T04, I43 | No-op (mutante idéntico al original). |
| T09, I36 | Equivalentes. Solo añaden una línea en blanco, que se ignora al cargar. |
| T16 | Equivalente. El único estado de go-intake es `pending`. |
| P29, P31, P32, P37, P38 | Equivalentes. `exactKeys`, `DisallowUnknownFields` y `json.Unmarshal` rechazan antes. |
| I29, I39 | No observables por la API pública. El almacén no expone el contenido de los recibos. Es la misma tautología del candidato F-03. |
| S20, S22, T05, I08, I25 | Eran inválidos, pero sus versiones válidas (G22, G23, T20, V08, V05) mueren 8/8. |
| I05 | Inválido, pero su forma válida **sobrevive** (V10b, 0/20): `Apply` guarda el puntero al artefacto del evento en lugar de una copia. Candidata menor. |
| I14, I20, I27, I41 | Candidatas, confirmadas. Sobreviven también a la suite unitaria. |
| T03 | Candidata, confirmada. El handler indexa por entrega y nunca hay dos IDs con la misma. Sobrevive también a la unitaria. |
| J2 | Candidata (F-03 de ronda 2). Sobrevive también a la unitaria. |
| J5, J6 | Fuera de PBT. Los mata la suite unitaria, pero con `TestWebhookEventValidatesAgainstSchema`, no con `TestWebhook` como dice la bitácora. |
| J7 | **La justificación es falsa.** La bitácora dice que lo mata la suite unitaria, pero sobrevive a PBT y a la unitaria. Aceptar un `traceparent` con trace-id de ceros es una laguna de test, no una prueba fuera de alcance. Es candidata. |

Detalle de I41: `sc.Buffer(…, 64<<10, 1<<10)` tiene un máximo efectivo de 64 KiB, porque Go usa el mayor entre `cap` y `max`. El mutante es equivalente por debajo de 64 KiB, no por encima de 1 KiB como dice la bitácora. La conclusión (candidata) se mantiene.

## Mutantes nuevos
Construí 278 mutantes nuevos, en `scratchpad/rev-u1t05-r3/` (`mk3.py`, `mk4.py`, `new*.json`, `merged.json`). Los corrí a 8 o 10 corridas, y a 20 los parciales. 185 mueren siempre. Casi todos los uuid por grupo, los SHA por clase, los `Classify` por evento, la firma, el parser (clases y mayúsculas) y los de `Store` (`Round`, `Truncate`, `runID`, orden, `Skipped`) mueren 8/8 o 10/10.

Mueren parcialmente:
- **A21** (primer carácter del tag admite `+`): 3/20.
- **A23** (registro con a lo sumo 2 segmentos de ruta): 16/20.

Sobreviven, en las clases que importan:
- **Tag:** admitir `! $ & , = ( ) % ; { } | ' < > [ ]` en la clase del tag (A05 a A17), y `+` como primer carácter (A21).
- **repoRE:** `[A-z0-9_.-]` en el owner (A48) y en el nombre (A49), un typo clásico que admite `[ \ ] ^ _ ` + "`".
- **Registro:** puerto limitado a 5 dígitos (A22), un máximo de 3 segmentos de ruta (A24), host solo en minúsculas (A31).
- **Zero SHA:** tratar como borrado un `after` con prefijo de 39, 20, 8 o 7 ceros, o con sufijo de 39, o que contenga 20 ceros (C59, C60, C61, C62, C65, C66). Falta un SHA válido de frontera como `0…01`.
- **Classify:** `TrimSpace` del `head.repo` del PR (C69).
- **Enum:** aceptar `"pr"` como `github_event` en ui-api (U81).
- **Firma:** `Verify` que rechaza cuerpos mayores de 4096, de 64 KiB o de 1 MiB (G01 a G03), y truncar el cuerpo a 64 KiB en ambos lados (G05). `FixedBodies` termina en 4096.
- **FakeVerifier:** `AcceptOnly` con `EqualFold` (G21).
- **Almacén go-intake:** `json:"sha"`, `json:"state"` o `omitempty` de `Notification` (T23, T24, T25), tautológico como F-03; sobreviven también a la unitaria. Una línea final de 1 byte sin salto de línea (T30).
- **ui-api:** `Confirm` con principal vacío (V20), que persiste una línea que luego el reload descarta; `List` nil vs vacío (V04, lo mata la unitaria); reloj por defecto (V16); líneas de más de 64 KiB (V17, V18).
- **Fuera del dominio generado, equivalentes o sin sentido:** topes de longitud de repo (A40, A41, A50, A51, A52), tag que valida el SHA (A57), `Sign` que rellena con ceros (G18, equivalente en HMAC), tope de 1000 bytes (G09), topes de `Apply` y `List` (V21 a V26), recursión de duplicados en arrays (U99, no hay arrays en el esquema) y tope de longitud de `notification_id` (U93).

## Hallazgos
### F-01 · NARANJA (arbitrable) · `gen/fixed.go` · Faltan barridos de caracteres en las clases de `Resolve`
En ronda 2 ya sobrevivía `[0-9a-gA-F]` en el uuid. Hoy sigue sin barrerse el resto de la clase de caracteres de tag y de repo. Los casos que sobreviven son una puntuación suelta en el tag, `[A-z…]` en el repo, `+` como primer carácter del tag, y un zero-SHA por prefijo (SHA con 39 ceros más un dígito).
- **Lo pedido es barato:**
  - Recorrer los 128 ASCII como carácter del tag (inicial, medio y final), del owner, del nombre y del host del registro.
  - Añadir los SHA `0…01` y `10…0` a los válidos de push.
  - Añadir un `head.repo` con espacios a `FixedValidClassified`.
  - Añadir un `github_event` rechazado más (`"pr"`, `""`, mayúsculas) a las variantes del parser de ui-api.
  - Añadir un cuerpo de más de 64 KiB a `FixedBodies`.
- **Arbitraje:** la tarea pide variantes inválidas controladas, y la propiedad 5 solo exige patrón y no `:latest`. Si el orquestador considera que esto es más de lo exigido, F-01 pasa a candidata y todo lo demás es VERDE.
- **Falsedad de la bitácora:** sigue diciendo "0 supervivientes no equivalentes" en la clase firma, resolvedor, `Classify` y parser.

### F-02 · AMARILLO · Los corpus de `fixed.go` se pueden vaciar sin que falle nada
Vaciar `FixedBadTags`, `FixedBadRegistries`, `FixedBadRepos`, `FixedValidTags`, `FixedValidSHAs`, `FixedRejected`, `FixedValidClassified`, `FixedResolvable`, `FixedUnresolvable` o `FixedBodies` deja los tests en verde (F02 a F13, 0/8). También se pueden reducir los largos de secreto a `{0,1}`, hacer constante el secreto o quitar el barrido por posición del SHA (F14, F16, F17, F18).
- **Mutantes que sí mueren:** vaciar `FixedBadSHAs` (F01), vaciar los largos de secreto (F15) y `GU7`.
- **Remedio:** un test de tamaño mínimo para cada corpus.
- **Generadores:** `Sha40` sin letras o sin `f` (GU4, GU5), `Free()` solo ASCII o sin la cadena de 500 (GI1, GU1, GU2) y años acotados (GU6) pasan la cobertura, que solo mide entropía de unos pocos campos.

### F-03 · AMARILLO · `PBT.md` y bitácora
- **Lo tautológico:** el round-trip de `Notification` y `Record` de go-intake también es tautológico (T23, T24, T25), pero `PBT.md` de go-intake solo lo dice del evento.
- **J7:** la bitácora lo atribuye a la suite unitaria y no es así.
- **I41:** el tope efectivo de 64 KiB lo hace equivalente por debajo de 64 KiB, no por encima de 1 KiB como dice la bitácora.
- **I05:** se marcó como inválido y su forma válida sobrevive.

### Candidatas (huecos menores, no defectos de la tarea)
- **Zero SHA y límites del dominio:** zero-SHA por prefijo (si el orquestador lo deja fuera de F-01).
- **go-intake:** J7 (trace-id de ceros); T30.
- **ui-api:**
  - I05/V10b (aliasing del artefacto);
  - I14 (orden de `List` con empates de sub-segundo);
  - I20 (`ConfirmedAt` sin `.UTC()`);
  - I27 (memoria antes de persistir);
  - I41, V17, V18 (líneas de más de 64 KiB);
  - V20 (principal vacío, persistido pero descartado al recargar);
  - T03 (dos IDs con la misma entrega);
  - J2, T23, T24, T25 (contraste de JSON con OpenAPI);
  - U81.
- **Firma:** G01, G03, G05 (cuerpos grandes), G21 (`FakeVerifier` con `EqualFold`).

## Supervivientes no equivalentes
- **En la clase, F-01:** A05 a A17, A21, A22, A24, A31, A48, A49, C59 a C62, C65, C66, C69, U81.
- **Candidatas fuera de la clase:** J2, J7, I05 (V10b), I14, I20, I27, I41, T03, T23, T24, T25, T30, G01 a G03, G05, G21, V17, V18, V20.
- **Equivalentes o no observables, confirmados:** S19, R32, T04, T09, T16, P29, P31, P32, P37, P38, I29, I36, I39, I43, A57, A33, G18, U99.

## Fuera del dominio y de los ejecutados
Mis temporales y las salidas quedaron solo en mi subdirectorio del scratchpad. Los 7 `.patch` del codificador no los apliqué: solo pasé `git apply --check`.

El primer intento de batería llenó el disco (1,5 h de cache de Go compartida); borré la cache de Go (`go clean -cache`) y relancé el resto con una lógica de limpieza. Algunos mutantes nuevos corrieron a 8 corridas, no a 20. Los 212 de ronda 2 que murieron del todo los corrí a 10 o 20 según el bloque.

Rutas relevantes:
- `/home/user/wt-U1-T05/services/go-intake/internal/gen/fixed.go`
- `/home/user/wt-U1-T05/services/ui-api/internal/gen/gen.go`
- `/home/user/wt-U1-T05/bitacoras/U1-T05.md`
- `/tmp/claude-0/-home-user/4ca8050b-f1f3-5eab-a78d-10b43e5bda2f/scratchpad/rev-u1t05-r3/merged.json`

VEREDICTO: NO-VERDE
