# Ronda 1 — U5-T02

VEREDICTO: NO-VERDE

Hash de la tarea verificado: 55bd7320b5b45be806235523112b3ca99e8725c0 (coincide). HEAD del worktree: ab0c6ed. Worktree limpio antes y después de mi revisión. Las pruebas negativas las hice sobre `git archive` en un directorio temporal y sobre archivos en el scratchpad.

## Criterios de aceptación, verificados por mí (comandos literales de la tarea)
| # | Criterio | Resultado |
|---|---|---|
| 1 | 10 esquemas con nombres exactos | pasa: `10` |
| 2 | compilan en draft2020 | pasa: `10` |
| 3 | envoltorio común y `$id` con `/v1/` | pasa: solo `FIN` |
| 4 | `validate.sh` | pasa: `rc=0`, 12 válidos y 30 inválidos, ambos ≥10. Tardó 1m20s |
| 5 | prueba negativa sobre copia (se borra `event_id` del primer válido) | pasa: `rc=1` |
| 6 | OpenAPI | pasa: redocly 1.25.0 `rc=0` ("Woohoo! valid"), `9` rutas |
| 7 | workflow | pasa: `1`, actionlint `rc=0` sin salida, `0` acciones sin SHA |
| 8 | árbol limpio y sin `.gitkeep` | pasa: `0` y `0` (corrido sobre HEAD ab0c6ed) |

Alcance "Fuera": el diff solo toca `contracts/`, `bitacoras/U5-T02.md` y `.github/workflows/{contracts.yml,.gitkeep}`. No hay `ci.yml`, `deploy/`, `README.md`, `package.json` ni `node_modules`. No aparecen `warm.quarantined` ni `run.started`.

## Puntos que me pediste juzgar
1. **Bitácora.** El rojo inicial está con el comando literal y la salida `FALTA notify.created`. No hay abreviaturas tipo `$ CA-1`. El único `...` (línea 16) es una enumeración en prosa ("deploy.done.json, ..."), no un comando: AMARILLO como mucho. Sí hay un problema con CA-8, ver F-02.
2. **Borrado de `.github/workflows/.gitkeep`.** Es correcto. La regla de U5-T01 dice que `.gitkeep` solo va en directorios vacíos, y el directorio ahora tiene `contracts.yml`. CA-8 solo mira `contracts/`, pero el borrado no es desborde. No hay conflicto real: `git merge-tree --write-tree tarea/U5-T02 tarea/U5-T03` y con `tarea/U5-T04` terminan sin conflictos, porque el borrado idéntico se fusiona limpio. Un orden de fusión raro no lo rompe.
3. **Resolución de esquema por prefijo más largo.** Revisé el criterio `case "$base." in "$n."*)` (el punto final evita que `run.done` coincida con otro prefijo). No hay prefijos anidados entre los 10 nombres, así que `run.done` y `run.confirmed` no se cruzan. `deploy.done` y `deploy.failed` caen en `deploy`, y `rehearsal.*` en `rehearsal`. Corrí el script de resolución sobre los 42 ejemplos y todos resuelven al esquema esperado. Una debilidad, no de resolución sino de verificación: el script no comprueba que el `type` de un ejemplo coincida con su nombre de archivo. Un `deploy.done.json` con `type: deploy.failed` pasaría. Es AMARILLO (F-04).
4. **Restricción de `data` y `type`.**
   - Los 10 esquemas tienen `data.additionalProperties:false`, `required` propio y envoltorio con `additionalProperties:false`.
   - `type` es enum de un valor (o de dos en `deploy` y `rehearsal`) y `version` es `const 1`.
   - Corrí ajv a mano sobre los 30 inválidos con `--errors=line`. Cada uno falla con **un solo error y por la razón que dice su nombre**: `required` de la propiedad indicada, `additionalProperties` (`foo`), `enum` de `type`, `minItems`, `minimum`, `const true`, etc. Ninguno falla por el envoltorio salvo los `wrong-type`, que es lo que declaran. Salida en `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/4ed52fd5-a142-46f0-ac9b-995972cfe97d/scratchpad/inv.txt`.
   - Confirmé que `event_id` no uuid, `occurred_at` no date-time, `version:2` y una propiedad extra en el envoltorio fallan.
   - El agujero está en la correlación `type`/`data`, ver F-01.
