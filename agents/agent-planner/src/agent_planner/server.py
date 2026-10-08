"""Servidor HTTP minimo (biblioteca estandar). POST /v1/plan, /healthz, /readyz, /metrics."""
from __future__ import annotations

import json
import logging
import re
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from .errors import BudgetExceeded, InvalidRequest, Limits, NoSurface, PlanRejected
from .llm import LLMClient, LLMError
from .planner import plan

MAX_BODY = 256 * 1024
RESULTS = ("ok", "no_surface", "plan_rejected", "invalid_request", "budget_exceeded", "llm_unavailable")
_SAFE_ID = re.compile(r"^[A-Za-z0-9._-]{1,64}$")
log = logging.getLogger("agent_planner")


class Metrics:
    def __init__(self) -> None:
        self._lock = threading.Lock()
        self.requests = {r: 0 for r in RESULTS}
        self.tokens = {"input": 0, "output": 0}

    def inc(self, result: str, usage: dict | None = None) -> None:
        with self._lock:
            self.requests[result] += 1
            for k, v in (usage or {}).items():
                self.tokens[k] += int(v)

    def render(self) -> str:
        with self._lock:
            out = ["# TYPE aqs_planner_requests_total counter"]
            out += [f'aqs_planner_requests_total{{result="{r}"}} {n}' for r, n in self.requests.items()]
            out.append("# TYPE aqs_planner_tokens_total counter")
            out += [f'aqs_planner_tokens_total{{direction="{d}"}} {n}' for d, n in self.tokens.items()]
            return "\n".join(out) + "\n"


def _safe_run_id(body) -> str:
    try:
        rid = body["surface"]["run_id"]
    except (KeyError, TypeError):
        return "desconocido"
    return rid if isinstance(rid, str) and _SAFE_ID.match(rid) else "desconocido"


def _log(run_id: str, result: str, reason: str) -> None:
    log.info(json.dumps({"run_id": run_id, "result": result, "reason": reason}, sort_keys=True))


def make_server(host: str, port: int, llm: LLMClient, limits: Limits) -> ThreadingHTTPServer:
    metrics = Metrics()

    class Handler(BaseHTTPRequestHandler):
        timeout = 10
        protocol_version = "HTTP/1.1"

        def log_message(self, *a):  # sin log de acceso: puede contener rutas
            pass

        def _send(self, code: int, body: bytes, ctype: str = "application/json") -> None:
            self.send_response(code)
            self.send_header("Content-Type", ctype)
            self.send_header("Content-Length", str(len(body)))
            self.send_header("Connection", "close")
            self.end_headers()
            self.wfile.write(body)
            self.close_connection = True

        def _json(self, code: int, obj) -> None:
            self._send(code, json.dumps(obj, sort_keys=True).encode())

        def _err(self, code: int, error: str, detail: str, run_id: str = "desconocido") -> None:
            if error in RESULTS:
                metrics.inc(error)
            _log(run_id, error, detail)
            self._json(code, {"error": error, "detail": detail})

        def do_GET(self):
            if self.path == "/healthz" or self.path == "/readyz":
                self._json(200, {"status": "ok"})
            elif self.path == "/metrics":
                self._send(200, metrics.render().encode(), "text/plain; version=0.0.4")
            else:
                self._json(404, {"error": "not_found"})

        def do_POST(self):
            if self.path != "/v1/plan":
                return self._json(404, {"error": "not_found"})
            ctype = (self.headers.get("Content-Type") or "").split(";")[0].strip().lower()
            if ctype != "application/json":
                return self._json(415, {"error": "unsupported_media_type"})
            try:
                n = int(self.headers.get("Content-Length", ""))
            except ValueError:
                return self._err(422, "invalid_request", "content_length")
            if n < 0:
                return self._err(422, "invalid_request", "content_length")
            if n > MAX_BODY:
                return self._json(413, {"error": "payload_too_large"})
            raw = self.rfile.read(n)
            try:
                body = json.loads(raw)
            except (ValueError, RecursionError):
                return self._err(422, "invalid_request", "json_invalido")
            if not isinstance(body, dict) or set(body) != {"surface", "workflow"}:
                return self._err(422, "invalid_request", "cuerpo_invalido")
            rid = _safe_run_id(body)
            usage: dict = {}
            try:
                result = plan(body["surface"], body["workflow"], llm, limits, usage)
            except InvalidRequest as e:
                return self._err(422, "invalid_request", str(e), rid)
            except NoSurface as e:
                return self._err(422, "no_surface", str(e), rid)
            except PlanRejected as e:
                metrics.inc("plan_rejected", usage)
                _log(rid, "plan_rejected", e.reason)
                return self._json(422, {"error": "plan_rejected", "detail": e.reason})
            except BudgetExceeded as e:
                metrics.inc("budget_exceeded", usage)
                _log(rid, "budget_exceeded", e.kind)
                return self._json(504, {"error": "budget_exceeded", "detail": e.kind})
            except LLMError:
                return self._err(502, "llm_unavailable", "llm_unavailable", rid)
            metrics.inc("ok", usage)
            _log(rid, "ok", "ok")
            self._json(200, result)

    return ThreadingHTTPServer((host, port), Handler)
