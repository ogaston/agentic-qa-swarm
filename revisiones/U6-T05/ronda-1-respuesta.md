# Respuesta del codificador a la ronda 1 — U6-T05

F-01: Corregido en 653768118f1a714c9b5b3431d1c55937b41b7924: se añaden tres pruebas. (a) `src/session/nav.test.tsx` comprueba los tres enlaces con su `href`. (b) `RunPage.test.tsx` «cambiar de :id no muestra datos de la corrida anterior ni vuelve a pedirla». (c) `WarmPage.test.tsx` casos negativos `dirty`+`reset_verified=false` y `ready`+`true` sin aviso. Mutaciones (quitar el enlace «Mis corridas», quitar `key={id}`, quitar `data.state === 'ready' &&`): cada una hace fallar exactamente la prueba prevista (`nav`, `key`, `warm`).
F-02: Corregido en 653768118f1a714c9b5b3431d1c55937b41b7924: `usePolling` espera `max(intervalMs, retryAfter*1000)` cuando el error trae `Retry-After`. Prueba roja previa: 5 peticiones en 12 s; verde: 1 petición en 12 s y la segunda a los 30 s.
F-03: Corregido en 653768118f1a714c9b5b3431d1c55937b41b7924: el JSDoc dice que desmontar descarta el resultado y no aborta la petición en curso.
F-04: Corregido en 653768118f1a714c9b5b3431d1c55937b41b7924: `/warm` y `/runs/:id` muestran «Mostrando la última lectura correcta.» junto al error cuando hay un dato previo. Prueba roja previa en `WarmPage.test.tsx`.
