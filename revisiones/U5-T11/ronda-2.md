# Ronda 2 — U5-T11

VEREDICTO: VERDE

Hash de la tarea verificado: 883699dfa754f3d638a7452588e5826d07c33247 (coincide). HEAD 2ad0f6f. Usé los comandos literales con los alias K, Y y C, y mi propio `mktemp -d`. No usé kubectl, flux ni apply.

## Criterios de aceptación, verificados por mí
| # | Criterio | Resultado |
|---|---|---|
| 1 | ConfigMap en aqs-system | pasa: `minio-init-script ns=aqs-system` |
| 2 | Sin objetos sin namespace salvo Namespace | pasa: `Namespace` en dev y `Namespace` en prod, nada más |
| 3 | Job monta el ConfigMap renombrado | pasa: `aqs-system minio-init-script` |
| 4 | U5-T05 sigue pasando, con harness | pasa: `Job/minio-init`, `Service/minio`, `StatefulSet/minio`, `IGUAL`, `harness rc=0`. El harness tardó 6,6 s y no dejó ningún contenedor `aqs-minio-test` (conteo 0) |
| 5 | kubeconform, verify, test | pasa: 49 válidos en dev y en prod con `Invalid: 0, Errors: 0, Skipped: 0`. `verify`: 47 de 47, rc=0, es decir 5 pruebas más que en la ronda 1. `dev` y `prod`: 441 de 441, rc=0 |
| 6 | La regla deniega sobre el build real | pasa: `cm-sin-ns rc=1`, `cm-ns-null rc=1`, `cronjob-ns-vacio rc=1`, `namespace-ok rc=0`. En cada rc=1 conté además el mensaje "metadata.namespace es obligatorio" en la salida: el denegado es de esta regla y no de otra |
| 7 | Solo los 5 archivos permitidos | pasa: bitacoras/U5-T11.md, job-init.yaml, kustomization.yaml (minio), namespace.rego, namespace_test.rego |
| 8 | Árbol limpio | pasa: `0`, también después de todas mis mutaciones y fixtures |

## Verificación por hallazgo de la ronda 1

### F-01 (NARANJA) · cerrado
Fixtures concatenados al build real de prod. Todos dan rc=1 y exactamente 1 mensaje de la regla:
- Mis fixtures de la ronda 1: `metadata` ausente, `metadata: null`, `metadata: "x"` y `kind` ausente.
- Evasiones nuevas: `metadata: []`, `metadata: {}`, `kind: null`, `kind: ""`, `kind: 5`, `kind: [Namespace]`, `kind: {a: b}`, `metadata.name: [a]`, `namespace: [a]`, `namespace: {a: b}` y el objeto vacío `{}`.
- `kind: namespace` (otras mayúsculas): rc=1. No se trata como exento, porque `in` compara exacto. Es correcto, ya que Kubernetes distingue mayúsculas en `kind`.
- Input escalar (`5`, `"x"`, `null`): rc=0 con 0 mensajes, por la guarda `is_object(input)`. Juzgo que no importa. Un escalar no es un objeto de Kubernetes y kubeconform lo rechaza con `Errors: 1` ("cannot unmarshal number into ... map"). Ninguna otra regla de `policy/` lo cubre tampoco. No evade el namespace de ningún objeto real. Lo dejo como observación, no como hallazgo.
- Una lista `[1]` da rc=1 sin mensaje de esta regla. Es un error de conftest, no una evasión.

### F-02 (AMARILLO) · cerrado
`namespace: " "`, `"\n"` y `"\t"` dan rc=1 con el mensaje de la regla (`trim_space`). La prueba `test_deniega_namespace_solo_espacios` lo cubre.

### F-03 (AMARILLO) · cerrado
Las pruebas positivas (`test_permite_namespace_valido`, `test_permite_kind_de_cluster`) referencian `ns_msg(...)` y no un literal. Hice las mutaciones sobre copias en `mktemp`:
- Cambiar solo el texto del mensaje: 47 de 47 siguen pasando. El texto ya no está duplicado, así que lo esperado es que no se rompa nada.
- Cambiar el texto del mensaje y además quitar `not ns_valido`: falla `test_permite_namespace_valido`. Esto demuestra que, con el texto cambiado, las positivas siguen detectando un falso positivo. Antes quedaban vacías.
- Quitar solo `not kind in cluster_kinds`: falla `test_permite_kind_de_cluster`.
- Las negativas nuevas (`metadata` ausente, `null`, no-objeto y `kind` ausente) comprueban `ns_msg(...) in deny`, o sea el mensaje de esta regla y no un deny cualquiera.

## Otros puntos
- Objetos legítimos: dev y prod pasan 441 de 441. La lista de kinds de clúster sigue siendo exacta, con 11 entradas.
- Alcance: 5 archivos, sin desborde. El cambio de la ronda 2 toca solo `namespace.rego`, `namespace_test.rego` y la bitácora.
- Bitácora: la sección "Ronda 2" trae los comandos y sus salidas, y se corresponden con lo que obtuve yo. No incluye el fixture `"\n"`, que sí corrí yo y da rc=1. Es menor.

## Hallazgos en pie
Ningún ROJO ni NARANJA. Ningún AMARILLO bloqueante.

## Tareas candidatas
- Ninguna nueva. Las observaciones de la ronda 1 siguen vigentes: los kinds de clúster no listados (`ClusterIssuer`, `RuntimeClass` y similares) darían falso positivo cuando entren al repositorio, y la lista es editable.

## Rutas de transcripciones largas
- Ninguna.

VEREDICTO: VERDE
INFORME: revisiones/U5-T11/ronda-2.md
