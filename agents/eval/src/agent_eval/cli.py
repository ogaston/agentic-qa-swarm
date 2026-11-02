"""CLI: `python -m agent_eval run | compare | add-regression`."""
from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path
from urllib.parse import urlsplit

from . import thresholds as T
from .dataset import DEFAULT_DATASET, DatasetError
from .regression import add_regression
from .report import NOTE, NOTE_MODEL, all_met, compare_table, dumps, evaluate


class LiveConfigError(Exception):
    pass


def live_kwargs(env) -> dict:
    """Modo `--llm http`: el modelo se elige SOLO por entorno, con las mismas variables y validaciones que los
    Deployments de agent-planner y agent-reporter (LLM_BASE_URL, LLM_MODEL, LLM_API_KEY_FILE y, opcionales,
    PLANNER_*/REPORTER_* para topes y LLM_ALLOW_INSECURE_HTTP/LLM_INSECURE_HTTP_HOST). Nunca imprime la clave."""
    from agent_planner.config import ConfigError, load_config
    from agent_reporter.reporter import Limits as ReporterLimits
    from agent_reporter.server import _http_llm as reporter_http_llm

    e = dict(env)
    e["LLM_PROVIDER"] = "http"
    try:
        pcfg = load_config(e)
    except ConfigError as err:
        raise LiveConfigError(str(err)) from None
    try:
        rllm = reporter_http_llm(e)
        rlimits = ReporterLimits.from_env(e)
    except (SystemExit, ValueError) as err:
        raise LiveConfigError(str(err)) from None
    return {
        "model": e["LLM_MODEL"],
        "allow_hosts": (urlsplit(e["LLM_BASE_URL"]).hostname,),
        "planner_llm": pcfg.llm,
        "planner_limits": pcfg.limits,
        "reporter_llm": rllm,
        "reporter_limits": rlimits,
    }


def _run(args) -> int:
    kwargs = live_kwargs(os.environ) if args.llm == "http" else {}
    rep = evaluate(args.dataset, **kwargs)
    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    (out / "report.json").write_text(dumps(rep), encoding="utf-8")
    for k in T.METRIC_ORDER:
        op, bound, _ = T.THRESHOLDS[k]
        tag = "OK" if rep["thresholds_met"][k] else "FALLA"
        print(f"{tag} {k} {rep['metrics'][k]:.4f} (umbral {op} {float(bound):g})")
    note = NOTE if rep["valid_for"] == "pipeline" else NOTE_MODEL
    print(f"NOTA llm={rep['llm']} valid_for={rep['valid_for']}: {note}")
    return 0 if all_met(rep) else 1


def _compare(args) -> int:
    reports = [json.loads(Path(p).read_text(encoding="utf-8")) for p in args.reports]
    sys.stdout.write(compare_table(reports))
    return 0


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
    r.add_argument("--llm", choices=["fake", "http"], default="fake",
                   help="fake: respuestas canned (pipeline). http: modelo real definido por entorno.")
    c = sub.add_parser("compare")
    c.add_argument("reports", nargs="+", help="report.json de cada modelo")
    a = sub.add_parser("add-regression")
    a.add_argument("--dataset", default=str(DEFAULT_DATASET))
    a.add_argument("--artifact", required=True)
    a.add_argument("--finding", required=True)
    a.add_argument("--label", required=True, choices=["sin-hallazgos", "inconcluso"])
    args = ap.parse_args(argv)
    try:
        if args.cmd == "run":
            return _run(args)
        if args.cmd == "compare":
            return _compare(args)
        return _add(args)
    except DatasetError as e:
        print(f"error: {e}", file=sys.stderr)
        return 2
    except LiveConfigError as e:
        print(f"error: configuracion del modelo: {e}", file=sys.stderr)
        return 2
