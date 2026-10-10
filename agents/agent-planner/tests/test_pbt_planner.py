"""Propiedades (Hypothesis) del planner: PBT-02 round-trip, PBT-03 invariantes. Correr: pytest -k pbt."""
import copy
import json
import math

import pytest
from hypothesis import assume, given, settings
from hypothesis import strategies as st

import gen
from agent_planner.errors import BudgetExceeded, Limits, PlanRejected
from agent_planner.fakes.fake_llm import FakeLLM, prompt_hash
from agent_planner.k6.render import extract_flow, render_flow
from agent_planner.k6.validate import validate_script
from agent_planner.llm import UnscriptedPrompt
from agent_planner.planner import est_tokens, plan as run_plan
from agent_planner.prompt import CLOSE_TAG, INSTRUCTIONS, OPEN_TAG, build_prompt
from agent_planner.schemas_loader import validator
from agent_planner.validate import FLOW_ID_RE, parse_and_validate


# 1 (PBT-02) FlowPlan: parse(serialize(plan)) == plan y valida contra el esquema
@given(plan=gen.flow_plan(), ensure_ascii=st.booleans(), sort_keys=st.booleans())
def test_pbt_flow_plan_roundtrip_and_schema(plan, ensure_ascii, sort_keys):
    wire = json.dumps(plan, ensure_ascii=ensure_ascii, sort_keys=sort_keys)
    assert json.loads(wire) == plan
    assert list(validator("flow-plan.schema.json").iter_errors(json.loads(wire))) == []
    assert 1 <= len(plan["flows"]) <= gen.MAX_FLOWS


# 2 (PBT-02) k6: extract(render(flow)) == flow, ASCII puro, determinista
@given(plan=gen.flow_plan(), data=st.data())
def test_pbt_k6_render_extract_roundtrip(plan, data):
    f = data.draw(st.sampled_from(plan["flows"]))
    s1 = render_flow(plan, f)
    assert extract_flow(s1) == f
    assert render_flow(copy.deepcopy(plan), copy.deepcopy(f)) == s1
    assert s1.isascii()  # el literal escapa todo no-ASCII: el script no depende de la codificacion
    assert sum(1 for ln in s1.split("\n") if ln.startswith("const FLOW = ")) == 1
    validate_script(s1, plan, f)


def _observed(surf):
    return {(e["method"], e["path"]) for e in surf["endpoints"]}


def _cites_unobserved(raw, surf):
    """Oraculo independiente: True si el JSON del modelo cita algun (method, path) fuera de la superficie."""
    obs = _observed(surf)
    if not isinstance(raw, dict) or not isinstance(raw.get("flows"), list):
        return False
    for fl in raw["flows"]:
        for st_ in (fl.get("steps") if isinstance(fl, dict) and isinstance(fl.get("steps"), list) else []):
            if isinstance(st_, dict) and (st_.get("method"), st_.get("path")) not in obs:
                return True
    return False


# 3 (PBT-03) validador: si acepta, invariantes; si cita un endpoint no observado, rechaza siempre
@given(s=gen.scenario())
def test_pbt_validator_accept_implies_invariants(s):
    try:
        out = parse_and_validate(s.text, s.surface, s.workflow, s.limits)
    except PlanRejected as e:
        out = None
        assert isinstance(e.reason, str) and e.reason
    raw = json.loads(s.text) if s.kind != "broken_json" else None
    if s.kind in ("unobserved", "wrong_method"):
        assert _cites_unobserved(raw, s.surface)
    if raw is not None and _cites_unobserved(raw, s.surface):
        assert out is None, "acepto un paso con endpoint no observado"
    if s.kind != "valid":
        assert out is None, f"acepto un plan de clase {s.kind}"
    else:
        assert out is not None, "rechazo un plan valido"
    if out is None:
        return
    obs = _observed(s.surface)
    assert out["run_id"] == s.surface["run_id"] and out["workflow"] == s.workflow
    assert len(out["flows"]) <= s.limits.max_flows
    ids = [f["flow_id"] for f in out["flows"]]
    assert len(ids) == len(set(ids))
    for f in out["flows"]:
        assert FLOW_ID_RE.fullmatch(f["flow_id"]) and "\n" not in f["flow_id"]
        assert len(f["steps"]) <= s.limits.max_steps
        for st_ in f["steps"]:
            assert (st_["method"], st_["path"]) in obs


# 3b (PBT-03) cada clase de respuesta se ejercita SIEMPRE (no depende de que el azar la sortee): la clase `valid`
# se acepta y toda clase rota se rechaza; una mutacion que quite un chequeo muere con cualquier seed.
@pytest.mark.parametrize("kind", gen.KINDS)
@settings(max_examples=60)
@given(data=st.data())
def test_pbt_validator_each_class(kind, data):
    s = data.draw(gen.scenario(kind=kind))
    assert s.kind == kind
    if kind == "valid":
        out = parse_and_validate(s.text, s.surface, s.workflow, s.limits)  # no debe lanzar
        assert out == json.loads(s.text)
    else:
        with pytest.raises(PlanRejected):
            parse_and_validate(s.text, s.surface, s.workflow, s.limits)


