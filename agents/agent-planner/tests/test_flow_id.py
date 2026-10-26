"""U11-T04: flow_id normalizado por el planner antes de validar; cadena completa sin clúster."""
import json

from agent_planner.planner import plan
from helpers import LIMITS, SURFACE, good_plan, llm_for


def test_cadena_flow_id_malo_llega_normalizado():
    bad = good_plan()
    bad["flows"][0]["flow_id"] = "Oversell_Probe"
    out = plan(SURFACE, "wf-1", llm_for(SURFACE, "wf-1", json.dumps(bad)), LIMITS)
    assert out["flows"][0]["flow_id"] == "oversell-probe"


import copy
import re

from hypothesis import assume, given, settings
from hypothesis import strategies as st

from agent_planner.flow_id import FLOW_ID_PATTERN, _clean, normalize_flow_id, normalize_plan_flow_ids
from helpers import load, ROOT

FLOW_RE = re.compile(FLOW_ID_PATTERN)


def test_normalize_ejemplo_del_spec():
    assert normalize_flow_id("Oversell_Invariant Probe!") == "oversell-invariant-probe"


def test_normalize_largo_acotado_estable_y_distinto():
    a = normalize_flow_id("x" * 60)
    assert len(a) <= 35 and FLOW_RE.fullmatch(a)
    assert a == normalize_flow_id("x" * 60)  # estable entre llamadas
    b = normalize_flow_id("x" * 59 + "y")  # mismo comienzo, otro id
    assert a != b


def test_normalize_vacio_o_solo_simbolos_es_flujo():
    for raw in ("", "!!!", "___", "   ", "ñ日本"):
        assert normalize_flow_id(raw) == "flujo", repr(raw)


def test_normalize_colision_dentro_del_plan():
    plan = {"flows": [{"flow_id": "Flujo"}, {"flow_id": "flujo!"}, {"flow_id": "flujo"}]}
    ids = [f["flow_id"] for f in normalize_plan_flow_ids(plan)["flows"]]
    assert ids == ["flujo", "flujo-2", "flujo-3"]


def test_normalize_colision_respeta_el_limite_de_35():
    base = "a" * 35
    plan = {"flows": [{"flow_id": base}, {"flow_id": base}]}
    ids = [f["flow_id"] for f in normalize_plan_flow_ids(plan)["flows"]]
    assert len(set(ids)) == 2 and all(len(i) <= 35 and FLOW_RE.fullmatch(i) for i in ids)


def test_normalize_plan_ignora_lo_que_no_es_cadena():
    plan = {"flows": [{"flow_id": 7}, {"name": "sin id"}, "no-objeto"]}
    assert normalize_plan_flow_ids(copy.deepcopy(plan)) == plan


@given(st.text())
def test_flow_id_pbt_normalizacion_cumple_patron_e_idempotente(raw):
    out = normalize_flow_id(raw)
    assert FLOW_RE.fullmatch(out) and len(out) <= 35
    assert normalize_flow_id(out) == out


@given(st.lists(st.text(), min_size=1, max_size=8))
def test_flow_id_pbt_plan_unico_y_en_patron(raws):
    flows = normalize_plan_flow_ids({"flows": [{"flow_id": r} for r in raws]})["flows"]
    ids = [f["flow_id"] for f in flows]
    assert len(set(ids)) == len(ids)
    assert all(FLOW_RE.fullmatch(i) for i in ids)


def test_fixtures_planner_kind_pasan_el_validador():
    from agent_planner.validate import validate_plan_dict
    from agent_planner.errors import Limits

    fx = load(ROOT / "deploy" / "flux" / "kind" / "agents" / "planner" / "target-app.json")
    out = validate_plan_dict(copy.deepcopy(fx["plan"]), fx["surface"], fx["workflow"], Limits(max_flows=5, max_steps=10))
    assert out["flows"]


# F-01: sustitutos sueltos (json.loads los produce con \ud800) no deben romper la normalizacion.
def test_normalize_sustituto_suelto_no_lanza_y_cumple_patron():
    for raw in ("a" * 40 + "\ud800", "\ud800", "x\udfff" * 30):
        out = normalize_flow_id(raw)
        assert FLOW_RE.fullmatch(out) and len(out) <= 35
        assert normalize_flow_id(out) == out


def test_plan_sustituto_suelto_en_flow_id_no_lanza():
    bad = good_plan()
    bad["flows"][0]["flow_id"] = "a" * 40 + "\ud800"
    out = plan(SURFACE, "wf-1", llm_for(SURFACE, "wf-1", json.dumps(bad)), LIMITS)
    assert FLOW_RE.fullmatch(out["flows"][0]["flow_id"])


_LIMPIO = st.sampled_from("abcdefghijklmnopqrstuvwxyz0123456789-")
_ALFABETO_CON_SUSTITUTOS = st.one_of(_LIMPIO, st.characters(blacklist_categories=()))


