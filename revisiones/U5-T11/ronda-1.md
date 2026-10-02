# Ronda 1 — U5-T11

VEREDICTO: NO-VERDE

Hash de la tarea verificado: 883699dfa754f3d638a7452588e5826d07c33247 (coincide). HEAD a718d74. Usé los comandos literales con los alias K, Y y C, y un `mktemp -d` propio. No usé kubectl, flux ni apply.

## Criterios de aceptación, verificados por mí
| # | Criterio | Resultado |
|---|---|---|
| 1 | ConfigMap en aqs-system | pasa: `minio-init-script ns=aqs-system` |
| 2 | Sin objetos sin namespace salvo Namespace | pasa: `Namespace` en dev y `Namespace` en prod, nada más |
| 3 | Job monta el ConfigMap renombrado | pasa: `aqs-system minio-init-script` |
| 4 | U5-T05 sigue pasando, con harness | pasa: `Job/minio-init`, `Service/minio`, `StatefulSet/minio`, `IGUAL`, `harness rc=0` |
| 5 | kubeconform, verify, test | pasa: 49 válidos en dev y en prod con `Invalid: 0, Errors: 0, Skipped: 0`. `verify`: 42 pruebas, 42 ok, rc=0. `dev` y `prod`: 441 de 441, rc=0 |
| 6 | La regla deniega sobre el build real | pasa: `cm-sin-ns rc=1`, `cm-ns-null rc=1`, `cronjob-ns-vacio rc=1`, `namespace-ok rc=0`. Comprobé además que cada rc=1 incluye el mensaje de la regla nueva, no otra regla |
| 7 | Solo los 5 archivos permitidos | pasa: bitacoras/U5-T11.md, job-init.yaml, kustomization.yaml (minio), namespace.rego, namespace_test.rego |
| 8 | Árbol limpio | pasa: `0` |

Sobre el harness de CA-4: corrió en unos 7 s, con salida `evidence AES256`. Eso es la lectura de vuelta real del cifrado, no una ejecución saltada. Al terminar no quedó ningún contenedor `aqs-minio-test`. Ese conteo lo hice después del harness. No toqué los `aqs-backup-test`.

Mutación de las pruebas negativas: deshabilité la regla en una copia de `policy` y las 4 pruebas `test_deniega_*` fallaron (38 de 42). Las pruebas negativas son reales: no pasan gracias a otras reglas de `package main`.

## Hallazgos

### F-01 · NARANJA · policy/namespace.rego:27 · La regla se evade con `metadata` ausente, `null` o no-objeto (y con `kind` ausente)
El alcance pide denegar "cualquier objeto sin `metadata.namespace`". El cuerpo de `deny` calcula el mensaje con `object.get(input.metadata, "name", "?")` y con `input.kind`. Si `input.metadata` no existe, es `null` o no es un objeto, la expresión queda indefinida. Entonces la regla no produce ningún mensaje y no deniega. Se evade, no hay un deny con otro texto.

Fixtures concatenados al build real de prod, con `$C test --policy policy --all-namespaces`:
```
{apiVersion: v1, kind: ConfigMap}                          -> rc=0, 0 mensajes de la regla (debería denegar)
{apiVersion: v1, kind: ConfigMap, metadata: null}          -> rc=0, 0 mensajes (debería denegar)
{apiVersion: v1, kind: ConfigMap, metadata: "x"}           -> rc=0, 0 mensajes (debería denegar)
{apiVersion: v1, metadata: {name: nokind}}                 -> rc=0, 0 mensajes (kind ausente)
```
Con la forma que sí se contempló (`metadata` presente sin namespace, `namespace: null`, `""`, `5`, `[a]`, y `name` ausente o no-string) sí deniega, rc=1. El único hueco es el cálculo del mensaje.

Los criterios CA-1..CA-8 pasan porque CA-6 solo prueba objetos con `metadata` presente. Aun así, el alcance dice "cualquier objeto". Además, `namespace_test.rego` no tiene ninguna prueba con `metadata` ausente, así que el hueco no se vio.

Corrección sugerida, para el codificador: construir el mensaje sin depender de `metadata`, por ejemplo `m := object.get(input, "metadata", {})` más una guarda `is_object`, y `object.get(input, "kind", "?")`. Hay que añadir pruebas para metadata ausente, `null` y no-objeto. Si `kind` ausente se considera fuera de alcance (kubeconform ya lo rechaza), que quede dicho en la bitácora.

### F-02 · AMARILLO · policy/namespace.rego:22-25 · `namespace: " "` y `"\n"` pasan
Un namespace de solo espacios o con salto de línea da rc=0, sin mensaje. El alcance dice "no-string o vacío", y `" "` no es vacío, así que no incumple la letra de la tarea. Kubernetes lo rechazaría en el apply. No bloquea. Se podría cubrir con `trim_space(...) != ""`.

### F-03 · AMARILLO · policy/namespace_test.rego:19-27 · Las pruebas positivas dependen del texto exacto del mensaje
Las pruebas positivas son `not "<mensaje literal>" in r`. Si alguien cambia el texto del mensaje en `namespace.rego`, estas pruebas siguen pasando de forma vacía y dejan de detectar un falso positivo. Hice la mutación contraria, quitar `not ns_valido`, y esas dos pruebas sí fallan. Hoy no ocultan nada, pero el acoplamiento es frágil. Sería más sólido definir el mensaje o la regla auxiliar una sola vez y referenciarla desde las pruebas.

## Puntos que pediste juzgar
- Kind con ámbito de namespace tratado como de clúster: no encontré ninguno. La lista es exacta (11 kinds) y comparada con `in`.
- Kinds de clúster no listados (`ClusterIssuer`, `RuntimeClass`, `GatewayClass`, `Node`): dan rc=1, con mensaje, como falsos positivos. No importan hoy: `git ls-files deploy | xargs grep` no encuentra ningún `ClusterIssuer`, `RuntimeClass`, `GatewayClass`, `ClusterPolicy` ni `ClusterSecretStore`. CA-2 confirma que el build solo tiene `Namespace` sin namespace. Pasaría a importar cuando entre alguno de esos kinds, y la lista es editable.
- Objetos legítimos: dev y prod pasan 441 de 441 sin fallos, incluidos los CRs de Flux y de Prometheus Operator, que llevan namespace.
- Pruebas positivas: no ocultan un falso negativo hoy (ver F-03). Las 4 negativas fallan si se apaga la regla.
- Alcance: 5 archivos, sin desborde. `security.rego` y `isolation*.rego` no se tocaron.
- Worktree limpio. Borré mi log temporal.

## Tareas candidatas
- Ninguna nueva. El hueco de F-01 es una limitación de la regla de esta misma tarea.

## Rutas de transcripciones largas
- Ninguna.

VEREDICTO: NO-VERDE
NARANJA|policy/namespace.rego:27|La regla se evade con metadata ausente, null o no-objeto (y kind ausente)
AMARILLO|policy/namespace.rego:22-25|namespace de solo espacios o salto de línea pasa
AMARILLO|policy/namespace_test.rego:19-27|Pruebas positivas acopladas al texto exacto del mensaje
INFORME: revisiones/U5-T11/ronda-1.md
