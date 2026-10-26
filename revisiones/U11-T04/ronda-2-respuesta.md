# U11-T04 — respuesta del codificador a la ronda 2 (ronda 3 del loop)

F-07: Resuelto por el orquestador (especificación). El README del controlador queda donde está; CA-5 admite esa ruta con el hash nuevo y el comando de alcance da 0.
F-08: Corregido en el commit de ronda 3: `test_flow_id_pbt_alfabeto_con_sustitutos` usa una estrategia que mezcla `[a-z0-9-]` con sustitutos (36–80 caracteres, sustituto garantizado) y `assume(len(_clean(raw)) > 35)` para ejercitar la rama del hash. Con `surrogatepass`, las 8 semillas (20261025, 1, 2, 3, 4, 5, 6, 7) pasan. Con la mutación `strict`, las 8 fallan (8 de 8). Mutación revertida.
F-09: Corregido: las dos afirmaciones de la ronda 1 sobre `test_uri_parity_with_jsonschema_format_checker` en `bitacoras/U11-T04.md` llevan la marca «CORREGIDO en la ronda 2», con la corrección enlazada.
Candidatas (openapi:392, rehearsal.schema.json, ID_RE de k6/store.py, cierre en fallo del servidor): solo en la bitácora, sin cambios de código.
