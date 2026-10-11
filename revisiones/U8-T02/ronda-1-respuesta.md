# Respuesta a la ronda 1 — U8-T02

F-01: Corregido en el commit de ronda 2 (ver bitácora «Ronda 2»):
- (1) sin-duplicados: ahora cuenta los registros de creación `confirmed` de la corrida en `runs.jsonl` del
  controlador antes y después de inyectar el `run.confirmed` duplicado; exige 1 y 1. Mutación (controlador sin
  dedupe): `antes=1 después=3`, `FALLA sin-duplicados`, rc=1.
- (2) reentrega-tras-reinicio: ya no decide el replay. Exige que el durable `ui-api-inbox` acuse el mensaje
  publicado con ui-api parada (`ack_floor.stream_seq >= secuencia`). Mutación (ui-api sin Ack al durable):
  `FALLA reentrega-tras-reinicio ... ack_floor=0, secuencia=1`, rc=1.

F-02: Corregido: `TestNATSSubscriberMismoEventIDDistintaNotificacion` (mismo event_id, notification_id n-1 y n-2;
solo n-1 queda). Mutación (Store sin dedupe por event_id): `FAIL` con `n-2` aplicado. Store restaurado.
- El caso «Ack antes de aplicar» no se añade: el handler del suscriptor es síncrono y no devuelve error, así que
  no hay un fallo de aplicación que provocar. La reentrega sin Ack la prueba ahora el script (F-01 2) y el
  controlador ya tenía `TestNATSSourceAckYReentregaSinAck`.

F-03: Corregido: `/readyz` de ui-api en modo nats incluye el chequeo `replay`, que falla hasta que
`NATSSubscriber.replayed` queda en true. Prueba: `TestTransportNATSSinServidorArrancaYNoEstaListo` comprueba que
`replay` existe y falla antes del replay.

F-04: (b) Los tests de selección de transporte se escribieron después; queda anotado en la bitácora y no se toca.
(c) Subidas transitivas por resolución de versiones: aceptadas. (d) base: sin cambios.

F-05: No se toca, por decisión de diseño:
- `ConnectNATS`/`NATSReady` copiados por servicio: cada servicio es un módulo Go independiente; un paquete común
  exigiría un módulo nuevo fuera de alcance.
- WARN por tick mientras el stream no existe: solo durante `nats-init`; el log es informativo y no afecta al estado.
- Efímero del replay: lo elimina NATS por inactividad (umbral por defecto del servidor).