5. **OpenAPI.** Cubre los 9 endpoints con `operationId`, `requestBody` con esquema donde aplica (webhook, confirm, policies, login), respuestas con esquema referenciado y códigos de éxito y de error (400/401/403/404/409/422/429), parámetros de ruta y consulta, y `components/schemas` definidos. Pasa lint. Cumple lo pedido ("forma, sin lógica"). Detalles menores en F-04.

## Hallazgos

### F-01 · NARANJA · `contracts/events/deploy.schema.json`, `contracts/events/rehearsal.schema.json` · `type` y `data` no están correlacionados en los esquemas con dos tipos
Los dos esquemas que agrupan dos valores de `type` aceptan datos contradictorios. Lo comprobé con ajv y todos dieron `valid`:
- `rehearsal.passed` con `data.passed: false`.
- `rehearsal.failed` con `data.passed: true`.
- `deploy.failed` sin `data.reason`.
- `deploy.done` con `data.reason`.

Un consumidor no puede fiarse del contrato: el mismo evento puede decir "pasó" y "falló" a la vez. El plan de pruebas exige demostrar que `data` está realmente restringido, y en estos dos esquemas la restricción es parcial. Los 30 inválidos no incluyen ningún caso de esta clase, así que la suite no lo detecta. Falta un `if/then` o `oneOf` por tipo (p. ej. `rehearsal.passed` ⇒ `passed: const true`; `deploy.failed` ⇒ `reason` requerido; `deploy.done` ⇒ sin `reason`) y los inválidos correspondientes. Es una sola clase: se arregla en estos dos archivos.

### F-02 · NARANJA · `bitacoras/U5-T02.md` (última entrada, CA-8) · La evidencia registrada de CA-8 no muestra un pase
La bitácora pega para CA-8 la salida `1` / `0`, pero la tarea espera `0` / `0`. El `1` es el propio `bitacoras/U5-T02.md` sin commitear. El commit `ab0c6ed` ("bitacora, CA-8") añade ese resultado sin explicación y sin rerun en árbol limpio. En la bitácora, CA-8 queda sin verificar, y la evidencia no puede justificar la marca "Verde". Yo lo corrí sobre HEAD y da `0` y `0`, así que el defecto es de evidencia, no de código. Lo dejo NARANJA por el precedente de U5-T01/T04 sobre evidencia imprecisa en la bitácora. Falta anotar el rerun limpio o aclarar por qué se registró `1`.

### F-03 · AMARILLO · `contracts/validate.sh` · Tiempo y robustez
- Lanza `npx` por cada ejemplo (42 invocaciones, 1m20s localmente). Se podría agrupar con `-d` múltiple o un solo proceso.
- Un inválido pasa si ajv falla por cualquier causa (JSON roto, herramienta ausente). Hoy no ocurre, lo verifiqué a mano, pero el script no lo garantiza. Un caso extra o `--errors` con la razón esperada sería más sólido.

### F-04 · AMARILLO · varios · Detalles menores
- `validate.sh` no comprueba que el `type` del ejemplo coincida con el nombre del archivo (ver punto 3).
- `contracts/openapi/control-plane.yaml`: `Notification.artifact` es `string`, mientras que en el evento `notify.created` es un objeto `{kind, ref}`. Eso rompe la coherencia entre contrato REST y contrato de evento. `/auth/logout` no declara 4xx más allá de 401. `/webhooks/github` acepta `type: object` abierto, defendible porque el payload es de GitHub.
- Los 12 válidos comparten el mismo `event_id` y `trace_id`. Es solo ejemplo.
- `bitacoras/U5-T02.md:16` contiene `...` en prosa.

## Tareas candidatas (fuera de alcance)
- Alinear la forma de `artifact` entre OpenAPI (`Notification`) y el evento `notify.created`, cuando U1 implemente el control plane.
- Hacer más rápida la ejecución de `validate.sh` en CI (una sola invocación de ajv) si el workflow se vuelve cuello de botella.

## Rutas de transcripciones largas
- `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/4ed52fd5-a142-46f0-ac9b-995972cfe97d/scratchpad/inv.txt` (errores de ajv de los 30 inválidos)

VEREDICTO: NO-VERDE
NARANJA|contracts/events/deploy.schema.json, contracts/events/rehearsal.schema.json|type y data no correlacionados (rehearsal.passed con passed:false, deploy.failed sin reason validan)
NARANJA|bitacoras/U5-T02.md (CA-8)|La evidencia registrada de CA-8 muestra 1 (esperado 0) sin rerun ni explicación
INFORME: revisiones/U5-T02/ronda-1.md
