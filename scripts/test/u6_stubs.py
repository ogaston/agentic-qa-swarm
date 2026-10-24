#!/usr/bin/env python3
"""Stubs de U6-T06 para el recorrido local del dashboard (solo 127.0.0.1, biblioteca estándar).

  warm  -- simula go-warm-manager: GET /warm exige el token de servicio (archivo) y devuelve
           un WarmState listo con reset_verified=true.
  run   -- simula go-run-controller: GET /runs/{id} valida el token de la persona contra
           go-identity GET /auth/session y devuelve deploying -> running -> done en llamadas
           sucesivas al mismo id.

Ambos responden 200 en /healthz. Nunca registran cabeceras ni tokens.

Uso:
  u6_stubs.py warm --port P --token-file F
  u6_stubs.py run  --port P --identity-url URL
"""

import argparse
import hmac
import json
import re
import sys
import threading
import urllib.error
import urllib.parse
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

HOST = "127.0.0.1"
RUN_PATH = re.compile(r"^/runs/(run-[0-9a-f]{16})(?:/plan)?$")
SEQUENCE = ["confirmed", "warm_ready", "deploying", "inferring", "rehearsing", "running",
            "resetting", "reporting", "done"]
TRACE_ID = "0af7651916cd43dd8448eb211c80319c"
REPORT_URI = "s3://aqs-reports/{run}/report.json"


def bearer(handler):
    value = handler.headers.get("Authorization", "")
    if not value.startswith("Bearer "):
        return ""
    return value[len("Bearer "):].strip()


class Base(BaseHTTPRequestHandler):
    server_version = "u6-stub"

    def log_message(self, fmt, *args):  # sin ruido: el log no incluye cabeceras
        sys.stderr.write("%s %s\n" % (self.command, self.path.split("?")[0]))

    def send_json(self, code, obj, extra=None):
        body = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", "no-store")
        for k, v in (extra or {}).items():
            self.send_header(k, v)
        self.end_headers()
        self.wfile.write(body)

    def unauthorized(self):
        self.send_json(
            401,
            {"code": "unauthorized", "message": "token ausente o inválido"},
            {"WWW-Authenticate": "Bearer"},
        )


def make_warm_handler(token):
    class Warm(Base):
        def do_GET(self):
            if self.path == "/healthz":
                return self.send_json(200, {"status": "ok"})
            if self.path == "/warm":
                if not hmac.compare_digest(bearer(self), token):
                    return self.unauthorized()
                return self.send_json(
                    200,
                    {
                        "warm_id": "warm-local",
                        "state": "ready",
                        "reset_verified": True,
                        "baseline_version": "local-1",
                    },
                )
            return self.send_json(404, {"code": "not_found", "message": "no existe"})

    return Warm


def plan_for(run_id):
    return {"run_id": run_id, "workflow": "checkout", "flows": [
        {"flow_id": "f-happy", "name": "Compra feliz", "invariant": "el total coincide con la suma de ítems",
         "invariant_probe": False, "steps": [
             {"method": "POST", "path": "/cart/items", "expect_status": 201},
             {"method": "POST", "path": "/checkout", "expect_status": 200}]},
        {"flow_id": "f-double", "name": "Doble cobro", "invariant": "un pedido no se cobra dos veces",
         "invariant_probe": True, "steps": [
             {"method": "POST", "path": "/checkout", "expect_status": 200},
             {"method": "POST", "path": "/checkout", "expect_status": 409}]}]}


def report_for(run_id):
    return {"run_id": run_id, "verdict": "bug", "summary": "Se cobró dos veces un pedido idéntico.",
            "report_uri": REPORT_URI.format(run=run_id), "findings": [
                {"finding_id": "F-1", "root_cause": "POST /checkout no es idempotente",
                 "invariant": "un pedido no se cobra dos veces", "method": "POST", "path": "/checkout",
                 "evidence_uris": ["s3://aqs-evidence/%s/req-2.har" % run_id]}]}


def make_run_handler(identity_url):
    counts = {}
    lock = threading.Lock()

    class Run(Base):
        def do_GET(self):
            if self.path == "/healthz":
                return self.send_json(200, {"status": "ok"})
            path, _, query = self.path.partition("?")
            if path == "/v1/reports":  # simula agent-reporter: el reporte validado de la corrida
                m = re.search(r"run-[0-9a-f]{16}", urllib.parse.unquote(query))
                if not m:
                    return self.send_json(404, {"code": "not_found", "message": "no existe"})
                return self.send_json(200, report_for(m.group(0)))
            m = RUN_PATH.match(path)
            if not m:
                return self.send_json(404, {"code": "not_found", "message": "no existe"})
            token = bearer(self)
            if not token:
                return self.unauthorized()
            try:
                req = urllib.request.Request(
                    identity_url.rstrip("/") + "/auth/session",
                    headers={"Authorization": "Bearer " + token},
                )
                with urllib.request.urlopen(req, timeout=2) as resp:
                    if resp.status != 200:
                        return self.unauthorized()
            except urllib.error.HTTPError:
                return self.unauthorized()
            except (urllib.error.URLError, OSError):
                return self.send_json(
                    503,
                    {"code": "identity_unavailable", "message": "identidad no disponible"},
                    {"Retry-After": "1"},
                )
            run_id = m.group(1)
            if path.endswith("/plan"):
                return self.send_json(200, plan_for(run_id))
            with lock:
                n = counts.get(run_id, 0)
                counts[run_id] = n + 1
            state = SEQUENCE[min(n, len(SEQUENCE) - 1)]
            body = {"id": run_id, "state": state, "trace_id": TRACE_ID}
            if state == "done":
                body.update(verdict="bug", report_uri=REPORT_URI.format(run=run_id))
            return self.send_json(200, body)

    return Run


def main():
    ap = argparse.ArgumentParser()
    sub = ap.add_subparsers(dest="kind", required=True)
    w = sub.add_parser("warm")
    w.add_argument("--port", type=int, required=True)
    w.add_argument("--token-file", required=True)
    r = sub.add_parser("run")
    r.add_argument("--port", type=int, required=True)
    r.add_argument("--identity-url", required=True)
    args = ap.parse_args()

    if args.kind == "warm":
        with open(args.token_file, encoding="utf-8") as fh:
            token = fh.read().strip()
        handler = make_warm_handler(token)
    else:
        handler = make_run_handler(args.identity_url)

    server = ThreadingHTTPServer((HOST, args.port), handler)
    server.serve_forever()


if __name__ == "__main__":
    main()
