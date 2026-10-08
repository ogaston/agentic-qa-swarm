import math
from fractions import Fraction

import pytest

from agent_eval import metrics as M
from agent_eval import thresholds as T
from support import RC, fnd, rec


def test_noise_zero_findings_is_zero_not_nan():
    v = M.ruido([rec("golden", "sin-hallazgos", "sin-hallazgos")])
    assert v == 0 and not math.isnan(float(v)) and float(v) == 0.0
    assert M.ruido([]) == 0


def test_noise_counts_findings_not_artifacts():
    rs = [
        rec("bug-sembrado", "bug", "bug", [fnd(), fnd(path="/otro", fid="g"), fnd(invariant="z", fid="h")], RC),
        rec("bug-sembrado", "bug", "bug", [fnd()], RC),
    ]
    assert M.ruido(rs) == Fraction(2, 4)  # por artefacto daria 1/2 con otros datos; aqui 2 de 4 hallazgos


def test_noise_one_artifact_many_findings_distinguishes_unit():
    rs = [
        rec("golden", "sin-hallazgos", "bug", [fnd(), fnd(fid="g"), fnd(fid="h")]),
        rec("bug-sembrado", "bug", "bug", [fnd()], RC),
    ]
    assert M.ruido(rs) == Fraction(3, 4)


def test_noise_any_finding_in_non_bug_artifact_is_false_positive():
    for v in ("sin-hallazgos", "inconcluso"):
        assert M.is_false_positive({"verdict": v, "root_cause": None}, fnd())


def test_noise_wrong_root_cause_is_false_positive_right_one_is_not():
    exp = {"verdict": "bug", "root_cause": RC}
    assert not M.is_false_positive(exp, fnd())
    assert M.is_false_positive(exp, fnd(path="/x"))
    assert M.is_false_positive(exp, fnd(method="GET"))
    assert M.is_false_positive(exp, fnd(invariant="otro"))


def test_precision_all_right_all_wrong_and_edge():
    ok = rec("golden", "sin-hallazgos", "sin-hallazgos")
    bad = rec("golden", "sin-hallazgos", "bug", [fnd()])
    assert M.precision([ok, ok]) == 1
    assert M.precision([bad, bad]) == 0
    assert M.precision([ok, bad]) == Fraction(1, 2)
    assert M.precision([]) == 0


def test_precision_bug_requires_matching_root_cause():
    right = rec("bug-sembrado", "bug", "bug", [fnd()], RC)
    wrong = rec("bug-sembrado", "bug", "bug", [fnd(path="/x")], RC)
    assert M.precision([right]) == 1 and M.precision([wrong]) == 0


def test_precision_ignores_unlabelled_kind_and_counts_regression():
    na = rec("no-arranca", "error", "error")
    assert M.precision([na]) == 0
    reg = rec("regresion-fp", "sin-hallazgos", "bug", [fnd()])
    assert M.precision([reg]) == 0


def test_factualidad_counts_seeded_bugs_only():
    hit = rec("bug-sembrado", "bug", "bug", [fnd()], RC)
    miss = rec("bug-sembrado", "bug", "inconcluso", [], RC)
    gold = rec("golden", "sin-hallazgos", "sin-hallazgos")
    assert M.factualidad([hit, miss, gold]) == Fraction(1, 2)
    assert M.factualidad([gold]) == 0
    # extra hallazgos no inflan el numerador: el artefacto cuenta una vez
    assert M.factualidad([rec("bug-sembrado", "bug", "bug", [fnd(), fnd(fid="g")], RC)]) == 1


def test_adherence_ratio_and_empty():
    cks = [{"rule": "A1", "ok": True, "detail": ""}, {"rule": "A2", "ok": False, "detail": ""}]
    assert M.adherencia([rec("golden", "x", "x", checks=cks)]) == Fraction(1, 2)
    assert M.adherencia([]) == 0


def test_threshold_boundaries_are_strict():
    assert not T.is_met("ruido", Fraction(1, 5))
    assert T.is_met("ruido", Fraction(19, 100))
    assert not T.is_met("precision", Fraction(4, 5))
    assert T.is_met("precision", Fraction(81, 100))
    assert T.is_met("adherencia", Fraction(1))
    assert not T.is_met("adherencia", Fraction(99, 100))
    assert not T.is_met("factualidad", Fraction(4, 5))
