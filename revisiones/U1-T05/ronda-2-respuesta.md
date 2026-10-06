# Respuesta a la ronda 2 — U1-T05

F-01: Corregido en la ronda 3: casos de frontera deterministas (`gen/fixed.go`) recorridos siempre por las propiedades (SHA en cada posición y largo, mayúsculas puras, `refs/headsX`, `x/refs/tags/v1`, todas las acciones y eventos, uuid por grupo, `version` 0/2/-1/1.0, secretos de 1 a 1000 bytes); batería de 241 mutantes del revisor con 20 corridas: ninguno de la clase sobrevive (tabla en la bitácora) y se corrigió la afirmación de la ronda 2.
F-02: Corregido: `GeneratorCoverage` exige entropía mínima; un `Sha40` constante falla 20/20.
F-03: Tarea candidata propuesta: round-trips de `Notification` y `Receipt` sin contraste con el OpenAPI; documentado en `PBT.md` que son tautológicos salvo el esquema.
F-04: Tarea candidata propuesta: huecos menores del almacén (`ErrNotFound` ya cubierto; empates sub-segundo, `.UTC()`, líneas largas, fallo de escritura).
