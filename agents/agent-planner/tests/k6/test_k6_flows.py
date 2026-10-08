import copy
import http.server
import json
import os
import subprocess
import threading
from pathlib import Path

import pytest

from agent_planner.k6 import (DirFlowStore, FakeFlowStore, FlowValidationError, K6InspectError,
                              extract_flow, inspect_with_k6, publish_flows, render_flow,
                              validate_plan, validate_script)
from agent_planner.k6 import render as render_mod
from agent_planner.k6.examples import ROOT, example_plans

TRES = json.loads((ROOT / "contracts/plans/examples/valid/flow-plan.tres-flujos.json").read_text())
GOLDEN = Path(__file__).parent / "golden" / "tres-flujos__f1.k6.js"
EVIL = ['a"b', "a\\b", "x*/y", "</script>", "l1\nl2", "u u ", "'); fail('x", "\r\n"]


def plan_with(**flow_over):
    p = copy.deepcopy(TRES)
    p["flows"] = p["flows"][:1]
    p["flows"][0].update(flow_over)
    return p


def ok_k6(path):
    return None


def test_render_golden():
    assert render_flow(TRES, TRES["flows"][0]) == GOLDEN.read_text()


def test_render_deterministic():
    f = TRES["flows"][1]
    assert render_flow(TRES, f).encode() == render_flow(copy.deepcopy(TRES), copy.deepcopy(f)).encode()


def test_extract_roundtrip_examples():
    n = 0
    for _, plan in example_plans():
        for f in plan["flows"]:
            s = render_flow(plan, f)
            assert extract_flow(s) == f
            validate_script(s, plan, f)
            n += 1
    assert n >= 12


def test_step_order_preserved():
    f = TRES["flows"][0]
    assert [s["method"] for s in extract_flow(render_flow(TRES, f))["steps"]] == ["POST", "GET"]


@pytest.mark.parametrize("evil", EVIL)
@pytest.mark.parametrize("field", ["name", "invariant", "path"])
def test_k6_injection_roundtrip(field, evil):
    if field == "path":
        plan = plan_with(steps=[{"method": "GET", "path": "/" + evil, "expect_status": 200}])
    else:
        plan = plan_with(**{field: evil})
    flow = plan["flows"][0]
    s = render_flow(plan, flow)
    assert extract_flow(s) == flow
    validate_script(s, plan, flow)
    assert "\n" not in s.split("const FLOW = ")[1].split(";\n")[0]


def test_k6_injection_outside_literal_rejected():
    flow = TRES["flows"][0]
    s = render_flow(TRES, flow)
    for bad in (s + "\neval('1');\n", s.replace("export default", "fail('x');\nexport default"),
                s.replace("vus: 1", "vus: 2")):
        with pytest.raises(FlowValidationError) as e:
            validate_script(bad, TRES, flow)
        assert e.value.capa == "script"


def test_k6_forbidden_patterns_rejected():
    flow = TRES["flows"][0]
    s = render_flow(TRES, flow)
    for extra in ("eval(x)", "new Function('a')", "require('fs')", "open('f')", "__ENV.SECRET", "__ENV['A']"):
        with pytest.raises(FlowValidationError):
            validate_script(s + "\n" + extra + "\n", TRES, flow)


def test_k6_forbidden_flow_mismatch_rejected():
    flow = TRES["flows"][0]
    other = copy.deepcopy(flow)
    other["steps"][0]["expect_status"] = 500
    with pytest.raises(FlowValidationError):
        validate_script(render_flow(TRES, flow), TRES, other)


def test_k6_forbidden_template_byte_change(monkeypatch):
    t = render_mod.load_template()
    monkeypatch.setattr(render_mod, "load_template", lambda: t.replace("vus: 1", "vus: 9"))
    with pytest.raises(FlowValidationError) as e:
        validate_script(render_flow(TRES, TRES["flows"][0]), TRES, TRES["flows"][0])
    assert "hash" in e.value.detalle


def test_template_no_dangerous_calls():
    t = (Path(render_mod.__file__).with_name("template.js")).read_text()
    import re
    assert not re.search(r"\b(eval|Function|require|open)\(", t)
    assert len(re.findall(r"import .* from", t)) == 2


# ---------- publish ----------

def test_publish_invalid_plan_leaves_store_empty():
    store = FakeFlowStore()
    bad = copy.deepcopy(TRES)
    bad["flows"][0]["steps"][0]["path"] = "sin-barra"
    with pytest.raises(FlowValidationError) as e:
        publish_flows(bad, store, k6=ok_k6)
    assert e.value.capa == "plan" and store.objects == {}