# 4 (PBT-03) prompt: prefijo fijo, un solo bloque de datos, funcion pura
@given(surf=gen.surface(valid=False), wf=gen.workflow())
def test_pbt_prompt_prefix_single_block_pure(surf, wf):
    p = build_prompt(surf, wf)
    assert p.startswith(INSTRUCTIONS + OPEN_TAG + "\n")
    rest = p[len(INSTRUCTIONS):]
    assert rest.count(OPEN_TAG) == 1 and rest.count(CLOSE_TAG) == 1
    assert rest.endswith("\n" + CLOSE_TAG + "\n")
    block = rest[len(OPEN_TAG) + 1: -len(CLOSE_TAG) - 2]
    assert "<" not in block and "\n" not in block
    assert json.loads(block) == {"surface": surf, "workflow": wf}  # el dato viaja sin perdida
    assert prompt_hash(build_prompt(copy.deepcopy(surf), wf)) == prompt_hash(p)
    shuffled = dict(reversed(list(surf.items())))
    assert build_prompt(shuffled, wf) == p  # no depende del orden de claves


# 5 (PBT-03) tope de entrada: si lo supera el modelo no recibe ninguna llamada
@given(surf=gen.surface(nonempty=True), wf=gen.workflow(), data=st.data())
def test_pbt_input_budget_blocks_llm_call(surf, wf, data):
    est = est_tokens(build_prompt(surf, wf))
    cap = data.draw(st.one_of(st.integers(max(est - 3, 1), est + 3), st.integers(1, 9000)))
    lim = Limits(max_input_tokens=cap)
    llm = FakeLLM()
    if math.ceil(len(build_prompt(surf, wf)) / 4) > cap:
        with pytest.raises(BudgetExceeded) as ei:
            run_plan(surf, wf, llm, lim)
        assert ei.value.kind == "input" and llm.calls == 0
    else:
        with pytest.raises(UnscriptedPrompt):  # el fake sin guion falla cerrado, pero la llamada ocurre
            run_plan(surf, wf, llm, lim)
        assert llm.calls == 1


# cobertura de generadores: cada clase relevante aparece en 500 sorteos
@settings(max_examples=500, database=None)
@given(data=st.data())
def _draw_classes(data, seen):
    surf = data.draw(gen.surface())
    seen.add("superficie_vacia" if not surf["endpoints"] else "superficie_con_endpoints")
    eps = [(e["method"], e["path"]) for e in surf["endpoints"]]
    if len(eps) != len(set(eps)):
        seen.add("endpoints_duplicados")
    if any("</datos-superficie>" in e["path"] for e in surf["endpoints"]):
        seen.add("path_delimitador")
    if any(e["path"] == "/.." for e in surf["endpoints"]):
        seen.add("path_dotdot")
    if any(not e["path"].isascii() for e in surf["endpoints"]):
        seen.add("path_unicode")
    if any(len(e["path"]) > gen.LONG_PATH_LEN for e in surf["endpoints"]):
        seen.add("path_largo")
    if any(not e["path"].startswith("/") for e in data.draw(gen.surface(valid=False))["endpoints"]):
        seen.add("path_invalido")
    n = len(data.draw(gen.flow_plan())["flows"])
    seen.add("plan_max_flows" if n == gen.MAX_FLOWS else ("plan_1_flujo" if n == 1 else "plan_intermedio"))
    fid = data.draw(gen.flow_id(valid=False))
    seen.add("flow_id_invalido" if not FLOW_ID_RE.fullmatch(fid) else "flow_id_invalido_aceptado")
    seen.add("flow_id_valido" if FLOW_ID_RE.fullmatch(data.draw(gen.flow_id())) else "flow_id_valido_rechazado")
    sc = data.draw(gen.scenario())
    seen.add("resp_" + sc.kind)
    if sc.kind != "broken_json" and _cites_unobserved(json.loads(sc.text), sc.surface):
        seen.add("resp_endpoint_no_observado")
    try:
        parse_and_validate(sc.text, sc.surface, sc.workflow, sc.limits)
        seen.add("resp_aceptada")
    except PlanRejected:
        seen.add("resp_rechazada")


def test_pbt_generator_coverage():
    seen = set()
    _draw_classes(seen=seen)
    esperadas = {
        "superficie_vacia", "superficie_con_endpoints", "endpoints_duplicados", "path_delimitador", "path_dotdot",
        "path_unicode", "path_largo", "path_invalido", "plan_max_flows", "plan_1_flujo", "flow_id_invalido",
        "flow_id_valido", "resp_endpoint_no_observado", "resp_aceptada", "resp_rechazada",
        *("resp_" + k for k in gen.KINDS),
    }
    assert esperadas <= seen, sorted(esperadas - seen)
    assert not {"flow_id_invalido_aceptado", "flow_id_valido_rechazado"} & seen
