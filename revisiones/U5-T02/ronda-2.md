# Ronda 2 — U5-T02

VEREDICTO: VERDE

Hash de la tarea verificado: 55bd7320b5b45be806235523112b3ca99e8725c0 (coincide). HEAD del worktree: 5465419. El worktree estaba limpio (`git status --short` vacío) antes y después de mi revisión. Las pruebas negativas las hice sobre `git archive` en directorios temporales dentro del scratchpad y sobre archivos sueltos en el scratchpad.

## Criterios de aceptación, verificados por mí (comandos literales de la tarea, sobre HEAD 5465419)
| # | Criterio | Resultado |
|---|---|---|
| 1 | 10 esquemas con nombres exactos | pasa: `10` |
| 2 | compilan en draft2020 | pasa: `10` |
| 3 | envoltorio común y `$id` con `/v1/` | pasa: solo `FIN` |
| 4 | `bash contracts/validate.sh` | pasa: `rc=0` (12 ok válidos, 34 ok inválidos, 0 FALLA). Conteos: `12` y `34`, ambos ≥10. Tardó 59 s |
| 5 | prueba negativa sobre copia (se borra `event_id` del primer válido) | pasa: `rc=1` |
| 6 | OpenAPI | pasa: redocly 1.25.0 `rc=0` y `9` rutas |
| 7 | workflow | pasa: `1`, actionlint `rc=0` sin salida, `0` acciones sin SHA |
| 8 | árbol limpio y sin `.gitkeep` | pasa: `0` y `0` |

## Hallazgos de la ronda 1, por identificador

### F-01 (NARANJA) · resuelto
`if/then` en `deploy.schema.json` y `rehearsal.schema.json`. Probé 24 variantes con ajv directo (`ajv validate --spec=draft2020 -c ajv-formats`), más allá de los 4 inválidos añadidos:
- Válidos que siguen pasando: `deploy.done`, `deploy.failed` (con `reason`), `rehearsal.passed` (`passed:true`) y `rehearsal.failed` (`passed:false`). Los 4 dan `valid`.
- Deploy, todos `invalid`:
  - `deploy.failed` sin `reason`.
  - `deploy.failed` con `reason` vacío o `null`.
  - `deploy.done` con `reason` (cadena o `null`).
  - `type` ausente o `deploy.other`.
  - `data` `null` o array.
  - Propiedad extra en `data`.
- Rehearsal, todos `invalid`:
  - `rehearsal.passed` con `passed` `false`, `"true"`, `1`, `null` o ausente.
  - `rehearsal.failed` con `passed` `true`, `0` o ausente.
  - `type` `rehearsal.x`.
  - `data` cadena.

No encontré combinaciones contradictorias que validen. Las cláusulas de `then` usan `properties` y quedan vacuamente verdaderas si falta `data`, pero `data` es `required`, así que no hay hueco.

### F-02 (NARANJA) · resuelto
La bitácora explica que el `1` era la propia bitácora sin commitear. Yo corrí CA-8 sobre HEAD y da `0` y `0`.

### F-03 (AMARILLO, arbitrado a corregir solo la robustez) · resuelto
La función `run()` devuelve 0 si ajv acepta. Devuelve 1 solo si la salida coincide con la regex anclada `^<ruta> invalid$`. En cualquier otro caso devuelve 2, que es siempre error. Pruebas sobre copias:
- Un inválido con JSON roto y `invalid` en el nombre (`run.done.invalid-json-broken.json`): `FALLA ... no rechazado por el esquema`, `rc=1`. No pasa en falso.
- Un inválido vacío: `FALLA`, `rc=1`.
- Esquema `warm.ready` con JSON roto: FALLA en el válido y en los 3 inválidos, `rc=1`.
- Esquema `run.done` que no compila (`type:"foo"`): FALLA en el válido y en los 3 inválidos, `rc=1`.
- PATH sin `npx`: 46 FALLA, 0 ok, `rc=1`.

Nota menor: si el nombre de un archivo contuviera metacaracteres de regex, el script fallaría hacia el lado seguro (devolvería 2), nunca hacia un falso pase.

### F-04 (AMARILLO, arbitrado: `type` frente al nombre y `Notification.artifact`) · resuelto
- Copia con `deploy.done.json` cuyo `type` es `deploy.failed` (con `reason`, para que siga siendo válido por esquema): `FALLA type 'deploy.failed' no coincide con el nombre del archivo`.
- Copia con `run.done.json` sin `type`: `FALLA type 'null' no coincide ...`. `rc=1` en ambos casos.
- `Notification.artifact` pasó a objeto `{kind, ref}`, con `kind` en `[build-from-repo, published-image]`, `ref` con `minLength 1` y `additionalProperties:false`. Es coherente con el evento `notify.created`.
- El lint de redocly sigue en `rc=0`.
- Los opcionales (4xx de logout, `event_id` distintos) no se atendieron y no bloquean.

## Alcance
`git diff ab0c6ed..tarea/U5-T02` toca solo `bitacoras/U5-T02.md`, `contracts/validate.sh`, `contracts/openapi/control-plane.yaml`, `deploy.schema.json`, `rehearsal.schema.json` y 4 ejemplos inválidos nuevos bajo `contracts/events/examples/invalid/`. No hay desborde nuevo: no aparecen `ci.yml`, `deploy/`, `README.md`, `package.json`, `warm.quarantined` ni `run.started`.

## Hallazgos nuevos
- AMARILLO · `bitacoras/U5-T02.md`, sección Ronda 2: en el bloque de la prueba negativa del JSON roto, el `rc=0` que aparece es el del `grep` final de la tubería, no el del script. El bloque siguiente sí registra `rc=1`. No cambia nada del código ni de los criterios y no bloquea.

## Tareas candidatas
- Las dos de la ronda 1 siguen en pie. Una es acelerar `validate.sh` con una sola invocación de ajv. La otra es alinear el contrato REST y el de eventos cuando U1 implemente el control plane.

## Rutas de transcripciones largas
- `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/4ed52fd5-a142-46f0-ac9b-995972cfe97d/scratchpad/ca4.txt` (salida completa de CA-4)

VEREDICTO: VERDE
INFORME: revisiones/U5-T02/ronda-2.md
