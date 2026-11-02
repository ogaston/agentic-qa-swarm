"""Rutas con plantilla (entrega del módulo 8): un plan puede citar `/orders/{id}` o una instancia concreta
(`/orders/1`). Hallazgo de la evaluación con modelos reales: deepseek-flash concretaba los parámetros y el
validador rechazaba planes correctos. La instancia se acepta con reglas estrictas por segmento."""
import copy
import json

import pytest

from agent_planner.errors import Limits, PlanRejected
from agent_planner.validate import match_endpoint, parse_and_validate

SURFACE = {
    "run_id": "run-1",
    "base_url": "http://app.aqs-test.svc:8080",
    "source": "openapi",
    "endpoints": [
        {"method": "POST", "path": "/orders",
         "request_body": {"type": "object", "required": ["sku"], "properties": {"sku": {"type": "string"}}}},
        {"method": "GET", "path": "/orders/{id}"},
        {"method": "POST", "path": "/orders/{id}/cancel"},
        {"method": "PUT", "path": "/orders/{id}",
         "request_body": {"type": "object", "required": ["qty"], "properties": {"qty": {"type": "integer"}}}},
        {"method": "GET", "path": "/orders/export"},
    ],
}
EP = SURFACE["endpoints"]


def plan_with(*steps):
    return {"run_id": "run-1", "workflow": "wf-1", "flows": [{
        "flow_id": "f-1", "name": "f", "invariant": "i",
        "steps": [dict(s, expect_status=200) for s in steps]}]}


def run(p):
    return parse_and_validate(json.dumps(p), SURFACE, "wf-1", Limits())


@pytest.mark.parametrize("method,path,want", [
    ("GET", "/orders/1", ("GET", "/orders/{id}")),
    ("GET", "/orders/{id}", ("GET", "/orders/{id}")),               # la plantilla literal sigue valiendo
    ("GET", "/orders/abc-DEF_9.~x", ("GET", "/orders/{id}")),
    ("POST", "/orders/999999999/cancel", ("POST", "/orders/{id}/cancel")),
    ("GET", "/orders/export", ("GET", "/orders/export")),           # exacta antes que plantilla
    ("POST", "/orders", ("POST", "/orders")),
])
def test_match_acepta(method, path, want):
    assert match_endpoint(method, path, EP) == want


@pytest.mark.parametrize("method,path", [
    ("GET", "/orders/"),                 # segmento vacío
    ("GET", "/orders/.."),               # recorrido
    ("GET", "/orders/."),
    ("GET", "/orders/1/2"),              # segmentos de más
    ("GET", "/orders/a%2Fb"),            # / codificada
    ("GET", "/orders/1?x=1"),            # query
    ("GET", "/orders/1#f"),
    ("GET", "/orders/a b"),
    ("GET", "//evil.example/orders/1"),
    ("DELETE", "/orders/1"),             # método no observado
    ("POST", "/orders/1"),               # POST solo existe sin parámetro
    ("GET", "/products/1"),
    ("GET", None),
])
def test_match_rechaza(method, path):
    assert match_endpoint(method, path, EP) is None


def test_plan_con_instancias_pasa_y_no_se_reescribe():
    p = plan_with({"method": "POST", "path": "/orders", "body": {"sku": "A"}},
                  {"method": "GET", "path": "/orders/1"},
                  {"method": "POST", "path": "/orders/1/cancel"})
    assert run(copy.deepcopy(p)) == p


def test_el_cuerpo_se_valida_contra_la_plantilla():
    with pytest.raises(PlanRejected) as e:
        run(plan_with({"method": "PUT", "path": "/orders/7", "body": {"qty": "no-entero"}}))
    assert e.value.reason.startswith("body_invalido:0/0")
    with pytest.raises(PlanRejected) as e:
        run(plan_with({"method": "PUT", "path": "/orders/7"}))
    assert e.value.reason == "body_requerido:0/0"
    assert run(plan_with({"method": "PUT", "path": "/orders/7", "body": {"qty": 2}}))


def test_instancia_invalida_sigue_rechazada():
    with pytest.raises(PlanRejected) as e:
        run(plan_with({"method": "GET", "path": "/orders/.."}))
    assert e.value.reason == "endpoint_no_observado:0/0"
