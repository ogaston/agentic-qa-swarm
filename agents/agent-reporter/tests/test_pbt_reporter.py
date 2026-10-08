"""Propiedades (Hypothesis) del reporter: PBT-02 round-trip, PBT-03 invariantes. Correr: pytest -k pbt."""
import json
import re

from hypothesis import given, settings
from hypothesis import strategies as st

import gen
from agent_reporter.errors import ReportRejected
from agent_reporter.fakes.fake_llm import FakeLLM
from agent_reporter.redact import redact_secrets
from agent_reporter.report_model import validate_report
from agent_reporter.reporter import CLOSE, OPEN, Limits, correlate_postmortem, prepare
from support import report_validator

MARKER = re.compile(rb"\n\[\.\.\.recortado: (\d+) bytes\.\.\.\]\n")


class MemReader:
    def __init__(self, objects):
        self.objects = objects

    def get(self, uri):
        return self.objects[uri]


def _block(prompt):
    assert prompt.count(OPEN) == 1 and prompt.count(CLOSE) == 1
    return json.loads(prompt.split(OPEN + "\n")[1].split("\n" + CLOSE)[0])


# 6 (PBT-02) Report: parse(serialize(r)) == r y valida contra report.schema.json
@given(rep=gen.report(), ensure_ascii=st.booleans(), sort_keys=st.booleans())
def test_pbt_report_roundtrip_and_schema(rep, ensure_ascii, sort_keys):
    wire = json.dumps(rep, ensure_ascii=ensure_ascii, sort_keys=sort_keys)
    assert json.loads(wire) == rep
    assert list(report_validator().iter_errors(json.loads(wire))) == []
    assert validate_report(json.loads(wire)) == []  # el espejo manual coincide con el esquema


# 7 (PBT-03) redact_secrets
@given(st=gen.secret_text())
def test_pbt_redact_removes_seeded_secrets_idempotent_bounded(st):
    body, secs = st
    raw = body.encode("utf-8")
    out = redact_secrets(raw)
    for s in secs:
        for needle in s.needles:
            assert needle.encode() not in out, f"sobrevive un fragmento de {s.kind}"
    assert redact_secrets(out) == out  # idempotente
    n = out.count(b"[REDACTED:")
    assert n >= 1
    assert len(out) <= len(raw) + 32 * n  # el crecimiento esta acotado por el numero de reemplazos


@given(txt=gen.clean_text(80))
def test_pbt_redact_clean_text_unchanged(txt):
    raw = txt.encode("utf-8")
    assert redact_secrets(raw) == raw


# 8 (PBT-03) validador del reporter: si acepta, bug => flujo fallido, sin-hallazgos => todos pasados, URIs recibidas
@given(ev=gen.evidence_set(), data=st.data())
def test_pbt_report_validator_accept_implies_invariants(ev, data):
    kind, txt = data.draw(gen.model_response(ev))
    lim = Limits()
    reader = MemReader(ev.objects)
    prep = prepare(ev.run_id, ev.uris, reader, lim)
    llm = FakeLLM()
    llm.register(prep.prompt, txt)
    try:
        rep = correlate_postmortem(ev.run_id, ev.uris, reader, llm, lim)
    except ReportRejected as e:
        assert e.reason
        return
    assert kind in ("valid",), f"acepto una respuesta de clase {kind}"
    states = list(ev.flows.values())
    if rep["verdict"] == "bug":
        assert "failed" in states
    if rep["verdict"] == "sin-hallazgos":
        assert states and all(s == "passed" for s in states)
    for f in rep["findings"]:
        assert set(f["evidence_uris"]) <= set(ev.uris)
    assert rep["run_id"] == ev.run_id and validate_report(rep) == []


# 9 (PBT-03) recorte de evidencia: topes, determinismo, cabeza y cola
@st.composite
def _evidence_case(draw):
    max_obj = draw(st.integers(1, 300))
    max_total = draw(st.integers(1, 900))
    n = draw(st.integers(1, 8))
    blobs = [draw(gen.evidence_blob(max_obj)).encode("utf-8") for _ in range(n)]
    return max_obj, max_total, blobs


