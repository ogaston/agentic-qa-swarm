import http.client
import json
import logging
import threading

import pytest

from agent_planner.errors import Limits
from agent_planner.fakes.fake_llm import FakeLLM
from agent_planner.llm import LLMUnavailable
from agent_planner.prompt import build_prompt
from agent_planner.server import make_server
from jsonschema import Draft202012Validator
from helpers import ROOT, SURFACE, good_plan, load

FLOW = Draft202012Validator(load(ROOT / "contracts" / "plans" / "flow-plan.schema.json"))


class Srv:
    def __init__(self, llm, limits=Limits()):
        self.llm = llm
        self.httpd = make_server("127.0.0.1", 0, llm, limits)
        self.port = self.httpd.server_address[1]
        self.t = threading.Thread(target=self.httpd.serve_forever, daemon=True)
        self.t.start()

    def req(self, method, path, body=None, headers=None, raw=None):
        c = http.client.HTTPConnection("127.0.0.1", self.port, timeout=5)
        data = raw if raw is not None else (json.dumps(body).encode() if body is not None else None)
        h = {"Content-Type": "application/json"} if method == "POST" else {}
        h.update(headers or {})
        c.request(method, path, body=data, headers=h)
        r = c.getresponse()
        out = (r.status, r.read())
        c.close()
        return out

    def close(self):
        self.httpd.shutdown(); self.httpd.server_close()


@pytest.fixture
def make():
    made = []

    def f(llm=None, **kw):
        s = Srv(llm or FakeLLM(), **kw); made.append(s); return s
    yield f
    for s in made:
        s.close()


def good_llm():
    f = FakeLLM()
    f.register(build_prompt(SURFACE, "wf-1"), json.dumps(good_plan()), input_tokens=11, output_tokens=7)
    return f


REQ = {"surface": SURFACE, "workflow": "wf-1"}


def test_server_200_valid_plan_no_extra_keys(make):
    st, body = make(good_llm()).req("POST", "/v1/plan", REQ)
    plan = json.loads(body)
    assert st == 200 and not list(FLOW.iter_errors(plan)) and plan == good_plan()


def test_server_422_invalid_empty_surface_and_rejected_plan(make):
    s = make(good_llm())
    st, b = s.req("POST", "/v1/plan", {"surface": {"run_id": "r"}, "workflow": "wf-1"})
    assert st == 422 and json.loads(b)["error"] == "no_surface"
    st, b = s.req("POST", "/v1/plan", {"surface": dict(SURFACE, endpoints=[]), "workflow": "wf-1"})
    assert st == 422 and json.loads(b)["error"] == "no_surface"
    bad = FakeLLM(); bad.register(build_prompt(SURFACE, "wf-1"), "{}")
    st, b = make(bad).req("POST", "/v1/plan", REQ)
    assert st == 422 and json.loads(b)["error"] == "plan_rejected"
    st, b = s.req("POST", "/v1/plan", {"surface": SURFACE})
    assert st == 422 and json.loads(b)["error"] == "invalid_request"
    st, b = s.req("POST", "/v1/plan", raw=b"{no json")
    assert st == 422 and json.loads(b)["error"] == "invalid_request"
    st, b = s.req("POST", "/v1/plan", {"surface": SURFACE, "workflow": ""})
    assert st == 422 and json.loads(b)["error"] == "invalid_request"


def test_server_504_budget(make):
    st, b = make(good_llm(), limits=Limits(max_input_tokens=10)).req("POST", "/v1/plan", REQ)
    assert st == 504 and json.loads(b)["error"] == "budget_exceeded"
    slow = FakeLLM(); slow.register(build_prompt(SURFACE, "wf-1"), "x", latency_s=99.0)
    st, b = make(slow).req("POST", "/v1/plan", REQ)
    assert st == 504


