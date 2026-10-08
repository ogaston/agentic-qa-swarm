// aqs-k6-template: v1
import http from 'k6/http';
import { check, fail } from 'k6';

export const options = { vus: 1, iterations: 1, thresholds: { checks: ['rate==1'] } };

const FLOW = {"flow_id": "f1", "invariant": "el total coincide con la suma de lineas", "name": "flujo 1", "steps": [{"expect_status": 201, "method": "POST", "path": "/orders/1"}, {"expect_status": 200, "method": "GET", "path": "/orders/1"}]};

export default function () {
  const base = __ENV.BASE_URL;
  if (typeof base !== 'string' || !/^https?:\/\//.test(base)) {
    // fail() solo aborta la iteracion (k6 saldria con 0): un check fallido rompe el umbral.
    check(null, { base_url_http: () => false });
    fail('BASE_URL ausente o no es http(s)');
  }
  const root = base.replace(/\/+$/, '');
  for (let i = 0; i < FLOW.steps.length; i++) {
    const step = FLOW.steps[i];
    const res = http.request(step.method, root + step.path);
    const name = FLOW.flow_id + ':' + i;
    const checks = {};
    checks[name] = (r) => r.status === step.expect_status;
    check(res, checks);
  }
}
