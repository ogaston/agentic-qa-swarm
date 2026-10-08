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


def evaluate(dataset: Path | str, **run_kwargs) -> dict:
    dataset = Path(dataset)
    arts = load_dataset(dataset)
    ev = expected_validator(dataset)
    records = []
    with NetworkGuard() as guard:
        for a in arts:
            records.append(run_artifact(a, load_expected(a, ev), guard, **run_kwargs))
    return build_report(records)


def build_report(records: list[dict]) -> dict:
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
        "llm": "fake",
        "valid_for": "pipeline",
        "note": NOTE,
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
