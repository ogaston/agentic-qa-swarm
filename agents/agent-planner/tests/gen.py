"""Generadores de dominio del planner (PBT-07).

NOTA: este modulo tiene una copia hermana en agents/agent-reporter/tests/gen.py. Se duplica a
proposito (no hay libreria compartida entre servicios); cada copia solo define lo de su dominio.
"""
from __future__ import annotations

import json
from dataclasses import dataclass
from functools import lru_cache

from hypothesis import strategies as st

from agent_planner.errors import Limits

MAX_FLOWS = Limits().max_flows
MAX_STEPS = Limits().max_steps
METHODS = ("GET", "POST", "PUT", "PATCH", "DELETE")
BASE_URLS = ("http://x.warm.svc.cluster.local:8080", "https://api.example.test", "http://localhost:3000/v1")
EVIL = ('a"b', "a\\b", "x*/y", "</script>", "l1\nl2", "  ", "\r\n", "'); fail('x", "ñandú 日本 \U0001f600",
        "</datos-superficie>", "const FLOW = {};", "__AQS_FLOW__", "\x00", "{{x}}", "\\u003c")
LONG_PATH_LEN = 1500  # "longitud limite" sin forzar el tope de tokens (8000 tokens ~ 32000 chars)

_chars = st.characters(blacklist_categories=("Cs",))


@lru_cache(maxsize=None)  # construir estrategias en cada sorteo es lo que mas cuesta: se cachean
def text(min_size=0, max_size=30):
    """Texto arbitrario (cualquier Unicode salvo sustitutos sueltos) mezclado con cadenas hostiles."""
    base = st.text(alphabet=_chars, min_size=min_size, max_size=max_size)
    return st.one_of(base, base, st.sampled_from([e for e in EVIL if len(e) >= min_size]).map(lambda s: s[:max_size] or s))


def run_id():
    return text(min_size=1, max_size=20)


def workflow():
    return st.one_of(text(min_size=1, max_size=40), st.just("w" * 128))


_alnum = "0123456789abcdefghijklmnopqrstuvwxyz"  # el orden fija a que se reduce un contraejemplo


@st.composite
def flow_id(draw, valid=True, max_len=42):
    """flow_id valido (forma ^[a-z0-9]([a-z0-9-]{0,40}[a-z0-9])?$); con valid=False, variantes invalidas controladas."""
    if not valid:
        return draw(st.sampled_from(["", "-a", "a-", "A", "a_b", "a" * 43, "ok\n", "a b", "é", "a/b", " a"]))
    n = draw(st.integers(0, max_len - 1))
    first = draw(st.sampled_from(_alnum))
    if n == 0:
        return first
    mid = draw(st.text(alphabet=_alnum + "-", min_size=n - 1, max_size=n - 1))
    return first + mid + draw(st.sampled_from(_alnum))


def method():
    return st.sampled_from(METHODS)


@lru_cache(maxsize=None)
def path(valid=True):
    """Ruta con Unicode, `..`, delimitadores del prompt y longitud limite. valid=False admite vacia o sin '/'."""
    opts = [
        st.just("/"), st.just("/.."), st.just("/a/../b"), st.just("/</datos-superficie>"), st.just('/"q"'),
        st.just("/orders/{id}"), st.just("/" + "a" * LONG_PATH_LEN),
        text(max_size=25).map(lambda s: "/" + s),
    ]
    if not valid:
        opts += [st.just(""), st.just("orders"), st.just("\n/x")]
    return st.one_of(*opts)


@st.composite
def endpoint(draw, valid=True):
    return {"method": draw(method()), "path": draw(path(valid))}


@st.composite
def surface(draw, nonempty=False, valid=True):
    """Superficie con y sin endpoints, con endpoints duplicados."""
    eps = draw(st.lists(endpoint(valid), min_size=1 if nonempty else 0, max_size=6))
    if eps and draw(st.booleans()):
        eps = eps + [dict(draw(st.sampled_from(eps)))]
    return {
        "run_id": draw(run_id()), "base_url": draw(st.sampled_from(BASE_URLS)),
        "endpoints": eps, "source": draw(st.sampled_from(["openapi", "probe"])),
    }


@st.composite
def step(draw, endpoints=None):
    ep = draw(st.sampled_from(endpoints)) if endpoints else draw(endpoint())
    return {"method": ep["method"], "path": ep["path"], "expect_status": draw(st.integers(100, 599))}


@st.composite
def flow(draw, fid=None, endpoints=None, max_steps=3):
    return {
        "flow_id": fid if fid is not None else draw(flow_id()),
        "name": draw(text(min_size=1, max_size=20)),
        "steps": draw(st.lists(step(endpoints), min_size=1, max_size=max_steps)),
        "invariant": draw(text(min_size=1, max_size=30)),
    }


