import json

import pytest
from jsonschema import Draft202012Validator

from agent_eval import cli, report
from agent_eval.dataset import DatasetError, load_dataset
from support import DS, SCHEMA, copy_ds, edit, report_ok


def test_report_schema_valid_declares_scope_and_counts():
    rep = report.evaluate(DS)
    assert report_ok(rep)
    assert rep["llm"] == "fake" and rep["valid_for"] == "pipeline" and "NO mide la calidad" in rep["note"]
    assert len(rep["per_artifact"]) == 12
    assert sum(len(r["checks"]) for r in rep["per_artifact"]) > 40


def test_report_schema_rejects_missing_or_changed_scope():
    rep = report.evaluate(DS)
    v = Draft202012Validator(SCHEMA)
    for k, val in (("llm", "real"), ("valid_for", "model-quality")):
        bad = dict(rep, **{k: val})
        assert list(v.iter_errors(bad))
    for k in ("llm", "valid_for", "thresholds_met"):
        bad = {a: b for a, b in rep.items() if a != k}
        assert list(v.iter_errors(bad))


def test_report_is_deterministic_byte_for_byte(tmp_path):
    cli.main(["run", "--out", str(tmp_path / "a")])
    cli.main(["run", "--out", str(tmp_path / "b")])
    assert (tmp_path / "a/report.json").read_bytes() == (tmp_path / "b/report.json").read_bytes()


def test_dataset_loader_rejects_hostile_ids(tmp_path):
    for bad in ("../x", "a\n", "A", "a b", "", "a--", "-a", "x/y", "bug-ecom-01\n"):
        ds = copy_ds(tmp_path)
        edit(ds / "manifest.json", lambda d: d["artifacts"].__setitem__(0, {"id": bad, "kind": "golden"}))
        with pytest.raises(DatasetError):
            load_dataset(ds)
        import shutil; shutil.rmtree(ds)


def test_dataset_loader_rejects_bad_kind_duplicates_and_symlinked_dir(tmp_path):
    ds = copy_ds(tmp_path)
    edit(ds / "manifest.json", lambda d: d["artifacts"][0].update(kind="otra"))
    with pytest.raises(DatasetError):
        load_dataset(ds)
    ds2 = tmp_path / "ds2"
    import shutil
    shutil.copytree(DS, ds2)
    edit(ds2 / "manifest.json", lambda d: d["artifacts"].append(dict(d["artifacts"][0])))
    with pytest.raises(DatasetError):
        load_dataset(ds2)
    ds3 = tmp_path / "ds3"
    shutil.copytree(DS, ds3)
    shutil.rmtree(ds3 / "artifacts/golden-ecom-01")
    (ds3 / "artifacts/golden-ecom-01").symlink_to(ds3 / "artifacts/golden-logis-01")
    with pytest.raises(DatasetError):
        load_dataset(ds3)


def test_cli_dataset_error_exit_2(tmp_path):
    ds = copy_ds(tmp_path)
    (ds / "manifest.json").write_text("{")
    assert cli.main(["run", "--dataset", str(ds), "--out", str(tmp_path / "o")]) == 2


def test_cli_invalid_expected_exit_2(tmp_path):
    ds = copy_ds(tmp_path)
    edit(ds / "artifacts/golden-ecom-01/expected.json", lambda d: d.pop("reporter"))
    assert cli.main(["run", "--dataset", str(ds), "--out", str(tmp_path / "o")]) == 2


@pytest.mark.parametrize("bad", ["golden-ecom-01\n", "a b", "A", "", "a_b", "x/y", "..", "a\x00", 5, None])
def test_id_check_is_full_match_and_rejects_non_strings(bad):
    from agent_eval.dataset import _check_id
    with pytest.raises(DatasetError):
        _check_id(bad, "t")


def test_id_check_accepts_valid():
    from agent_eval.dataset import _check_id
    assert _check_id("bug-ecom-01-fp-2", "t") == "bug-ecom-01-fp-2"


def test_regression_dir_with_newline_name_is_rejected(tmp_path):
    ds = copy_ds(tmp_path)
    (ds / "regressions").mkdir()
    (ds / "regressions" / "x-fp-1\n").mkdir()
    with pytest.raises(DatasetError):
        load_dataset(ds)
