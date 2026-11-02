"""Modo `run --llm http` (entrega del módulo 8): el modelo se cambia SOLO por entorno y el mismo dataset de 12
casos lo evalúa. Sin red: dos «modelos» servidos por un gateway compatible con OpenAI en loopback.

- `modelo-bueno` responde lo que el dataset espera (las respuestas canned, ahora viajando por HTTP real).
- `modelo-roto` responde `{}` a todo: el arnés tiene que detectarlo (rc 1) sin tocar código.
"""
from __future__ import annotations

import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import pytest

from agent_eval import cli, report
from agent_eval.dataset import load_dataset, read_json
from agent_eval.runner import WORKFLOW, StripReader, canned_plan
from agent_planner.prompt import build_prompt
from agent_reporter.precision import _canned
from agent_reporter.reporter import Limits as ReporterLimits
from agent_reporter.reporter import prepare
from support import DS, report_ok


def _answers() -> dict[str, str]:
    """prompt exacto -> respuesta esperada, por artefacto del dataset (planner y reporter)."""
    out: dict[str, str] = {}
    for art in load_dataset(DS):
        surface = read_json(art.dir / "surface.json")
        resp = read_json(art.dir / "planner.response.json")
        out[build_prompt(surface, WORKFLOW)] = "{}" if "error" in resp else json.dumps(
            canned_plan(surface, WORKFLOW, resp))
        ev_path = art.dir / "evidence-uris.json"
        if ev_path.is_file():
            ev = read_json(ev_path)
            reader = StripReader(art.dir / "evidence", f"/runs/{ev['run_id']}/")
            prompt = prepare(ev["run_id"], ev["uris"], reader, ReporterLimits()).prompt
            out[prompt] = _canned(art.dir, ev["uris"])
    return out


