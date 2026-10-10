"""Adaptador LLM HTTP compatible con OpenAI (U3-T07). Solo biblioteca estandar.

Sin reintentos ni circuito (el circuito hacia U3 vive en el controlador de U2). La clave se lee de
archivo y nunca aparece en logs ni en el mensaje de un error. Duplicado con una nota en
agent_reporter/llm_http.py: si cambia uno, cambia el otro.
"""
from __future__ import annotations

import ipaddress
import json
import socket
import urllib.error
import urllib.request
from pathlib import Path
from urllib.parse import urlsplit

from .llm import LLMResult, LLMTimeout, LLMUnavailable


def _is_loopback(host: str) -> bool:
    if host == "localhost":
        return True
    try:
        return ipaddress.ip_address(host).is_loopback
    except ValueError:
        return False


def check_base_url(url: str) -> str:
    """https en cualquier host; http solo hacia loopback. Otro caso: ValueError."""
    u = urlsplit(url)
    host = u.hostname or ""
    if u.scheme == "https" and host:
        return url.rstrip("/")
    if u.scheme == "http" and host and _is_loopback(host):
        return url.rstrip("/")
    raise ValueError("LLM_BASE_URL debe ser https (http solo hacia loopback)")


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):  # la clave no viaja a otro destino
        return None


class HttpLLM:
    def __init__(self, base_url: str, model: str, api_key_file: str | Path) -> None:
        self._base = check_base_url(base_url)
        if not model:
            raise ValueError("LLM_MODEL no configurado")
        self._model = model
        self._key = Path(api_key_file).read_text(encoding="utf-8").strip()
        if not self._key:
            raise ValueError("LLM_API_KEY_FILE vacio")
        self._opener = urllib.request.build_opener(_NoRedirect)

    def complete(self, prompt: str, *, max_tokens: int, timeout_s: float) -> LLMResult:
        body = json.dumps({
            "model": self._model,
            "messages": [{"role": "user", "content": prompt}],
            "max_tokens": max_tokens,
            "temperature": 0,
        }).encode("utf-8")
        req = urllib.request.Request(
            self._base + "/chat/completions",
            data=body,
            method="POST",
            headers={"Content-Type": "application/json", "Authorization": "Bearer " + self._key},
        )
        try:
            with self._opener.open(req, timeout=timeout_s) as resp:
                raw = resp.read()
        except urllib.error.HTTPError:
            raise LLMUnavailable("http_error") from None
        except urllib.error.URLError as e:
            if isinstance(e.reason, (TimeoutError, socket.timeout)):
                raise LLMTimeout("timeout") from None
            raise LLMUnavailable("inalcanzable") from None
        except (TimeoutError, socket.timeout):
            raise LLMTimeout("timeout") from None
        except OSError:
            raise LLMUnavailable("inalcanzable") from None
        try:
            doc = json.loads(raw.decode("utf-8"))
        except (ValueError, UnicodeDecodeError):
            raise LLMUnavailable("respuesta_no_json") from None
        try:
            text = doc["choices"][0]["message"]["content"]
            usage = doc["usage"]
            inp, out = int(usage["prompt_tokens"]), int(usage["completion_tokens"])
        except (KeyError, IndexError, TypeError, ValueError):
            raise LLMUnavailable("respuesta_sin_campos") from None
        if not isinstance(text, str):
            raise LLMUnavailable("respuesta_sin_campos")
        return LLMResult(text, inp, out)
