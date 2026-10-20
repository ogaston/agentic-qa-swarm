"""Adaptador LLM HTTP (U3-T07) contra un servidor simulado en loopback que cuenta peticiones."""
import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import pytest

from agent_reporter.llm import LLMTimeout, LLMUnavailable
from agent_reporter.llm_http import HttpLLM, check_base_url

MARKER = "sk-MARCADOR-UNICO-7f3a91"
OK_BODY = json.dumps({
    "choices": [{"message": {"role": "assistant", "content": "respuesta del modelo"}}],
    "usage": {"prompt_tokens": 11, "completion_tokens": 5},
}).encode()


class Srv:
    def __init__(self, status=200, body=OK_BODY, delay=0.0, location=None):
        self.status, self.body, self.delay, self.location = status, body, delay, location
        self.requests = []
        outer = self

        class H(BaseHTTPRequestHandler):
            def log_message(self, *a):
                pass

            def do_POST(self):
                n = int(self.headers.get("Content-Length", "0"))
                raw = self.rfile.read(n)
                outer.requests.append((self.path, dict(self.headers), raw))
                if outer.delay:
                    threading.Event().wait(outer.delay)
                self.send_response(outer.status)
                if outer.location:
                    self.send_header("Location", outer.location)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(outer.body)))
                self.end_headers()
                self.wfile.write(outer.body)

        self.httpd = ThreadingHTTPServer(("127.0.0.1", 0), H)
        self.httpd.daemon_threads = True
        self.url = f"http://127.0.0.1:{self.httpd.server_address[1]}"
        threading.Thread(target=self.httpd.serve_forever, daemon=True).start()

    def close(self):
        self.httpd.shutdown()
        self.httpd.server_close()


@pytest.fixture
def keyfile(tmp_path):
    p = tmp_path / "key"
    p.write_text(MARKER + "\n", encoding="utf-8")
    return p


@pytest.fixture
def make(keyfile):
    servers = []

    def _make(**kw):
        s = Srv(**kw)
        servers.append(s)
        return s

    yield _make
    for s in servers:
        s.close()


def client(srv, keyfile, timeout_s=5.0):
    return HttpLLM(srv.url, "modelo-x", keyfile)


def call(c, timeout_s=5.0):
    return c.complete("prompt de prueba", max_tokens=321, timeout_s=timeout_s)


def test_llm_http_exito_devuelve_texto_y_tokens(make, keyfile):
    s = make()
    r = call(client(s, keyfile))
    assert r.text == "respuesta del modelo"
    assert (r.input_tokens, r.output_tokens) == (11, 5)


def test_llm_http_envia_max_tokens_y_temperatura_cero(make, keyfile):
    s = make()
    call(client(s, keyfile))
    path, headers, raw = s.requests[0]
    body = json.loads(raw)
    assert path == "/chat/completions"
    assert body == {"model": "modelo-x", "messages": [{"role": "user", "content": "prompt de prueba"}],
                    "max_tokens": 321, "temperature": 0}


def test_llm_http_timeout_es_llm_timeout(make, keyfile):
    s = make(delay=2.0)
    with pytest.raises(LLMTimeout):
        call(client(s, keyfile), timeout_s=0.2)


@pytest.mark.parametrize("status", [500, 503])
def test_llm_http_5xx_es_llm_unavailable(make, keyfile, status):
    s = make(status=status, body=b"{}")
    with pytest.raises(LLMUnavailable):
        call(client(s, keyfile))


def test_llm_http_respuesta_no_json_es_llm_unavailable(make, keyfile):
    s = make(body=b"<html>no json</html>")
    with pytest.raises(LLMUnavailable):
        call(client(s, keyfile))


def test_llm_http_sin_choices_es_llm_unavailable(make, keyfile):
    s = make(body=json.dumps({"usage": {"prompt_tokens": 1, "completion_tokens": 1}}).encode())
    with pytest.raises(LLMUnavailable):
        call(client(s, keyfile))


def test_llm_http_rechaza_http_no_loopback():
    assert check_base_url("http://127.0.0.1:18400") == "http://127.0.0.1:18400"
    assert check_base_url("https://api.example.com/v1")
    for bad in ("http://api.example.com", "http://10.0.0.5:8080", "ftp://127.0.0.1"):
        with pytest.raises(ValueError):
            check_base_url(bad)


def test_llm_http_clave_en_authorization_y_no_filtrada(make, keyfile, capfd, caplog):
    import logging
    caplog.set_level(logging.DEBUG)
    s = make(status=500, body=b"{}")
    with pytest.raises(LLMUnavailable) as exc:
        call(client(s, keyfile))
    _, headers, _ = s.requests[0]
    assert headers["Authorization"] == f"Bearer {MARKER}"
    assert MARKER not in str(exc.value) and MARKER not in repr(exc.value)
    out, err = capfd.readouterr()
    assert MARKER not in out + err and MARKER not in caplog.text


def test_llm_http_exactamente_una_peticion_por_llamada(make, keyfile):
    s = make(status=500, body=b"{}")
    with pytest.raises(LLMUnavailable):
        call(client(s, keyfile))
    assert len(s.requests) == 1


def test_llm_http_no_sigue_redirecciones(make, keyfile):
    s = make(status=302, body=b"", location="http://127.0.0.1:1/otro")
    with pytest.raises(LLMUnavailable):
        call(client(s, keyfile))
    assert len(s.requests) == 1


GW = "10.20.30.40"  # IP literal privada de ejemplo (gateway de laboratorio)


def test_http_gateway_rechazado_sin_variable():
    with pytest.raises(ValueError):
        check_base_url(f"http://{GW}:8000/v1")


def test_http_gateway_aceptado_solo_con_host_exacto():
    url = f"http://{GW}:8000/v1"
    assert check_base_url(url, insecure_http_host=GW) == url
    for bad in ("http://10.20.30.41:8000/v1", "http://api.example.com/v1", "http://8.8.8.8:8000/v1"):
        with pytest.raises(ValueError):
            check_base_url(bad, insecure_http_host=GW)
    with pytest.raises(ValueError):
        check_base_url("ftp://10.20.30.40/v1", insecure_http_host=GW)


def test_http_publico_o_dns_rechazado_aunque_coincida_el_host():
    with pytest.raises(ValueError):
        check_base_url("http://8.8.8.8:8000/v1", insecure_http_host="8.8.8.8")
    with pytest.raises(ValueError):
        check_base_url("http://api.example.com/v1", insecure_http_host="api.example.com")


def test_server_http_llm_gateway_solo_con_variable_y_host(tmp_path):
    import pytest as _pt
    from agent_reporter import server
    key = tmp_path / "k"
    key.write_text("x", encoding="utf-8")
    base = {"LLM_PROVIDER": "http", "LLM_BASE_URL": f"http://{GW}:8000/v1", "LLM_MODEL": "m", "LLM_API_KEY_FILE": str(key)}
    with _pt.raises(SystemExit):
        server._http_llm(base)
    with _pt.raises(SystemExit):
        server._http_llm({**base, "LLM_ALLOW_INSECURE_HTTP": "true", "LLM_INSECURE_HTTP_HOST": GW})
    assert server._http_llm({**base, "LLM_ALLOW_INSECURE_HTTP": "1", "LLM_INSECURE_HTTP_HOST": GW}) is not None