@st.composite
def _id_largo_con_sustituto(draw):
    """Id de 36 a 80 caracteres con al menos un sustituto suelto: fuerza la rama del hash (>35)."""
    n = draw(st.integers(36, 80))
    chars = draw(st.lists(_ALFABETO_CON_SUSTITUTOS, min_size=n, max_size=n))
    pos = draw(st.integers(0, n - 1))
    chars[pos] = "\ud800"  # sustituto garantizado, en posicion arbitraria
    return "".join(chars)


@given(_id_largo_con_sustituto())
@settings(max_examples=500)
def test_flow_id_pbt_alfabeto_con_sustitutos(raw):
    assume(len(_clean(raw)) > 35)  # garantiza que se ejercita la rama del hash (>35 tras limpiar)
    out = normalize_flow_id(raw)
    assert FLOW_RE.fullmatch(out) and len(out) <= 35
    assert normalize_flow_id(out) == out


def test_server_sustituto_en_flow_id_responde_200_normalizado():
    import http.client
    import threading

    from agent_planner.errors import Limits
    from agent_planner.server import make_server

    bad = good_plan()
    bad["flows"][0]["flow_id"] = "a" * 40 + "\ud800"
    httpd = make_server("127.0.0.1", 0, llm_for(SURFACE, "wf-1", json.dumps(bad)), Limits())
    threading.Thread(target=httpd.serve_forever, daemon=True).start()
    try:
        c = http.client.HTTPConnection("127.0.0.1", httpd.server_address[1], timeout=5)
        c.request("POST", "/v1/plan", body=json.dumps({"surface": SURFACE, "workflow": "wf-1"}).encode(),
                  headers={"Content-Type": "application/json"})
        r = c.getresponse()
        body = json.loads(r.read())
        assert r.status == 200, (r.status, body)
        assert FLOW_RE.fullmatch(body["flows"][0]["flow_id"])
    finally:
        httpd.shutdown(); httpd.server_close()


# F-02: la constante Python es la del esquema empaquetado (el que usa jsonschema en el validador).
def test_flow_id_parity_python_constante_con_esquema_empaquetado():
    schema = load(ROOT / "agents" / "agent-planner" / "src" / "agent_planner" / "schemas" / "flow-plan.schema.json")
    assert schema["properties"]["flows"]["items"]["properties"]["flow_id"]["pattern"] == FLOW_ID_PATTERN


# F-04: ejemplos inválidos (glob) rechazados por el esquema y normalizables; válidos aceptados tal cual.
def test_flow_id_examples_glob_schema_and_normalization():
    from jsonschema import Draft202012Validator
    from agent_planner.flow_id import normalize_plan_flow_ids
    from agent_planner.validate import validate_plan_dict
    from agent_planner.errors import Limits

    schema = load(ROOT / "agents" / "agent-planner" / "src" / "agent_planner" / "schemas" / "flow-plan.schema.json")
    v = Draft202012Validator(schema)
    d = ROOT / "contracts" / "plans" / "examples"
    inv = sorted((d / "invalid").glob("flow-plan.flow-id-*.json"))
    val = sorted((d / "valid").glob("flow-plan*.json"))
    assert len(inv) >= 5 and len(val) >= 2
    for f in inv:
        p = load(f)
        assert list(v.iter_errors(p)), f.name
        norm = normalize_plan_flow_ids(copy.deepcopy(p))
        assert not list(v.iter_errors(norm)), f.name
        surf = {"run_id": p["run_id"], "base_url": "http://x.warm.svc.cluster.local:8080", "source": "openapi",
                "endpoints": [{"method": s["method"], "path": s["path"]} for fl in p["flows"] for s in fl["steps"]]}
        validate_plan_dict(norm, surf, p["workflow"], Limits(max_flows=10, max_steps=10))
    for f in val:
        assert not list(v.iter_errors(load(f))), f.name


# F-05: cadena completa por plan(): id largo, colisión tras normalizar, plan de N flujos con ids malos.
def test_cadena_largo_colision_y_lote_pasan_patron_y_son_unicos():
    long_id = "Flujo " + "x" * 80
    flows = [
        {"flow_id": long_id, "name": "largo", "steps": [{"method": "POST", "path": "/orders", "expect_status": 201}],
         "invariant": "i"},
        {"flow_id": "Flujo_A", "name": "a1", "steps": [{"method": "POST", "path": "/orders", "expect_status": 201}],
         "invariant": "i"},
        {"flow_id": "flujo a", "name": "a2", "steps": [{"method": "POST", "path": "/orders", "expect_status": 201}],
         "invariant": "i"},
    ] + [
        {"flow_id": f"Mal_{k}!!", "name": f"n{k}", "steps": [{"method": "POST", "path": "/orders", "expect_status": 201}],
         "invariant": "i"} for k in range(4)
    ]
    p = {"run_id": "run-1", "workflow": "wf-1", "flows": flows}
    out = plan(SURFACE, "wf-1", llm_for(SURFACE, "wf-1", json.dumps(p)), LIMITS)
    ids = [f["flow_id"] for f in out["flows"]]
    assert len(ids) == len(set(ids)) == len(flows)
    assert all(FLOW_RE.fullmatch(i) and len(i) <= 35 for i in ids)
    assert ids[1] == "flujo-a" and ids[2] == "flujo-a-2"
