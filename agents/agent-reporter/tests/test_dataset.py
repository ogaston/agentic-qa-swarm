import json
from pathlib import Path

import pytest
from jsonschema import Draft202012Validator

ROOT = Path(__file__).resolve().parents[3]
DS = ROOT / "agents" / "dataset"
PLANS = ROOT / "contracts" / "plans"
EXPECTED_COMPOSITION = {"golden": 3, "bug-sembrado": 4, "trampa-esquema": 3, "no-arranca": 2}


def load(p):
    return json.loads(Path(p).read_text(encoding="utf-8"))


def validator(p):
    return Draft202012Validator(load(p))


def check_artifact_dir(d, surface_v, evid_v, meta_v, exp_v):
    """Devuelve la lista de problemas de un directorio de artefacto."""
    errs = []
    meta, exp = load(d / "meta.json"), load(d / "expected.json")
    errs += [f"meta: {e.message}" for e in meta_v.iter_errors(meta)]
    errs += [f"expected: {e.message}" for e in exp_v.iter_errors(exp)]
    errs += [f"surface: {e.message}" for e in surface_v.iter_errors(load(d / "surface.json"))]
    for n in ("planner.response.json", "reporter.response.json"):
        if not (d / n).is_file():
            errs.append(f"falta {n}")
    ev = d / "evidence-uris.json"
    if meta["kind"] == "no-arranca":
        if ev.exists() or (d / "evidence").exists():
            errs.append("no-arranca no debe tener evidencia")
        if load(d / "surface.json")["endpoints"] != [] or load(d / "surface.json")["source"] != "probe":
            errs.append("no-arranca: endpoints [] y source probe")
    else:
        doc = load(ev)
        errs += [f"evidence: {e.message}" for e in evid_v.iter_errors(doc)]
        for u in doc["uris"]:
            prefix = f"s3://aqs-evidence/runs/{doc['run_id']}/"
            if not u.startswith(prefix):
                errs.append(f"uri fuera de disposicion: {u}")
                continue
            flow, fname = u[len(prefix):].split("/", 1)
            if not (d / "evidence" / flow / fname).is_file():
                errs.append(f"sin archivo para {u}")
    errs += check_cross(d, meta, exp)
    return errs


KIND_VERDICT = {"golden": "sin-hallazgos", "bug-sembrado": "bug", "trampa-esquema": "inconcluso", "no-arranca": "error"}


def check_cross(d, meta, exp):
    errs = []
    if meta["id"] != d.name:
        errs.append("meta.id no coincide con el directorio")
    mk = {a["id"]: a["kind"] for a in load(DS / "manifest.json")["artifacts"]}.get(d.name)
    if mk != meta["kind"]:
        errs.append(f"meta.kind {meta['kind']} != manifest {mk}")
    if exp["reporter"]["verdict"] != KIND_VERDICT[meta["kind"]]:
        errs.append("veredicto incoherente con el tipo")
    if (exp["reporter"]["root_cause"] is not None) != (meta["kind"] == "bug-sembrado"):
        errs.append("root_cause solo para bug-sembrado")
    if exp["planner"]["outcome"] != ("error" if meta["kind"] == "no-arranca" else "plan"):
        errs.append("outcome del planner incoherente")
    pr = load(d / "planner.response.json")
    if exp["planner"]["outcome"] == "plan":
        have = {i for fl in pr["flows"] for i in fl["invariants"]}
        for inv in exp["planner"]["must_cover_invariants"]:
            if inv not in have:
                errs.append(f"planner no cubre: {inv}")
    rc = exp["reporter"]["root_cause"]
    if rc:
        if load(d / "reporter.response.json").get("root_cause") != rc:
            errs.append("root_cause esperado != reporter.response")
        if {"method": rc["method"], "path": rc["path"]} not in load(d / "surface.json")["endpoints"]:
            errs.append("root_cause fuera de surface")
        if {"method": rc["method"], "path": rc["path"]} not in [st for fl in pr["flows"] for st in fl["steps"]]:
            errs.append("planner no planifica el endpoint de la causa raiz")
    return errs


def validators():
    return (
        validator(PLANS / "surface-artifact.schema.json"),
        validator(PLANS / "evidence-uris.schema.json"),
        validator(DS / "schema" / "meta.schema.json"),
        validator(DS / "schema" / "expected.schema.json"),
    )


def test_dataset_consistent():
    man = load(DS / "manifest.json")
    kinds = {}
    for a in man["artifacts"]:
        kinds[a["kind"]] = kinds.get(a["kind"], 0) + 1
        assert not check_artifact_dir(DS / "artifacts" / a["id"], *validators()), a["id"]
    assert kinds == EXPECTED_COMPOSITION
    assert len(list((DS / "artifacts").iterdir())) == 12
    domains = {load(DS / "artifacts" / a["id"] / "meta.json")["domain"] for a in man["artifacts"] if a["kind"] == "bug-sembrado"}
    assert domains == {"ecommerce", "fintech", "logistica"}


def test_dataset_negative_bad_base_url_and_extra_field(tmp_path):
    import shutil
    src = DS / "artifacts" / "golden-ecom-01"
    for mutate in (lambda s: s.update(base_url="sin-esquema"), lambda s: s.update(extra=1)):
        d = tmp_path / "x"
        shutil.rmtree(d, ignore_errors=True)
        shutil.copytree(src, d)
        s = load(d / "surface.json")
        mutate(s)
        (d / "surface.json").write_text(json.dumps(s), encoding="utf-8")
        assert check_artifact_dir(d, *validators())


def test_dataset_negative_kind_mismatch_and_uncovered_invariant(tmp_path):
    import shutil
    d = tmp_path / "golden-ecom-01"
    shutil.copytree(DS / "artifacts" / "golden-ecom-01", d)
    m = load(d / "meta.json"); m["kind"] = "bug-sembrado"
    (d / "meta.json").write_text(json.dumps(m), encoding="utf-8")
    assert any("manifest" in e for e in check_artifact_dir(d, *validators()))
    shutil.copytree(DS / "artifacts" / "golden-ecom-01", tmp_path / "g2" / "golden-ecom-01")
    d2 = tmp_path / "g2" / "golden-ecom-01"
    e = load(d2 / "expected.json"); e["planner"]["must_cover_invariants"] = ["invariante inexistente"]
    (d2 / "expected.json").write_text(json.dumps(e), encoding="utf-8")
    assert any("planner no cubre" in x for x in check_artifact_dir(d2, *validators()))


def test_dataset_no_secret_shaped_text():
    import re
    pat = re.compile(r"(?i)(sk-[a-z0-9]{8,}|bearer\s+[a-z0-9._-]{8,}|password\s*[=:]|api[_-]?key\s*[=:]|AKIA[0-9A-Z]{12,})")
    for f in (DS / "artifacts").rglob("*"):
        if f.is_file():
            assert not pat.search(f.read_text(encoding="utf-8")), f
