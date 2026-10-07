# Ronda 1 — respuesta del codificador (U1-T07)

F-01: Corregido en 3a715c8: un aborto del contexto padre ya no cuenta como fallo ni abre el circuito (`Verify` libera la sonda sin reabrir y devuelve 503); rojo `circuit_open:5 error:5` -> verde con `TestHTTPVerifierParentCancelDoesNotCountAsIdentityFailure`.
F-02: Corregido en 3a715c8: la valla del fake compara `prod` y `production` sin distinguir mayúsculas ni espacios; prueba en `TestFakeAuthIsFencedAndIdentityNeedsURL`.
F-03: Fuera de alcance: tarea candidata propuesta (el outbox JSONL es transitorio, C-45; el coste O(n) desaparece con el transporte real; el relevo está acotado por el mutex y no afecta a la corrección).
F-04: Corregido en 3a715c8: `PublishPendingCount` cuenta recibos sin marca en vez de restar, así que ids huérfanos no dan negativos (`TestPublishPendingCountIgnoresOrphanPublishedIDs`).
F-05: No es un defecto: acuse; contar 5xx/cuerpo inválido/otros códigos como fallo es coherente con «denegar» y está documentado en la bitácora (200/401 reinician la cuenta; sin redirecciones).
