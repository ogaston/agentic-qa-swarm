"""CLI: `python -m agent_eval run | add-regression`."""
from __future__ import annotations

import argparse
import sys
from pathlib import Path

from . import thresholds as T
from .dataset import DEFAULT_DATASET, DatasetError
from .regression import add_regression
from .report import NOTE, all_met, dumps, evaluate



def _run(args) -> int:
    rep = evaluate(args.dataset)
    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    (out / "report.json").write_text(dumps(rep), encoding="utf-8")
    for k in T.METRIC_ORDER:
        op, bound, _ = T.THRESHOLDS[k]
        tag = "OK" if rep["thresholds_met"][k] else "FALLA"
        print(f"{tag} {k} {rep['metrics'][k]:.4f} (umbral {op} {float(bound):g})")
    print(f"NOTA llm=fake valid_for=pipeline: {NOTE}")
    return 0 if all_met(rep) else 1


def _add(args) -> int:
    d = add_regression(args.dataset, args.artifact, args.finding, args.label)
    print(f"regresion creada: {d.name}")
    return 0


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(prog="agent_eval")
    sub = ap.add_subparsers(dest="cmd", required=True)
    r = sub.add_parser("run")
    r.add_argument("--dataset", default=str(DEFAULT_DATASET))
    r.add_argument("--out", default="out")
    a = sub.add_parser("add-regression")
    a.add_argument("--dataset", default=str(DEFAULT_DATASET))
    a.add_argument("--artifact", required=True)
    a.add_argument("--finding", required=True)
    a.add_argument("--label", required=True, choices=["sin-hallazgos", "inconcluso"])
    args = ap.parse_args(argv)
    try:
        return _run(args) if args.cmd == "run" else _add(args)
    except DatasetError as e:
        print(f"error: {e}", file=sys.stderr)
        return 2