def test_publish_bad_flow_id_rejected():
    store = FakeFlowStore()
    bad = copy.deepcopy(TRES)
    bad["flows"][0]["flow_id"] = "../x"
    with pytest.raises(FlowValidationError):
        publish_flows(bad, store, k6=ok_k6)
    assert store.objects == {}


def test_publish_second_flow_rejected_by_k6_publishes_nothing():
    store = FakeFlowStore()
    calls = []

    def inspector(path):
        calls.append(path.name)
        if path.name == "f2.k6.js":
            raise K6InspectError("rc=107")

    with pytest.raises(FlowValidationError) as e:
        publish_flows(TRES, store, k6=inspector)
    assert (e.value.flow_id, e.value.capa) == ("f2", "k6")
    assert store.objects == {}


def test_publish_write_failure_midway_cleans_partials():
    store = FakeFlowStore(fail_put_on=2)
    with pytest.raises(OSError):
        publish_flows(TRES, store, k6=ok_k6)
    assert store.objects == {}


def test_publish_readback_hash_mismatch():
    store = FakeFlowStore(corrupt_get=True)
    with pytest.raises(OSError):
        publish_flows(TRES, store, k6=ok_k6)
    assert store.objects == {}


def test_publish_dir_exact_files_and_idempotent(tmp_path):
    store = DirFlowStore(tmp_path)
    r1 = publish_flows(TRES, store, k6=ok_k6)
    files = sorted(p.name for p in (tmp_path / "flows" / "run-001").iterdir())
    assert files == ["f1.k6.js", "f2.k6.js", "f3.k6.js"]
    before = {p.name: p.read_bytes() for p in (tmp_path / "flows" / "run-001").iterdir()}
    r2 = publish_flows(TRES, store, k6=ok_k6)
    after = {p.name: p.read_bytes() for p in (tmp_path / "flows" / "run-001").iterdir()}
    assert r1 == r2 and before == after


HOSTILE_KEYS = ["flows/../x/a.k6.js", "flows/Run/a.k6.js", "flows/a/b.k6.js/../../x",
                "flows/a/b.k6.js\n", "flows/a\n/b.k6.js", "flows/a/b.k6.js/", "/flows/a/b.k6.js",
                "x/flows/a/b.k6.js"]


@pytest.mark.parametrize("key", HOSTILE_KEYS)
def test_publish_dir_store_hostile_keys_all_ops(tmp_path, key):
    s = DirFlowStore(tmp_path)
    for op in (lambda: s.put(key, b"x"), lambda: s.get(key), lambda: s.delete(key)):
        with pytest.raises(ValueError):
            op()
    assert list(tmp_path.rglob("*")) == []


@pytest.mark.parametrize("run,flow", [("run\n", "f1"), ("run", "f1\n"), ("Run", "f1"), ("run", "../f"),
                                      ("", "f1"), ("run", "a/b")])
def test_flow_key_rejects_hostile_ids(run, flow):
    from agent_planner.k6.store import flow_key
    with pytest.raises(ValueError):
        flow_key(run, flow)


def test_publish_rollback_failure_is_noted():
    class Dead(FakeFlowStore):
        def put(self, key, data):
            if self.puts >= 1:
                self.puts += 1
                raise OSError("muerto")
            super().put(key, data)

        def delete(self, key):
            raise OSError("delete muerto")

    with pytest.raises(OSError) as e:
        publish_flows(TRES, Dead(), k6=ok_k6)
    assert any("rollback fallo" in n for n in e.value.__notes__)


def test_publish_dir_store_rejects_traversal(tmp_path):
    with pytest.raises(ValueError):
        DirFlowStore(tmp_path).put("flows/../x/a.k6.js", b"x")
    with pytest.raises(ValueError):
        DirFlowStore(tmp_path).put("flows/Run/a.k6.js", b"x")


@pytest.mark.parametrize("bad", ["f1\n", "f1\r\n"])
@pytest.mark.parametrize("store_cls", [FakeFlowStore, "dir"])
def test_publish_newline_flow_id_rejected(bad, store_cls, tmp_path):
    store = DirFlowStore(tmp_path) if store_cls == "dir" else store_cls()
    plan = copy.deepcopy(TRES)
    plan["flows"][0]["flow_id"] = bad
    with pytest.raises(FlowValidationError) as e:
        publish_flows(plan, store, k6=ok_k6)
    assert e.value.capa == "plan"
    assert not (isinstance(store, FakeFlowStore) and store.objects)


