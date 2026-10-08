import argparse
from pathlib import Path

from .examples import example_plans
from .render import render_flow
from .validate import validate_plan


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(prog="agent_planner.k6")
    sub = ap.add_subparsers(dest="cmd", required=True)
    r = sub.add_parser("render-examples")
    r.add_argument("--out", required=True)
    args = ap.parse_args(argv)
    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    n = 0
    for name, plan in example_plans():
        validate_plan(plan)
        for flow in plan["flows"]:
            (out / f"{name}__{flow['flow_id']}.k6.js").write_text(render_flow(plan, flow), encoding="utf-8")
            n += 1
    print(f"{n} scripts en {out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
