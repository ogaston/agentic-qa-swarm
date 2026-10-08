"""Regresiones fijas halladas por test_pbt_evidence_truncation_* (U3-T06): el recorte debe respetar el tope."""
from agent_reporter.reporter import Limits, _truncate, prepare
import json


class _Mem:
    def __init__(self, objs):
        self.objs = objs

    def get(self, uri):
        return self.objs[uri]


def _contents(objs, **lim):
    uris = list(objs)
    p = prepare("run-1", uris, _Mem(objs), Limits(**lim))
    block = p.prompt.split("<datos-evidencia>\n")[1].split("\n</datos-evidencia>")[0]
    return [o["content"].encode() for o in json.loads(block)["objects"]]


def test_truncate_never_exceeds_limit_when_marker_does_not_fit():
    # ejemplo reducido por Hypothesis: max_object=1, max_total=1, evidencia b"01" -> antes: 28 bytes (la marca sola)
    for limit in range(0, 40):
        out, cut = _truncate(b"x" * 100, limit)
        assert cut and len(out) <= limit, limit


def test_total_budget_includes_exhausted_budget_objects():
    uris = {f"s3://b/runs/run-1/f{i}/logs.txt": b"z" * 500 for i in range(6)}
    cs = _contents(uris, max_object_bytes=300, max_total_bytes=700)
    assert sum(len(c) for c in cs) <= 700


def test_truncate_does_not_split_utf8_characters():
    data = ("日" * 100).encode()  # 300 bytes, 3 por caracter
    for limit in range(0, 120):
        out, _ = _truncate(data, limit)
        assert len(out) <= limit
        out.decode("utf-8")  # estricto: no hay caracteres partidos (antes: U+FFFD que crece al re-codificar)