@pytest.fixture(scope="module")
def gateway():
    answers = _answers()
    seen: list[tuple[str, str]] = []

    class H(BaseHTTPRequestHandler):
        def log_message(self, *a):
            pass

        def do_POST(self):
            body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            prompt, model = body["messages"][0]["content"], body["model"]
            seen.append((model, self.headers.get("Authorization", "")))
            text = answers.get(prompt, "{}") if model == "modelo-bueno" else "{}"
            raw = json.dumps({"choices": [{"message": {"content": text}}],
                              "usage": {"prompt_tokens": len(prompt) // 4, "completion_tokens": len(text) // 4}})
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(raw.encode())

    srv = ThreadingHTTPServer(("127.0.0.1", 0), H)
    t = threading.Thread(target=srv.serve_forever, daemon=True)
    t.start()
    yield f"http://127.0.0.1:{srv.server_port}/v1", seen
    srv.shutdown()


def _env(monkeypatch, tmp_path, base, model):
    key = tmp_path / "key"
    key.write_text("sk-prueba-no-real\n")
    for k, v in {"LLM_BASE_URL": base, "LLM_MODEL": model, "LLM_API_KEY_FILE": str(key)}.items():
        monkeypatch.setenv(k, v)


def test_buen_modelo_por_http_cumple_umbrales_y_queda_etiquetado(gateway, monkeypatch, tmp_path, capsys):
    base, seen = gateway
    _env(monkeypatch, tmp_path, base, "modelo-bueno")
    assert cli.main(["run", "--llm", "http", "--out", str(tmp_path / "a")]) == 0
    rep = json.loads((tmp_path / "a" / "report.json").read_text())
    assert report_ok(rep)
    assert rep["llm"] == "http:modelo-bueno" and rep["valid_for"] == "model"
    assert rep["metrics"] == {"factualidad": 1.0, "precision": 1.0, "ruido": 0.0, "adherencia": 1.0}
    assert rep["dataset"]["artifacts"] == 12
    assert sum(report.case_ok(a) for a in rep["per_artifact"]) == 12
    # Las llamadas viajaron por HTTP con el modelo del entorno y la clave del archivo.
    assert seen and all(m == "modelo-bueno" and auth == "Bearer sk-prueba-no-real" for m, auth in seen)
    out = capsys.readouterr().out
    assert "llm=http:modelo-bueno valid_for=model" in out and "sk-prueba" not in out


def test_cambiar_solo_LLM_MODEL_cambia_el_resultado_y_el_arnes_lo_detecta(gateway, monkeypatch, tmp_path):
    base, _ = gateway
    _env(monkeypatch, tmp_path, base, "modelo-roto")
    assert cli.main(["run", "--llm", "http", "--out", str(tmp_path / "b")]) == 1
    rep = json.loads((tmp_path / "b" / "report.json").read_text())
    assert report_ok(rep) and rep["llm"] == "http:modelo-roto"
    assert rep["thresholds_met"]["precision"] is False and rep["thresholds_met"]["factualidad"] is False
    # Falla cerrado: planes y reportes inválidos se rechazan; los casos no-arranca siguen bien (no llaman al modelo).
    ok = {a["id"] for a in rep["per_artifact"] if report.case_ok(a)}
    assert ok == {"noarranca-ecom-01", "noarranca-logis-01"}


def test_compare_tabula_los_dos_modelos(gateway, monkeypatch, tmp_path, capsys):
    base, _ = gateway
    for model, d in (("modelo-bueno", "a"), ("modelo-roto", "b")):
        _env(monkeypatch, tmp_path, base, model)
        cli.main(["run", "--llm", "http", "--out", str(tmp_path / d)])
    capsys.readouterr()
    assert cli.main(["compare", str(tmp_path / "a" / "report.json"), str(tmp_path / "b" / "report.json")]) == 0
    rows = capsys.readouterr().out.splitlines()
    assert rows[0].startswith("| llm | ")
    assert rows[2].startswith("| http:modelo-bueno |") and "| 12/12 | OK |" in rows[2]
    assert rows[3].startswith("| http:modelo-roto |") and "| 2/12 | FALLA |" in rows[3]


@pytest.mark.parametrize("missing", ["LLM_BASE_URL", "LLM_MODEL", "LLM_API_KEY_FILE"])
def test_sin_configuracion_completa_no_corre(gateway, monkeypatch, tmp_path, capsys, missing):
    base, _ = gateway
    _env(monkeypatch, tmp_path, base, "modelo-bueno")
    monkeypatch.delenv(missing)
    assert cli.main(["run", "--llm", "http", "--out", str(tmp_path / "c")]) == 2
    assert "configuracion del modelo" in capsys.readouterr().err
    assert not (tmp_path / "c" / "report.json").exists()


def test_http_en_claro_hacia_host_no_local_se_rechaza(monkeypatch, tmp_path, capsys):
    _env(monkeypatch, tmp_path, "http://203.0.113.7/v1", "modelo-bueno")
    assert cli.main(["run", "--llm", "http", "--out", str(tmp_path / "d")]) == 2


def test_modo_fake_sin_cambios():
    rep = report.evaluate(DS)
    assert rep["llm"] == "fake" and rep["valid_for"] == "pipeline" and report.all_met(rep)


def test_guarda_de_red_permite_solo_el_host_del_llm():
    from agent_eval.netguard import NetworkBlocked, NetworkGuard

    g = NetworkGuard(allow=["203.0.113.7"])
    assert g._allowed("203.0.113.7") and g._allowed("127.0.0.1")
    with pytest.raises(NetworkBlocked):
        g._check_addr(("198.51.100.1", 443))
    assert g.attempts == ["198.51.100.1"]


def test_el_informe_guarda_el_codigo_fijo_del_error(gateway, monkeypatch, tmp_path):
    """Sin el código, un fallo sistemático (p. ej. content vacío de un modelo de razonamiento) es indistinguible."""
    base, _ = gateway
    _env(monkeypatch, tmp_path, base, "modelo-roto")
    cli.main(["run", "--llm", "http", "--out", str(tmp_path / "e")])
    rep = json.loads((tmp_path / "e" / "report.json").read_text())
    errs = {a["id"]: a["planner"]["error"] for a in rep["per_artifact"]}
    assert errs["golden-ecom-01"].startswith("PlanRejected:")
    assert errs["noarranca-ecom-01"] == "NoSurface"  # A3 sigue comparando el nombre exacto


def test_err_incluye_kind_y_reason():
    from agent_eval.runner import _err
    from agent_planner.errors import BudgetExceeded, PlanRejected
    from agent_reporter.errors import BudgetExceeded as RBudget

    assert _err(BudgetExceeded("time")) == "BudgetExceeded:time"
    assert _err(PlanRejected("json_invalido")) == "PlanRejected:json_invalido"
    assert _err(RBudget("salida")) == "BudgetExceeded:salida"
    from agent_planner.llm import LLMUnavailable

    assert _err(LLMUnavailable("http_error")) == "LLMUnavailable:http_error"
    assert _err(ValueError("x")) == "ValueError"
