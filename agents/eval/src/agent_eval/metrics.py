"""Metricas sobre los registros por artefacto. Fracciones exactas; denominador 0 => 0 definido, nunca NaN."""
from __future__ import annotations

from fractions import Fraction

from .dataset import LABELLED_KINDS


def _frac(n: int, d: int) -> Fraction:
    return Fraction(n, d) if d else Fraction(0)


def _triple(f: dict):
    return (f["invariant"], f["method"], f["path"])


def _rc(expected: dict):
    rc = expected.get("root_cause")
    return (rc["invariant"], rc["method"], rc["path"]) if rc else None


def is_false_positive(expected: dict, finding: dict) -> bool:
    """Falso positivo: hallazgo donde no se espera `bug`, o con causa raiz distinta de la esperada."""
    if expected.get("verdict") != "bug":
        return True
    return _triple(finding) != _rc(expected)


def factualidad(records) -> Fraction:
    bugs = [r for r in records if r["kind"] == "bug-sembrado"]
    hit = sum(1 for r in bugs if any(_triple(f) == _rc(r["expected"]) for f in r["reporter"]["findings"]))
    return _frac(hit, len(bugs))


def _verdict_ok(r) -> bool:
    exp, got = r["expected"], r["reporter"]
    if got["verdict"] != exp["verdict"]:
        return False
    if exp["verdict"] == "bug":
        return any(_triple(f) == _rc(exp) for f in got["findings"])
    return True


def precision(records) -> Fraction:
    lab = [r for r in records if r["kind"] in LABELLED_KINDS]
    return _frac(sum(1 for r in lab if _verdict_ok(r)), len(lab))


def ruido(records) -> Fraction:
    total = fp = 0
    for r in records:
        for f in r["reporter"]["findings"]:
            total += 1
            fp += is_false_positive(r["expected"], f)
    return _frac(fp, total)


def adherencia(records) -> Fraction:
    cks = [c for r in records for c in r["checks"]]
    return _frac(sum(1 for c in cks if c["ok"]), len(cks))


def all_metrics(records) -> dict[str, Fraction]:
    return {
        "factualidad": factualidad(records),
        "precision": precision(records),
        "ruido": ruido(records),
        "adherencia": adherencia(records),
    }
