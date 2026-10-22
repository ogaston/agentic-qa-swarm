# Ronda 1 — respuesta del codificador (U9-T03)

F-01: Corregido en 1a88f0f: el plan se pide en cuanto la corrida es `failed`, sin depender de `ultimaFase` (que solo se llena con estados no fallidos vistos en este montaje). Si no hay plan, la API responde `plan_not_ready` y la vista muestra «Plan aún no disponible». Pruebas nuevas en RunPage.resultados.test.tsx: «failed abierto directamente pide el plan una vez y muestra «Flujos ejecutados»» (roja antes: plan pedido 0 veces) y «failed llegando desde rehearsing pide el plan y lo muestra».

F-02: Corregido en 1a88f0f: pruebas añadidas para 404 de reports con otro código («Sin acceso al informe»), error de red (status 0, aviso con «Reintentar»), error genérico (aviso común con el código), 502 en el plan (Reintentar y segunda petición) y failed sin plan («Plan aún no disponible»).

Candidatas (arbitraje del coordinador, no tocadas): 404 transitorio tras confirmar; notificación pending de prueba del smoke; «familia» = name/flow_id.
