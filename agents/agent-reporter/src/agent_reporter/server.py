"""Servidor HTTP minimo (stdlib). Ni detail ni logs llevan contenido de evidencia o del modelo."""
from __future__ import annotations

import json
import logging
import os
import sys
from dataclasses import dataclass
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlsplit

from agent_reporter.errors import (
    BudgetExceeded,
    EvidenceUnavailable,
    InvalidReportUri,
    NoEvidence,
    ReportRejected,
    StoreUnavailable,
)
from agent_reporter.llm import LLMClient, LLMError
from agent_reporter.metrics import METRICS
from agent_reporter.reporter import Limits, correlate_postmortem, correlate_postmortem_uri, prepare
from agent_reporter.s3 import report_key_of

MAX_BODY = 64 * 1024
log = logging.getLogger("agent_reporter")


@dataclass
class Service:
    reader: object
    llm: LLMClient
    store: object
    publisher: object
    limits: Limits
    # Bucket de los reportes guardados (GET /v1/reports). Vacío = el GET responde invalid_uri a todo.
    report_bucket: str = ""


def handle_get_report(svc: Service, uri: str) -> tuple[int, bytes | dict]:
    """GET /v1/reports: valida la URI contra el bucket del servicio ANTES de tocar el store (U9-T01)."""
    try:
        report_key_of(uri, svc.report_bucket)
    except InvalidReportUri:
        return 400, {"error": "invalid_uri"}
    try:
        return 200, svc.store.get(uri)
    except InvalidReportUri:
        return 400, {"error": "invalid_uri"}
    except Exception:  # noqa: BLE001 - el mensaje del proveedor no sale
        return 502, {"error": "store_unavailable"}


def handle_report(svc: Service, body: bytes) -> tuple[int, dict]:
    try:
        req = json.loads(body.decode("utf-8"))
    except (ValueError, UnicodeDecodeError):
        return 422, {"error": "invalid_request"}
    if (
        not isinstance(req, dict)
        or set(req) != {"run_id", "evidence_uris"}
        or not isinstance(req["run_id"], str)
        or not req["run_id"]
        or not isinstance(req["evidence_uris"], list)
        or not all(isinstance(u, str) for u in req["evidence_uris"])
    ):
        return 422, {"error": "invalid_request"}
    try:
        report, uri = correlate_postmortem_uri(
            req["run_id"], req["evidence_uris"], svc.reader, svc.llm, svc.limits,
            store=svc.store, publisher=svc.publisher,
        )
        if uri is None:  # E-1: sin URI del store no hay respuesta válida (falla cerrado)
            return 500, {"error": "internal"}
        return 200, {**report, "report_uri": uri}
    except NoEvidence:
        return 422, {"error": "no_evidence"}
    except ReportRejected as e:
        return 422, {"error": "report_rejected", "detail": e.reason}
    except BudgetExceeded as e:
        return 504, {"error": "budget_exceeded", "detail": str(e)}
    except EvidenceUnavailable:
        return 502, {"error": "evidence_unavailable"}
    except (LLMError, StoreUnavailable) as e:
        return 502, {"error": "llm_unavailable" if isinstance(e, LLMError) else "store_unavailable"}
    except Exception:  # noqa: BLE001 - nunca filtrar el mensaje
        return 500, {"error": "internal"}


def make_handler(svc: Service):
    class H(BaseHTTPRequestHandler):
        def log_message(self, fmt, *args):  # sin contenido de request
            pass

        def _send(self, code, payload, ctype="application/json"):
            data = payload if isinstance(payload, bytes) else json.dumps(payload).encode()
            self.send_response(code)
            self.send_header("Content-Type", ctype)
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

        def do_GET(self):
            url = urlsplit(self.path)
            if url.path in ("/healthz", "/readyz") and not url.query:
                self._send(200, {"status": "ok"})
            elif url.path == "/metrics":
                self._send(200, METRICS.render().encode(), "text/plain; version=0.0.4")
            elif url.path == "/v1/reports":
                uris = parse_qs(url.query, keep_blank_values=True).get("uri", [])
                if len(uris) != 1:
                    return self._send(400, {"error": "invalid_uri"})
                code, payload = handle_get_report(svc, uris[0])
                if code != 200:
                    return self._send(code, payload)
                return self._send(200, payload, "application/json")
            else:
                self._send(404, {"error": "not_found"})

        def do_POST(self):
            if self.path != "/v1/report":
                return self._send(404, {"error": "not_found"})
            if self.headers.get("Content-Type", "").split(";")[0].strip().lower() != "application/json":
                METRICS.request("unsupported_media_type")
                return self._send(415, {"error": "unsupported_media_type"})
            try:
                n = int(self.headers.get("Content-Length", "-1"))
            except ValueError:
                n = -1
            if n < 0:
                METRICS.request("invalid_request")
                return self._send(422, {"error": "invalid_request"})
            if n > MAX_BODY:
                METRICS.request("payload_too_large")
                return self._send(413, {"error": "payload_too_large"})
            code, payload = handle_report(svc, self.rfile.read(n))
            result = "ok" if code == 200 else payload["error"]
            METRICS.request(result)
            log.info("POST /v1/report status=%s result=%s", code, result)
            self._send(code, payload)

    return H


