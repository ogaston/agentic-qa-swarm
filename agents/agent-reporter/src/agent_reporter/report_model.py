"""Validacion estructural del Report (espejo manual de contracts/plans/report.schema.json).

El runtime no depende de jsonschema; una prueba verifica la paridad con el esquema real.
"""
from __future__ import annotations

import re

_URI = re.compile(r"^(s3|https)://[^\s]+$")
METHODS = ("GET", "POST", "PUT", "PATCH", "DELETE")
VERDICTS = ("bug", "sin-hallazgos", "inconcluso")
_FINDING_KEYS = {"finding_id", "root_cause", "invariant", "method", "path", "evidence_uris"}


def _nes(v) -> bool:
    return isinstance(v, str) and len(v) > 0


def validate_evidence_uris(doc) -> list[str]:
    errs = []
    if not isinstance(doc, dict) or set(doc) != {"run_id", "uris"}:
        return ["forma"]
    if not _nes(doc["run_id"]):
        errs.append("run_id")
    u = doc["uris"]
    if not isinstance(u, list) or not u:
        errs.append("uris vacio")
    elif not all(isinstance(x, str) and _URI.match(x) for x in u):
        errs.append("uri invalida")
    return errs


def validate_report(r) -> list[str]:
    if not isinstance(r, dict) or set(r) != {"run_id", "verdict", "summary", "findings"}:
        return ["forma"]
    errs = []
    if not _nes(r["run_id"]):
        errs.append("run_id")
    if r["verdict"] not in VERDICTS:
        errs.append("verdict")
    if not isinstance(r["summary"], str):
        errs.append("summary")
    f = r["findings"]
    if not isinstance(f, list):
        return errs + ["findings"]
    if r["verdict"] == "bug" and len(f) < 1:
        errs.append("bug sin hallazgos")
    if r["verdict"] in ("sin-hallazgos", "inconcluso") and f:
        errs.append("hallazgos con veredicto sin bug")
    for x in f:
        if not isinstance(x, dict) or set(x) != _FINDING_KEYS:
            errs.append("hallazgo: forma")
            continue
        for k in ("finding_id", "root_cause", "invariant"):
            if not _nes(x[k]):
                errs.append(f"hallazgo: {k}")
        if x["method"] not in METHODS:
            errs.append("hallazgo: method")
        if not (isinstance(x["path"], str) and x["path"].startswith("/")):
            errs.append("hallazgo: path")
        u = x["evidence_uris"]
        if not isinstance(u, list) or not u or not all(isinstance(i, str) and _URI.match(i) for i in u):
            errs.append("hallazgo: evidence_uris")
    return errs