@given(case=_evidence_case())
def test_pbt_evidence_truncation_bounded_deterministic_keeps_head_and_tail(case):
    max_obj, max_total, blobs = case
    uris = [f"{gen.BUCKET}/run-1/f{i}/logs.txt" for i in range(len(blobs))]
    reader = MemReader(dict(zip(uris, blobs)))
    lim = Limits(max_object_bytes=max_obj, max_total_bytes=max_total)
    p1 = prepare("run-1", uris, reader, lim)
    assert prepare("run-1", uris, reader, lim).prompt == p1.prompt  # determinista
    objs = _block(p1.prompt)["objects"]
    assert [o["uri"] for o in objs] == uris
    used = 0
    for o, orig in zip(objs, blobs):
        content = o["content"].encode("utf-8")
        eff = min(max_obj, max(max_total - used, 0))
        used += len(content)
        assert len(content) <= eff, "el objeto supera su tope"
        assert o["truncated"] == (len(orig) > eff)
        if not o["truncated"]:
            assert content == orig
            continue
        m = MARKER.search(content)
        if m is None:  # el tope no admite ni la marca: solo cabeza
            assert orig.startswith(content)
            continue
        head, tail = content[: m.start()], content[m.end():]
        assert orig.startswith(head) and orig.endswith(tail)
        assert int(m.group(1)) == len(orig) - eff
        if eff >= 120:  # con presupuesto razonable se conservan ambos extremos
            assert head and tail, "el recorte descarto la cabeza o la cola"
    assert used <= max_total, "la evidencia del prompt supera REPORTER_MAX_TOTAL_BYTES"


@settings(max_examples=500, database=None)
@given(data=st.data())
def _draw_classes(data, seen):
    ev = data.draw(gen.evidence_set())
    seen.add("evidencia_" + ("con_fallido" if "failed" in ev.flows.values() else "sin_fallido"))
    if ev.flows and all(s == "passed" for s in ev.flows.values()):
        seen.add("evidencia_todo_pasado")
    if any(s == "unknown" for s in ev.flows.values()):
        seen.add("evidencia_con_desconocido")
    if any(b"datos-evidencia" in b for b in ev.objects.values()):
        seen.add("logs_con_instrucciones")
    if any(b"datos-evidencia" not in b for u, b in ev.objects.items() if u.endswith("logs.txt")):
        seen.add("logs_sin_instrucciones")
    kind, txt = data.draw(gen.model_response(ev))
    seen.add("resp_" + kind)
    try:
        prep = prepare(ev.run_id, ev.uris, MemReader(ev.objects), Limits())
        llm = FakeLLM()
        llm.register(prep.prompt, txt)
        rep = correlate_postmortem(ev.run_id, ev.uris, MemReader(ev.objects), llm, Limits())
        seen.add("aceptado_" + rep["verdict"])
    except ReportRejected:
        seen.add("rechazado")
    for v in gen.VERDICTS:
        if data.draw(gen.report()) ["verdict"] == v:
            seen.add("report_" + v)
    seen.add("secreto_" + data.draw(gen.secret()).kind)
    body, secs = data.draw(gen.secret_text())
    seen.update("contexto_con_" + s.kind for s in secs)


def test_pbt_generator_coverage():
    seen = set()
    _draw_classes(seen=seen)
    esperadas = {
        "evidencia_con_fallido", "evidencia_sin_fallido", "evidencia_todo_pasado", "evidencia_con_desconocido",
        "logs_con_instrucciones", "logs_sin_instrucciones", "rechazado",
        *("resp_" + k for k in gen.KINDS), *("aceptado_" + v for v in gen.VERDICTS),
        *("report_" + v for v in gen.VERDICTS), *("secreto_" + k for k in gen.SECRET_KINDS),
    }
    assert esperadas <= seen, sorted(esperadas - seen)
