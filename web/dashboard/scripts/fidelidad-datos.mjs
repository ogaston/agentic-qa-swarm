// U10-T06: datos que la compuerta de fidelidad sirve a la app (respuestas page.route).
// Reproducen los datos del mockup (mockups/swarm-mock.html, NOTIFICATIONS) en la forma del contrato
// (contracts/openapi/control-plane.yaml, Notification: artifact = {kind, ref}, kind ∈ build-from-repo | published-image).
// Solo datos: no contiene tolerancias ni lógica de comparación.

export const NOTIFICACIONES = [
  {
    id: 'notif-7f4790e-01',
    github_event: 'commit',
    repo: 'omarjayg/target-orders-app',
    sha: '7f4790e8b21c4a901e330c84a1d9b4c02e11ef99',
    artifact: { kind: 'published-image', ref: 'ghcr.io/omarjayg/target-orders:7f4790e' },
    state: 'pending',
  },
  {
    id: 'notif-c4e21ab-02',
    github_event: 'pull_request',
    repo: 'omarjayg/target-orders-app',
    sha: 'c4e21ab90d114f88a3201b77e51920a11099bc12',
    artifact: { kind: 'build-from-repo', ref: 'refs/pull/18/merge' },
    state: 'pending',
  },
  {
    id: 'notif-v143-03',
    github_event: 'tag',
    repo: 'omarjayg/target-orders-app',
    sha: '991a04bc72e189f0021a774c89012e33a81200f1',
    artifact: { kind: 'published-image', ref: 'ghcr.io/omarjayg/target-orders:v1.4.3' },
    state: 'pending',
  },
  {
    id: 'notif-a1c9e02-04',
    github_event: 'commit',
    repo: 'omarjayg/target-orders-app',
    sha: 'a1c9e0277b4001984a1d9b4c02e11ef881203944',
    artifact: { kind: 'published-image', ref: 'ghcr.io/omarjayg/target-orders:a1c9e02' },
    state: 'confirmed',
  },
  {
    id: 'notif-55e019a-05',
    github_event: 'pull_request',
    repo: 'omarjayg/target-orders-app',
    sha: '55e019a44c81920f11234a901e330c84a1d9b4c0',
    artifact: { kind: 'build-from-repo', ref: 'refs/pull/15/merge' },
    state: 'confirmed',
  },
];
