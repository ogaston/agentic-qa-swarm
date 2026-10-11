"""REPORTER_FAKE_FIXTURES con «prompt» precalculado (U8-T05): el arranque no lee evidencia.

Sin esto, en kind el reporter con REPORTER_EVIDENCE=s3 leeria MinIO al arrancar para calcular el prompt
de cada fixture: el pod quedaria en bucle hasta que alguien sembrase la evidencia.
"""
from __future__ import annotations

import json

from agent_reporter.server import build_service


def _env(tmp_path, fixtures):
    path = tmp_path / "fixtures.json"
    path.write_text(json.dumps(fixtures), encoding="utf-8")
    return {
        "LLM_PROVIDER": "fake",
        "REPORTER_ALLOW_FAKE": "1",
        # Directorio inexistente: si el arranque intenta leer evidencia, falla.
        "REPORTER_EVIDENCE_DIR": str(tmp_path / "no-existe"),
        "REPORTER_FAKE_FIXTURES": str(path),
    }


def test_fixture_con_prompt_no_lee_evidencia_al_arrancar(tmp_path):
    prompt = "PROMPT PRECALCULADO\n"
    response = {"verdict": "sin-hallazgos", "summary": "ok", "findings": []}
    svc = build_service(_env(tmp_path, [{"run_id": "r1", "prompt": prompt, "response": response}]))
    res = svc.llm.complete(prompt, max_tokens=10, timeout_s=5)
    assert json.loads(res.text) == response


def test_fixture_sin_prompt_sigue_calculando_el_prompt(tmp_path):
    # Compatibilidad: la forma antigua (run_id + evidence_uris) sigue leyendo la evidencia al arrancar.
    import pytest

    from agent_reporter.errors import EvidenceUnavailable

    fixtures = [{"run_id": "r1", "evidence_uris": ["s3://evidence/runs/r1/flow-1/result.json"], "response": {}}]
    with pytest.raises(EvidenceUnavailable):
        build_service(_env(tmp_path, fixtures))
