# NATS JetStream (U8-T01)

Transporte de eventos de la plataforma (decisión C-45). Un StatefulSet `nats` de una réplica en
`aqs-system`, con JetStream en un PVC, y el Job `nats-init` que crea el stream `AQS_EVENTS`.

## Endpoints

| Qué | Dirección | Quién lo alcanza |
|---|---|---|
| Cliente | `nats://nats.aqs-system.svc:4222` (Service) | solo pods con `aqs.io/nats-client: "true"` |
| Monitorización | `:8222` dentro del pod (sin Service) | solo las sondas `/healthz` del propio pod |

## Aislamiento

- `default-deny-ingress` y `allow-same-namespace` (en `security/networkpolicies-system.yaml`):
  `allow-same-namespace` excluye a NATS, así que un pod del namespace sin la etiqueta no llega.
- `nats-clients-only` (`networkpolicy.yaml`): ingreso al 4222 solo desde pods con
  `aqs.io/nats-client: "true"`. Los llevan `go-intake`, `ui-api` y `go-run-controller`
  (`control-plane.yaml`) y el Job `nats-init`.
- Sin autenticación en NATS en esta tarea. Candidata: credenciales NKey.

## Subjects

- Patrón: `aqs.events.<tipo>`, con el tipo del evento tal cual.
  - `aqs.events.notify.created`
  - `aqs.events.run.confirmed`
  - (los demás tipos que publique cada servicio en U8-T02 siguen el mismo patrón)
- El stream `AQS_EVENTS` captura `aqs.events.>`.

## Stream `AQS_EVENTS` (creado por `nats-init`)

| Parámetro | Valor |
|---|---|
| Subjects | `aqs.events.>` |
| Storage | file |
| Retención | limits |
| `max_age` | 168 h (7 días) |
| `duplicate_window` | 2 min (120 s), deduplicación por `Nats-Msg-Id` |
| Réplicas | 1 |

`nats-init` es idempotente: si el stream existe, no lo modifica.

## Consumidores

Los consumidores durables los crea cada servicio en U8-T02. No se crean aquí.

## Comprobación manual

```bash
kubectl -n aqs-system run nats-probe --rm -i --restart=Never --labels=aqs.io/nats-client=true \
  --image=docker.io/natsio/nats-box:0.14.5 -- \
  nats --server nats://nats.aqs-system.svc:4222 stream info AQS_EVENTS --json
```

El pod de prueba lleva la etiqueta de cliente: sin ella, la NetworkPolicy lo bloquea.
