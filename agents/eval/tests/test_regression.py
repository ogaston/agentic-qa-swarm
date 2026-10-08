import hashlib
import json
from pathlib import Path

import pytest

from agent_eval import cli, report
from agent_eval.dataset import DatasetError
from agent_eval.regression import add_regression
from support import DS, by_id, copy_ds, edit, report_ok


def tree_hash(root: Path):
    return sorted((str(p.relative_to(root)), hashlib.sha256(p.read_bytes()).hexdigest())
                  for p in root.rglob("*") if p.is_file())


def test_add_regression_creates_layout_and_run_counts_it(tmp_path, capsys):
    ds = copy_ds(tmp_path)
    before = tree_hash(DS)
    assert cli.main(["add-regression", "--dataset", str(ds), "--artifact", "bug-ecom-01", "--finding", "flow-1",
                     "--label", "sin-hallazgos"]) == 0
    d = ds / "regressions" / "bug-ecom-01-fp-1"
    for n in ("meta.json", "surface.json", "evidence-uris.json", "expected.json", "reporter.response.json"):
        assert (d / n).is_file(), n
    assert (d / "evidence/flow-1/result.json").is_file()
    meta = json.loads((d / "meta.json").read_text())
    assert meta["kind"] == "regresion-fp" and meta["id"] == "bug-ecom-01-fp-1"
    exp = json.loads((d / "expected.json").read_text())
    assert exp["reporter"] == {"verdict": "sin-hallazgos", "root_cause": None}
    assert (d / "reporter.response.json").read_bytes() == (ds / "artifacts/bug-ecom-01/reporter.response.json").read_bytes()
    rc = cli.main(["run", "--dataset", str(ds), "--out", str(tmp_path / "o")])
    rep = json.loads((tmp_path / "o/report.json").read_text())
    assert report_ok(rep)
    assert (rc, rep["dataset"]["artifacts"], rep["dataset"]["regressions"], rep["metrics"]["ruido"]) == (1, 12, 1, 0.2)
    assert rep["thresholds_met"]["ruido"] is False
    assert by_id(rep, "bug-ecom-01-fp-1")["reporter"]["findings"][0]["false_positive"] is True
    assert rep["dataset"]["labelled"] == 11 and rep["metrics"]["precision"] == 10 / 11
    assert tree_hash(DS) == before  # el dataset original no se toca


def test_regression_without_it_the_same_dataset_is_green(tmp_path):
    ds = copy_ds(tmp_path)
    assert cli.main(["run", "--dataset", str(ds), "--out", str(tmp_path / "o")]) == 0


def test_regression_numbering_increments(tmp_path):
    ds = copy_ds(tmp_path)
    a = add_regression(ds, "bug-ecom-01", "flow-1", "inconcluso")
    b = add_regression(ds, "bug-ecom-01", "flow-2", "sin-hallazgos")
    assert (a.name, b.name) == ("bug-ecom-01-fp-1", "bug-ecom-01-fp-2")
    assert not [p for p in (ds / "regressions").iterdir() if p.name.startswith(".")]


def test_regression_counted_by_evaluate(tmp_path):
    ds = copy_ds(tmp_path)
    add_regression(ds, "bug-ecom-01", "flow-1", "sin-hallazgos")
    assert report.evaluate(ds)["dataset"]["regressions"] == 1


@pytest.mark.parametrize("art,fid,label", [
    ("../bug-ecom-01", "f", "inconcluso"), ("bug-ecom-01\n", "f", "inconcluso"), ("nope-01", "f", "inconcluso"),
    ("bug-ecom-01", "f\n", "inconcluso"), ("bug-ecom-01", "../../x", "inconcluso"), ("bug-ecom-01", "", "inconcluso"),
    ("bug-ecom-01", "f", "bug"), ("bug-ecom-01", "f", "sin-hallazgos\n"), ("noarranca-ecom-01", "f", "inconcluso"),
])
def test_regression_rejects_hostile_or_invalid_inputs(tmp_path, art, fid, label):
    ds = copy_ds(tmp_path)
    with pytest.raises(DatasetError):
        add_regression(ds, art, fid, label)
    assert not (ds / "regressions").exists() or not list((ds / "regressions").iterdir())


def test_regression_rejects_symlink_in_evidence(tmp_path):
    ds = copy_ds(tmp_path)
    secret = tmp_path / "ajeno.txt"
    secret.write_text("no copiar")
    (ds / "artifacts/bug-ecom-01/evidence/flow-1/link.txt").symlink_to(secret)
    with pytest.raises(DatasetError):
        add_regression(ds, "bug-ecom-01", "flow-1", "inconcluso")
    assert not list((ds / "regressions").iterdir())  # sin restos


def test_regression_unknown_finding_warns_but_proceeds(tmp_path, capsys):
    ds = copy_ds(tmp_path)
    assert cli.main(["add-regression", "--dataset", str(ds), "--artifact", "bug-ecom-01", "--finding", "f1",
                     "--label", "sin-hallazgos"]) == 0
    assert "aviso" in capsys.readouterr().err


def test_regression_cli_bad_label_is_argparse_error(tmp_path):
    ds = copy_ds(tmp_path)
    with pytest.raises(SystemExit):
        cli.main(["add-regression", "--dataset", str(ds), "--artifact", "bug-ecom-01", "--finding", "f", "--label", "bug"])


def test_regression_corrupt_meta_is_rejected_on_run(tmp_path):
    ds = copy_ds(tmp_path)
    d = add_regression(ds, "bug-ecom-01", "flow-1", "inconcluso")
    edit(d / "meta.json", lambda m: m.update(kind="golden"))
    assert cli.main(["run", "--dataset", str(ds), "--out", str(tmp_path / "o")]) == 2


def test_regression_duplicate_artifact_and_finding_is_rejected_clearly(tmp_path):
    ds = copy_ds(tmp_path)
    add_regression(ds, "bug-ecom-01", "flow-1", "inconcluso")
    with pytest.raises(DatasetError, match="ya existe la regresion bug-ecom-01-fp-1"):
        add_regression(ds, "bug-ecom-01", "flow-1", "sin-hallazgos")
    assert sorted(p.name for p in (ds / "regressions").iterdir()) == ["bug-ecom-01-fp-1"]
