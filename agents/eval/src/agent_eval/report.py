"""Evaluacion completa y construccion del informe (determinista: sin fechas ni rutas)."""
from __future__ import annotations

import json
from pathlib import Path

from . import metrics as M
from . import thresholds as T
from .dataset import LABELLED_KINDS, expected_validator, load_dataset, load_expected
from .netguard import NetworkGuard
from .runner import run_artifact

NOTE = (
    "Evaluacion con LLM fake (respuestas canned): valida el pipeline, las validaciones y la redaccion, y que una "
    "regresion se detecta. NO mide la calidad de un modelo real (U3-T07)."
)
NOTE_MODEL = (
    "Evaluacion con modelo real configurado solo por entorno (LLM_BASE_URL, LLM_MODEL, LLM_API_KEY_FILE): mide "
    "la calidad de ESE modelo sobre el dataset sintetico de 12 casos, con los umbrales del PRD. Un dataset "
    "sintetico pequeno no generaliza a apps reales."
)


def evaluate(dataset: Path | str, *, model: str | None = None, allow_hosts=(), **run_kwargs) -> dict:
    """`model=None`: LLM fake (pipeline). Con `model` (etiqueta del modelo real) los run_kwargs traen
    planner_llm/reporter_llm y `allow_hosts` el host del LLM, el unico destino no local permitido."""
    dataset = Path(dataset)
    arts = load_dataset(dataset)
    ev = expected_validator(dataset)
    records = []
    with NetworkGuard(allow=allow_hosts) as guard:
        for a in arts:
            records.append(run_artifact(a, load_expected(a, ev), guard, **run_kwargs))
    return build_report(records, model=model)


def build_report(records: list[dict], model: str | None = None) -> dict:
    m = M.all_metrics(records)
    for r in records:
        r["reporter"]["findings"] = [
            dict(f, false_positive=M.is_false_positive(r["expected"], f)) for f in r["reporter"]["findings"]
        ]
    met = {k: T.is_met(k, m[k]) for k in T.METRIC_ORDER}
    return {
        "dataset": {
            "artifacts": sum(1 for r in records if r["kind"] != "regresion-fp"),
            "labelled": sum(1 for r in records if r["kind"] in LABELLED_KINDS),
            "regressions": sum(1 for r in records if r["kind"] == "regresion-fp"),
        },
        "llm": "fake" if model is None else f"http:{model}",
        "valid_for": "pipeline" if model is None else "model",
        "note": NOTE if model is None else NOTE_MODEL,
        "metrics": {k: float(m[k]) for k in T.METRIC_ORDER},
        "thresholds": {
            k: {"op": T.THRESHOLDS[k][0], "value": float(T.THRESHOLDS[k][1]), "source": T.THRESHOLDS[k][2]}
            for k in T.METRIC_ORDER
        },
        "thresholds_met": met,
        "per_artifact": records,
    }


def dumps(report: dict) -> str:
    return json.dumps(report, sort_keys=True, indent=2, ensure_ascii=True) + "\n"


def all_met(report: dict) -> bool:
    return all(report["thresholds_met"].get(k) is True for k in T.METRIC_ORDER)


def compare_table(reports: list[dict]) -> str:
    """Tabla markdown: una fila por informe (modelo) con las cuatro metricas, umbrales cumplidos y aciertos por caso."""
    head = "| llm | " + " | ".join(T.METRIC_ORDER) + " | casos ok | umbrales |"
    lines = [head, "|" + "---|" * (len(T.METRIC_ORDER) + 3)]
    for r in reports:
        ok = sum(1 for a in r["per_artifact"] if case_ok(a))
        vals = " | ".join(f"{r['metrics'][k]:.2f}" for k in T.METRIC_ORDER)
        met = "OK" if all_met(r) else "FALLA"
        lines.append(f"| {r['llm']} | {vals} | {ok}/{len(r['per_artifact'])} | {met} |")
    return "\n".join(lines) + "\n"


def case_ok(a: dict) -> bool:
    """Un caso acierta si planner y reporter dan el resultado esperado y todas sus reglas A1-A7 pasan."""
    exp = a["expected"]
    if a["planner"]["outcome"] != exp["planner"]:
        return False
    if a["reporter"]["verdict"] != exp.get("verdict"):
        return False
    if any(f.get("false_positive") for f in a["reporter"]["findings"]):
        return False
    rc = exp.get("root_cause")
    if exp.get("verdict") == "bug" and rc and not any(
        (f["invariant"], f["method"], f["path"]) == (rc["invariant"], rc["method"], rc["path"])
        for f in a["reporter"]["findings"]
    ):
        return False
    return all(c["ok"] for c in a["checks"])
