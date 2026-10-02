# Ronda 1 — U5-T12

VEREDICTO: VERDE

Hash de la tarea: 704f6370a43c479cfb84cf0ebd5305d4c2f91db9 (coincide). HEAD de la rama: 6da89d1.
`origin/main` y `main` están en 6858c43, y ese es también el merge-base de la rama (ya incluye U5-T09).
`git merge-tree --write-tree origin/main tarea/U5-T12` dio rc=0 y el árbol 307666a, así que no hay conflicto.

## Criterios de aceptación, verificados por mí
| # | Criterio | Resultado |
|---|---|---|
| CA-1 | Comando literal con `egress: null` sobre el build real de prod | `rc=1`, pasa |
| CA-2 | Matriz sobre el build real | egress-no-lista rc=1; ipblock-vacio rc=1; ipblock-null rc=1; from-null-mas-ipblock rc=1; ok-ingress-intra-egress-null rc=0; ok-otro-namespace rc=0. Pasa |
| CA-3 | `verify` sin tubería, más dev y prod | `55 tests, 55 passed, 0 warnings, 0 failures, 0 exceptions, 0 skipped`, **`verify rc=0` real** (sin `\| tail`); `dev rc=0`; `prod rc=0`. En main hay 47 pruebas, así que +8 (el mínimo era +5). Pasa |
| CA-4 | Solo archivos permitidos; 0 líneas borradas en las pruebas | Archivos: `bitacoras/U5-T12.md`, `policy/security.rego`, `policy/security_test.rego`. Líneas borradas en `security_test.rego`: `0`. Pasa |
| CA-5 | Árbol limpio | `git status --short \| wc -l` da `0`. Pasa |

En los casos rc=1 de CA-1 y CA-2 comprobé que lo que falla es la regla de ipBlock y no otra. El mensaje es `NetworkPolicy/zz: ipBlock prohibido en aqs-test`, y en el caso `to-str` el fallo es de `egress.rego`.

## Punto 2: intentos de evasión sobre el build real
| Caso | rc | Juicio |
|---|---|---|
| `ingress: [{from:[{ipBlock}]}], egress: null` | 1 | denegado |
| `ports` y `from` en la misma regla | 1 | denegado |
| peer `null` o `"x"` junto a un peer ipBlock | 1 | denegado |
| regla `null` o `"x"` junto a una regla con ipBlock | 1 | denegado |
| ipBlock en `egress[].to` | 1 | denegado |
| `ingress: {}` (objeto) | 0 | sin impacto, ver abajo |
| `ingress: {from:[{ipBlock}]}` (objeto con ipBlock) | 0 | sin impacto, ver abajo |
| `from: {}` | 0 | sin impacto, ver abajo |
| clave `IPBlock` (otras mayúsculas) | 0 | no importa, ver abajo |
| `spec: null` | 0 | sin impacto, ver abajo |

- **`ingress` como objeto y `from: {}`:** el API de Kubernetes exige un arreglo, y `{}` no declara ningún peer. El normalizador los trata como `[]`, que es lo que pide la tarea.
- **Objeto con ipBlock dentro de `ingress`:** no es un NetworkPolicy válido y el servidor lo rechaza. El diseño exigido por la tarea ("no arreglo → `[]`") lo deja pasar, así que no es un defecto de esta tarea. Lo anoto en AMARILLO (F-01).
- **Clave `IPBlock`:** Kubernetes no la reconoce como `ipBlock`, y un peer sin campos válidos es inválido. No es una evasión práctica.
- **`spec: null`:** no hay reglas que evaluar, y el API tampoco lo acepta.

## Punto 3: la candidata en `egress.rego` (hallazgo sobre main, no defecto de U5-T12)
Falla CERRADO. En conftest, el `eval_type_error` de `count(rule.to)` hace que `egress_intra` y `egress_dns` queden indefinidas. Entonces `not egress_rule_ok(rule)` se cumple y se emite el deny. Lo comprobé sobre el build real de prod:
- `egress: [{to: null}]`: `rc=1`, `aqs-test: NetworkPolicy zz, egress[0] no es intra-namespace ni DNS...`.
- `egress: [{to: "x"}]`: `rc=1`, con el mismo mensaje.
- `egress: [{ports:[...]}]` sin `to`: `rc=1`.
- `egress: [{ports: null, to:[{podSelector:{}}]}]`: `rc=0`. Es intra-namespace legítimo y `ports` no se evalúa en ese camino, así que es correcto.

