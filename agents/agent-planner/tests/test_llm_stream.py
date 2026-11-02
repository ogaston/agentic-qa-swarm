"""LLM_STREAM=1 (entrega del módulo 8): proveedores que solo responden en streaming (Qwen-3.8-Flash en Together
devolvía 400 streaming_required). El adaptador pide SSE y agrega el texto; sin red (loopback)."""
import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import pytest

from agent_planner.llm import LLMTimeout, LLMUnavailable
from agent_planner.llm_http import HttpLLM


def sse(*chunks, done=True):
    out = b"".join(b"data: " + (c if isinstance(c, bytes) else json.dumps(c).encode()) + b"\n\n" for c in chunks)
    return out + (b"data: [DONE]\n\n" if done else b"")


def delta(text, **extra):
    return {"choices": [{"index": 0, "delta": dict({"content": text}, **extra)}]}


@pytest.fixture
def srv():
    state = {"body": b"", "delay": 0.0, "requests": []}

    class H(BaseHTTPRequestHandler):
        def log_message(self, *a):
            pass

        def do_POST(self):
            state["requests"].append(json.loads(self.rfile.read(int(self.headers["Content-Length"]))))
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.end_headers()
            for line in state["body"].split(b"\n\n"):
                if state["delay"]:
                    threading.Event().wait(state["delay"])
                self.wfile.write(line + b"\n\n")
                self.wfile.flush()

    httpd = ThreadingHTTPServer(("127.0.0.1", 0), H)
    httpd.daemon_threads = True
    threading.Thread(target=httpd.serve_forever, daemon=True).start()
    state["url"] = f"http://127.0.0.1:{httpd.server_address[1]}/v1"
    yield state
    httpd.shutdown()
    httpd.server_close()


@pytest.fixture
def key(tmp_path):
    p = tmp_path / "k"
    p.write_text("sk-x\n")
    return p


def test_agrega_el_texto_y_usa_usage(srv, key):
    srv["body"] = sse(delta("{\"a\""), delta("", reasoning_content="pensando..."), delta(": 1}"),
                      {"choices": [], "usage": {"prompt_tokens": 40, "completion_tokens": 7}})
    r = HttpLLM(srv["url"], "m", key, stream=True).complete("p", max_tokens=99, timeout_s=5)
    assert (r.text, r.input_tokens, r.output_tokens) == ('{"a": 1}', 40, 7)
    req = srv["requests"][0]
    assert req["stream"] is True and req["stream_options"] == {"include_usage": True} and req["max_tokens"] == 99


def test_sin_usage_estima_con_len_sobre_4(srv, key):
    srv["body"] = sse(delta("x" * 9))
    r = HttpLLM(srv["url"], "m", key, stream=True).complete("p" * 10, max_tokens=99, timeout_s=5)
    assert (r.text, r.input_tokens, r.output_tokens) == ("x" * 9, 3, 3)


def test_chunk_no_json_falla_cerrado(srv, key):
    srv["body"] = sse(delta("a"), b"{roto")
    with pytest.raises(LLMUnavailable):
        HttpLLM(srv["url"], "m", key, stream=True).complete("p", max_tokens=9, timeout_s=5)


def test_plazo_total_aunque_cada_lectura_llegue_a_tiempo(srv, key):
    srv["body"] = sse(*[delta("a")] * 8)
    srv["delay"] = 0.2  # cada chunk llega antes del timeout por lectura, pero el total supera 0,5 s
    with pytest.raises(LLMTimeout):
        HttpLLM(srv["url"], "m", key, stream=True).complete("p", max_tokens=9, timeout_s=0.5)


def test_sin_stream_no_lo_pide(srv, key):
    srv["body"] = json.dumps({"choices": [{"message": {"content": "ok"}}],
                              "usage": {"prompt_tokens": 1, "completion_tokens": 1}}).encode()
    r = HttpLLM(srv["url"], "m", key).complete("p", max_tokens=9, timeout_s=5)
    assert r.text == "ok" and "stream" not in srv["requests"][0]
