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

// Corrida de la vista /runs/:id (mockups/swarm-mock.html, applyRunDemo('bug')). Plan de dos flujos con los
// métodos y estados del mockup (GET/POST con 201; POST con 409) e informe con su único hallazgo. Sin cuerpos
// de solicitud: el mockup no los muestra y los flujos van plegados.
export const RUN_ID = 'run-84a1d9b4c02e11ef';

export const PLAN_FIDELIDAD = {
  run_id: RUN_ID,
  workflow: 'default',
  flows: [
    {
      flow_id: 'f-camino-feliz',
      name: 'Camino feliz de consulta y pedido válido',
      invariant: 'Un pedido con unidades dentro del stock disponible devuelve 201 y reduce el inventario exactamente en la cantidad pedida.',
      invariant_probe: false,
      steps: [
        { method: 'GET', path: '/products', expect_status: 200 },
        { method: 'POST', path: '/orders', expect_status: 201 },
      ],
    },
    {
      flow_id: 'f-sondeo-stock',
      name: 'Sondeo de agotamiento de stock',
      invariant: 'Solicitar una cantidad mayor al stock disponible debe ser rechazado con 409 o 422 sin alterar el saldo.',
      invariant_probe: true,
      steps: [{ method: 'POST', path: '/orders', expect_status: 409 }],
    },
  ],
};

export const INFORME_FIDELIDAD = {
  run_id: RUN_ID,
  verdict: 'bug',
  summary:
    'Se detectó una violación del invariante de inventario en el endpoint de creación de pedidos: al enviar una cantidad superior al stock disponible, el servicio descontó existencias dejando el saldo en negativo (-3 unidades) y respondió 201 Created en lugar de rechazar la operación.',
  findings: [
    {
      finding_id: 'finding-stock-negativo-01',
      root_cause:
        'El handler de POST /orders deduce la cantidad solicitada sin validar previamente la disponibilidad actual cuando el pedido incluye el flag promocional.',
      invariant: 'El stock de un producto nunca puede quedar menor que 0 tras crear o confirmar un pedido (stock >= 0).',
      method: 'POST',
      path: '/orders',
      evidence_uris: [
        `s3://evidence/${RUN_ID}/runner-flow-stock-probe.json`,
        `s3://evidence/${RUN_ID}/http-trace-orders-post.log`,
      ],
    },
  ],
};

// Mis corridas (mockups/swarm-mock.html, vista «runs»): dos recibos, del más reciente al más antiguo.
// La hora se envía en UTC; la app la muestra con toLocaleString() del navegador (dato de presentación).
export const RECIBOS_FIDELIDAD = [
  {
    run_id: RUN_ID,
    notification_id: 'notif-7f4790e-01',
    confirmed_by: 'operador-demo',
    confirmed_at: '2026-10-21T20:41:12Z',
  },
  {
    run_id: 'run-3c09e12a77b40019',
    notification_id: 'notif-a1c9e02-02',
    confirmed_by: 'operador-demo',
    confirmed_at: '2026-10-21T18:15:04Z',
  },
];

// Entorno (mockups/swarm-mock.html, vista «warm», estado «Listo»). warm_id = aqs-test: el mockup lo muestra en el
// título y en la cuarta tarjeta el identificador es env-aqs-test-01; la app tiene un solo campo, así que se usa aqs-test
// en ambos sitios (diferencia de dato registrada en la bitácora).
export const WARM_FIDELIDAD = {
  warm_id: 'aqs-test',
  state: 'ready',
  reset_verified: true,
  baseline_version: 'v1.4.2-baseline',
};