Conclusión: no hay evasión de `egress.rego`. El único efecto es un mensaje de error genérico en lugar de uno específico. Severidad AMARILLO (F-02), tarea candidata de robustez de baja prioridad. No bloquea nada.

## Punto 4: diff y mutación
- `security_test.rego` solo tiene +35 líneas añadidas al final (desde la línea 96), sin ninguna borrada. `git diff 6858c43..tarea/U5-T12 -- policy/security.rego` toca únicamente la regla de ipBlock (líneas 30-36) y añade el helper `lista`. El resto de las reglas no se tocó. `default_deny`, `egress*`, `isolation*`, `namespace*` y `deploy/` no aparecen en el diff.
- Mutación reproducida por mí en un `mktemp -d`. Quité `lista(...)` de los cuatro campos y volví a `peer.ipBlock`, y `conftest verify` falló con `rc=1`: `policy/security.rego:33: eval_type_error: array.concat: operand 1 must be array but got null`. Es real, pero es un error de evaluación que aborta todo el `verify`, no un fallo de una prueba con nombre. Las pruebas nuevas la detectan igualmente (se ejecutan todas bajo `verify`).
- Plan de pruebas: hay una prueba negativa por cada caso rc=1 de CA-1 y CA-2, más dos positivas (`egress: null` con ingress intra-namespace, y otro namespace). Se cumple. Las pruebas comprueban `count(deny)` y no solo códigos de salida; las demás comprobaciones de rc sirven para el build real.
- Bitácora: tiene el rojo inicial `rc=0`, el comando de CA-1 con `pwd`, la mutación y la decisión del helper `netpol_t12`. Los comandos están descritos, no pegados literalmente, para CA-2 y CA-3. Lo anoto como AMARILLO (F-03).
- Valores incrustados: el literal `"ipBlock"` está en la regla porque es el nombre del campo de la API, no un valor visible para el usuario, así que no cuenta como incrustado en el sentido de la regla.

## Hallazgos
### F-01 · AMARILLO · security.rego:33-34 · Un `ingress`/`rule` objeto con `ipBlock` queda tratado como `[]`
Es una consecuencia directa del diseño que fija la tarea ("no arreglo → `[]`"), y el API de Kubernetes lo rechaza. No bloquea.

### F-02 · AMARILLO · egress.rego:20,33 (sobre main, NO defecto de U5-T12) · `to: null` o no-lista produce `eval_type_error` silencioso
Falla cerrado, con mensaje genérico. Tarea candidata de robustez, no una evasión.

### F-03 · AMARILLO · bitacoras/U5-T12.md · La bitácora resume CA-2 y CA-3 en lugar de pegar el comando literal y su salida
No bloquea.

Ningún hallazgo ROJO ni NARANJA en pie.

## Tareas candidatas
- Robustez de `egress.rego` frente a `to`/`ports` null o no-lista: usar un `lista()` equivalente para que el mensaje del deny sea específico (F-02).

## Aviso sobre el worktree
Por error mío ejecuté `git checkout --detach origin/main` en `wt-U5-T12` durante la mutación. Lo revertí de inmediato con `git checkout tarea/U5-T12`. Estado final: HEAD 6da89d1, rama `tarea/U5-T12`, 0 archivos modificados. No hay cambios de contenido; solo un defecto en cómo manejé el entorno. No hay otros residuos (los temporales estaban en `mktemp -d` y se borraron).

Rutas relevantes: `/home/omarjayg/Javeriana/topicos-especiales/wt-U5-T12/policy/security.rego`, `/home/omarjayg/Javeriana/topicos-especiales/wt-U5-T12/policy/security_test.rego`, `/home/omarjayg/Javeriana/topicos-especiales/wt-U5-T12/bitacoras/U5-T12.md`.

VEREDICTO: VERDE
AMARILLO|security.rego:33-34|Objeto con ipBlock dentro de ingress/rule se trata como lista vacía (consecuencia del diseño pedido)
AMARILLO|egress.rego:20,33 (main, fuera de alcance)|`to`/`ports` null produce eval_type_error pero falla CERRADO (rc=1); no es evasión
AMARILLO|bitacoras/U5-T12.md|CA-2 y CA-3 resumidos, no pegados literalmente
INFORME: revisiones/U5-T12/ronda-1.md