@pytest.mark.parametrize("store_cls", [FakeFlowStore, "dir"])
def test_publish_newline_run_id_rejected(store_cls, tmp_path):
    store = DirFlowStore(tmp_path) if store_cls == "dir" else store_cls()
    plan = copy.deepcopy(TRES)
    plan["run_id"] = "run\n"
    with pytest.raises(FlowValidationError) as e:
        publish_flows(plan, store, k6=ok_k6)
    assert e.value.capa == "plan"
    assert not (isinstance(store, FakeFlowStore) and store.objects)


def test_publish_republish_failure_keeps_previous_version():
    store = FakeFlowStore()
    publish_flows(TRES, store, k6=ok_k6)
    before = dict(store.objects)
    newer = copy.deepcopy(TRES)
    for f in newer["flows"]:
        f["invariant"] = "otra invariante"
    store.puts, store.fail_put_on = 0, 2
    with pytest.raises(OSError):
        publish_flows(newer, store, k6=ok_k6)
    assert store.objects == before


def test_k6_forbidden_filter_isolated(monkeypatch):
    from agent_planner.k6 import validate as val
    t = render_mod.load_template() + "\neval(1);\n"
    monkeypatch.setattr(render_mod, "load_template", lambda: t)
    monkeypatch.setattr(val, "load_template", lambda: t)
    monkeypatch.setattr(val, "template_sha256", lambda: val.TEMPLATE_SHA256)
    flow = TRES["flows"][0]
    with pytest.raises(FlowValidationError) as e:
        validate_script(render_flow(TRES, flow), TRES, flow)
    assert "prohibida" in e.value.detalle


# ---------- herramienta real ----------

@pytest.mark.k6
def test_k6_inspect_real_accepts_and_rejects(tmp_path):
    f = tmp_path / "ok.k6.js"
    f.write_text(render_flow(TRES, TRES["flows"][0]))
    inspect_with_k6(f)
    bad = tmp_path / "roto.js"
    bad.write_text(f.read_text().replace("export default function", "export default function("))
    with pytest.raises(K6InspectError) as e:
        inspect_with_k6(bad)
    assert "107" in str(e.value)


@pytest.mark.k6
def test_k6_publish_real_inspector(tmp_path):
    publish_flows(TRES, DirFlowStore(tmp_path))
    assert len(list((tmp_path / "flows/run-001").iterdir())) == 3


class _H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200); self.end_headers()

    def do_POST(self):
        self.send_response(201); self.end_headers()

    def log_message(self, *a):
        pass


def _k6_run(script_dir, name, port):
    env_args = ["-e", f"BASE_URL=http://127.0.0.1:{port}"] if port else []
    k6 = os.environ.get("K6_BIN")
    if k6:
        cmd = [k6, "run", "-q", *env_args, str(script_dir / name)]
    else:
        cmd = ["podman", "run", "--rm", "--user", "0:0", "--network", "host", "-v", f"{script_dir}:/w:z", "-w", "/w",
               "docker.io/grafana/k6:0.55.0", "run", "-q", *env_args, f"/w/{name}"]
    return subprocess.run(cmd, capture_output=True, text=True, timeout=180).returncode


@pytest.fixture
def server():
    srv = http.server.ThreadingHTTPServer(("127.0.0.1", 0), _H)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    yield srv.server_address[1]
    srv.shutdown()


@pytest.mark.k6
def test_k6_run_passes_when_status_matches(tmp_path, server):
    (tmp_path / "ok.k6.js").write_text(render_flow(TRES, TRES["flows"][0]))
    assert _k6_run(tmp_path, "ok.k6.js", server) == 0


@pytest.mark.k6
def test_k6_run_fails_when_status_differs(tmp_path, server):
    plan = plan_with(steps=[{"method": "GET", "path": "/x", "expect_status": 404}])
    (tmp_path / "ko.k6.js").write_text(render_flow(plan, plan["flows"][0]))
    assert _k6_run(tmp_path, "ko.k6.js", server) == 99


@pytest.mark.k6
def test_k6_run_fails_without_base_url(tmp_path):
    (tmp_path / "nb.k6.js").write_text(render_flow(TRES, TRES["flows"][0]))
    assert _k6_run(tmp_path, "nb.k6.js", None) == 99