def test_server_502_provider_down(make):
    f = FakeLLM(); f.register(build_prompt(SURFACE, "wf-1"), error=LLMUnavailable("x"))
    st, b = make(f).req("POST", "/v1/plan", REQ)
    assert st == 502 and json.loads(b)["error"] == "llm_unavailable"
    st, b = make(FakeLLM()).req("POST", "/v1/plan", REQ)  # prompt sin guion: el fake falla cerrado
    assert st == 502


def test_server_413_size_and_415_type(make):
    s = make(good_llm())
    c = http.client.HTTPConnection("127.0.0.1", s.port, timeout=5)
    c.putrequest("POST", "/v1/plan")
    c.putheader("Content-Type", "application/json")
    c.putheader("Content-Length", str(256 * 1024 + 1))
    c.endheaders()
    assert c.getresponse().status == 413
    c.close()
    assert s.req("POST", "/v1/plan", raw=b"{}", headers={"Content-Type": "text/plain"})[0] == 415
    assert s.req("POST", "/v1/plan", raw=b"{}", headers={"Content-Type": "application/json; charset=utf-8"})[0] == 422


def test_server_health_ready_metrics_and_routes(make):
    s = make(good_llm())
    assert s.req("GET", "/healthz")[0] == 200 and s.req("GET", "/readyz")[0] == 200
    assert s.req("GET", "/nada")[0] == 404 and s.req("POST", "/otro", {})[0] == 404
    s.req("POST", "/v1/plan", REQ)
    s.req("POST", "/v1/plan", {"surface": SURFACE})
    st, b = s.req("GET", "/metrics")
    m = b.decode()
    assert st == 200
    assert 'aqs_planner_requests_total{result="ok"} 1' in m
    assert 'aqs_planner_requests_total{result="invalid_request"} 1' in m
    assert 'aqs_planner_tokens_total{direction="input"} 11' in m
    assert 'aqs_planner_tokens_total{direction="output"} 7' in m


def test_server_no_leak_marker_in_response_logs_and_metrics(make, caplog):
    marker = "MARCADOR-UNICO-7f3a9c"
    caplog.set_level(logging.DEBUG)
    surf = dict(SURFACE, run_id="run-nl",
                endpoints=SURFACE["endpoints"] + [{"method": "GET", "path": "/" + marker}])
    cases = []
    s = make(FakeLLM())  # 502
    cases.append(s.req("POST", "/v1/plan", {"surface": surf, "workflow": "wf-1"}))
    hij = good_plan(surf); hij["flows"][0]["steps"][0]["path"] = "/" + marker + "x"
    hij["flows"][0]["invariant"] = marker
    f = FakeLLM(); f.register(build_prompt(surf, "wf-1"), json.dumps(hij))
    s2 = make(f)
    cases.append(s2.req("POST", "/v1/plan", {"surface": surf, "workflow": "wf-1"}))   # 422 rechazado
    cases.append(s2.req("POST", "/v1/plan", {"surface": dict(surf, endpoints=[]), "workflow": "wf-1"}))
    cases.append(s2.req("POST", "/v1/plan", {"surface": dict(surf, extra=marker), "workflow": "wf-1"}))
    cases.append(s2.req("POST", "/v1/plan", {"surface": surf, "workflow": marker * 20}))
    s3 = make(f, limits=Limits(max_input_tokens=5))
    cases.append(s3.req("POST", "/v1/plan", {"surface": surf, "workflow": "wf-1"}))
    assert [c[0] for c in cases] == [502, 422, 422, 422, 422, 504]
    for st, body in cases:
        assert marker not in body.decode()
    for srv in (s, s2, s3):
        assert marker not in srv.req("GET", "/metrics")[1].decode()
    assert caplog.records and marker not in caplog.text
    assert '"run_id": "run-nl"' in caplog.text
    # un run_id con forma no segura (espacios) no se vuelca al log
    caplog.clear()
    odd = dict(surf, run_id="run con espacios " + marker)
    s.req("POST", "/v1/plan", {"surface": odd, "workflow": "wf-1"})
    assert caplog.records and marker not in caplog.text and "desconocido" in caplog.text