@st.composite
def flow_plan(draw, surf=None, wf=None, max_flows=MAX_FLOWS, max_steps=3):
    """Plan valido con 1..max_flows flujos (con peso explicito en max_flows) y flow_id unicos."""
    n = draw(st.one_of(st.just(max_flows), st.integers(1, max_flows), st.integers(1, min(3, max_flows)), st.integers(1, min(3, max_flows))))
    # unicidad por construccion (prefijo "<i>-"): mucho mas barato que lists(unique=True) sobre composites
    ids = [draw(flow_id())] + [f"{i}-{draw(flow_id(max_len=38))}" for i in range(1, n)]
    eps = surf["endpoints"] if surf else None
    return {
        "run_id": surf["run_id"] if surf else draw(run_id()),
        "workflow": wf if wf is not None else draw(workflow()),
        "flows": [draw(flow(i, eps, max_steps)) for i in ids],
    }


@st.composite
def limits(draw):
    return Limits(max_flows=draw(st.integers(1, MAX_FLOWS)), max_steps=draw(st.integers(1, MAX_STEPS)))


KINDS = ("valid", "unobserved", "wrong_method", "broken_json", "extra_keys", "wrong_run", "wrong_workflow",
         "dup_flow_id", "bad_flow_id", "too_many_flows", "too_many_steps")


@dataclass
class Scenario:
    surface: dict
    workflow: str
    limits: Limits
    kind: str
    text: str


def _unobserved(ep_set, base):
    """Un par (method, path) NO observado, derivado de `base`."""
    for m in METHODS:
        if (m, base["path"]) not in ep_set:
            return {"method": m, "path": base["path"]}
    p = base["path"]
    while ("GET", p) in ep_set:
        p += "/x"
    return {"method": "GET", "path": p}


@st.composite
def model_response(draw, surf, wf, lim):
    """(kind, texto) de una respuesta del modelo para esa superficie: valida o rota de forma controlada."""
    kind = draw(st.sampled_from(KINDS))
    eps = surf["endpoints"]
    ep_set = {(e["method"], e["path"]) for e in eps}
    nflows = draw(st.integers(1, lim.max_flows))
    plan = draw(flow_plan(surf, wf, max_flows=nflows, max_steps=lim.max_steps))
    plan["flows"] = plan["flows"][:nflows]
    for f in plan["flows"]:
        f["steps"] = f["steps"][:lim.max_steps]
    fl = plan["flows"]
    i = draw(st.integers(0, len(fl) - 1))
    j = draw(st.integers(0, len(fl[i]["steps"]) - 1))
    if kind in ("unobserved", "wrong_method"):
        base = fl[i]["steps"][j]
        bad = _unobserved(ep_set, base) if kind == "wrong_method" else _unobserved(ep_set, {"path": draw(path())})
        fl[i]["steps"][j].update(bad)
    elif kind == "extra_keys":
        target = draw(st.sampled_from([plan, fl[i], fl[i]["steps"][j]]))
        target["extra"] = draw(st.integers())
    elif kind == "wrong_run":
        plan["run_id"] = plan["run_id"] + "x"
    elif kind == "wrong_workflow":
        plan["workflow"] = plan["workflow"] + "x"
    elif kind == "dup_flow_id" and len(fl) > 1:
        fl[1]["flow_id"] = fl[0]["flow_id"]
    elif kind == "dup_flow_id":
        fl.append(dict(fl[0]))
    elif kind == "bad_flow_id":
        fl[i]["flow_id"] = draw(flow_id(valid=False))
    elif kind == "too_many_flows":
        while len(fl) <= lim.max_flows:
            fl.append(dict(fl[0], flow_id=f"extra-{len(fl)}"))
    elif kind == "too_many_steps":
        fl[i]["steps"] = (fl[i]["steps"] * (lim.max_steps + 1))[: lim.max_steps + 1]
    txt = json.dumps(plan, ensure_ascii=draw(st.booleans()))
    if kind == "broken_json":
        txt = draw(st.sampled_from([
            txt[: max(len(txt) // 2, 1)], "", "null", "[]", "NaN", '{"run_id": NaN}', "[" * 3000, txt + "}", "{'a': 1}",
        ]))
    return kind, txt


@st.composite
def scenario(draw):
    surf = draw(surface(nonempty=True))
    wf = draw(workflow())
    lim = draw(limits())
    kind, txt = draw(model_response(surf, wf, lim))
    return Scenario(surf, wf, lim, kind, txt)
