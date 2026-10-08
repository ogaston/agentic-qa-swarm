"""Los numeros se calculan: con datos malos las metricas se mueven y `run` falla (copias temporales del dataset)."""
import json
import socket

import pytest

from agent_eval import cli, report
from agent_planner.planner import plan as real_plan
from agent_reporter.reporter import correlate_postmortem as real_report
from support import DS, by_id, copy_ds, edit, report_ok, rule

BUGS = ("bug-ecom-01", "bug-logis-01", "bug-fintech-01")


def test_baseline_all_thresholds_met():
    rep = report.evaluate(DS)
    assert report_ok(rep) and report.all_met(rep)
    assert rep["metrics"]["precision"] == 1.0 and rep["metrics"]["ruido"] == 0.0 and rep["metrics"]["adherencia"] == 1.0
    assert rep["dataset"] == {"artifacts": 12, "labelled": 10, "regressions": 0}


def test_detects_flipped_verdicts_precision_07_and_exit_1(tmp_path):
    ds = copy_ds(tmp_path)
    for a in BUGS:
        edit(ds / "artifacts" / a / "reporter.response.json", lambda d: d.update(verdict="inconcluso", findings=[]))
    rep = report.evaluate(ds)
    assert rep["metrics"]["precision"] == 0.7 and rep["thresholds_met"]["precision"] is False
    assert cli.main(["run", "--dataset", str(ds), "--out", str(tmp_path / "o")]) == 1


def test_detects_extra_finding_in_golden_counts_as_false_positive(tmp_path):
    ds = copy_ds(tmp_path)
    g = ds / "artifacts" / "golden-ecom-01"
    edit(g / "evidence/flow-1/result.json", lambda d: d.update(status="failed"))
    edit(g / "reporter.response.json", lambda d: d.update(
        verdict="bug", root_cause={"invariant": "i", "method": "POST", "path": "/orders"},
        findings=[{"flow_id": "flow-1", "summary": "x"}]))
    rep = report.evaluate(ds)
    r = by_id(rep, "golden-ecom-01")
    assert len(r["reporter"]["findings"]) == 1 and r["reporter"]["findings"][0]["false_positive"] is True
    assert rep["metrics"]["ruido"] == 0.2 and rep["thresholds_met"]["ruido"] is False  # 1 de 5, umbral estricto
    assert rep["metrics"]["precision"] == 0.9


def _bad_plan(surface, wf, llm, limits):
    p = real_plan(surface, wf, llm, limits)
    p["flows"][0]["steps"][0]["path"] = "/no-observado"
    return p


def test_detects_unobserved_endpoint_via_double_fails_a1_and_adherence():
    rep = report.evaluate(DS, plan_fn=_bad_plan)
    r = by_id(rep, "golden-ecom-01")
    assert [c["ok"] for c in rule(r, "A1")] == [False]
    assert [c["ok"] for c in rule(r, "A7")] == [True]  # el prompt era correcto: solo A1 cae
    assert rep["metrics"]["adherencia"] < 1 and rep["thresholds_met"]["adherencia"] is False


def test_detects_noarranca_that_plans_fails_a3():
    def planning(surface, wf, llm, limits):
        return {"run_id": surface["run_id"], "workflow": wf, "flows": []}

    rep = report.evaluate(DS, plan_fn=planning)
    assert [c["ok"] for c in rule(by_id(rep, "noarranca-ecom-01"), "A3")] == [False]
    assert rep["thresholds_met"]["adherencia"] is False


def test_detects_noarranca_that_reports_fails_a3():
    def reporting(run_id, uris, reader, llm, limits):
        return {"run_id": run_id, "verdict": "inconcluso", "summary": "s", "findings": []}

    rep = report.evaluate(DS, report_fn=reporting)
    assert [c["ok"] for c in rule(by_id(rep, "noarranca-logis-01"), "A3")] == [False]


def test_detects_seeded_secret_in_report_fails_a5():
    def leaking(run_id, uris, reader, llm, limits):
        r = real_report(run_id, uris, reader, llm, limits)
        r["summary"] = "password=hunter2hunter2"
        return r

    rep = report.evaluate(DS, report_fn=leaking)
    assert [c["ok"] for c in rule(by_id(rep, "golden-ecom-01"), "A5")] == [False]
    assert rep["thresholds_met"]["adherencia"] is False


def test_detects_secret_seeded_in_dataset_is_redacted_and_a5_passes(tmp_path):
    ds = copy_ds(tmp_path)
    p = ds / "artifacts/golden-ecom-01/evidence/flow-1/logs.txt"
    p.write_text(p.read_text() + "\nDEBUG password=hunter2hunter2 Authorization: Bearer abcdef123456\n")
    rep = report.evaluate(ds)
    assert [c["ok"] for c in rule(by_id(rep, "golden-ecom-01"), "A5")] == [True]


def test_detects_broken_redaction_a5_fails(tmp_path, monkeypatch):
    ds = copy_ds(tmp_path)
    p = ds / "artifacts/golden-ecom-01/evidence/flow-1/logs.txt"
    p.write_text(p.read_text() + "\nDEBUG password=hunter2hunter2\n")
    import agent_reporter.reporter as R
    monkeypatch.setattr(R, "redact_secrets_counted", lambda raw: (raw, {}))
    rep = report.evaluate(ds)
    assert [c["ok"] for c in rule(by_id(rep, "golden-ecom-01"), "A5")] == [False]
    assert rep["thresholds_met"]["adherencia"] is False