def make_server(svc: Service, host: str = "127.0.0.1", port: int = 0) -> ThreadingHTTPServer:
    return ThreadingHTTPServer((host, port), make_handler(svc))


def _http_llm(e):
    from agent_reporter.llm_http import HttpLLM

    base, model, key_file = e.get("LLM_BASE_URL", ""), e.get("LLM_MODEL", ""), e.get("LLM_API_KEY_FILE", "")
    if not base or not model or not key_file:
        raise SystemExit("LLM_PROVIDER=http requiere LLM_BASE_URL, LLM_MODEL y LLM_API_KEY_FILE")
    # U8-T09 (ronda 4): opt-in explícito a http hacia UN host privado (kind). Por defecto, no.
    insecure_host = e.get("LLM_INSECURE_HTTP_HOST", "") or None if e.get("LLM_ALLOW_INSECURE_HTTP", "") == "1" else None
    try:
        return HttpLLM(base, model, key_file, insecure_http_host=insecure_host)
    except (ValueError, OSError):
        raise SystemExit("configuracion LLM_PROVIDER=http invalida") from None


def _register_fixtures(llm, reader, path: str, limits) -> None:
    """REPORTER_FAKE_FIXTURES (U3-T07): [{run_id, evidence_uris, response}] -> prompt exacto -> respuesta.

    U8-T05: una entrada con «prompt» precalculado se registra tal cual, sin leer evidencia al arrancar
    (con REPORTER_EVIDENCE=s3 el arranque no depende de MinIO).
    """
    with open(path, encoding="utf-8") as fh:
        items = json.load(fh)
    for it in items:
        if "prompt" in it:
            llm.register(it["prompt"], json.dumps(it["response"]))
            continue
        prep = prepare(it["run_id"], it["evidence_uris"], reader, limits)
        llm.register(prep.prompt, json.dumps(it["response"]))


def build_service(env=None) -> Service:
    """LLM_PROVIDER=fake solo con REPORTER_ALLOW_FAKE=1 y fuera de prod; LLM_PROVIDER=http (U3-T07). Otro: no arranca."""
    from agent_reporter.fakes.evidence import DirEvidenceReader
    from agent_reporter.fakes.fake_llm import FakeLLM
    from agent_reporter.fakes.publisher import RecordingPublisher
    from agent_reporter.fakes.store import MemoryReportStore

    e = os.environ if env is None else env
    provider = e.get("LLM_PROVIDER", "")
    limits = Limits.from_env(e)
    store, reader = MemoryReportStore(), None
    if e.get("REPORTER_EVIDENCE", "dir") == "s3":
        from agent_reporter.s3 import S3EvidenceReader, S3ReportStore, make_s3_client

        bucket = e.get("EVIDENCE_BUCKET", "")
        if not bucket:
            raise SystemExit("REPORTER_EVIDENCE=s3 requiere EVIDENCE_BUCKET")
        s3 = make_s3_client(e)
        reader, store = S3EvidenceReader(s3, bucket), S3ReportStore(s3, bucket)
    elif e.get("REPORTER_EVIDENCE", "dir") == "dir":
        reader = DirEvidenceReader(e.get("REPORTER_EVIDENCE_DIR", "/evidence"))
    else:
        raise SystemExit("REPORTER_EVIDENCE no soportado")
    if provider == "http":
        llm = _http_llm(e)
    elif provider == "fake":
        if e.get("REPORTER_ALLOW_FAKE") != "1":
            raise SystemExit("LLM_PROVIDER=fake requiere REPORTER_ALLOW_FAKE=1")
        if e.get("REPORTER_ENV") == "prod":
            raise SystemExit("LLM_PROVIDER=fake prohibido con REPORTER_ENV=prod")
        llm = FakeLLM()
        if e.get("REPORTER_FAKE_FIXTURES"):
            _register_fixtures(llm, reader, e["REPORTER_FAKE_FIXTURES"], limits)
    else:
        raise SystemExit("LLM_PROVIDER no soportado")
    report_bucket = e.get("EVIDENCE_BUCKET", "") if e.get("REPORTER_EVIDENCE", "dir") == "s3" else ""
    return Service(reader, llm, store, RecordingPublisher(), limits, report_bucket=report_bucket)


def main() -> None:
    logging.basicConfig(level=logging.INFO, stream=sys.stderr)
    svc = build_service()
    srv = make_server(svc, os.environ.get("REPORTER_HOST", "0.0.0.0"), int(os.environ.get("PORT", "8080")))
    srv.serve_forever()


if __name__ == "__main__":
    main()
