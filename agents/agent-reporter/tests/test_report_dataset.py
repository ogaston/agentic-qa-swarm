import json

import pytest

from agent_reporter.errors import NoEvidence
from agent_reporter.fakes.fake_llm import FakeLLM
from agent_reporter.reporter import Limits, correlate_postmortem, prepare
from agent_reporter.precision import THRESHOLD, _StripReader, _canned, main, precision
from support import ROOT, report_validator

DS = ROOT / "agents/dataset"
IDS = [a["id"] for a in json.loads((DS / "manifest.json").read_text())["artifacts"]]


@pytest.mark.parametrize("aid", IDS)
def test_dataset_report_matches_expected_verdict(aid):
    art = DS / "artifacts" / aid
    exp = json.loads((art / "expected.json").read_text())["reporter"]
    if exp["verdict"] == "error":
        with pytest.raises(NoEvidence):
            correlate_postmortem("run-" + aid, [], _StripReader(art, "/"), FakeLLM())
        return
    ev = json.loads((art / "evidence-uris.json").read_text())
    reader = _StripReader(art / "evidence", f"/runs/{ev['run_id']}/")
    llm = FakeLLM()
    llm.register(prepare(ev["run_id"], ev["uris"], reader, Limits()).prompt, _canned(art, ev["uris"]))
    rep = correlate_postmortem(ev["run_id"], ev["uris"], reader, llm)
    assert not list(report_validator().iter_errors(rep))
    assert rep["verdict"] == exp["verdict"]


def test_dataset_precision_meets_threshold():
    ok, total = precision()
    assert total == 7 and ok / total >= THRESHOLD


def test_dataset_precision_drops_when_a_verdict_is_inverted(tmp_path):
    import shutil
    shutil.copytree(DS, tmp_path / "ds")
    p = tmp_path / "ds/artifacts/bug-ecom-01/reporter.response.json"
    d = json.loads(p.read_text()); d["verdict"] = "sin-hallazgos"; d["findings"] = []; d["root_cause"] = None
    p.write_text(json.dumps(d))
    ok, total = precision(tmp_path / "ds")
    base, _ = precision()
    assert ok < base and total == 7


def test_dataset_precision_cli_exit_code(tmp_path, capsys):
    import shutil
    assert main([]) == 0 and '"evaluated": 7' in capsys.readouterr().out
    shutil.copytree(DS, tmp_path / "ds")
    for a in ("bug-ecom-01", "bug-logis-01", "bug-fintech-01"):
        p = tmp_path / "ds/artifacts" / a / "reporter.response.json"
        d = json.loads(p.read_text()); d.update(verdict="inconcluso", findings=[]); p.write_text(json.dumps(d))
    assert main(["--dataset", str(tmp_path / "ds")]) == 1