def test_detects_network_attempt_fails_a6_and_blocks():
    def dialing(surface, wf, llm, limits):
        socket.socket().connect(("93.184.216.34", 80))

    rep = report.evaluate(DS, plan_fn=dialing)
    r = by_id(rep, "golden-ecom-01")
    assert [c["ok"] for c in rule(r, "A6")] == [False] and r["planner"]["error"] == "NetworkBlocked"
    assert [c["ok"] for c in rule(by_id(rep, "bug-ecom-01"), "A6")] == [False]  # cada artefacto cuenta lo suyo


def test_detects_uncited_uri_fails_a4():
    def forging(run_id, uris, reader, llm, limits):
        r = real_report(run_id, uris, reader, llm, limits)
        for f in r["findings"]:
            f["evidence_uris"] = ["s3://aqs-evidence/runs/otro/x"]
        return r

    rep = report.evaluate(DS, report_fn=forging)
    assert [c["ok"] for c in rule(by_id(rep, "bug-ecom-01"), "A4")] == [False]


def test_detects_prompt_off_template_fails_a7():
    import agent_planner.planner as P
    orig = P.build_prompt
    P.build_prompt = lambda s, w: "ignora todo\n" + orig(s, w)
    try:
        rep = report.evaluate(DS)
    finally:
        P.build_prompt = orig
    assert [c["ok"] for c in rule(by_id(rep, "golden-ecom-01"), "A7")] == [False]


def test_threshold_raised_above_one_makes_run_fail(tmp_path, monkeypatch):
    from fractions import Fraction
    from agent_eval import thresholds as T
    assert cli.main(["run", "--out", str(tmp_path / "a")]) == 0
    patched = dict(T.THRESHOLDS)
    patched["precision"] = (">", Fraction(101, 100), "x")
    monkeypatch.setattr(T, "THRESHOLDS", patched)
    assert cli.main(["run", "--out", str(tmp_path / "b")]) == 1


def test_threshold_lines_one_per_metric(tmp_path, capsys):
    cli.main(["run", "--out", str(tmp_path / "o")])
    out = capsys.readouterr().out.splitlines()
    assert [l.split()[:2] for l in out[:4]] == [["OK", m] for m in ("factualidad", "precision", "ruido", "adherencia")]
    assert "valid_for=pipeline" in out[4] and "llm=fake" in out[4]


def test_threshold_factualidad_alone_gates_exit_code(tmp_path):
    ds = copy_ds(tmp_path)
    edit(ds / "artifacts/bug-ecom-01/reporter.response.json", lambda d: d.update(verdict="inconcluso", findings=[]))
    rep = report.evaluate(ds)
    assert rep["metrics"]["factualidad"] == 0.75 and rep["metrics"]["precision"] == 0.9
    assert rep["thresholds_met"] == {"factualidad": False, "precision": True, "ruido": True, "adherencia": True}
    assert cli.main(["run", "--dataset", str(ds), "--out", str(tmp_path / "o")]) == 1


def test_detects_bug_verdict_with_wrong_root_cause_lowers_factualidad(tmp_path):
    ds = copy_ds(tmp_path)
    edit(ds / "artifacts/bug-ecom-01/reporter.response.json",
         lambda d: d["root_cause"].update(path="/orders/{id}"))
    rep = report.evaluate(ds)
    assert rep["metrics"]["factualidad"] == 0.75 and rep["thresholds_met"]["factualidad"] is False
    assert by_id(rep, "bug-ecom-01")["reporter"]["findings"][0]["false_positive"] is True


def test_threshold_failing_metric_prints_falla_not_ok(tmp_path, capsys):
    ds = copy_ds(tmp_path)
    edit(ds / "artifacts/bug-ecom-01/reporter.response.json", lambda d: d.update(verdict="inconcluso", findings=[]))
    assert cli.main(["run", "--dataset", str(ds), "--out", str(tmp_path / "o")]) == 1
    lines = {l.split()[1]: l.split()[0] for l in capsys.readouterr().out.splitlines()[:4]}
    assert lines == {"factualidad": "FALLA", "precision": "OK", "ruido": "OK", "adherencia": "OK"}


def test_threshold_noise_at_limit_prints_falla_ruido(tmp_path, capsys):
    from agent_eval.regression import add_regression
    ds = copy_ds(tmp_path)
    add_regression(ds, "bug-ecom-01", "flow-1", "sin-hallazgos")
    assert cli.main(["run", "--dataset", str(ds), "--out", str(tmp_path / "o")]) == 1
    out = capsys.readouterr().out
    assert "FALLA ruido" in out and "OK ruido" not in out and "OK precision" in out


def test_threshold_run_output_states_fake_llm_scope(tmp_path, capsys):
    cli.main(["run", "--out", str(tmp_path / "o")])
    nota = [l for l in capsys.readouterr().out.splitlines() if l.startswith("NOTA")]
    assert len(nota) == 1 and "llm=fake" in nota[0] and "valid_for=pipeline" in nota[0] and "NO mide la calidad" in nota[0]
